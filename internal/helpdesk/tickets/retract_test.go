package tickets_test

import (
	"slices"
	"testing"

	"github.com/useopendesk/opendesk/internal/auth"
	"github.com/useopendesk/opendesk/internal/domain"
)

// TestRetractChatReply checks a chat reply taken back becomes a note, leaves the visitor's
// widget (a public message.withdrawn, never message.deleted: the team keeps it as a note),
// the conversation's preview and its first response, and that only its author or an admin
// may take back only an agent's chat reply that was not emailed as a follow-up.
func TestRetractChatReply(t *testing.T) {
	e, ws := newEnv(t)
	tk := createTicket(t, e, ws, map[string]any{"kind": "note", "contact": map[string]any{"email": "visitor@example.com"}})
	e.Exec(`UPDATE tickets SET channel = 'chat' WHERE id = $1`, tk.ID)
	first := inbound(t, e, ws, tk.ID, "Is it billed per member?")
	var reply domain.Message
	e.Do("POST", "/v1/tickets/"+tk.ID.String()+"/messages", ws.Agent.As(ws),
		map[string]any{"kind": "reply", "body_html": "<p>Internal: ask finance first</p>"}).Want(t, 201).JSON(t, &reply)
	path := "/v1/tickets/" + tk.ID.String() + "/messages/" + reply.ID.String() + "/retract"

	other := e.AddUser(ws, auth.RoleAgent, "Other Agent")
	e.Do("POST", path, other.As(ws), nil).Want(t, 403)
	e.Do("POST", path, ws.Viewer.As(ws), nil).Want(t, 403)
	_, seq := e.Events(ws.ID, 0)

	var got domain.Message
	e.Do("POST", path, ws.Agent.As(ws), nil).Want(t, 200).JSON(t, &got)
	if got.Kind != "note" {
		t.Fatalf("retracted kind %q", got.Kind)
	}
	var public int
	if err := e.D.DB.Pool.QueryRow(e.Ctx, `SELECT count(*) FROM events WHERE workspace_id = $1 AND seq > $2
		AND type = 'message.withdrawn' AND visibility = 'public' AND data->>'message_id' = $3`, ws.ID, seq, reply.ID.String()).Scan(&public); err != nil {
		t.Fatal(err)
	}
	if public != 1 {
		t.Fatalf("public message.withdrawn events %d", public)
	}
	if n := countRows(t, e, `SELECT count(*) FROM events WHERE workspace_id = $1 AND seq > $2 AND type = 'message.deleted'`, ws.ID, seq); n != 0 {
		t.Fatalf("message.deleted events %d: the team keeps the note", n)
	}
	if n := countRows(t, e, `SELECT count(*) FROM tickets WHERE id = $1 AND first_responded_at IS NULL`, tk.ID); n != 1 {
		t.Fatal("a reply taken back still counts as the first response")
	}
	if types, _ := e.Events(ws.ID, seq); !slices.Contains(types, "message.updated") || !slices.Contains(types, "activity.created") {
		t.Fatalf("retract events %v", types)
	}
	var preview string
	if err := e.D.DB.Pool.QueryRow(e.Ctx, `SELECT last_message_preview FROM tickets WHERE id = $1`, tk.ID).Scan(&preview); err != nil {
		t.Fatal(err)
	}
	if preview != first.BodyText {
		t.Fatalf("preview %q, want the customer's message", preview)
	}
	// The team's note now, the newest one.
	if tk := getTicket(t, e, ws, tk.ID); tk.LastNote == nil || tk.LastNote.ID != reply.ID {
		t.Fatalf("last_note after retract: %+v", tk.LastNote)
	}
	msgs, err := domain.ListMessages(e.Ctx, e.D.DB.Pool, ws.ID, tk.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range msgs {
		if m.ID == reply.ID {
			t.Fatal("the visitor still sees the retracted reply")
		}
	}
	// Once a note, there is nothing left to take back; a customer's message never is one.
	e.Do("POST", path, ws.Admin.As(ws), nil).Want(t, 422)
	e.Do("POST", "/v1/tickets/"+tk.ID.String()+"/messages/"+first.ID.String()+"/retract", ws.Admin.As(ws), nil).Want(t, 422)

	// Email is delivered when it is sent.
	mail := createTicket(t, e, ws, map[string]any{"kind": "note", "contact": map[string]any{"email": "mail@example.com"}})
	var sent domain.Message
	e.Do("POST", "/v1/tickets/"+mail.ID.String()+"/messages", ws.Agent.As(ws),
		map[string]any{"kind": "reply", "body_html": "<p>Hello</p>"}).Want(t, 201).JSON(t, &sent)
	e.Do("POST", "/v1/tickets/"+mail.ID.String()+"/messages/"+sent.ID.String()+"/retract", ws.Admin.As(ws), nil).Want(t, 422)

	// A chat reply the visitor missed, emailed to them as a follow-up (queued or sent), is theirs.
	var missed domain.Message
	e.Do("POST", "/v1/tickets/"+tk.ID.String()+"/messages", ws.Agent.As(ws),
		map[string]any{"kind": "reply", "body_html": "<p>Are you still there?</p>"}).Want(t, 201).JSON(t, &missed)
	for _, status := range []string{"pending", "sent"} {
		e.Exec(`UPDATE messages SET delivery_status = $2 WHERE id = $1`, missed.ID, status)
		e.Do("POST", "/v1/tickets/"+tk.ID.String()+"/messages/"+missed.ID.String()+"/retract", ws.Agent.As(ws), nil).Want(t, 422)
	}
	// An earlier reply stays the first response when a later one is taken back.
	e.Exec(`UPDATE messages SET delivery_status = 'none' WHERE id = $1`, missed.ID)
	var later domain.Message
	e.Do("POST", "/v1/tickets/"+tk.ID.String()+"/messages", ws.Agent.As(ws),
		map[string]any{"kind": "reply", "body_html": "<p>One more thing</p>"}).Want(t, 201).JSON(t, &later)
	e.Do("POST", "/v1/tickets/"+tk.ID.String()+"/messages/"+later.ID.String()+"/retract", ws.Agent.As(ws), nil).Want(t, 200)
	if n := countRows(t, e, `SELECT count(*) FROM tickets WHERE id = $1 AND first_responded_at = $2`, tk.ID, missed.CreatedAt); n != 1 {
		t.Fatal("the first response moved when a later reply was taken back")
	}
}
