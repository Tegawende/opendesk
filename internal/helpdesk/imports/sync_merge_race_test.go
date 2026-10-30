package imports

import (
	"github.com/jackc/pgx/v5"
	"github.com/useopendesk/opendesk/internal/domain"
	"github.com/useopendesk/opendesk/internal/helpdesk/imports/source"
	"testing"
	"time"
)

func TestReview244ConcurrentAgentNote(t *testing.T) {
	e := newEnv(t)
	x := e.start(t, e.zendeskBody())
	if d := e.drive(t, x.ID); d.Status != StatusCompleted {
		t.Fatal(d.Status)
	}
	src, dst := e.ticketByRef(t, "101"), e.ticketByRef(t, "102")
	r := e.load(t, x.ID)
	w := &writer{s: e.s, x: &r}
	// An agent has written a note and holds the same ticket lock used by AddMessage.
	agent, err := e.D.DB.Pool.Begin(e.Ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer agent.Rollback(e.Ctx)
	note, err := domain.AddMessage(e.Ctx, agent, e.D.Jobs, domain.NewMessage{WorkspaceID: e.ws.ID, TicketID: src, Kind: "note", Direction: "outbound", AuthorType: "user", AuthorUserID: &e.ws.Admin.ID, BodyText: "Local agent note"})
	if err != nil {
		t.Fatal(err)
	}
	var pid int
	if err := agent.QueryRow(e.Ctx, "SELECT pg_backend_pid()").Scan(&pid); err != nil {
		t.Fatal(err)
	}
	var messages []source.Message
	rows, err := e.D.DB.Pool.Query(e.Ctx, `SELECT r.external_id FROM import_records r JOIN messages m ON m.id=r.local_id WHERE r.workspace_id=$1 AND r.kind='message' AND m.ticket_id=$2`, e.ws.ID, src)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		messages = append(messages, source.Message{ID: id})
	}
	rows.Close()
	done := make(chan error, 1)
	go func() {
		done <- e.D.DB.Tx(e.Ctx, func(tx pgx.Tx) error { return w.absorb(e.Ctx, tx, dst, &source.Ticket{Messages: messages}) })
	}()
	deadline := time.Now().Add(5 * time.Second)
	for {
		var blocked bool
		e.Scalar(`SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE $1=ANY(pg_blocking_pids(pid)))`, &blocked, pid)
		if blocked {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("import did not reach merge lock")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err := agent.Commit(e.Ctx); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	var retained bool
	e.Scalar(`SELECT ticket_id=$2 FROM messages WHERE id=$1`, &retained, note.ID, src)
	if !retained {
		t.Fatal("import merged away the ticket and moved the committed local agent note")
	}
}
