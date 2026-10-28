package phone_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/useopendesk/opendesk/internal/domain"
	"github.com/useopendesk/opendesk/internal/helpdesk/tickets"
	"github.com/useopendesk/opendesk/internal/jobs"
)

func TestReview239TranscriptionAvailability(t *testing.T) {
	h := newHarness(t)
	h.sendAll(t, h.recorded(t, "answered"))
	body := h.recorded(t, "intelligence")[1]
	if code := h.send(t, body); code != 200 {
		t.Fatalf("webhook status %d", code)
	}
	queued := h.e.Jobs(jobs.PhoneInboundArgs{}.Kind(), h.ws.ID)
	if len(queued) != 1 {
		t.Fatalf("availability queued %d jobs", len(queued))
	}
	var a jobs.PhoneInboundArgs
	_ = json.Unmarshal(queued[0], &a)
	h.f.TranscriptionStatus = 404
	if err := h.s.ProcessInbound(h.e.Ctx, a); err == nil {
		t.Fatal("unavailable transcript must retry")
	}
	h.f.TranscriptionStatus = 429
	if err := h.s.ProcessInbound(h.e.Ctx, a); err == nil {
		t.Fatal("rate limit must retry")
	}
	h.f.TranscriptionStatus = 0
	if err := h.s.ProcessInbound(h.e.Ctx, a); err != nil {
		t.Fatal(err)
	}
	if err := h.s.ProcessInbound(h.e.Ctx, a); err != nil {
		t.Fatal(err)
	}
	call := h.ticketOfCall(t, "1290001")
	entries := h.callEntries(t, call.ID)
	if len(entries) != 1 || entries[0].Data["transcript"] != "Agent: Bonjour, LampTwist support.\nCustomer: Hello, my bulb arrived broken." {
		t.Fatalf("entries: %+v", entries)
	}
	gets := 0
	for _, r := range h.f.Requests() {
		if r == "GET /v1/calls/1290001/transcription" {
			gets++
		}
	}
	if gets != 3 {
		t.Fatalf("transcript GETs %d, want 404 + 429 + success only", gets)
	}
}

