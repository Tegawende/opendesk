package tickets_test

import (
	"testing"

	"github.com/useopendesk/opendesk/internal/domain"
	"github.com/useopendesk/opendesk/internal/helpdesk/hdtest"
)

func TestInboundDirection(t *testing.T) {
	e, ws := newEnv(t)
	contact := map[string]any{"email": "form@example.com", "name": "Form Visitor"}

	// A ticket starts with the customer's own message: stored as theirs, sent to no one.
	tk := createTicket(t, e, ws, map[string]any{"subject": "From the form", "contact": contact, "direction": "inbound",
		"body_html": "<p>My order is late</p>", "tags": []string{"web-form"}})
	var msgs struct{ Data []domain.Message }
	e.Do("GET", "/v1/tickets/"+tk.ID.String()+"/messages", ws.Viewer.As(ws), nil).Want(t, 200).JSON(t, &msgs)
	if len(msgs.Data) != 1 {
		t.Fatalf("messages: %+v", msgs.Data)
	}
	m := msgs.Data[0]
	if m.Direction != "inbound" || m.Author.Type != "contact" || m.DeliveryStatus != "none" || m.BodyText != "My order is late" {
		t.Fatalf("inbound message: %+v", m)
	}
	if tk.FirstRespondedAt != nil {
		t.Fatal("a customer's message is not a first response")
	}

	// A later customer message, then an agent's reply as before.
	e.Do("POST", "/v1/tickets/"+tk.ID.String()+"/messages", ws.Agent.As(ws),
		map[string]any{"kind": "reply", "direction": "inbound", "body_html": "<p>Still waiting</p>"}).Want(t, 201)
	e.Do("POST", "/v1/tickets/"+tk.ID.String()+"/messages", ws.Agent.As(ws),
		map[string]any{"kind": "reply", "body_html": "<p>On it</p>"}).Want(t, 201)
	e.Do("GET", "/v1/tickets/"+tk.ID.String()+"/messages", ws.Viewer.As(ws), nil).Want(t, 200).JSON(t, &msgs)
	got := []string{}
	for _, m := range msgs.Data {
		got = append(got, m.Direction)
	}
	if len(got) != 3 || got[0] != "inbound" || got[1] != "inbound" || got[2] != "outbound" {
		t.Fatalf("directions %v", got)
	}

	// Refusals.
	e.Do("POST", "/v1/tickets/"+tk.ID.String()+"/messages", ws.Agent.As(ws),
		map[string]any{"kind": "note", "direction": "inbound", "body_html": "x"}).Want(t, 422)
	e.Do("POST", "/v1/tickets/"+tk.ID.String()+"/messages", ws.Agent.As(ws),
		map[string]any{"kind": "reply", "direction": "inbound", "body_html": "<p> </p>"}).Want(t, 422)
	e.Do("POST", "/v1/tickets", ws.Agent.As(ws), map[string]any{"subject": "No contact", "direction": "inbound", "body_html": "x"}).Want(t, 422)
	e.Do("POST", "/v1/tickets", ws.Agent.As(ws), map[string]any{"subject": "Note", "contact": contact, "direction": "inbound",
		"kind": "note", "body_html": "x"}).Want(t, 422)
	e.Do("POST", "/v1/tickets", ws.Agent.As(ws), map[string]any{"subject": "Chat", "channel": "chat", "contact_id": tk.Contact.ID.String(),
		"direction": "inbound", "body_html": "x"}).Want(t, 422)
	e.Do("POST", "/v1/tickets", ws.Viewer.As(ws), map[string]any{"subject": "x", "contact": contact, "direction": "inbound", "body_html": "y"}).Want(t, 403)
	e.Do("POST", "/v1/tickets", hdtest.Auth{}, map[string]any{"subject": "x", "direction": "inbound"}).Want(t, 401)
}
