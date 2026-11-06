package domain

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/useopendesk/opendesk/internal/db"
	"github.com/useopendesk/opendesk/internal/events"
)

// RedactReplay hides, in replayed events (GET /v1/events), what messages deleted since
// said: an event stored while it still read in full is replayed with the message as it is
// now, its marker (Tombstone); a notification about it loses its preview; and a ticket
// event about a ticket with a deleted reply carries the ticket's preview as it is now, and
// one whose last_note was deleted since (or sent as a reply) the ticket's last_note as it
// is now.
func RedactReplay(ctx context.Context, q db.Querier, wid uuid.UUID, evs []events.Envelope) error {
	var refs []ref
	var tickets []uuid.UUID
	var notes []ref
	for i, e := range evs {
		switch e.Type {
		case "message.created", "message.updated":
			var d struct {
				Message struct {
					ID uuid.UUID `json:"id"`
				} `json:"message"`
			}
			if json.Unmarshal(e.Data, &d) == nil && d.Message.ID != uuid.Nil {
				refs = append(refs, ref{i, d.Message.ID})
			}
		case "notification.created":
			var d struct {
				Notification struct {
					Data struct {
						MessageID string `json:"message_id"`
					} `json:"data"`
				} `json:"notification"`
			}
			if json.Unmarshal(e.Data, &d) == nil {
				if id, err := uuid.Parse(d.Notification.Data.MessageID); err == nil {
					refs = append(refs, ref{i, id})
				}
			}
		case "ticket.created", "ticket.updated":
			var d struct {
				Ticket struct {
					ID       uuid.UUID `json:"id"`
					LastNote *struct {
						ID uuid.UUID `json:"id"`
					} `json:"last_note"`
				} `json:"ticket"`
			}
			if json.Unmarshal(e.Data, &d) == nil && d.Ticket.ID != uuid.Nil {
				tickets = append(tickets, d.Ticket.ID)
				if d.Ticket.LastNote != nil && d.Ticket.LastNote.ID != uuid.Nil {
					notes = append(notes, ref{i, d.Ticket.LastNote.ID})
				}
			}
		}
	}
	if len(refs) > 0 {
		ids := make([]uuid.UUID, len(refs))
		for i, r := range refs {
			ids[i] = r.msg
		}
		rows, err := q.Query(ctx, MessageSelect+` WHERE m.workspace_id = $1 AND m.id = ANY($2) AND `+HiddenMessageSQL, wid, ids)
		if err != nil {
			return err
		}
		list, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (Message, error) { return ScanMessage(r) })
		if err != nil {
			return err
		}
		hidden := map[uuid.UUID]Message{}
		for _, m := range list {
			hidden[m.ID] = Tombstone(m)
		}
		for _, r := range refs {
			m, ok := hidden[r.msg]
			if !ok {
				continue
			}
			var data map[string]any
			if err := json.Unmarshal(evs[r.i].Data, &data); err != nil {
				continue
			}
			if evs[r.i].Type == "notification.created" {
				if n, ok := data["notification"].(map[string]any); ok {
					if nd, ok := n["data"].(map[string]any); ok {
						delete(nd, "preview")
					}
				}
			} else {
				data["message"] = m
			}
			raw, err := json.Marshal(data)
			if err != nil {
				return err
			}
			evs[r.i].Data = raw
		}
	}
	if err := redactLastNotes(ctx, q, wid, evs, notes); err != nil {
		return err
	}
	if len(tickets) > 0 {
		rows, err := q.Query(ctx, `SELECT t.id, t.last_message_preview FROM tickets t WHERE t.workspace_id = $1 AND t.id = ANY($2)
			AND EXISTS (SELECT 1 FROM messages m WHERE m.workspace_id = t.workspace_id AND m.ticket_id = t.id AND m.kind = 'reply' AND m.deleted_at IS NOT NULL)`,
			wid, tickets)
		if err != nil {
			return err
		}
		previews := map[uuid.UUID]string{}
		for rows.Next() {
			var id uuid.UUID
			var p string
			if err := rows.Scan(&id, &p); err != nil {
				rows.Close()
				return err
			}
			previews[id] = p
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
		if len(previews) == 0 {
			return nil
		}
		for i, e := range evs {
			if e.Type != "ticket.created" && e.Type != "ticket.updated" {
				continue
			}
			var data map[string]any
			if err := json.Unmarshal(e.Data, &data); err != nil {
				continue
			}
			t, ok := data["ticket"].(map[string]any)
			if !ok {
				continue
			}
			id, err := uuid.Parse(asString(t["id"]))
			if err != nil {
				continue
			}
			p, ok := previews[id]
			if !ok {
				continue
			}
			t["last_message_preview"] = p
			raw, err := json.Marshal(data)
			if err != nil {
				return err
			}
			evs[i].Data = raw
		}
	}
	return nil
}

// redactLastNotes gives the ticket events in notes whose last_note is hidden now (deleted,
// or sent as a reply) the ticket's last_note as it is now.
func redactLastNotes(ctx context.Context, q db.Querier, wid uuid.UUID, evs []events.Envelope, notes []ref) error {
	if len(notes) == 0 {
		return nil
	}
	ids := make([]uuid.UUID, len(notes))
	for i, n := range notes {
		ids[i] = n.msg
	}
	// For each hidden note, its ticket's last_note now.
	rows, err := q.Query(ctx, `SELECT m.id, (SELECT `+LastNoteSQL+` FROM tickets t WHERE t.workspace_id = m.workspace_id AND t.id = m.ticket_id)
		FROM messages m WHERE m.workspace_id = $1 AND m.id = ANY($2) AND `+HiddenMessageSQL, wid, ids)
	if err != nil {
		return err
	}
	now := map[uuid.UUID]*LastNote{}
	for rows.Next() {
		var id uuid.UUID
		var raw []byte
		if err := rows.Scan(&id, &raw); err != nil {
			rows.Close()
			return err
		}
		n, err := scanLastNote(raw)
		if err != nil {
			rows.Close()
			return err
		}
		now[id] = n
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	for _, n := range notes {
		current, ok := now[n.msg]
		if !ok {
			continue
		}
		var data map[string]any
		if err := json.Unmarshal(evs[n.i].Data, &data); err != nil {
			continue
		}
		t, ok := data["ticket"].(map[string]any)
		if !ok {
			continue
		}
		t["last_note"] = current
		raw, err := json.Marshal(data)
		if err != nil {
			return err
		}
		evs[n.i].Data = raw
	}
	return nil
}

// ref is the message an event at index i of a replay names.
type ref struct {
	i   int
	msg uuid.UUID
}

func asString(v any) string {
	s, _ := v.(string)
	return s
}
