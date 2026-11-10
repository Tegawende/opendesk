package tickets_test

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/useopendesk/opendesk/internal/crypto"
	"github.com/useopendesk/opendesk/internal/domain"
	"github.com/useopendesk/opendesk/internal/helpdesk/contacts"
	"github.com/useopendesk/opendesk/internal/helpdesk/hdtest"
	"github.com/useopendesk/opendesk/internal/helpdesk/tickets"
	"github.com/useopendesk/opendesk/internal/helpdesk/widget"
	"github.com/useopendesk/opendesk/internal/ids"
	"github.com/useopendesk/opendesk/internal/jobs"
)

type chatSession struct {
	VisitorToken string            `json:"visitor_token"`
	Contact      domain.ContactRef `json:"contact"`
	Unread       int               `json:"unread"`
	UnreadLatest *domain.Message   `json:"unread_latest"`
}

type chatList struct {
	Data []struct {
		ID     uuid.UUID `json:"id"`
		Unread int       `json:"unread"`
	} `json:"data"`
}

func chatEnv(t *testing.T) (*hdtest.Env, hdtest.WS) {
	t.Helper()
	e := hdtest.New(t, tickets.Register, widget.Register, contacts.Register)
	return e, e.NewWorkspace()
}

func visitorSession(t *testing.T, e *hdtest.Env, body map[string]any) chatSession {
	t.Helper()
	var s chatSession
	e.Do("POST", "/v1/widget/session", hdtest.Auth{}, body).Want(t, 200).JSON(t, &s)
	return s
}

// visitorWithContact is a visitor who has written in the chat: a session gets its contact with
// the visitor's first message.
func visitorWithContact(t *testing.T, e *hdtest.Env, body map[string]any) chatSession {
	t.Helper()
	s := visitorSession(t, e, body)
	e.Do("POST", "/v1/widget/conversations", hdtest.Auth{Visitor: s.VisitorToken}, map[string]any{"body": "Hello"}).Want(t, 201)
	resume := map[string]any{"visitor_token": s.VisitorToken}
	for k, v := range body {
		resume[k] = v
	}
	s = visitorSession(t, e, resume)
	if s.Contact.ID == uuid.Nil {
		t.Fatal("no contact after the first message")
	}
	return s
}

func startChat(e *hdtest.Env, ws hdtest.WS, a hdtest.Auth, body map[string]any) *hdtest.Resp {
	body["channel"] = "chat"
	if _, ok := body["body_html"]; !ok {
		body["body_html"] = "<p>Hi, about your order</p>"
	}
	return e.Do("POST", "/v1/tickets", a, body)
}

func newContact(e *hdtest.Env, ws hdtest.WS, email string, externalID *string) uuid.UUID {
	id := ids.New()
	e.Exec(`INSERT INTO contacts (id, workspace_id, email, name, external_id) VALUES ($1, $2, $3, 'Someone', $4)`, id, ws.ID, email, externalID)
	return id
}

