package widget

// A widget session is not a contact. The widget starts a session on every page of the host
// site, so a contact per session is one per visitor and per crawler, almost none of whom ever
// write. A session starts without one and remembers what the host page said (sessionIdentity);
// the contact is made, or found for a verified user id, when the visitor starts a conversation:
// their first message, or a file they attach to it.

import (
	"context"
	"net/http"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/useopendesk/opendesk/internal/auth"
	"github.com/useopendesk/opendesk/internal/db"
	"github.com/useopendesk/opendesk/internal/httpx"
)

// sessionIdentity is what a session without a contact remembers for it
// (widget_sessions.identity). UserID is set only when the session verified it (user_hash).
type sessionIdentity struct {
	UserID    string         `json:"user_id,omitempty"`
	Email     string         `json:"email,omitempty"`
	Name      string         `json:"name,omitempty"`
	AvatarURL string         `json:"avatar_url,omitempty"`
	Company   *widgetCompany `json:"company,omitempty"`
	Fields    map[string]any `json:"fields,omitempty"`
}

// merge keeps what the session knew and takes what this request adds; a new verified user id
// replaces the identity, since it is someone else's.
func (s sessionIdentity) merge(userID, email, name string) sessionIdentity {
	if userID != "" && userID != s.UserID {
		s = sessionIdentity{UserID: userID}
	}
	if email != "" {
		s.Email = email
	}
	if name != "" {
		s.Name = name
	}
	return s
}

// verifiedProfile adds what only a verified session may set: the picture, the company and the
// widget_ fields, applied when the contact is made (syncProfile).
func (s sessionIdentity) verifiedProfile(avatar string, company *widgetCompany, fields map[string]any) sessionIdentity {
	if s.UserID == "" {
		return s
	}
	if avatar != "" {
		s.AvatarURL = avatar
	}
	if company != nil {
		s.Company = company
	}
	if len(fields) > 0 {
		if s.Fields == nil {
			s.Fields = map[string]any{}
		}
		for k, v := range fields {
			s.Fields[k] = v
		}
	}
	return s
}

// knownContact is the existing contact of a verified user id, or zero: nothing is made.
func knownContact(ctx context.Context, tx pgx.Tx, wid uuid.UUID, userID string) (uuid.UUID, error) {
	var id uuid.UUID
	err := tx.QueryRow(ctx, `SELECT id FROM contacts WHERE workspace_id = $1 AND external_id = $2`, wid, userID).Scan(&id)
	if db.IsNotFound(err) {
		return uuid.Nil, nil
	}
	return id, err
}

// visitorContact is the visitor's contact, made when their session has none yet, from what
// the session remembered, and kept on the session from then on. The caller's transaction
// undoes it when the conversation is refused.
func (h *handler) visitorContact(ctx context.Context, tx pgx.Tx, p *auth.Principal) (uuid.UUID, error) {
	if p.ContactID != uuid.Nil {
		return p.ContactID, nil
	}
	var current *uuid.UUID
	var verified bool
	var idn sessionIdentity
	if err := tx.QueryRow(ctx, `SELECT contact_id, verified, identity FROM widget_sessions WHERE workspace_id = $1 AND id = $2 FOR UPDATE`,
		p.WorkspaceID, p.VisitorID).Scan(&current, &verified, &idn); err != nil {
		return uuid.Nil, err
	}
	id := uuid.Nil
	if current != nil {
		id = *current // another request of the session made it first
	} else {
		var err error
		if verified && idn.UserID != "" {
			id, err = h.verifiedContact(ctx, tx, p.WorkspaceID, idn.UserID, idn.Email, idn.Name)
		} else {
			id, err = h.newVisitorContact(ctx, tx, p.WorkspaceID, idn.Email, idn.Name)
		}
		if err != nil {
			return uuid.Nil, err
		}
		if verified {
			if err := widgetPicture(ctx, h.d, tx, p.WorkspaceID, id, idn.AvatarURL); err != nil {
				return uuid.Nil, err
			}
			if err := syncProfile(ctx, tx, p.WorkspaceID, id, idn.Company, idn.Fields); err != nil {
				return uuid.Nil, err
			}
		}
		if _, err := tx.Exec(ctx, `UPDATE widget_sessions SET contact_id = $3, identity = '{}' WHERE workspace_id = $1 AND id = $2`,
			p.WorkspaceID, p.VisitorID, id); err != nil {
			return uuid.Nil, err
		}
	}
	var blocked bool
	if err := tx.QueryRow(ctx, `SELECT blocked FROM contacts WHERE workspace_id = $1 AND id = $2`, p.WorkspaceID, id).Scan(&blocked); err != nil {
		return uuid.Nil, err
	}
	if blocked {
		return uuid.Nil, httpx.E(http.StatusForbidden, "blocked", "This conversation is not available.")
	}
	p.ContactID = id
	return id, nil
}

// visitorContactNow is visitorContact in a transaction of its own: a file uploaded before the
// first message.
func (h *handler) visitorContactNow(ctx context.Context, p *auth.Principal) error {
	if p.ContactID != uuid.Nil {
		return nil
	}
	return h.d.DB.Tx(ctx, func(tx pgx.Tx) error {
		_, err := h.visitorContact(ctx, tx, p)
		return err
	})
}
