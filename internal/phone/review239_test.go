package phone_test

import (
	"context"
	"encoding/json"
	"github.com/jackc/pgx/v5"
	"github.com/useopendesk/opendesk/internal/helpdesk/hd"
	"github.com/useopendesk/opendesk/internal/helpdesk/hdtest"
	"strings"
	"testing"
	"time"

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

func TestReview239CallPlacementDuringMerge(t *testing.T) {
	h := newHarness(t)
	var src, dst domain.Ticket
	for _, out := range []*domain.Ticket{&src, &dst} {
		h.e.Do("POST", "/v1/tickets", h.ws.Admin.As(h.ws), map[string]any{"subject": "Merge placement", "channel": "api", "kind": "note", "body_html": "<p>Existing</p>"}).Want(t, 201).JSON(t, out)
	}
	if src.ID.String() > dst.ID.String() {
		t.Fatal("source must sort before destination")
	}
	contact := uuid.New()
	h.e.Exec(`INSERT INTO contacts (id,workspace_id,name,phone) VALUES ($1,$2,'Caller','+32470123456')`, contact, h.ws.ID)
	h.e.Exec(`UPDATE tickets SET contact_id=$2 WHERE id=$1`, src.ID, contact)
	ctx, cancel := context.WithTimeout(h.e.Ctx, 15*time.Second)
	defer cancel()
	hold, err := h.e.D.DB.Pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer hold.Rollback(context.Background())
	var pid int32
	if err := hold.QueryRow(ctx, `SELECT pg_backend_pid() FROM tickets WHERE id=$1 FOR UPDATE`, dst.ID).Scan(&pid); err != nil {
		t.Fatal(err)
	}
	merged := make(chan *hdtest.Resp, 1)
	go func() {
		merged <- h.e.Do("POST", "/v1/tickets/"+src.ID.String()+"/merge", h.ws.Admin.As(h.ws), map[string]any{"into_id": dst.ID})
	}()
	wait := func(want int) {
		t.Helper()
		for {
			var n int
			h.e.Scalar(`WITH RECURSIVE behind(pid) AS (SELECT $1::int UNION SELECT a.pid FROM pg_stat_activity a, behind b WHERE b.pid=ANY(pg_blocking_pids(a.pid))) SELECT count(*)-1 FROM behind`, &n, pid)
			if n >= want {
				return
			}
			if ctx.Err() != nil {
				t.Fatalf("waiters %d want %d", n, want)
			}
			time.Sleep(10 * time.Millisecond)
		}
	}
	wait(1)
	if code := h.send(t, h.recorded(t, "answered")[0]); code != 200 {
		t.Fatal(code)
	}
	raw := h.e.Jobs(jobs.PhoneInboundArgs{}.Kind(), h.ws.ID)
	if len(raw) != 1 {
		t.Fatal(len(raw))
	}
	var a jobs.PhoneInboundArgs
	if err := json.Unmarshal(raw[0], &a); err != nil {
		t.Fatal(err)
	}
	processed := make(chan error, 1)
	go func() { processed <- h.s.ProcessInbound(ctx, a) }()
	wait(2)
	if err := hold.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	(<-merged).Want(t, 200)
	if err := <-processed; err != nil {
		t.Fatal(err)
	}
	var owner uuid.UUID
	h.e.Scalar(`SELECT ticket_id FROM phone_calls WHERE workspace_id=$1`, &owner, h.ws.ID)
	var calls int
	h.e.Scalar(`SELECT count(*) FROM activities WHERE ticket_id=$1 AND type='call'`, &calls, src.ID)
	if owner == src.ID || calls != 0 {
		t.Fatalf("call attached to merged source: owner=%s source=%s source entries=%d", owner, src.ID, calls)
	}
}

func TestReview239MergeWhileCallDecisionWaits(t *testing.T) {
	h := newHarness(t)
	h.sendAll(t, h.recorded(t, "answered"))
	src := h.ticketOfCall(t, "1290001")
	var call uuid.UUID
	h.e.Scalar(`SELECT id FROM phone_calls WHERE workspace_id=$1`, &call, h.ws.ID)
	var dst domain.Ticket
	h.e.Do("POST", "/v1/tickets", h.ws.Admin.As(h.ws), map[string]any{"subject": "Destination", "channel": "api", "kind": "note", "body_html": "<p>Existing</p>"}).Want(t, 201).JSON(t, &dst)
	h.e.Do("POST", "/v1/automations", h.ws.Admin.As(h.ws), map[string]any{"name": "Decide on call", "trigger": "call_ended", "enabled": true, "conditions": []any{map[string]any{"field": "ai_decide", "op": "gt", "value": "0.5", "question": "Reopen?"}}, "actions": []any{map[string]any{"type": "set_status", "value": "open"}}}).Want(t, 201)
	entered, release := make(chan struct{}), make(chan struct{})
	prev := hd.GetAIHooks()
	defer hd.SetAIHooks(prev)
	hd.SetAIHooks(&hd.AIHooks{Decide: func(ctx context.Context, tx pgx.Tx, wid, tid uuid.UUID, q []string) (map[string]float64, error) {
		if tid == src.ID {
			close(entered)
			select {
			case <-release:
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
		return map[string]float64{"Reopen?": 1}, nil
	}})
	ctx, cancel := context.WithTimeout(h.e.Ctx, 15*time.Second)
	defer cancel()
	ended := make(chan error, 1)
	go func() {
		ended <- tickets.RunTrigger(ctx, h.e.D, jobs.TicketTriggerArgs{WorkspaceID: h.ws.ID, TicketID: src.ID, CallID: &call, Trigger: jobs.TriggerCallEnded})
	}()
	select {
	case <-entered:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	merged := make(chan *hdtest.Resp, 1)
	go func() {
		merged <- h.e.Do("POST", "/v1/tickets/"+src.ID.String()+"/merge", h.ws.Admin.As(h.ws), map[string]any{"into_id": dst.ID})
	}()
	select {
	case response := <-merged:
		response.Want(t, 200)
		h.e.Exec(`UPDATE tickets SET status='closed' WHERE id=$1`, dst.ID)
		close(release)
	case <-time.After(3 * time.Second):
		close(release)
		<-ended
		<-merged
		t.Fatal("merge blocked behind the model decision")
	}
	if err := <-ended; err != nil {
		t.Fatal(err)
	}
	var sourceStatus, destinationStatus string
	h.e.Scalar(`SELECT status FROM tickets WHERE id=$1`, &sourceStatus, src.ID)
	h.e.Scalar(`SELECT status FROM tickets WHERE id=$1`, &destinationStatus, dst.ID)
	if sourceStatus != "closed" || destinationStatus != "open" {
		t.Fatalf("source=%s destination=%s", sourceStatus, destinationStatus)
	}
}
