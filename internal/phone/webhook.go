package phone

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"

	"github.com/useopendesk/opendesk/internal/jobs"
	"github.com/useopendesk/opendesk/internal/phone/provider"
)

const (
	maxWebhookBody = 1 << 20
	// webhookPerMinute bounds what one connection's URL takes in: a call is a handful of
	// events, so this is far above a busy line and keeps a flood from filling the queue.
	webhookPerMinute = 600
)

// webhook receives a provider's events. The connection is found by its opaque id and
// the event is verified with the webhook's own secret before anything is read from it;
// nothing in the payload decides which workspace or ticket it reaches. A verified event
// is stored (normalized) and queued in one transaction, then the provider gets its 200
// at once: Aircall waits five seconds and disables a webhook that keeps failing.
func (s *Service) webhook(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if !s.d.Limiter.Allow("phone-hook:"+id.String(), webhookPerMinute, time.Minute) {
		w.WriteHeader(http.StatusTooManyRequests)
		return
	}
	c, err := scanConnection(s.d.DB.Pool.QueryRow(r.Context(), connectionSelect+` WHERE id = $1`, id))
	if err != nil || c.Provider != chi.URLParam(r, "provider") || len(c.webhookSecretEnc) == 0 {
		http.NotFound(w, r)
		return
	}
	a, ok := s.adapters[c.Provider]
	if !ok {
		http.NotFound(w, r)
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxWebhookBody))
	if err != nil {
		w.WriteHeader(http.StatusRequestEntityTooLarge)
		return
	}
	secret, err := s.d.Box.Open(c.webhookSecretEnc)
	if err != nil {
		s.d.Log.Error("phone webhook secret", "connection_id", c.ID, "err", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if a.Verify(body, string(secret)) != nil {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	ev, err := a.Parse(body)
	if errors.Is(err, provider.ErrIgnored) {
		w.WriteHeader(http.StatusOK)
		return
	}
	if err != nil {
		// Verified but not understood: acknowledged, so the provider keeps the webhook on.
		s.d.Log.Warn("phone event not understood", "provider", c.Provider, "connection_id", c.ID, "err", err)
		w.WriteHeader(http.StatusOK)
		return
	}
	if err := s.persist(r, c, ev); err != nil {
		s.d.Log.Error("phone event not stored", "connection_id", c.ID, "event", ev.Type, "err", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (s *Service) persist(r *http.Request, c PhoneConnection, ev provider.Event) error {
	ctx := r.Context()
	raw, err := json.Marshal(ev)
	if err != nil {
		return err
	}
	return s.d.DB.Tx(ctx, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `INSERT INTO phone_inbound_events (connection_id, external_event_id, workspace_id, payload)
			VALUES ($1, $2, $3, $4) ON CONFLICT DO NOTHING`, c.ID, ev.ID, c.WorkspaceID, raw)
		if err != nil || tag.RowsAffected() == 0 {
			return err // a retry of an event already stored
		}
		if _, err := tx.Exec(ctx, `UPDATE phone_connections SET last_event_at = now() WHERE id = $1`, c.ID); err != nil {
			return err
		}
		_, err = s.d.Jobs.InsertTx(ctx, tx, jobs.PhoneInboundArgs{WorkspaceID: c.WorkspaceID, ConnectionID: c.ID, ExternalEventID: ev.ID},
			&river.InsertOpts{MaxAttempts: 10})
		return err
	})
}
