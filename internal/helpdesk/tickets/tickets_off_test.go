package tickets_test

import (
	"testing"

	"github.com/useopendesk/opendesk/internal/helpdesk/hdtest"
	"github.com/useopendesk/opendesk/internal/helpdesk/tickets"
)

// With Tickets off every route tagged Tickets answers feature_disabled, for members and API
// keys alike, and answers again once it is back on.
func TestTicketsOffRefusesTicketRoutes(t *testing.T) {
	e, ws := newEnv(t)
	tk := createTicket(t, e, ws, map[string]any{"kind": "note"})
	key := e.APIKey(ws, "tickets:read", "tickets:write")
	e.SetSettings(ws, map[string]any{"tickets_enabled": false})

	for _, a := range []struct {
		name string
		auth hdtest.Auth
	}{{"member", ws.Admin.As(ws)}, {"api key", hdtest.Auth{Bearer: key}}} {
		for _, r := range [][2]string{{"GET", "/v1/tickets"}, {"GET", "/v1/tickets/counts"}, {"GET", "/v1/tickets/" + tk.ID.String()},
			{"GET", "/v1/tickets/" + tk.ID.String() + "/messages"}} {
			var out struct {
				Code, Product, Reason string
			}
			e.Do(r[0], r[1], a.auth, nil).Want(t, 403).JSON(t, &out)
			if out.Code != "feature_disabled" || out.Product != "tickets" || out.Reason != "workspace_disabled" {
				t.Fatalf("%s %s %s: %+v", a.name, r[0], r[1], out)
			}
		}
		e.Do("POST", "/v1/tickets", a.auth, map[string]any{"subject": "x", "body_html": "<p>x</p>", "kind": "note"}).Want(t, 403)
	}

	e.SetSettings(ws, map[string]any{"tickets_enabled": true})
	e.Do("GET", "/v1/tickets/"+tk.ID.String(), ws.Admin.As(ws), nil).Want(t, 200)
}

// Scheduled ticket work leaves a workspace alone while Tickets is off and catches up once it is on.
func TestTicketsOffPausesScheduledTicketWork(t *testing.T) {
	e, ws := newEnv(t)
	tk := createTicket(t, e, ws, map[string]any{"kind": "note"})
	e.Exec(`UPDATE tickets SET status = 'solved', solved_at = now() - interval '30 days' WHERE id = $1`, tk.ID)
	e.SetSettings(ws, map[string]any{"tickets_enabled": false})
	if err := tickets.AutoClose(e.Ctx, e.D); err != nil {
		t.Fatal(err)
	}
	var status string
	e.Scalar(`SELECT status FROM tickets WHERE id = $1`, &status, tk.ID)
	if status != "solved" {
		t.Fatalf("closed while off: %s", status)
	}
	e.SetSettings(ws, map[string]any{"tickets_enabled": true})
	if err := tickets.AutoClose(e.Ctx, e.D); err != nil {
		t.Fatal(err)
	}
	e.Scalar(`SELECT status FROM tickets WHERE id = $1`, &status, tk.ID)
	if status != "closed" {
		t.Fatalf("not closed after on: %s", status)
	}
}

// Search keeps answering for contacts, articles and pages while Tickets is off, without a ticket.
func TestTicketsOffSearchLeavesTicketsOut(t *testing.T) {
	e, ws := newEnv(t)
	createTicket(t, e, ws, map[string]any{"kind": "note", "subject": "Zebrafinch migration"})
	search := func() int {
		var out struct {
			Tickets []map[string]any `json:"tickets"`
		}
		e.Do("GET", "/v1/search?q=Zebrafinch", ws.Admin.As(ws), nil).Want(t, 200).JSON(t, &out)
		return len(out.Tickets)
	}
	if search() != 1 {
		t.Fatal("ticket not found while on")
	}
	e.SetSettings(ws, map[string]any{"tickets_enabled": false})
	if n := search(); n != 0 {
		t.Fatalf("%d tickets found while off", n)
	}
}
