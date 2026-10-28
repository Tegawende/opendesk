package tickets_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/useopendesk/opendesk/internal/helpdesk/hd"
	"github.com/useopendesk/opendesk/internal/helpdesk/tickets"
	"github.com/useopendesk/opendesk/internal/jobs"
)

func TestCallEndedConcurrentCreatedMerge(t *testing.T) {
	e, ws := newEnv(t)
	mb := addMailbox(t, e, ws)
	dst := customerMail(t, e, ws, mb, "caller@example.test", "Call", "earlier")
	src := customerMail(t, e, ws, mb, "caller@example.test", "Call", "later")
	call := uuid.New()
	e.Exec(`INSERT INTO phone_calls (id,workspace_id,provider,external_call_id,ticket_id,direction,stage) VALUES ($1,$2,'aircall',$1::uuid::text,$3,'inbound','ended')`, call, ws.ID, src)
	addAutomation(t, e, ws, jobs.TriggerTicketCreated, 0, []map[string]string{{"field": "ai_decide", "op": "gt", "value": "0.5", "question": "Merge?"}}, []map[string]any{mergeAction("7", false)})
	addAutomation(t, e, ws, jobs.TriggerCallEnded, 0, []map[string]string{}, []map[string]string{{"type": "set_status", "value": "open"}})
	entered, release := make(chan struct{}), make(chan struct{})
	prev := hd.GetAIHooks()
	defer hd.SetAIHooks(prev)
	hd.SetAIHooks(&hd.AIHooks{Decide: func(ctx context.Context, tx pgx.Tx, wid, tid uuid.UUID, q []string) (map[string]float64, error) {
		close(entered)
		select {
		case <-release:
			return map[string]float64{"Merge?": 1}, nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}})
	ctx, cancel := context.WithTimeout(e.Ctx, 15*time.Second)
	defer cancel()
	created, ended := make(chan error, 1), make(chan error, 1)
	go func() {
		created <- tickets.RunTrigger(ctx, e.D, jobs.TicketTriggerArgs{WorkspaceID: ws.ID, TicketID: src, Trigger: jobs.TriggerTicketCreated})
	}()
	select {
	case <-entered:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	go func() {
		ended <- tickets.RunTrigger(ctx, e.D, jobs.TicketTriggerArgs{WorkspaceID: ws.ID, TicketID: src, CallID: &call, Trigger: jobs.TriggerCallEnded})
	}()
	for {
		var waiting bool
		e.Scalar(`SELECT EXISTS (SELECT 1 FROM pg_locks WHERE locktype='advisory' AND NOT granted AND (classid::bigint << 32 | objid::bigint) = hashtextextended('ticket_trigger:' || $1::text, 0))`, &waiting, src)
		if waiting {
			break
		}
		if ctx.Err() != nil {
			close(release)
			t.Fatal("call-ended never waited for created trigger")
		}
		time.Sleep(10 * time.Millisecond)
	}
	close(release)
	if err := <-created; err != nil {
		t.Errorf("created merge: %v", err)
	}
	if err := <-ended; err != nil {
		t.Errorf("call ended: %v", err)
	}
	var owner uuid.UUID
	e.Scalar(`SELECT ticket_id FROM phone_calls WHERE id=$1`, &owner, call)
	s := mergedInto(t, e, src)
	if owner != dst || s.Into == nil || *s.Into != dst || s.Status != "closed" {
		t.Fatalf("owner=%s source=%+v want destination=%s and source closed", owner, s, dst)
	}
}