// The team starts a chat with a visitor who used the widget: it opens in their widget, they
// see it unread there, and reading it marks it seen for the team.
func TestAgentStartsChat(t *testing.T) {
	e, ws := chatEnv(t)
	s := visitorWithContact(t, e, map[string]any{"key": ws.WidgetKey, "email": "visitor@example.com"})
	if s.Unread != 0 || s.UnreadLatest != nil {
		t.Fatal("unread before anything was sent", s.Unread)
	}
	// The visitor left the page: the first message also goes by email.
	e.Exec(`UPDATE widget_sessions SET last_seen_at = now() - interval '1 hour' WHERE contact_id = $1`, s.Contact.ID)

	var tk domain.Ticket
	startChat(e, ws, ws.Agent.As(ws), map[string]any{"contact_id": s.Contact.ID}).Want(t, 201).JSON(t, &tk)
	if tk.Channel != "chat" || tk.WidgetID == nil || *tk.WidgetID != ws.WidgetID || tk.BrandID != ws.BrandID || tk.Subject != "Hi, about your order" {
		t.Fatalf("ticket %+v", tk)
	}
	if tk.VisitorReadAt != nil {
		t.Fatal("seen before the visitor opened it")
	}
	if n := len(e.Jobs(jobs.ChatFollowUpArgs{}.Kind(), ws.ID)); n != 1 {
		t.Fatalf("chat follow-up jobs: %d", n)
	}

	// The visitor's next page load: one unread reply, with the message to preview.
	back := visitorSession(t, e, map[string]any{"key": ws.WidgetKey, "visitor_token": s.VisitorToken})
	if back.Unread != 1 || back.UnreadLatest == nil || back.UnreadLatest.TicketID != tk.ID || !strings.Contains(back.UnreadLatest.BodyText, "about your order") {
		t.Fatalf("session unread %d %+v", back.Unread, back.UnreadLatest)
	}
	va := hdtest.Auth{Visitor: s.VisitorToken}
	var list chatList
	e.Do("GET", "/v1/widget/conversations", va, nil).Want(t, 200).JSON(t, &list)
	if len(list.Data) != 2 || list.Data[0].ID != tk.ID || list.Data[0].Unread != 1 { // and the visitor's own
		t.Fatalf("conversations %+v", list.Data)
	}

	// Someone else typing the same email in the widget never sees it.
	other := visitorSession(t, e, map[string]any{"key": ws.WidgetKey, "email": "visitor@example.com"})
	var none chatList
	e.Do("GET", "/v1/widget/conversations", hdtest.Auth{Visitor: other.VisitorToken}, nil).Want(t, 200).JSON(t, &none)
	if len(none.Data) != 0 || other.Unread != 0 {
		t.Fatal("a claimed email saw the chat")
	}

	// Fetching it is not reading it: the widget fetches in the background, closed or not.
	e.Do("GET", "/v1/widget/conversations/"+tk.ID.String()+"/messages", va, nil).Want(t, 200)
	e.Do("GET", "/v1/widget/conversations", va, nil).Want(t, 200).JSON(t, &list)
	if list.Data[0].Unread != 1 {
		t.Fatal("a fetch read it", list.Data[0].Unread)
	}
	var fetched domain.Ticket
	e.Do("GET", "/v1/tickets/"+tk.ID.String(), ws.Agent.As(ws), nil).Want(t, 200).JSON(t, &fetched)
	if fetched.VisitorReadAt != nil {
		t.Fatal("visitor_read_at set by a fetch")
	}

	// Reading it on screen (/seen): no longer unread, and the team sees when.
	e.Do("POST", "/v1/widget/conversations/"+tk.ID.String()+"/seen", va, nil).Want(t, 204)
	e.Do("GET", "/v1/widget/conversations", va, nil).Want(t, 200).JSON(t, &list)
	if list.Data[0].Unread != 0 {
		t.Fatal("still unread after reading")
	}
	var got domain.Ticket
	e.Do("GET", "/v1/tickets/"+tk.ID.String(), ws.Agent.As(ws), nil).Want(t, 200).JSON(t, &got)
	if got.VisitorReadAt == nil {
		t.Fatal("visitor_read_at not set")
	}
	if again := visitorSession(t, e, map[string]any{"key": ws.WidgetKey, "visitor_token": s.VisitorToken}); again.Unread != 0 {
		t.Fatal("unread after reading", again.Unread)
	}

	// A note is not a reply the visitor reads, and does not count.
	e.Do("POST", "/v1/tickets/"+tk.ID.String()+"/messages", ws.Agent.As(ws), map[string]any{"kind": "note", "body_html": "<p>internal</p>"}).Want(t, 201)
	if n := visitorSession(t, e, map[string]any{"key": ws.WidgetKey, "visitor_token": s.VisitorToken}).Unread; n != 0 {
		t.Fatal("a note counted as unread", n)
	}
}

