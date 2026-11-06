package email

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/useopendesk/opendesk/internal/db"
	"github.com/useopendesk/opendesk/internal/displayname"
	"github.com/useopendesk/opendesk/internal/domain"
	"github.com/useopendesk/opendesk/internal/templates"
)

// renderEmail puts a reply into its brand's layout with the signature and, when the
// layout quotes them, the replies before message (contracts/templates.md).
func (s *service) renderEmail(ctx context.Context, ref templates.Ref, p templates.Parts, message *uuid.UUID) (templates.Email, error) {
	tc, err := templates.Load(ctx, s.d.DB.Pool, s.d.Cfg, ref)
	if err != nil {
		return templates.Email{}, err
	}
	design, err := templates.LoadDesign(ctx, s.d.DB.Pool, ref.WorkspaceID, tc.BrandID)
	if err != nil {
		return templates.Email{}, err
	}
	p.Signature = tc.Signature()
	if design.Layout.History != "none" && ref.TicketID != nil {
		limit := 1
		if design.Layout.History == "all" {
			limit = 10
		}
		if p.History, err = loadHistory(ctx, s.d.DB.Pool, ref.WorkspaceID, *ref.TicketID, message, limit); err != nil {
			return templates.Email{}, err
		}
	}
	return tc.RenderEmail(design, p), nil
}

// senderSQL are who wrote a message m and the address it was sent from (the customer's From,
// or the address a reply went out from; for a customer's message without one, such as chat,
// their contact's), with senderJoins.
const senderSQL = `COALESCE(NULLIF(m.from_name, ''), u.name, c.name, m.from_email, ''),
	COALESCE(NULLIF(m.from_email, ''), CASE WHEN m.direction = 'inbound' THEN c.email::text END, '')`

const senderJoins = `LEFT JOIN users u ON u.id = m.author_user_id
	LEFT JOIN contacts c ON c.id = m.author_contact_id AND c.workspace_id = m.workspace_id`

// loadHistory returns up to limit public replies of the ticket before message (or the
// latest ones), newest first: what the customer wrote, and what was sent or posted to them.
func loadHistory(ctx context.Context, q db.Querier, wid, ticketID uuid.UUID, message *uuid.UUID, limit int) ([]templates.HistoryItem, error) {
	var before *uuid.UUID
	if message != nil && *message != uuid.Nil {
		before = message
	}
	rows, err := q.Query(ctx, `SELECT `+senderSQL+`, m.created_at, m.body_html, m.body_text
		FROM messages m `+senderJoins+`
		WHERE m.workspace_id = $1 AND m.ticket_id = $2 AND `+domain.CustomerConversationSQL+`
			AND ($3::uuid IS NULL OR (m.id <> $3 AND m.created_at <= (SELECT created_at FROM messages WHERE workspace_id = $1 AND id = $3)))
		ORDER BY m.created_at DESC, m.id DESC LIMIT $4`, wid, ticketID, before, limit)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (templates.HistoryItem, error) {
		var h templates.HistoryItem
		var at time.Time
		err := r.Scan(&h.Author, &h.Email, &at, &h.HTML, &h.Text)
		h.Author, h.At = displayname.Displayable(h.Author), at
		return h, err
	})
}
