package tickets_test

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/useopendesk/opendesk/internal/domain"
)

// Merging into a ticket that was itself merged lands in the ticket it went into, following the
// chain; a chain that leads back to the ticket being merged is 409, and nothing moves.
func TestMergeFollowsAMergedTarget(t *testing.T) {
	e, ws := newEnv(t)
	live := createTicket(t, e, ws, map[string]any{"subject": "Live"})
	middle := createTicket(t, e, ws, map[string]any{"subject": "Middle"})
	gone := createTicket(t, e, ws, map[string]any{"subject": "Gone"})
	// A chain two deep: gone went into middle, then middle into live.
	e.Do("POST", "/v1/tickets/"+gone.ID.String()+"/merge", ws.Agent.As(ws), map[string]any{"into_id": middle.ID.String()}).Want(t, 200)
	e.Do("POST", "/v1/tickets/"+middle.ID.String()+"/merge", ws.Agent.As(ws), map[string]any{"into_id": live.ID.String()}).Want(t, 200)

	fresh := createTicket(t, e, ws, map[string]any{"subject": "Fresh"})
	inbound(t, e, ws, fresh.ID, "same customer again")
	var got domain.Ticket
	e.Do("POST", "/v1/tickets/"+fresh.ID.String()+"/merge", ws.Agent.As(ws), map[string]any{"into_id": fmt.Sprintf("#%d", gone.Number)}).Want(t, 200).JSON(t, &got)
	if got.ID != live.ID {
		t.Fatalf("merged into %s, want the live ticket %s", got.ID, live.ID)
	}
	if n := countRows(t, e, `SELECT count(*) FROM messages WHERE ticket_id = $1`, fresh.ID); n != 0 {
		t.Fatalf("source still has %d messages", n)
	}
	var into *string
	e.Scalar(`SELECT data->>'into' FROM activities WHERE ticket_id = $1 AND type = 'merged'`, &into, fresh.ID)
	if into == nil || *into != live.ID.String() {
		t.Fatalf("source timeline names %v, want the live ticket", into)
	}

	// The live ticket merged into one already merged into it: 409, and it stays as it was.
	before := countRows(t, e, `SELECT count(*) FROM messages WHERE ticket_id = $1`, live.ID)
	r := e.Do("POST", "/v1/tickets/"+live.ID.String()+"/merge", ws.Agent.As(ws), map[string]any{"into_id": gone.ID.String()}).Want(t, 409)
	if body := string(r.Body); !strings.Contains(body, "merged into this one") {
		t.Fatalf("409 body %s", body)
	}
	if tk := getTicket(t, e, ws, live.ID); tk.Status != "open" {
		t.Fatalf("live ticket %s", tk.Status)
	}
	if n := countRows(t, e, `SELECT count(*) FROM messages WHERE ticket_id = $1`, live.ID); n != before {
		t.Fatalf("live ticket has %d messages, want %d", n, before)
	}
}

// A chain that runs through a ticket the caller cannot see is refused, as that ticket would be.
func TestMergeIntoAChainThroughAHiddenTicket(t *testing.T) {
	e, ws := newEnv(t)
	hidden, _ := privateTicket(t, e, ws)
	gone := createTicket(t, e, ws, map[string]any{"subject": "Gone"})
	e.Do("POST", "/v1/tickets/"+gone.ID.String()+"/merge", ws.Admin.As(ws), map[string]any{"into_id": hidden.ID.String()}).Want(t, 200)
	fresh := createTicket(t, e, ws, map[string]any{"subject": "Fresh"})
	e.Do("POST", "/v1/tickets/"+fresh.ID.String()+"/merge", ws.Agent.As(ws), map[string]any{"into_id": gone.ID.String()}).Want(t, 422)
	if tk := getTicket(t, e, ws, fresh.ID); tk.Status != "open" {
		t.Fatalf("source %s", tk.Status)
	}
}

