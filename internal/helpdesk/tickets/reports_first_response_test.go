package tickets_test

// Merged tickets count nowhere in the reports, a ticket the team started has no first
// response, and a first reply that names no member (an imported bot's) gives way to the first
// member's.

import (
	"testing"

	"github.com/google/uuid"

	"github.com/useopendesk/opendesk/internal/helpdesk/tickets"
	"github.com/useopendesk/opendesk/internal/ids"
)

func TestReportsLeaveOutMergedAndTeamStartedTickets(t *testing.T) {
	e, _ := newEnv(t)
	ws := e.NewWorkspace()
	e.SetSettings(ws, map[string]any{"timezone": "UTC"})
	anna := ws.Agent.ID
	n := 0
	ticket := func(created string, firstReply, solved any, extra string) uuid.UUID {
		n++
		id := ids.New()
		status := "open"
		if solved != nil {
			status = "solved"
		}
		e.Exec(`INSERT INTO tickets (id, workspace_id, number, brand_id, channel, assignee_id, status, created_at, first_responded_at, solved_at)
			VALUES ($1, $2, $3, $4, 'email', $5, $6, $7, $8, $9)`, id, ws.ID, n, ws.BrandID, anna, status, created, firstReply, solved)
		if extra != "" {
			e.Exec(`UPDATE tickets SET `+extra+` WHERE id = $1`, id)
		}
		return id
	}
	customer := func(tid uuid.UUID, at string) {
		e.Exec(`INSERT INTO messages (workspace_id, ticket_id, kind, direction, author_type, created_at) VALUES ($1, $2, 'reply', 'inbound', 'contact', $3)`,
			ws.ID, tid, at)
	}
	reply := func(tid uuid.UUID, author *uuid.UUID, at string) {
		e.Exec(`INSERT INTO messages (workspace_id, ticket_id, kind, direction, author_type, author_user_id, created_at) VALUES ($1, $2, 'reply', 'outbound', 'user', $3, $4)`,
			ws.ID, tid, author, at)
	}

	// A customer wrote, Anna answered 30 minutes later; solved after 2 hours.
	t1 := ticket("2026-03-02T10:00:00Z", "2026-03-02T10:30:00Z", "2026-03-02T12:00:00Z", "")
	customer(t1, "2026-03-02T10:00:00Z")
	reply(t1, &anna, "2026-03-02T10:30:00Z")
	// Anna wrote first (an outbound email); the customer answered later. No first response.
	t2 := ticket("2026-03-02T11:00:00Z", "2026-03-02T11:00:05Z", "2026-03-02T13:00:00Z", "")
	reply(t2, &anna, "2026-03-02T11:00:05Z")
	customer(t2, "2026-03-02T12:00:00Z")
	// An imported bot's auto-reply after a minute (a user reply with no member), Anna's after 2 hours.
	t3 := ticket("2026-03-03T09:00:00Z", "2026-03-03T09:01:00Z", nil, "")
	customer(t3, "2026-03-03T09:00:00Z")
	reply(t3, nil, "2026-03-03T09:01:00Z")
	reply(t3, &anna, "2026-03-03T11:00:00Z")
	// A duplicate merged into t3 five minutes after it came, which the merge closed.
	ticket("2026-03-03T10:00:00Z", "2026-03-03T10:02:00Z", "2026-03-03T10:05:00Z", "status = 'closed', merged_into_id = '"+t3.String()+"'")

	var o tickets.ReportOverview
	e.Do("GET", "/v1/reports/overview?"+march, ws.Admin.As(ws), nil).Want(t, 200).JSON(t, &o)
	// Created t1, t2, t3; solved t1 and t2, each after 2 hours; first responses t1 (30) and t3 (120).
	if o.Created != 3 || o.Solved != 2 || !eq(o.ResolutionP50Minutes, 120) || !eq(o.FirstResponseP50Minutes, 75) {
		t.Fatalf("overview created %d solved %d resolution %v first response %v", o.Created, o.Solved, o.ResolutionP50Minutes, o.FirstResponseP50Minutes)
	}
	created := 0
	for _, d := range o.ByDay {
		created += d.Created
	}
	if created != 3 {
		t.Fatalf("by_day created %d", created)
	}
	if len(o.ByAgent) != 1 || o.ByAgent[0].Solved != 2 || !eq(o.ByAgent[0].FirstResponseP50Minutes, 75) {
		t.Fatalf("by_agent %+v", o.ByAgent)
	}

	// The Agents report: the same.
	r := agentsReport(t, e, ws.Admin.As(ws), march)
	a := rowOf(t, r, anna)
	if a == nil || a.Solved != 2 || !eq(a.FirstResponseP50Minutes, 75) {
		t.Fatalf("anna %+v", a)
	}
}
