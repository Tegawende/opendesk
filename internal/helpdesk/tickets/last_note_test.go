package tickets_test

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/useopendesk/opendesk/internal/auth"
	"github.com/useopendesk/opendesk/internal/domain"
	"github.com/useopendesk/opendesk/internal/events"
	"github.com/useopendesk/opendesk/internal/helpdesk/hd"
	"github.com/useopendesk/opendesk/internal/helpdesk/hdtest"
)

// lastNoteEvent is the last_note of the newest ticket.updated about tid after seq that names
// last_note among its changes; ok is false when there is none.
func lastNoteEvent(t *testing.T, e *hdtest.Env, wid, tid uuid.UUID, seq int64) (note *domain.LastNote, ok bool) {
	t.Helper()
	var raw []byte
	err := e.D.DB.Pool.QueryRow(e.Ctx, `SELECT data->'ticket'->'last_note' FROM events WHERE workspace_id = $1 AND seq > $2
		AND type = 'ticket.updated' AND data->'ticket'->>'id' = $3 AND data->'changes' ? 'last_note' ORDER BY seq DESC LIMIT 1`,
		wid, seq, tid.String()).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, false
	}
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &note); err != nil {
		t.Fatal(err)
	}
	return note, true
}

func latestSeq(t *testing.T, e *hdtest.Env, wid uuid.UUID) int64 {
	t.Helper()
	_, seq := e.Events(wid, 0)
	return seq
}