// Only a contact the widget can show the chat to gets one.
func TestChatStartReach(t *testing.T) {
	e, ws := chatEnv(t)
	agent := ws.Agent.As(ws)

	// Known only by email: never used the chat.
	emailOnly := newContact(e, ws, "mail@example.com", nil)
	r := startChat(e, ws, agent, map[string]any{"contact_id": emailOnly}).Want(t, http.StatusUnprocessableEntity)
	if !strings.Contains(string(r.Body), "chat_unreachable") {
		t.Fatal(string(r.Body))
	}
	// A verified identity reaches them in any widget.
	ext := "user-9"
	verified := newContact(e, ws, "u9@example.com", &ext)
	startChat(e, ws, agent, map[string]any{"contact_id": verified}).Want(t, 201)
	// A visitor whose session ran out is no longer reachable.
	s := visitorWithContact(t, e, map[string]any{"key": ws.WidgetKey})
	e.Exec(`UPDATE widget_sessions SET last_seen_at = now() - interval '91 days' WHERE contact_id = $1`, s.Contact.ID)
	startChat(e, ws, agent, map[string]any{"contact_id": s.Contact.ID}).Want(t, http.StatusUnprocessableEntity)
	// Blocked.
	e.Exec(`UPDATE contacts SET blocked = true WHERE id = $1`, verified)
	startChat(e, ws, agent, map[string]any{"contact_id": verified}).Want(t, http.StatusUnprocessableEntity)
	// Is the contact list's flag the same rule.
	var c domain.Contact
	e.Do("GET", "/v1/contacts/"+verified.String(), agent, nil).Want(t, 200).JSON(t, &c)
	if c.ChatReachable {
		t.Fatal("a blocked contact shows as reachable")
	}
}

