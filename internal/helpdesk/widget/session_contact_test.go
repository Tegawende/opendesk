package widget_test

import (
	"testing"

	"github.com/google/uuid"

	"github.com/useopendesk/opendesk/internal/crypto"
	"github.com/useopendesk/opendesk/internal/helpdesk/hdtest"
)

// A widget session is not a contact: the widget starts one on every page of the host site,
// so a contact per session is one per visitor. The contact is made when the visitor starts a
// conversation, from what the session remembers.

type lazySession struct {
	VisitorToken   string `json:"visitor_token"`
	EmailCollected bool   `json:"email_collected"`
	WSToken        string `json:"ws_token"`
	Contact        *struct {
		ID    uuid.UUID `json:"id"`
		Email *string   `json:"email"`
		Name  string    `json:"name"`
	} `json:"contact"`
}

func lazyStart(t *testing.T, e *hdtest.Env, body map[string]any) lazySession {
	t.Helper()
	var s lazySession
	e.Do("POST", "/v1/widget/session", hdtest.Auth{}, body).Want(t, 200).JSON(t, &s)
	return s
}

func lazyRow(t *testing.T, e *hdtest.Env, sql string, args []any, dst ...any) {
	t.Helper()
	if err := e.D.DB.Pool.QueryRow(e.Ctx, sql, args...).Scan(dst...); err != nil {
		t.Fatalf("query %q: %v", sql, err)
	}
}

// contactOf is the contact of the visitor's session: made by their first message, zero before.
func contactOf(t *testing.T, e *hdtest.Env, s session) uuid.UUID {
	t.Helper()
	var id *uuid.UUID
	e.Scalar(`SELECT contact_id FROM widget_sessions WHERE token_hash = $1`, &id, crypto.HashToken(s.VisitorToken))
	if id == nil {
		return uuid.Nil
	}
	return *id
}

// talked starts a conversation for the visitor (which makes their contact) and resumes the
// session, which then names it.
func talked(t *testing.T, e *hdtest.Env, s session, resume map[string]any) session {
	t.Helper()
	e.Do("POST", "/v1/widget/conversations", hdtest.Auth{Visitor: s.VisitorToken}, map[string]any{"body": "Hello"}).Want(t, 201)
	resume["visitor_token"] = s.VisitorToken
	r := start(t, e, resume)
	if r.VisitorToken != s.VisitorToken || r.Contact.ID == uuid.Nil {
		t.Fatalf("resumed after the first message: %+v", r)
	}
	return r
}

func countContacts(t *testing.T, e *hdtest.Env, wid uuid.UUID) int {
	t.Helper()
	var n int
	e.Scalar(`SELECT count(*) FROM contacts WHERE workspace_id = $1`, &n, wid)
	return n
}

func TestSessionMakesNoContact(t *testing.T) {
	e, ws := setup(t)
	before := countContacts(t, e, ws.ID)

	// Page views: sessions, resumed or new, with what the host page knows. No contact.
	s := lazyStart(t, e, map[string]any{"key": ws.WidgetKey})
	if s.Contact != nil || s.VisitorToken == "" || s.WSToken != "" {
		t.Fatalf("anonymous session: %+v", s)
	}
	auth := hdtest.Auth{Visitor: s.VisitorToken}
	r := lazyStart(t, e, map[string]any{"key": ws.WidgetKey, "visitor_token": s.VisitorToken, "name": "Ana", "email": "ana@example.com"})
	if r.VisitorToken != s.VisitorToken || r.Contact != nil || !r.EmailCollected {
		t.Fatalf("resumed session: %+v", r)
	}
	for range 5 {
		lazyStart(t, e, map[string]any{"key": ws.WidgetKey})
	}
	if n := countContacts(t, e, ws.ID); n != before {
		t.Fatalf("contacts after sessions: %d, want %d", n, before)
	}
	// The widget's list works without one: no conversations.
	list := e.Do("GET", "/v1/widget/conversations", auth, nil).Want(t, 200).Map(t)
	if d, _ := list["data"].([]any); len(d) != 0 {
		t.Fatal(list)
	}

	// The first message makes the contact, with the name and email the session remembered.
	conv := e.Do("POST", "/v1/widget/conversations", auth, map[string]any{"body": "Hello"}).Want(t, 201).Map(t)
	if n := countContacts(t, e, ws.ID); n != before+1 {
		t.Fatalf("contacts after the first message: %d, want %d", n, before+1)
	}
	var cid uuid.UUID
	var name, email, source string
	lazyRow(t, e, `SELECT s.contact_id, c.name, c.email, c.custom->>'source' FROM widget_sessions s JOIN contacts c ON c.id = s.contact_id
		WHERE s.token_hash = $1`, []any{crypto.HashToken(s.VisitorToken)}, &cid, &name, &email, &source)
	if name != "Ana" || email != "ana@example.com" || source != "widget" {
		t.Fatal(name, email, source)
	}
	var ticketContact uuid.UUID
	e.Scalar(`SELECT contact_id FROM tickets WHERE id = $1`, &ticketContact, conv["conversation"].(map[string]any)["id"])
	if ticketContact != cid {
		t.Fatal("ticket contact", ticketContact, cid)
	}
	// From then on the session has it: its socket token, its conversation, no second contact.
	again := lazyStart(t, e, map[string]any{"key": ws.WidgetKey, "visitor_token": s.VisitorToken})
	if again.Contact == nil || again.Contact.ID != cid || again.WSToken == "" {
		t.Fatalf("session after the first message: %+v", again)
	}
	e.Do("POST", "/v1/widget/conversations", auth, map[string]any{"body": "Another question"}).Want(t, 201)
	if n := countContacts(t, e, ws.ID); n != before+1 {
		t.Fatalf("contacts after a second conversation: %d", n)
	}
}

