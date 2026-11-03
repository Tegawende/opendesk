package imports

import (
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/useopendesk/opendesk/internal/domain"
	"github.com/useopendesk/opendesk/internal/helpdesk/imports/source"
	"testing"
	"time"
)

func TestReview246ConcurrentContactOverride(t *testing.T) {
	e := newEnv(t)
	x := e.start(t, e.zendeskBody())
	if d := e.drive(t, x.ID); d.Status != StatusCompleted {
		t.Fatal(d.Status)
	}
	tid := e.ticketByRef(t, "101")
	var old, chosen uuid.UUID
	e.Scalar(`SELECT contact_id FROM tickets WHERE id=$1`, &old, tid)
	e.Scalar(`SELECT id FROM contacts WHERE workspace_id=$1 AND email='dora@customer.test'`, &chosen, e.ws.ID)
	r := e.load(t, x.ID)
	w, err := e.s.newWriter(e.Ctx, &r, e.f, e.f.scan)
	if err != nil {
		t.Fatal(err)
	}
	agent, err := e.D.DB.Pool.Begin(e.Ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer agent.Rollback(e.Ctx)
	// Same lock, update and audit performed by the agent's PATCH contact_id.
	if _, err := agent.Exec(e.Ctx, `UPDATE tickets SET contact_id=$2 WHERE id=$1`, tid, chosen); err != nil {
		t.Fatal(err)
	}
	if _, err := domain.LogActivity(e.Ctx, agent, e.ws.ID, tid, "user", &e.ws.Admin.ID, "contact_changed", map[string]any{"from": old, "to": chosen}); err != nil {
		t.Fatal(err)
	}
	var pid int
	if err := agent.QueryRow(e.Ctx, "SELECT pg_backend_pid()").Scan(&pid); err != nil {
		t.Fatal(err)
	}
	var followed *uuid.UUID
	done := make(chan error, 1)
	go func() {
		done <- e.D.DB.Tx(e.Ctx, func(tx pgx.Tx) error {
			var err error
			followed, err = w.follow(e.Ctx, tx, tid, &source.Ticket{Requester: &source.Person{ID: "new-requester", Email: "new@customer.test"}})
			return err
		})
	}()
	deadline := time.Now().Add(5 * time.Second)
	for {
		var blocked bool
		e.Scalar(`SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE $1=ANY(pg_blocking_pids(pid)))`, &blocked, pid)
		if blocked {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("import did not reach ticket update lock")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err := agent.Commit(e.Ctx); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if followed == nil || *followed != chosen {
		t.Fatalf("follow returned stale contact: %v, want %s", followed, chosen)
	}
	var actual uuid.UUID
	e.Scalar(`SELECT contact_id FROM tickets WHERE id=$1`, &actual, tid)
	if actual != chosen {
		t.Fatal("source requester overwrote concurrently committed local contact override")
	}
}