// An address a visitor types proves nothing: once mail from it joins their contact, the
// chat reaches that contact only through a verified identity.
func TestChatStartTypedEmail(t *testing.T) {
	e, ws := chatEnv(t)
	agent := ws.Agent.As(ws)
	ctx := context.Background()
	reachable := func(id uuid.UUID) bool {
		var c domain.Contact
		e.Do("GET", "/v1/contacts/"+id.String(), agent, nil).Want(t, 200).JSON(t, &c)
		return c.ChatReachable
	}

	at := "@" + "shop.test"
	typed := "ana" + at

	// A visitor types a free address: the widget's contact takes it, and holds only the chat.
	s := visitorWithContact(t, e, map[string]any{"key": ws.WidgetKey, "email": typed})
	if s.Contact.Email == nil || *s.Contact.Email != typed {
		t.Fatalf("contact %+v", s.Contact)
	}
	va := hdtest.Auth{Visitor: s.VisitorToken}
	e.Do("POST", "/v1/widget/conversations", va, map[string]any{"body": "Hello"}).Want(t, 201)
	if !reachable(s.Contact.ID) {
		t.Fatal("a pure widget visitor is not reachable")
	}
	startChat(e, ws, agent, map[string]any{"contact_id": s.Contact.ID}).Want(t, 201)

	// Mail from that address arrives: it joins the same contact.
	err := e.D.DB.Tx(ctx, func(tx pgx.Tx) error {
		cid, _, err := domain.UpsertContactByEmail(ctx, tx, ws.ID, typed, "Ana")
		if err != nil {
			return err
		}
		if cid != s.Contact.ID {
			t.Fatalf("mail made another contact %s", cid)
		}
		_, err = domain.CreateTicket(ctx, tx, domain.NewTicket{WorkspaceID: ws.ID, Subject: "My invoice", Channel: domain.ChannelEmail, ContactID: &cid})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	r := startChat(e, ws, agent, map[string]any{"contact_id": s.Contact.ID}).Want(t, http.StatusUnprocessableEntity)
	if !strings.Contains(string(r.Body), "chat_unreachable") {
		t.Fatal(string(r.Body))
	}
	if reachable(s.Contact.ID) {
		t.Fatal("a contact holding mail shows as reachable")
	}

	// The same when the mail answers the chat itself (a reply to its follow-up email).
	other := visitorWithContact(t, e, map[string]any{"key": ws.WidgetKey, "email": "ben" + at})
	var conv struct {
		Conversation struct{ ID uuid.UUID } `json:"conversation"`
	}
	e.Do("POST", "/v1/widget/conversations", hdtest.Auth{Visitor: other.VisitorToken}, map[string]any{"body": "Hi"}).Want(t, 201).JSON(t, &conv)
	if !reachable(other.Contact.ID) {
		t.Fatal("a pure widget visitor is not reachable")
	}
	e.Exec(`INSERT INTO messages (workspace_id, ticket_id, kind, direction, author_type, author_contact_id, body_text, email_message_id)
		VALUES ($1, $2, 'reply', 'inbound', 'contact', $3, 'by mail', '<reply-1@mail.test>')`, ws.ID, conv.Conversation.ID, other.Contact.ID)
	startChat(e, ws, agent, map[string]any{"contact_id": other.Contact.ID}).Want(t, http.StatusUnprocessableEntity)
	if reachable(other.Contact.ID) {
		t.Fatal("a chat answered by mail shows as reachable")
	}

	// A verified identity is reached whatever else their contact holds.
	e.Exec(`UPDATE contacts SET external_id = 'user-ana' WHERE id = $1`, s.Contact.ID)
	startChat(e, ws, agent, map[string]any{"contact_id": s.Contact.ID}).Want(t, 201)
	if !reachable(s.Contact.ID) {
		t.Fatal("a verified contact is not reachable")
	}
}

// The list's flag and the start check are one rule: a session counts in a widget that is on,
// and a chat of a brand needs the visitor to have used that brand's chat.
func TestChatStartBrandAndWidget(t *testing.T) {
	e, ws := chatEnv(t)
	agent := ws.Agent.As(ws)
	s := visitorWithContact(t, e, map[string]any{"key": ws.WidgetKey})
	var c domain.Contact
	e.Do("GET", "/v1/contacts/"+s.Contact.ID.String(), agent, nil).Want(t, 200).JSON(t, &c)
	if !c.ChatReachable {
		t.Fatal("a visitor with a live session is not reachable")
	}
	// Another brand's widget does not show this brand's visitor a chat.
	brand := newBrand(e, ws)
	e.Exec(`INSERT INTO widgets (id, workspace_id, brand_id, name, key) VALUES ($1, $2, $3, 'Other', $4)`, ids.New(), ws.ID, brand, "wk_other_"+brand.String())
	r := startChat(e, ws, agent, map[string]any{"contact_id": s.Contact.ID, "brand_id": brand}).Want(t, http.StatusUnprocessableEntity)
	if !strings.Contains(string(r.Body), "chat_unreachable") {
		t.Fatal(string(r.Body))
	}
	// Their only widget turned off: reachable nowhere, by either.
	e.SetWidgetSettings(ws, ws.WidgetID, map[string]any{"enabled": false})
	e.Do("GET", "/v1/contacts/"+s.Contact.ID.String(), agent, nil).Want(t, 200).JSON(t, &c)
	if c.ChatReachable {
		t.Fatal("a session in a widget that is off shows as reachable")
	}
	startChat(e, ws, agent, map[string]any{"contact_id": s.Contact.ID}).Want(t, http.StatusUnprocessableEntity)
}

// A chat without a subject is named after its first message as the visitor reads it.
func TestChatStartSubjectFilled(t *testing.T) {
	e, ws := chatEnv(t)
	s := visitorWithContact(t, e, map[string]any{"key": ws.WidgetKey, "name": "Ana Lopez"})
	var tk domain.Ticket
	startChat(e, ws, ws.Agent.As(ws), map[string]any{"contact_id": s.Contact.ID,
		"body_html": "<p>Hi {{ contact.first_name }}, your order shipped</p>"}).Want(t, 201).JSON(t, &tk)
	if tk.Subject != "Hi Ana, your order shipped" {
		t.Fatalf("subject %q", tk.Subject)
	}
	var list struct {
		Data []struct {
			Subject string `json:"subject"`
		} `json:"data"`
	}
	e.Do("GET", "/v1/widget/conversations", hdtest.Auth{Visitor: s.VisitorToken}, nil).Want(t, 200).JSON(t, &list)
	if len(list.Data) != 2 || list.Data[0].Subject != "Hi Ana, your order shipped" { // and the visitor's own
		t.Fatalf("widget conversations %+v", list.Data)
	}
}

// What a chat start refuses.
func TestChatStartValidation(t *testing.T) {
	e, ws := chatEnv(t)
	ext := "user-1"
	contact := newContact(e, ws, "c@example.com", &ext)
	agent := ws.Agent.As(ws)
	cases := map[string]map[string]any{
		"note":            {"contact_id": contact, "kind": "note"},
		"no message":      {"contact_id": contact, "body_html": "<p> </p>"},
		"new contact":     {"contact": map[string]any{"email": "new@example.com"}},
		"no contact":      {},
		"mailbox":         {"contact_id": contact, "mailbox_id": uuid.New()},
		"cc":              {"contact_id": contact, "cc": []string{"x@example.com"}},
		"unknown widget":  {"contact_id": contact, "widget_id": uuid.New()},
		"other brand":     {"contact_id": contact, "widget_id": ws.WidgetID, "brand_id": newBrand(e, ws)},
		"widget on email": {"contact_id": contact, "widget_id": ws.WidgetID, "channel_override": "api"},
	}
	for name, body := range cases {
		if body["channel_override"] != nil {
			delete(body, "channel_override")
			body["subject"] = "Hi"
			body["body_html"] = "<p>Hi</p>"
			e.Do("POST", "/v1/tickets", agent, body).Want(t, http.StatusUnprocessableEntity)
			continue
		}
		if r := startChat(e, ws, agent, body); r.Status != http.StatusUnprocessableEntity {
			t.Errorf("%s: %d %s", name, r.Status, r.Body)
		}
	}
	// Another workspace's widget is not found here.
	other := e.NewWorkspace()
	startChat(e, ws, agent, map[string]any{"contact_id": contact, "widget_id": other.WidgetID}).Want(t, http.StatusUnprocessableEntity)
	// A widget turned off.
	e.SetWidgetSettings(ws, ws.WidgetID, map[string]any{"enabled": false})
	startChat(e, ws, agent, map[string]any{"contact_id": contact, "widget_id": ws.WidgetID}).Want(t, http.StatusUnprocessableEntity)
	e.SetWidgetSettings(ws, ws.WidgetID, map[string]any{"enabled": true})
	// Viewers read only.
	startChat(e, ws, ws.Viewer.As(ws), map[string]any{"contact_id": contact}).Want(t, http.StatusForbidden)
	// Other channels still need a subject.
	e.Do("POST", "/v1/tickets", agent, map[string]any{"body_html": "<p>Hi</p>"}).Want(t, http.StatusUnprocessableEntity)
}

// The team may start a few chats with one contact an hour, not a stream of them.
func TestChatStartLimit(t *testing.T) {
	e, ws := chatEnv(t)
	ext := "user-2"
	contact := newContact(e, ws, "l@example.com", &ext)
	agent := ws.Agent.As(ws)
	// Starts refused for another reason spend nothing.
	for i := 0; i < 3; i++ {
		startChat(e, ws, agent, map[string]any{"contact_id": contact, "body_html": "<p> </p>"}).Want(t, http.StatusUnprocessableEntity)
	}
	for i := 0; i < 10; i++ {
		startChat(e, ws, agent, map[string]any{"contact_id": contact}).Want(t, 201)
	}
	startChat(e, ws, agent, map[string]any{"contact_id": contact}).Want(t, http.StatusTooManyRequests)
}

// A chat the team starts in a verified visitor's widget shows on their next visit.
func TestStartedChatReachesVerifiedVisitor(t *testing.T) {
	e, ws := chatEnv(t)
	sealed, err := e.D.Box.Seal([]byte("s3cret"))
	if err != nil {
		t.Fatal(err)
	}
	e.Exec(`UPDATE widgets SET secret_enc = $2 WHERE id = $1`, ws.WidgetID, sealed)
	ext := "user-77"
	contact := newContact(e, ws, "v@example.com", &ext)
	var tk domain.Ticket
	startChat(e, ws, ws.Agent.As(ws), map[string]any{"contact_id": contact}).Want(t, 201).JSON(t, &tk)
	s := visitorSession(t, e, map[string]any{"key": ws.WidgetKey, "user_id": ext, "user_hash": crypto.HMACHex([]byte("s3cret"), ext)})
	if s.Contact.ID != contact || s.Unread != 1 || s.UnreadLatest == nil || s.UnreadLatest.TicketID != tk.ID {
		t.Fatalf("verified visitor: %+v %d", s.Contact, s.Unread)
	}
}

func newBrand(e *hdtest.Env, ws hdtest.WS) uuid.UUID {
	id := ids.New()
	e.Exec(`INSERT INTO brands (id, workspace_id, name, slug) VALUES ($1, $2, 'Other', $3)`, id, ws.ID, "other-"+id.String())
	return id
}
