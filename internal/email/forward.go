package email

import (
	"context"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	xhtml "golang.org/x/net/html"
	"golang.org/x/net/html/atom"

	"github.com/useopendesk/opendesk/internal/db"
	"github.com/useopendesk/opendesk/internal/displayname"
	"github.com/useopendesk/opendesk/internal/domain"
	"github.com/useopendesk/opendesk/internal/helpdesk/hd"
	"github.com/useopendesk/opendesk/internal/templates"
)

// loadForwarded is the email a side conversation's first message forwards (forward_message_id,
// contracts/api.md "Threads"), checked again as it goes: still a public reply of the ticket's
// conversation with the customer. It forwards the email in full: the original as it came in or
// went out (full_html, with what it quoted, and for our replies the layout and signature the
// customer got) when one is kept, else the stored body. It is sanitized again with the email
// policy, without its style sheets, and its inline images point at their copies on the message
// mid, which outboundAttachments sends as inline parts.
func loadForwarded(ctx context.Context, q db.Querier, wid, ticketID, id, mid uuid.UUID) (*templates.Forwarded, error) {
	var f templates.Forwarded
	var at time.Time
	var raw string
	err := q.QueryRow(ctx, `SELECT `+senderSQL+`, m.created_at, COALESCE(NULLIF(m.subject, ''), t.subject), m.to_emails, m.cc_emails,
			COALESCE(NULLIF(m.full_html, ''), m.body_html)
		FROM messages m JOIN tickets t ON t.id = m.ticket_id AND t.workspace_id = m.workspace_id `+senderJoins+`
		WHERE m.workspace_id = $1 AND m.ticket_id = $2 AND m.id = $3 AND `+domain.CustomerConversationSQL, wid, ticketID, id).
		Scan(&f.FromName, &f.FromEmail, &at, &f.Subject, &f.To, &f.Cc, &raw)
	if db.IsNotFound(err) {
		// Deleted, or otherwise out of the conversation, since the thread started: its images
		// are already on the message, so nothing goes rather than the files alone.
		return nil, permanent("The forwarded email is no longer in the ticket's conversation.")
	}
	if err != nil {
		return nil, err
	}
	f.FromName, f.At = displayname.Displayable(f.FromName), at
	rows, err := q.Query(ctx, `SELECT o.id, c.id FROM attachments o
		JOIN attachments c ON c.workspace_id = o.workspace_id AND c.storage_key = o.storage_key AND c.message_id = $3 AND c.inline
		WHERE o.workspace_id = $1 AND o.message_id = $2 AND o.inline`, wid, id, mid)
	if err != nil {
		return nil, err
	}
	copies := map[uuid.UUID]uuid.UUID{}
	var orig, cp uuid.UUID
	if _, err := pgx.ForEachRow(rows, []any{&orig, &cp}, func() error {
		copies[orig] = cp
		return nil
	}); err != nil {
		return nil, err
	}
	f.HTML = forwardedImages(hd.SanitizeForwardedEmail(raw), copies)
	f.Text = htmlToText(f.HTML)
	return &f, nil
}

var attachmentRef = regexp.MustCompile(`(?i)/v1/attachments/([0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12})`)

// forwardedImages points a forwarded email's inline images at their copies (copies maps an
// attachment of the email to its copy on the forwarding message). Any other image of ours is
// left out: its link would not open for the recipient, and must not be handed out.
func forwardedImages(body string, copies map[uuid.UUID]uuid.UUID) string {
	doc := parseFragment(body)
	if doc == nil {
		return body
	}
	var drop []*xhtml.Node
	var walk func(*xhtml.Node)
	walk = func(n *xhtml.Node) {
		if n.Type == xhtml.ElementNode && n.DataAtom == atom.Img {
			for i, a := range n.Attr {
				if !strings.EqualFold(a.Key, "src") {
					continue
				}
				m := attachmentRef.FindStringSubmatch(a.Val)
				if m == nil {
					continue
				}
				orig, err := uuid.Parse(m[1])
				if c, ok := copies[orig]; err == nil && ok {
					n.Attr[i].Val = "/v1/attachments/" + c.String()
				} else {
					drop = append(drop, n)
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	for _, n := range drop {
		if n.Parent != nil {
			n.Parent.RemoveChild(n)
		}
	}
	return render(doc)
}
