package collab

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/useopendesk/opendesk/internal/db"
	"github.com/useopendesk/opendesk/internal/events"
	"github.com/useopendesk/opendesk/internal/helpdesk/hd"
)

// Notification types (contracts/collab.md "Notifications"). They have no ticket; their data
// names the conversation, who acted ("by", named as "actor" by hd.Notify) and its subject.
const (
	// NotifyAssigned: {thread_id, account_id, by, subject}.
	NotifyAssigned = "collab_assigned"
	// NotifyMention: {thread_id, comment_id, by, subject, preview}.
	NotifyMention = "collab_mention"
	// NotifyNewMail: {thread_id, account_id, subject, from: {name, email}, preview, count}.
	NotifyNewMail = "collab_new_mail"
)

// newMailWindow: mail older than this when it is stored is not new to anyone.
const newMailWindow = 24 * time.Hour

// notify stores a Collab notification for a member, in tx: in the app, pushed and emailed
// by their preferences, like every notification.
func (s *service) notify(ctx context.Context, tx pgx.Tx, wid, userID uuid.UUID, typ string, data map[string]any) error {
	return hd.Notify(ctx, tx, wid, userID, typ, nil, data)
}

// newMail is a message the sync just stored that may be new mail to tell people about.
type newMail struct {
	threadID uuid.UUID
	sentAt   time.Time
	from     Address
	subject  string
	preview  string
}

// noteNewMail keeps a message the sync stored for collab_new_mail when it is new mail: a
// pass that announces (never a first import or an import of old mail), unread, in the
// inbox and not in spam or trash, not a draft, not sent from opendesk, not from the
// account's own address, and received in the last day.
func (p *pass) noteNewMail(in *newMessage, threadID uuid.UUID, from Address, subject, preview string, sentAt time.Time) {
	if !p.announce || in.SentBy != nil || in.State.Draft || !in.State.Unread {
		return
	}
	addr := strings.TrimSpace(from.Email)
	if addr == "" || strings.EqualFold(addr, p.a.Email) || time.Since(sentAt) > newMailWindow {
		return
	}
	inInbox := false
	for _, id := range in.State.FolderIDs {
		f := p.folderByID(id)
		if f == nil || f.Role == nil {
			continue
		}
		switch *f.Role {
		case RoleInbox:
			inInbox = true
		case RoleSpam, RoleTrash, RoleDrafts, RoleSent:
			return
		}
	}
	if inInbox {
		p.newMail = append(p.newMail, newMail{threadID: threadID, sentAt: sentAt, from: from, subject: subject, preview: preview})
	}
}

// folderByID is one of the pass's folders by its stored id, or nil.
func (p *pass) folderByID(id uuid.UUID) *folderRow {
	for _, f := range p.folders {
		if f.ID == id {
			return f
		}
	}
	return nil
}