func TestReview239MergeMovesCall(t *testing.T) {
	for _, queuedBefore := range []bool{false, true} {
		for _, private := range []bool{false, true} {
			name := "before_end"
			if queuedBefore {
				name = "after_enqueue"
			}
			if private {
				name += "_private"
			}
			t.Run(name, func(t *testing.T) {
				h := newHarness(t)
				bodies := h.recorded(t, "answered")
				h.sendAll(t, bodies[:1])
				src := h.ticketOfCall(t, "1290001")
				var dst domain.Ticket
				h.e.Do("POST", "/v1/tickets", h.ws.Admin.As(h.ws), map[string]any{"subject": "Destination", "channel": "api", "kind": "note", "body_html": "<p>Existing</p>"}).Want(t, 201).JSON(t, &dst)
				if private {
					team := uuid.New()
					h.e.Exec(`INSERT INTO teams (id,workspace_id,name,private) VALUES ($1,$2,'Legal',true)`, team, h.ws.ID)
					h.e.Exec(`UPDATE tickets SET team_id=$2 WHERE id=$1`, dst.ID, team)
					h.e.Exec(`INSERT INTO team_members (team_id,user_id) VALUES ($1,$2)`, team, h.ws.Admin.ID)
				}
				h.e.Do("POST", "/v1/automations", h.ws.Admin.As(h.ws), map[string]any{"name": "Reopen ended calls", "trigger": "call_ended", "enabled": true, "conditions": []any{}, "actions": []any{map[string]any{"type": "set_status", "value": "open"}}}).Want(t, 201)
				var pending []jobs.TicketTriggerArgs
				if queuedBefore {
					for _, b := range bodies[1:] {
						if code := h.send(t, b); code != 200 {
							t.Fatal(code)
						}
					}
					for _, raw := range h.e.Jobs(jobs.PhoneInboundArgs{}.Kind(), h.ws.ID) {
						var a jobs.PhoneInboundArgs
						_ = json.Unmarshal(raw, &a)
						if err := h.s.ProcessInbound(h.e.Ctx, a); err != nil {
							t.Fatal(err)
						}
					}
					for _, raw := range h.e.Jobs(jobs.TicketTriggerArgs{}.Kind(), h.ws.ID) {
						var a jobs.TicketTriggerArgs
						_ = json.Unmarshal(raw, &a)
						if a.Trigger == jobs.TriggerCallEnded {
							pending = append(pending, a)
						}
					}
					if len(pending) != 1 {
						t.Fatalf("pending ended jobs %d", len(pending))
					}
				}
				h.e.Do("POST", "/v1/tickets/"+src.ID.String()+"/merge", h.ws.Admin.As(h.ws), map[string]any{"into_id": dst.ID}).Want(t, 200)
				h.e.Exec(`UPDATE tickets SET status='closed' WHERE id=$1`, dst.ID)
				if queuedBefore {
					for _, a := range pending {
						if err := tickets.RunTrigger(h.e.Ctx, h.e.D, a); err != nil {
							t.Fatal(err)
						}
					}
				} else {
					h.sendAll(t, bodies[1:])
				}
				var sourceStatus, destStatus, callTicket, activityTicket string
				h.e.Scalar(`SELECT status FROM tickets WHERE id=$1`, &sourceStatus, src.ID)
				h.e.Scalar(`SELECT status FROM tickets WHERE id=$1`, &destStatus, dst.ID)
				h.e.Scalar(`SELECT ticket_id::text FROM phone_calls WHERE workspace_id=$1`, &callTicket, h.ws.ID)
				h.e.Scalar(`SELECT ticket_id::text FROM activities WHERE workspace_id=$1 AND type='call'`, &activityTicket, h.ws.ID)
				if sourceStatus != "closed" || destStatus != "open" || callTicket != dst.ID.String() || activityTicket != callTicket {
					t.Fatalf("source=%s dest=%s call=%s activity=%s", sourceStatus, destStatus, callTicket, activityTicket)
				}
				var out struct {
					Data []domain.Activity `json:"data"`
				}
				h.e.Do("GET", "/v1/tickets/"+dst.ID.String()+"/activities", h.ws.Admin.As(h.ws), nil).Want(t, 200).JSON(t, &out)
				count := 0
				for _, entry := range out.Data {
					if entry.Type == "call" {
						count++
						if entry.Data["stage"] != "ended" {
							t.Fatal(entry.Data)
						}
					}
				}
				if count != 1 {
					t.Fatalf("call entries %d", count)
				}
				if private {
					h.e.Do("GET", "/v1/tickets/"+dst.ID.String()+"/activities", h.ws.Agent.As(h.ws), nil).Want(t, 404)
					rows, err := h.e.D.DB.Pool.Query(h.e.Ctx, `SELECT team_id::text FROM events WHERE workspace_id=$1 AND data->'activity'->>'ticket_id'=$2 AND type='activity.updated'`, h.ws.ID, dst.ID.String())
					if err != nil {
						t.Fatal(err)
					}
					defer rows.Close()
					eventCount := 0
					for rows.Next() {
						eventCount++
						var team *string
						if err := rows.Scan(&team); err != nil {
							t.Fatal(err)
						}
						if team == nil {
							t.Fatal("destination event has no private team")
						}
					}
					if err := rows.Err(); err != nil {
						t.Fatal(err)
					}
					if eventCount == 0 {
						t.Fatal("no destination activity event")
					}
				}
			})
		}
	}
}

func TestReview239BlockedTranscriptNotFetched(t *testing.T) {
	h := newHarness(t)
	h.e.Exec(`INSERT INTO contacts (id,workspace_id,name,phone,blocked) VALUES ($1,$2,'Blocked','+32470123456',true)`, uuid.New(), h.ws.ID)
	h.sendAll(t, h.recorded(t, "answered"))
	h.sendAll(t, h.recorded(t, "intelligence"))
	for _, r := range h.f.Requests() {
		if strings.HasSuffix(r, "/transcription") {
			t.Fatal("blocked call fetched transcript")
		}
	}
}