// A ticket still being worked, merged into a solved or closed one, reopens it with a status
// change on its timeline; a finished ticket merged there leaves it finished.
func TestMergeIntoFinishedTicketReopensIt(t *testing.T) {
	e, ws := newEnv(t)
	for _, status := range []string{"solved", "closed"} {
		old := createTicket(t, e, ws, map[string]any{"subject": "Old order"})
		e.Do("PATCH", "/v1/tickets/"+old.ID.String(), ws.Agent.As(ws), map[string]any{"status": status}).Want(t, 200)
		fresh := createTicket(t, e, ws, map[string]any{"subject": "A new question"})
		inbound(t, e, ws, fresh.ID, "a new question")

		var got domain.Ticket
		e.Do("POST", "/v1/tickets/"+fresh.ID.String()+"/merge", ws.Agent.As(ws), map[string]any{"into_id": old.ID.String()}).Want(t, 200).JSON(t, &got)
		if got.Status != "open" {
			t.Fatalf("%s target answered %q, want open", status, got.Status)
		}
		var to *string
		e.Scalar(`SELECT data->>'to' FROM activities WHERE ticket_id = $1 AND type = 'status_changed' ORDER BY created_at DESC, id DESC LIMIT 1`, &to, old.ID)
		if to == nil || *to != "open" {
			t.Fatalf("%s target's last status change %v, want open", status, to)
		}
		if tk := getTicket(t, e, ws, fresh.ID); tk.Status != "closed" {
			t.Fatalf("source %s", tk.Status)
		}
	}

	done := createTicket(t, e, ws, map[string]any{"subject": "Done"})
	e.Do("PATCH", "/v1/tickets/"+done.ID.String(), ws.Agent.As(ws), map[string]any{"status": "solved"}).Want(t, 200)
	old := createTicket(t, e, ws, map[string]any{"subject": "Old"})
	e.Do("PATCH", "/v1/tickets/"+old.ID.String(), ws.Agent.As(ws), map[string]any{"status": "closed"}).Want(t, 200)
	e.Do("POST", "/v1/tickets/"+done.ID.String()+"/merge", ws.Agent.As(ws), map[string]any{"into_id": old.ID.String()}).Want(t, 200)
	if tk := getTicket(t, e, ws, old.ID); tk.Status != "closed" {
		t.Fatalf("finished into finished: target %s, want closed", tk.Status)
	}
}

// merged=false leaves out tickets merged into another, merged=true lists only those.
func TestListMergedFilter(t *testing.T) {
	e, ws := newEnv(t)
	a := createTicket(t, e, ws, map[string]any{"subject": "A", "contact": map[string]any{"email": "pat@example.test"}})
	b := createTicket(t, e, ws, map[string]any{"subject": "B", "contact_id": a.Contact.ID.String()})
	e.Do("PATCH", "/v1/tickets/"+a.ID.String(), ws.Agent.As(ws), map[string]any{"status": "closed"}).Want(t, 200)
	c := createTicket(t, e, ws, map[string]any{"subject": "C", "contact_id": a.Contact.ID.String()})
	e.Do("POST", "/v1/tickets/"+c.ID.String()+"/merge", ws.Agent.As(ws), map[string]any{"into_id": b.ID.String()}).Want(t, 200)

	base := "/v1/tickets?contact_id=" + a.Contact.ID.String() + "&status=open,pending,on_hold,solved,closed"
	for q, want := range map[string][]string{
		"":              {a.ID.String(), b.ID.String(), c.ID.String()},
		"&merged=any":   {a.ID.String(), b.ID.String(), c.ID.String()},
		"&merged=false": {a.ID.String(), b.ID.String()},
		"&merged=true":  {c.ID.String()},
	} {
		got := ticketIDs(t, e.Do("GET", base+q, ws.Agent.As(ws), nil).Want(t, 200))
		slices.Sort(got)
		slices.Sort(want)
		if !slices.Equal(got, want) {
			t.Fatalf("list%s: %v, want %v", q, got, want)
		}
	}
	e.Do("GET", base+"&merged=maybe", ws.Agent.As(ws), nil).Want(t, 422)
}