func TestSessionRemembersVerifiedIdentity(t *testing.T) {
	e, ws := setup(t)
	before := countContacts(t, e, ws.ID)
	good := crypto.HMACHex([]byte("s3cret"), "shopify-77")
	s := lazyStart(t, e, map[string]any{"key": ws.WidgetKey, "user_id": "shopify-77", "user_hash": good, "name": "Uma", "email": "uma@example.com"})
	if s.Contact != nil {
		t.Fatalf("a signed-in visitor with no contact yet got one: %+v", s)
	}
	if n := countContacts(t, e, ws.ID); n != before {
		t.Fatal("contact made by a verified session", n)
	}
	// Someone not signed in on that browser does not inherit the remembered identity.
	anon := lazyStart(t, e, map[string]any{"key": ws.WidgetKey, "visitor_token": s.VisitorToken})
	if anon.VisitorToken == s.VisitorToken || anon.EmailCollected {
		t.Fatalf("anonymous resume of a verified session: %+v", anon)
	}
	e.Do("POST", "/v1/widget/conversations", hdtest.Auth{Visitor: s.VisitorToken}, map[string]any{"body": "Where is my order?"}).Want(t, 201)
	var ext, email string
	var verified bool
	lazyRow(t, e, `SELECT c.external_id, c.email, s.verified FROM widget_sessions s JOIN contacts c ON c.id = s.contact_id WHERE s.token_hash = $1`,
		[]any{crypto.HashToken(s.VisitorToken)}, &ext, &email, &verified)
	if ext != "shopify-77" || email != "uma@example.com" || !verified {
		t.Fatal(ext, email, verified)
	}
	// A contact that exists is linked at once (nothing is made): the visitor sees their chats.
	var cid uuid.UUID
	e.Scalar(`SELECT id FROM contacts WHERE workspace_id = $1 AND external_id = 'shopify-77'`, &cid, ws.ID)
	other := lazyStart(t, e, map[string]any{"key": ws.WidgetKey, "user_id": "shopify-77", "user_hash": good})
	if other.Contact == nil || other.Contact.ID != cid || other.WSToken == "" {
		t.Fatalf("second browser of a known customer: %+v", other)
	}
	if n := countContacts(t, e, ws.ID); n != before+1 {
		t.Fatal("contacts", n)
	}
}

func TestUploadMakesContact(t *testing.T) {
	e, ws := setup(t)
	s := lazyStart(t, e, map[string]any{"key": ws.WidgetKey})
	before := countContacts(t, e, ws.ID)
	auth := hdtest.Auth{Visitor: s.VisitorToken}
	att := e.Upload("/v1/widget/attachments", auth, "photo.png", pngBytes()).Want(t, 201).Map(t)
	if n := countContacts(t, e, ws.ID); n != before+1 {
		t.Fatal("an upload starts the conversation: contacts", n)
	}
	e.Do("POST", "/v1/widget/conversations", auth, map[string]any{"body": "See the photo", "attachment_ids": []any{att["id"]}}).Want(t, 201)
	if n := countContacts(t, e, ws.ID); n != before+1 {
		t.Fatal("contacts", n)
	}
}