// notifyNewMail tells who sees the account about new mail, in tx: one collab_new_mail per
// conversation and person. An unread one for the same conversation is brought up to date
// (sender, preview, count) and to the top instead of adding another; it is not pushed or
// emailed again. A private account tells its owner alone; a shared one its audience. A
// member who muted the account, or who sent the mail, is not told.
func (s *service) notifyNewMail(ctx context.Context, tx pgx.Tx, a *accountRow, list []newMail) error {
	if len(list) == 0 {
		return nil
	}
	type thread struct {
		last  newMail
		count int
	}
	byThread := map[uuid.UUID]*thread{}
	order := []uuid.UUID{}
	for _, m := range list {
		t := byThread[m.threadID]
		if t == nil {
			t = &thread{}
			byThread[m.threadID] = t
			order = append(order, m.threadID)
		}
		if t.count == 0 || !m.sentAt.Before(t.last.sentAt) {
			t.last = m
		}
		t.count++
	}
	recipients, err := newMailRecipients(ctx, tx, a)
	if err != nil || len(recipients) == 0 {
		return err
	}
	// The conversation's subject as stored (the first message's), not a reply's "Re:".
	rows, err := tx.Query(ctx, `SELECT id, subject FROM collab_threads WHERE account_id = $1 AND id = ANY($2)`, a.ID, order)
	if err != nil {
		return err
	}
	subjects := map[uuid.UUID]string{}
	for rows.Next() {
		var id uuid.UUID
		var subject string
		if err := rows.Scan(&id, &subject); err != nil {
			rows.Close()
			return err
		}
		subjects[id] = subject
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	for _, tid := range order {
		t := byThread[tid]
		subject := subjects[tid]
		if strings.TrimSpace(subject) == "" {
			subject = t.last.subject
		}
		data := map[string]any{"thread_id": tid.String(), "account_id": a.ID.String(), "subject": truncateRunes(subject, 200),
			"from": map[string]any{"name": t.last.from.Name, "email": t.last.from.Email}, "preview": truncateRunes(t.last.preview, 200)}
		for _, r := range recipients {
			if strings.EqualFold(r.email, t.last.from.Email) {
				continue // their own mail to the account
			}
			found, err := refreshNewMail(ctx, tx, a.WorkspaceID, r.id, tid, data, t.count)
			if err != nil {
				return err
			}
			if found {
				continue
			}
			fresh := map[string]any{"count": t.count}
			for k, v := range data {
				fresh[k] = v
			}
			if err := s.notify(ctx, tx, a.WorkspaceID, r.id, NotifyNewMail, fresh); err != nil {
				return err
			}
		}
	}
	return nil
}

type recipient struct {
	id    uuid.UUID
	email string
}

// newMailRecipients is who is told about the account's new mail: who sees it (its owner
// alone while it is private) and is still a member, less who muted it.
func newMailRecipients(ctx context.Context, q db.Querier, a *accountRow) ([]recipient, error) {
	audience, err := accountAudience(ctx, q, a.WorkspaceID, a.ID)
	if err != nil || len(audience) == 0 {
		return nil, err
	}
	rows, err := q.Query(ctx, `SELECT u.id, u.email FROM users u
		JOIN members m ON m.user_id = u.id AND m.workspace_id = $1
		WHERE u.id = ANY($2) AND NOT EXISTS (SELECT 1 FROM collab_account_prefs cp
			WHERE cp.account_id = $3 AND cp.user_id = u.id AND NOT cp.new_mail)
		ORDER BY u.id`, a.WorkspaceID, audience, a.ID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (recipient, error) {
		var out recipient
		return out, r.Scan(&out.id, &out.email)
	})
}

// refreshNewMail brings the member's unread collab_new_mail for the conversation up to
// date with count more messages and publishes it again, in tx. It reports whether there
// was one.
func refreshNewMail(ctx context.Context, tx pgx.Tx, wid, userID, threadID uuid.UUID, data map[string]any, count int) (bool, error) {
	var n hd.Notification
	err := tx.QueryRow(ctx, `UPDATE notifications SET data = $5::jsonb || jsonb_build_object('count', COALESCE((data->>'count')::int, 1) + $6),
			created_at = now()
		WHERE id = (SELECT id FROM notifications WHERE workspace_id = $1 AND user_id = $2 AND type = $3 AND read_at IS NULL
			AND data->>'thread_id' = $4 ORDER BY created_at DESC LIMIT 1)
		RETURNING id, type, data, created_at`, wid, userID, NotifyNewMail, threadID.String(), data, count).
		Scan(&n.ID, &n.Type, &n.Data, &n.CreatedAt)
	if db.IsNotFound(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	// The app replaces the notification it has with this id and moves it to the top.
	_, err = events.Publish(ctx, tx, events.Event{WorkspaceID: wid, Type: "notification.created",
		Users: []uuid.UUID{userID}, Data: map[string]any{"notification": n}})
	return true, err
}

// setNewMailPref stores whether the member is told about the account's new mail.
func setNewMailPref(ctx context.Context, q db.Querier, wid, accountID, userID uuid.UUID, on bool) error {
	_, err := q.Exec(ctx, `INSERT INTO collab_account_prefs (workspace_id, account_id, user_id, new_mail) VALUES ($1, $2, $3, $4)
		ON CONFLICT (account_id, user_id) DO UPDATE SET new_mail = EXCLUDED.new_mail, updated_at = now()`, wid, accountID, userID, on)
	return err
}

// mutedAccounts is which of the accounts the member muted new mail for.
func mutedAccounts(ctx context.Context, q db.Querier, userID uuid.UUID, accountIDs []uuid.UUID) ([]uuid.UUID, error) {
	rows, err := q.Query(ctx, `SELECT account_id FROM collab_account_prefs WHERE user_id = $1 AND account_id = ANY($2) AND NOT new_mail`,
		userID, accountIDs)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
}
