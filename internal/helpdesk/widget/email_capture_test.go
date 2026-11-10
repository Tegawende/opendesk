package widget_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/useopendesk/opendesk/internal/helpdesk/hdtest"
)

func TestRequiredEmailCollisionCaptureAndResume(t *testing.T) {
	e, ws := setup(t)
	existing := start(t, e, map[string]any{"key": ws.WidgetKey, "email": "existing@example.com"})
	existingAuth := hdtest.Auth{Visitor: existing.VisitorToken}
	ticket := e.Do("POST", "/v1/widget/conversations", existingAuth, map[string]any{"body": "Private history"}).Want(t, 201).Map(t)
	existing.Contact.ID = contactOf(t, e, existing) // made by the first message
	e.SetWidgetSettings(ws, uuid.Nil, map[string]any{"require_email": true})
	anon := start(t, e, map[string]any{"key": ws.WidgetKey})
	if anon.EmailCollected {
		t.Fatal("anonymous visitor already collected email")
	}
	collected := start(t, e, map[string]any{"key": ws.WidgetKey, "visitor_token": anon.VisitorToken, "email": "existing@example.com"})
	// The same session, still without a contact.
	if !collected.EmailCollected || collected.Contact.Email != nil || collected.VisitorToken != anon.VisitorToken || collected.Contact.ID != uuid.Nil {
		t.Fatalf("captured session: %+v", collected)
	}
	resumed := start(t, e, map[string]any{"key": ws.WidgetKey, "visitor_token": collected.VisitorToken})
	if !resumed.EmailCollected || resumed.Contact.Email != nil {
		t.Fatalf("resumed session: %+v", resumed)
	}
	auth := hdtest.Auth{Visitor: resumed.VisitorToken}
	e.Do("POST", "/v1/widget/conversations", auth, map[string]any{"body": "Hello"}).Want(t, 201)
	if mine := contactOf(t, e, resumed); mine == uuid.Nil || mine == existing.Contact.ID {
		t.Fatal("the visitor's contact", mine)
	}
	var claimed string
	e.Scalar(`SELECT custom->>'widget_email' FROM contacts WHERE id = $1`, &claimed, contactOf(t, e, resumed))
	if claimed != "existing@example.com" {
		t.Fatal("claimed address", claimed)
	}
	e.Do("GET", "/v1/widget/conversations/"+ticket["conversation"].(map[string]any)["id"].(string)+"/messages", auth, nil).Want(t, 404)
}