// TestTicketLastNote checks Ticket.last_note is the newest internal note a member or an API key
// wrote on the ticket itself, follows edits, deletions and a note sent as a reply, leaves out
// Ollie, and tells the team each change in ticket.updated.
func TestTicketLastNote(t *testing.T) {
	e, ws := newEnv(t)
	agent := ws.Agent.As(ws)
	tk := createTicket(t, e, ws, map[string]any{"kind": "note", "body_html": "<p>First   note</p><p>on two lines</p>",
		"contact": map[string]any{"email": addr("customer")}})
	e.Exec(`UPDATE tickets SET channel = 'chat' WHERE id = $1`, tk.ID)
	path := "/v1/tickets/" + tk.ID.String()

	got := getTicket(t, e, ws, tk.ID)
	if got.LastNote == nil || got.LastNote.Preview != "First note on two lines" || got.LastNote.Author.Type != "user" ||
		got.LastNote.Author.Name != ws.Agent.Name || got.LastNote.Author.User == nil || got.LastNote.Author.User.ID != ws.Agent.ID {
		t.Fatalf("first note: %+v", got.LastNote)
	}
	first := got.LastNote.ID

	// A customer's message is not a note; it leaves last_note as it was.
	inbound(t, e, ws, tk.ID, "Where is my refund?")
	if got := getTicket(t, e, ws, tk.ID); got.LastNote == nil || got.LastNote.ID != first {
		t.Fatalf("after a customer message: %+v", got.LastNote)
	}

	seq := latestSeq(t, e, ws.ID)
	var second domain.Message
	e.Do("POST", path+"/messages", agent, map[string]any{"kind": "note", "body_html": "<p>Checked with billing, refund approved</p>"}).
		Want(t, 201).JSON(t, &second)
	if n, ok := lastNoteEvent(t, e, ws.ID, tk.ID, seq); !ok || n == nil || n.ID != second.ID || n.Preview != "Checked with billing, refund approved" {
		t.Fatalf("ticket.updated for a new note: %+v %v", n, ok)
	}
	stored := latestSeq(t, e, ws.ID)

	// Ollie's note is not one the list shows.
	if err := e.D.DB.Tx(e.Ctx, func(tx pgx.Tx) error {
		_, err := domain.AddMessage(e.Ctx, tx, e.D.Jobs, domain.NewMessage{WorkspaceID: ws.ID, TicketID: tk.ID, Kind: "note",
			Direction: "outbound", AuthorType: "system", BodyHTML: "<p>Ollie: looks like a refund</p>", BodyText: "Ollie: looks like a refund",
			Trigger: "none", AI: true, FromName: "Ollie"})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if _, ok := lastNoteEvent(t, e, ws.ID, tk.ID, stored); ok {
		t.Fatal("Ollie's note published a last_note change")
	}

	// The list carries it too, for members and API keys that read tickets.
	for _, a := range []hdtest.Auth{agent, ws.Viewer.As(ws), {Bearer: e.APIKey(ws, auth.ScopeTicketsRead)}} {
		var page struct {
			Data []domain.Ticket `json:"data"`
		}
		e.Do("GET", "/v1/tickets?status=open,pending,on_hold,solved,closed", a, nil).Want(t, 200).JSON(t, &page)
		found := false
		for _, row := range page.Data {
			if row.ID == tk.ID {
				found = true
				if row.LastNote == nil || row.LastNote.ID != second.ID {
					t.Fatalf("list row last_note: %+v", row.LastNote)
				}
			}
		}
		if !found {
			t.Fatal("ticket not in the list")
		}
	}

	// An edit shows at once.
	seq = latestSeq(t, e, ws.ID)
	e.Do("PATCH", path+"/messages/"+second.ID.String(), agent, map[string]any{"body_html": "<p>Refund approved and sent</p>"}).Want(t, 200)
	if got := getTicket(t, e, ws, tk.ID); got.LastNote == nil || got.LastNote.Preview != "Refund approved and sent" || got.LastNote.EditedAt == nil {
		t.Fatalf("after an edit: %+v", got.LastNote)
	}
	if n, ok := lastNoteEvent(t, e, ws.ID, tk.ID, seq); !ok || n == nil || n.Preview != "Refund approved and sent" {
		t.Fatalf("ticket.updated for an edit: %+v %v", n, ok)
	}

	// Deleted, the note before it is the last again, live and in a replay of the event that
	// carried the deleted one.
	seq = latestSeq(t, e, ws.ID)
	e.Do("DELETE", path+"/messages/"+second.ID.String(), agent, nil).Want(t, 204)
	if got := getTicket(t, e, ws, tk.ID); got.LastNote == nil || got.LastNote.ID != first {
		t.Fatalf("after a delete: %+v", got.LastNote)
	}
	if n, ok := lastNoteEvent(t, e, ws.ID, tk.ID, seq); !ok || n == nil || n.ID != first {
		t.Fatalf("ticket.updated for a delete: %+v %v", n, ok)
	}
	evs := []events.Envelope{{WorkspaceID: ws.ID}}
	if err := e.D.DB.Pool.QueryRow(e.Ctx, `SELECT id, seq, type, data FROM events WHERE workspace_id = $1 AND type = 'ticket.updated'
		AND data->'ticket'->'last_note'->>'id' = $2 ORDER BY seq DESC LIMIT 1`, ws.ID, second.ID.String()).
		Scan(&evs[0].ID, &evs[0].Seq, &evs[0].Type, &evs[0].Data); err != nil {
		t.Fatal(err)
	}
	if err := domain.RedactReplay(e.Ctx, e.D.DB.Pool, ws.ID, evs); err != nil {
		t.Fatal(err)
	}
	var replayed struct {
		Ticket domain.Ticket `json:"ticket"`
	}
	if len(evs) != 1 || evs[0].Type != "ticket.updated" || json.Unmarshal(evs[0].Data, &replayed) != nil ||
		replayed.Ticket.LastNote == nil || replayed.Ticket.LastNote.ID != first {
		t.Fatalf("replayed event: %+v", evs)
	}

	// A note sent as a reply is the customer's now: it is no longer the last note.
	var third domain.Message
	e.Do("POST", path+"/messages", agent, map[string]any{"kind": "note", "body_html": "<p>Your refund is on its way</p>"}).Want(t, 201).JSON(t, &third)
	seq = latestSeq(t, e, ws.ID)
	e.Do("POST", path+"/messages", agent, map[string]any{"kind": "reply", "body_html": "<p>Your refund is on its way</p>",
		"from_note_id": third.ID.String()}).Want(t, 201)
	if got := getTicket(t, e, ws, tk.ID); got.LastNote == nil || got.LastNote.ID != first {
		t.Fatalf("after sending a note as a reply: %+v", got.LastNote)
	}
	if n, ok := lastNoteEvent(t, e, ws.ID, tk.ID, seq); !ok || n == nil || n.ID != first {
		t.Fatalf("ticket.updated for a note sent as a reply: %+v %v", n, ok)
	}

	// An API key's note names the key.
	key := e.APIKey(ws, auth.ScopeTicketsRead, auth.ScopeTicketsWrite)
	e.Do("POST", path+"/messages", hdtest.Auth{Bearer: key}, map[string]any{"kind": "note", "body_html": "<p>Synced from billing</p>"}).Want(t, 201)
	if got := getTicket(t, e, ws, tk.ID); got.LastNote == nil || got.LastNote.Author.Type != "api" || got.LastNote.Author.Name != "test" ||
		got.LastNote.Author.User != nil || got.LastNote.Preview != "Synced from billing" {
		t.Fatalf("API key note: %+v", got.LastNote)
	}
}

// TestTicketLastNoteSideConversation checks a side conversation's messages, stored as notes,
// are never a ticket's last_note.
func TestTicketLastNoteSideConversation(t *testing.T) {
	e, ws := newEnv(t)
	mb := addMailbox(t, e, ws)
	tk := createTicket(t, e, ws, map[string]any{"channel": "email", "mailbox_id": mb.String(), "kind": "note", "body_html": "<p>Ask the courier</p>",
		"contact": map[string]any{"email": addr("customer"), "name": "Carla"}})
	e.Do("POST", "/v1/tickets/"+tk.ID.String()+"/threads", ws.Agent.As(ws), map[string]any{"to": []string{addr("courier")},
		"subject": "Parcel 1182", "body_html": "<p>Where is parcel 1182?</p>"}).Want(t, 201)
	if got := getTicket(t, e, ws, tk.ID); got.LastNote == nil || got.LastNote.Preview != "Ask the courier" {
		t.Fatalf("last_note with a side conversation: %+v", got.LastNote)
	}
}

// TestTicketLastNotePrivateTeam checks a private team's ticket, its last_note included, stays
// with the team: outsiders neither read nor list it, and its events go to the team alone.
func TestTicketLastNotePrivateTeam(t *testing.T) {
	e, ws := newEnv(t)
	lead := e.AddUser(ws, auth.RoleAgent, "Lena Lead")
	priv := addPrivateTeam(t, e, ws, map[uuid.UUID]string{lead.ID: hd.TeamLead})
	var tk domain.Ticket
	e.Do("POST", "/v1/tickets", ws.Admin.As(ws), map[string]any{"subject": "Breach", "body_html": "<p>Rotate the keys tonight</p>", "kind": "note",
		"team_id": priv.String()}).Want(t, 201).JSON(t, &tk)
	seq := latestSeq(t, e, ws.ID)
	e.Do("POST", "/v1/tickets/"+tk.ID.String()+"/messages", lead.As(ws), map[string]any{"kind": "note", "body_html": "<p>Keys rotated</p>"}).Want(t, 201)

	var got domain.Ticket
	e.Do("GET", "/v1/tickets/"+tk.ID.String(), lead.As(ws), nil).Want(t, 200).JSON(t, &got)
	if got.LastNote == nil || got.LastNote.Preview != "Keys rotated" {
		t.Fatalf("team member's last_note: %+v", got.LastNote)
	}
	e.Do("GET", "/v1/tickets/"+tk.ID.String(), ws.Agent.As(ws), nil).Want(t, 404)
	r := e.Do("GET", "/v1/tickets?status=open,pending,on_hold,solved,closed", ws.Agent.As(ws), nil).Want(t, 200)
	if strings.Contains(string(r.Body), "Keys rotated") || strings.Contains(string(r.Body), tk.ID.String()) {
		t.Fatalf("outsider lists the private ticket: %s", r.Body)
	}
	if _, ok := lastNoteEvent(t, e, ws.ID, tk.ID, seq); !ok {
		t.Fatal("no ticket.updated for the team's note")
	}
	var unscoped int
	if err := e.D.DB.Pool.QueryRow(e.Ctx, `SELECT count(*) FROM events WHERE workspace_id = $1 AND seq > $2 AND type = 'ticket.updated'
		AND data->'changes' ? 'last_note' AND team_id IS DISTINCT FROM $3`, ws.ID, seq, priv).Scan(&unscoped); err != nil {
		t.Fatal(err)
	}
	if unscoped != 0 {
		t.Fatalf("%d last_note events outside the private team", unscoped)
	}
}

// addr is a test address at example.com.
func addr(local string) string { return local + "@" + "example.com" }
