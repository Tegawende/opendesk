package tickets_test

// Merged tickets count nowhere in the reports, a ticket the team started has no first
// response, a bot's reply is not one, and a reply whose author's account was deleted still is.

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/useopendesk/opendesk/internal/auth"
	"github.com/useopendesk/opendesk/internal/domain"
	"github.com/useopendesk/opendesk/internal/helpdesk/tickets"
	"github.com/useopendesk/opendesk/internal/ids"
)

func TestReportsLeaveOutMergedAndTeamStartedTickets(t *testing.T) {
	e, _ := newEnv(t)
	ws := e.NewWorkspace()
	e.SetSettings(ws, map[string]any{"timezone": "UTC"})
	anna := ws.Agent.ID
	n := 0
	ticket := func(created string) uuid.UUID {
		n++
		id := ids.New()
		e.Exec(`INSERT INTO tickets (id, workspace_id, number, brand_id, channel, assignee_id, created_at)
			VALUES ($1, $2, $3, $4, 'email', $5, $6)`, id, ws.ID, n, ws.BrandID, anna, created)
		return id
	}
	solve := func(tid uuid.UUID, at string) {
		e.Exec(`UPDATE tickets SET status = 'solved', solved_at = $2 WHERE id = $1`, tid, at)
	}
	// Messages go through AddMessage, which sets first_responded_at the way the app does.
	add := func(nm domain.NewMessage, at string) {
		t.Helper()
		created, err := time.Parse(time.RFC3339, at)
		if err != nil {
			t.Fatal(err)
		}
		nm.WorkspaceID, nm.Kind, nm.BodyHTML, nm.BodyText, nm.Trigger, nm.CreatedAt = ws.ID, "reply", "<p>Hi</p>", "Hi", "none", created
		if err := e.D.DB.Tx(e.Ctx, func(tx pgx.Tx) error {
			_, err := domain.AddMessage(e.Ctx, tx, e.D.Jobs, nm)
			return err
		}); err != nil {
			t.Fatal(err)
		}
	}
	customer := func(tid uuid.UUID, at string) {
		add(domain.NewMessage{TicketID: tid, Direction: "inbound", AuthorType: "contact"}, at)
	}
	reply := func(tid uuid.UUID, author uuid.UUID, at string) {
		add(domain.NewMessage{TicketID: tid, Direction: "outbound", AuthorType: "user", AuthorUserID: &author}, at)
	}
	var bot uuid.UUID
	e.Scalar(`INSERT INTO bots (workspace_id, name, look, is_default) VALUES ($1, 'Ollie', '{}', true) RETURNING id`, &bot, ws.ID)

	// A customer wrote, Anna answered 30 minutes later; solved after 2 hours.
	t1 := ticket("2026-03-02T10:00:00Z")
	customer(t1, "2026-03-02T10:00:00Z")
	reply(t1, anna, "2026-03-02T10:30:00Z")
	solve(t1, "2026-03-02T12:00:00Z")
	// Anna wrote first (an outbound email); the customer answered later. No first response.
	t2 := ticket("2026-03-02T11:00:00Z")
	reply(t2, anna, "2026-03-02T11:00:05Z")
	customer(t2, "2026-03-02T12:00:00Z")
	solve(t2, "2026-03-02T13:00:00Z")
	// Ollie answered after a minute, Anna after 2 hours: the first response is Anna's.
	t3 := ticket("2026-03-03T09:00:00Z")
	customer(t3, "2026-03-03T09:00:00Z")
	add(domain.NewMessage{TicketID: t3, Direction: "outbound", AuthorType: "system", AI: true, BotID: &bot, FromName: "Ollie"},
		"2026-03-03T09:01:00Z")
	reply(t3, anna, "2026-03-03T11:00:00Z")
	// A duplicate merged into t3 five minutes after it came, which the merge closed.
	dup := ticket("2026-03-03T10:00:00Z")
	e.Exec(`UPDATE tickets SET status = 'closed', solved_at = '2026-03-03T10:05:00Z', first_responded_at = '2026-03-03T10:02:00Z',
		merged_into_id = $2 WHERE id = $1`, dup, t3)
	// Ben answered after an hour and has since deleted his account: still a first response.
	ben := e.AddUser(ws, auth.RoleAgent, "Ben Gone")
	t5 := ticket("2026-03-04T09:00:00Z")
	customer(t5, "2026-03-04T09:00:00Z")
	reply(t5, ben.ID, "2026-03-04T10:00:00Z")
	e.Exec(`DELETE FROM users WHERE id = $1`, ben.ID)
	var gone bool
	e.Scalar(`SELECT author_user_id IS NULL FROM messages WHERE ticket_id = $1 AND direction = 'outbound'`, &gone, t5)
	if !gone {
		t.Fatal("deleting the user kept the reply's author")
	}

	var o tickets.ReportOverview
	e.Do("GET", "/v1/reports/overview?"+march, ws.Admin.As(ws), nil).Want(t, 200).JSON(t, &o)
	// Created t1, t2, t3, t5; solved t1 and t2, each after 2 hours; first responses t1 (30),
	// t5 (60) and t3 (120).
	if o.Created != 4 || o.Solved != 2 || !eq(o.ResolutionP50Minutes, 120) || !eq(o.FirstResponseP50Minutes, 60) {
		t.Fatalf("overview created %d solved %d resolution %v first response %v", o.Created, o.Solved, o.ResolutionP50Minutes, o.FirstResponseP50Minutes)
	}
	created := 0
	for _, d := range o.ByDay {
		created += d.Created
	}
	if created != 4 {
		t.Fatalf("by_day created %d", created)
	}
	if len(o.ByAgent) != 1 || o.ByAgent[0].Solved != 2 || !eq(o.ByAgent[0].FirstResponseP50Minutes, 60) {
		t.Fatalf("by_agent %+v", o.ByAgent)
	}

	// The Agents report: the same.
	r := agentsReport(t, e, ws.Admin.As(ws), march)
	a := rowOf(t, r, anna)
	if a == nil || a.Solved != 2 || !eq(a.FirstResponseP50Minutes, 60) {
		t.Fatalf("anna %+v", a)
	}
}
