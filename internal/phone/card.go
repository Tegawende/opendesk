package phone

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/riverqueue/river"

	"github.com/useopendesk/opendesk/internal/db"
	"github.com/useopendesk/opendesk/internal/jobs"
	"github.com/useopendesk/opendesk/internal/phone/provider"
)

// cardTooLate: a card is for a ringing call; one that could not go out in time is dropped.
const cardTooLate = 3 * time.Minute

type cardWorker struct {
	river.WorkerDefaults[jobs.PhoneInsightCardArgs]
	s *Service
}

func (w *cardWorker) Work(ctx context.Context, job *river.Job[jobs.PhoneInsightCardArgs]) error {
	err := w.s.PushCard(ctx, job.Args)
	var limited *provider.RateLimited
	if errors.As(err, &limited) {
		return river.JobSnooze(limited.RetryAfter)
	}
	return err
}

// cardTickets is the predicate for the tickets aliased t a card may name. The card is read
// by anyone in the provider's app, so like anything sent without a reader in mind it leaves
// out tickets of private teams, as well as deleted, merged and spam ones.
const cardTickets = `t.deleted_at IS NULL AND t.merged_into_id IS NULL AND NOT t.spam
	AND NOT EXISTS (SELECT 1 FROM teams pt WHERE pt.workspace_id = t.workspace_id AND pt.id = t.team_id AND pt.private)`

// Card builds what the agent sees about a ringing call: who calls, the ticket the call
// is on and the caller's three latest other tickets, each linked to opendesk, in the
// workspace's language (callTexts).
func (s *Service) Card(ctx context.Context, q db.Querier, wid, callID uuid.UUID) (provider.Card, *PhoneConnection, string, error) {
	var ticketID, contactID, connID *uuid.UUID
	var external, number string
	var created time.Time
	err := q.QueryRow(ctx, `SELECT ticket_id, contact_id, connection_id, external_call_id, number, created_at FROM phone_calls
		WHERE workspace_id = $1 AND id = $2`, wid, callID).Scan(&ticketID, &contactID, &connID, &external, &number, &created)
	if err != nil {
		return provider.Card{}, nil, "", err
	}
	if connID == nil || time.Since(created) > cardTooLate {
		return provider.Card{}, nil, "", nil
	}
	c, err := scanConnection(q.QueryRow(ctx, connectionSelect+` WHERE workspace_id = $1 AND id = $2`, wid, *connID))
	if db.IsNotFound(err) || err == nil && !c.InsightCards {
		return provider.Card{}, nil, "", nil
	}
	if err != nil {
		return provider.Card{}, nil, "", err
	}
	lang, err := workspaceCallLanguage(ctx, q, wid)
	if err != nil {
		return provider.Card{}, nil, "", err
	}
	app := strings.TrimRight(s.d.Cfg.AppURL, "/")
	card := provider.Card{Title: number}
	if number == "" {
		card.Title = callText(lang, "number")
	}
	if contactID != nil {
		var name string
		if err := q.QueryRow(ctx, `SELECT name FROM contacts WHERE workspace_id = $1 AND id = $2`, wid, *contactID).Scan(&name); err == nil && name != "" {
			card.Title = name
		}
		card.TitleLink = app + "/contacts/" + contactID.String()
	}
	if ticketID != nil {
		var n int64
		var subject, status string
		err := q.QueryRow(ctx, `SELECT t.number, t.subject, t.status FROM tickets t WHERE t.workspace_id = $1 AND t.id = $2 AND `+cardTickets, wid, *ticketID).
			Scan(&n, &subject, &status)
		if err == nil {
			card.Lines = append(card.Lines, provider.CardLine{Label: callText(lang, "ticket"), Text: fmt.Sprintf("#%d %s", n, subject), Link: app + "/inbox/all/" + ticketID.String()})
		} else if !db.IsNotFound(err) {
			return provider.Card{}, nil, "", err
		}
	}
	if contactID != nil {
		rows, err := q.Query(ctx, `SELECT t.id, t.number, t.subject, t.status FROM tickets t WHERE t.workspace_id = $1 AND t.contact_id = $2
			AND t.id IS DISTINCT FROM $3 AND `+cardTickets+` ORDER BY t.updated_at DESC LIMIT 3`, wid, *contactID, ticketID)
		if err != nil {
			return provider.Card{}, nil, "", err
		}
		defer rows.Close()
		for rows.Next() {
			var id uuid.UUID
			var n int64
			var subject, status string
			if err := rows.Scan(&id, &n, &subject, &status); err != nil {
				return provider.Card{}, nil, "", err
			}
			card.Lines = append(card.Lines, provider.CardLine{Label: callText(lang, "earlier"), Text: fmt.Sprintf("#%d %s · %s", n, subject, callText(lang, "status."+status)),
				Link: app + "/inbox/all/" + id.String()})
		}
		if err := rows.Err(); err != nil {
			return provider.Card{}, nil, "", err
		}
	}
	return card, &c, external, nil
}

// PushCard sends a ringing call's card to the provider.
func (s *Service) PushCard(ctx context.Context, a jobs.PhoneInsightCardArgs) error {
	card, c, external, err := s.Card(ctx, s.d.DB.Pool, a.WorkspaceID, a.CallID)
	if db.IsNotFound(err) || err == nil && c == nil {
		return nil
	}
	if err != nil {
		return err
	}
	ad, ok := s.adapters[c.Provider]
	if !ok {
		return nil
	}
	creds, err := s.credentials(*c)
	if err != nil {
		return err
	}
	return ad.PushCard(ctx, s.http, creds, external, card)
}

type pruneWorker struct {
	river.WorkerDefaults[jobs.PhonePruneArgs]
	s *Service
}

// Work removes provider events applied more than eventRetention ago (a week), and any
// older than 30 days. The webhook drops events older than eventRetention, so a retry is
// always recognized while it is accepted.
func (w *pruneWorker) Work(ctx context.Context, _ *river.Job[jobs.PhonePruneArgs]) error {
	_, err := w.s.d.DB.Pool.Exec(ctx, `DELETE FROM phone_inbound_events
		WHERE (processed_at IS NOT NULL AND processed_at < now() - make_interval(secs => $1)) OR received_at < now() - interval '30 days'`,
		eventRetention.Seconds())
	return err
}
