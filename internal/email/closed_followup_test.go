package email

import (
	"testing"

	"github.com/useopendesk/opendesk/internal/crypto"
	"github.com/useopendesk/opendesk/internal/ids"
)

// With followup_keeps_assignee on, a customer's reply to a closed ticket opens a follow-up
// assigned to whoever had the closed ticket, with an assigned activity naming it and their
// notification; not when that person is now a viewer, suspended, not on the private team the
// follow-up lands in, or nobody, and never with the setting off (the default). The activity
// names the closed ticket only when it is not in another, private team.
func TestFollowUpKeepsTheAssignee(t *testing.T) {
	viewer := func(w *world) string {
		uid := ids.New()
		mustExec(t, w.d, `INSERT INTO users (id, email, name) VALUES ($1, $2, 'Vic Viewer')`, uid, "vic-"+crypto.RandomString(6)+"@acme.test")
		mustExec(t, w.d, `INSERT INTO members (workspace_id, user_id, role) VALUES ($1, $2, 'viewer')`, w.wid, uid)
		return uid.String()
	}
	member := func(w *world) string { return w.uid.String() }
	suspended := func(w *world) string {
		mustExec(t, w.d, `UPDATE users SET suspended_at = now() WHERE id = $1`, w.uid)
		return w.uid.String()
	}
	for _, tc := range []struct {
		name    string
		on      bool
		who     func(w *world) string // the closed ticket's assignee, "" for none
		private string                // the follow-up is routed to a private team: "on" it or "off" it for the assignee
		prev    string                // the closed ticket is in a private team: "other" than the follow-up's, or the "same"
		kept    bool
	}{
		{"a member", true, member, "", "", true},
		{"a viewer now", true, viewer, "", "", false},
		{"suspended now", true, suspended, "", "", false},
		{"nobody", true, func(*world) string { return "" }, "", "", false},
		{"on the private team", true, member, "on", "", true},
		{"not on the private team", true, member, "off", "", false},
		{"closed in another private team", true, member, "", "other", true},
		{"closed in the same private team", true, member, "on", "same", true},
		{"setting off", false, member, "", "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := newWorld(t)
			m := w.mailbox(InboundForwarding, OutboundSystem, nil)
			if tc.on {
				mustExec(t, w.d, `UPDATE workspaces SET settings = settings || '{"followup_keeps_assignee": true}' WHERE id = $1`, w.wid)
			}
			from := customer()
			tk, ours := firstTicket(t, w, m, from)
			who := tc.who(w)
			if who != "" {
				mustExec(t, w.d, `UPDATE tickets SET assignee_id = $2 WHERE id = $1`, tk.ID, who)
			}
			mustExec(t, w.d, `UPDATE tickets SET status = 'closed' WHERE id = $1`, tk.ID)
			team := ids.New()
			if tc.private != "" {
				mustExec(t, w.d, `INSERT INTO teams (id, workspace_id, name, private) VALUES ($1, $2, 'Billing', true)`, team, w.wid)
				if tc.private == "on" {
					mustExec(t, w.d, `INSERT INTO team_members (team_id, user_id) VALUES ($1, $2)`, team, w.uid)
				}
				mustExec(t, w.d, `UPDATE mailboxes SET team_id = $2 WHERE id = $1`, m.ID, team)
			}
			switch tc.prev {
			case "other":
				other := ids.New()
				mustExec(t, w.d, `INSERT INTO teams (id, workspace_id, name, private) VALUES ($1, $2, 'Legal', true)`, other, w.wid)
				mustExec(t, w.d, `INSERT INTO team_members (team_id, user_id) VALUES ($1, $2)`, other, w.uid)
				mustExec(t, w.d, `UPDATE tickets SET team_id = $2 WHERE id = $1`, tk.ID, other)
			case "same":
				mustExec(t, w.d, `UPDATE tickets SET team_id = $2 WHERE id = $1`, tk.ID, team)
			}
			id := w.receive(m, fixture(t, "gmail_reply.eml", "{{FROM}}", from, "{{TO}}", w.forwardAddr(m, ""), "{{ID}}", crypto.RandomString(8),
				"{{IRT}}", ours, "{{N}}", itoa(tk.Number)))
			msg := w.messageFor(id)
			if msg.TicketID == tk.ID {
				t.Fatal("a reply to a closed ticket must open a new one")
			}
			nt := w.ticket(msg.TicketID)
			if tc.private != "" && nt.TeamID == nil {
				t.Fatal("follow-up not routed to the private team")
			}
			got, want := "", ""
			if nt.Assignee != nil {
				got = nt.Assignee.ID.String()
			}
			if tc.kept {
				want = who
			}
			if got != want {
				t.Fatalf("follow-up assignee %q, want %q", got, want)
			}
			if tc.prev == "other" {
				// Whoever sees the follow-up may not see the closed ticket: it stays unnamed.
				n := w.count(`SELECT count(*) FROM activities WHERE ticket_id = $1 AND type = 'assigned' AND actor_type = 'system'
					AND data->>'via' = 'previous_ticket' AND data->'previous_ticket_id' = 'null' AND data->'previous_ticket_number' = 'null'`, nt.ID)
				if n != 1 {
					t.Fatalf("unnamed assigned activities: %d", n)
				}
			} else {
				n := w.count(`SELECT count(*) FROM activities WHERE ticket_id = $1 AND type = 'assigned' AND actor_type = 'system'
					AND data->>'via' = 'previous_ticket' AND data->>'previous_ticket_id' = $2 AND (data->>'previous_ticket_number')::bigint = $3`,
					nt.ID, tk.ID.String(), tk.Number)
				if (n == 1) != tc.kept {
					t.Fatalf("assigned activities: %d", n)
				}
			}
			notified := w.count(`SELECT count(*) FROM notifications WHERE ticket_id = $1 AND type = 'assigned'`, nt.ID)
			if (notified == 1) != tc.kept {
				t.Fatalf("assigned notifications: %d", notified)
			}
		})
	}
}
