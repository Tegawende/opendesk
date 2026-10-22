package collab

import (
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/useopendesk/opendesk/internal/auth"
)

// collab_new_mail (contracts/collab.md "Notifications"), through a Gmail sync.

// newMailNotes is the member's collab_new_mail notifications: unread ones first.
func (w *world) newMailNotes(p *auth.Principal) []map[string]any {
	w.t.Helper()
	rows, err := w.d.DB.Pool.Query(w.ctx, `SELECT data FROM notifications WHERE workspace_id = $1 AND user_id = $2 AND type = $3
		ORDER BY read_at IS NULL DESC, created_at DESC`, w.wid, p.UserID, NotifyNewMail)
	if err != nil {
		w.t.Fatal(err)
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var d map[string]any
		if err := rows.Scan(&d); err != nil {
			w.t.Fatal(err)
		}
		out = append(out, d)
	}
	return out
}

// arrive adds a message to the fake Gmail and records it in the history.
func arrive(g *fakeGmail, id, thread string, labels []string, m testMail) {
	if m.date.IsZero() {
		m.date = time.Now()
	}
	g.add(id, thread, labels, m)
	g.record("messagesAdded", id)
}

func TestNewMailNotifiesPrivateOwner(t *testing.T) {
	w := newWorld(t)
	g := newFakeGmail(t)
	w.useGmail(g)
	a := w.account(w.agent, ProviderGoogle, VisibilityPrivate, Audience{})
	// The first import: mail already there is not news.
	g.add("m1", "t1", []string{"INBOX", "UNREAD"}, testMail{from: "Frank <frank@example.com>", to: a.Email, subject: "Quote",
		msgID: "q1@example.com", body: "Can you send a quote?"})
	w.sync(a)
	if n := len(w.newMailNotes(w.agent)); n != 0 {
		t.Fatalf("the first import notified %d times", n)
	}

	// New mail in the inbox: its owner is told, nobody else.
	arrive(g, "m2", "t2", []string{"INBOX", "UNREAD"}, testMail{from: "Gina Lee <gina@example.com>", to: a.Email, subject: "Renewal",
		msgID: "r1@example.com", body: "Time to renew the contract."})
	// Own mail sent elsewhere, a draft, spam and read mail are not news.
	arrive(g, "m3", "t3", []string{"SENT"}, testMail{from: a.Email, to: "gina@example.com", subject: "Hello", msgID: "s1@acme.test", body: "Hi"})
	arrive(g, "m4", "t4", []string{"INBOX", "UNREAD"}, testMail{from: "Box <" + a.Email + ">", to: a.Email, subject: "Note to self",
		msgID: "s2@acme.test", body: "Remember"})
	arrive(g, "m5", "t5", []string{"SPAM", "UNREAD"}, testMail{from: "Spam <spam@example.com>", to: a.Email, subject: "Win", msgID: "x1@example.com", body: "Prize"})
	arrive(g, "m6", "t6", []string{"DRAFT"}, testMail{from: a.Email, to: "gina@example.com", subject: "Draft", msgID: "d1@acme.test", body: "Later"})
	arrive(g, "m7", "t7", []string{"INBOX"}, testMail{from: "Hal <hal@example.com>", to: a.Email, subject: "Read already", msgID: "h1@example.com", body: "Seen"})
	arrive(g, "m8", "t8", []string{"INBOX", "UNREAD"}, testMail{from: "Old <old@example.com>", to: a.Email, subject: "Old news",
		msgID: "o1@example.com", body: "Back from the archive", date: time.Now().Add(-10 * 24 * time.Hour)})
	w.sync(a)

	notes := w.newMailNotes(w.agent)
	if len(notes) != 1 {
		t.Fatalf("owner notifications: %+v", notes)
	}
	d := notes[0]
	t2 := w.threadByRemote(a, "t2")
	from, _ := d["from"].(map[string]any)
	if d["thread_id"] != t2.ID.String() || d["account_id"] != a.ID.String() || d["subject"] != "Renewal" ||
		from["name"] != "Gina Lee" || from["email"] != "gina@example.com" || d["preview"] != "Time to renew the contract." || d["count"] != float64(1) {
		t.Fatalf("data: %+v", d)
	}
	for _, p := range []*auth.Principal{w.owner, w.agent2, w.viewer} {
		if n := len(w.newMailNotes(p)); n != 0 {
			t.Fatalf("someone else was told about a private account's mail (%d)", n)
		}
	}
	// The event reached its recipient alone.
	if n := w.count(`SELECT count(*) FROM events WHERE workspace_id = $1 AND type = 'notification.created'
		AND data->'notification'->>'type' = $2 AND NOT (user_ids = ARRAY[$3::uuid])`, w.wid, NotifyNewMail, w.agent.UserID); n != 0 {
		t.Fatalf("%d notification events reached others", n)
	}
}

func TestNewMailBurstIsOneNotification(t *testing.T) {
	w := newWorld(t)
	g := newFakeGmail(t)
	w.useGmail(g)
	a := w.account(w.agent, ProviderGoogle, VisibilityPrivate, Audience{})
	w.sync(a)
	// A burst on one conversation, across more than one batch, and over two syncs.
	for i := range syncBatch + 5 {
		id := "b" + string(rune('a'+i/26)) + string(rune('a'+i%26))
		arrive(g, id, "tb", []string{"INBOX", "UNREAD"}, testMail{from: "Nora <nora@example.com>", to: a.Email, subject: "Thread",
			msgID: id + "@example.com", body: "Message " + id, date: time.Now().Add(time.Duration(i) * time.Second)})
	}
	w.sync(a)
	arrive(g, "late", "tb", []string{"INBOX", "UNREAD"}, testMail{from: "Nora <nora@example.com>", to: a.Email, subject: "Re: Thread",
		msgID: "late@example.com", body: "One more", date: time.Now().Add(time.Minute)})
	w.sync(a)
	notes := w.newMailNotes(w.agent)
	if len(notes) != 1 || notes[0]["count"] != float64(syncBatch+6) || notes[0]["preview"] != "One more" || notes[0]["subject"] != "Thread" {
		t.Fatalf("burst: %d notifications, first %+v", len(notes), notes[0])
	}
	// Once read, the next message starts a new one.
	mustExec(t, w.d, `UPDATE notifications SET read_at = now() WHERE user_id = $1`, w.agent.UserID)
	arrive(g, "after", "tb", []string{"INBOX", "UNREAD"}, testMail{from: "Nora <nora@example.com>", to: a.Email, subject: "Re: Thread",
		msgID: "after@example.com", body: "And another", date: time.Now().Add(2 * time.Minute)})
	w.sync(a)
	if notes := w.newMailNotes(w.agent); len(notes) != 2 || notes[0]["count"] != float64(1) {
		t.Fatalf("after reading: %+v", notes)
	}
}

func TestNewMailSharedAndMuted(t *testing.T) {
	w := newWorld(t)
	g := newFakeGmail(t)
	w.useGmail(g)
	team := w.team("Sales", w.agent2)
	a := w.account(w.agent, ProviderGoogle, VisibilityShared, Audience{TeamIDs: []uuid.UUID{team}})
	w.sync(a)

	// The audience mutes for themselves through the API: Bea mutes, Adam keeps it on.
	var out MailAccount
	if code := doJSON(t, http.MethodPut, w.api(w.agent2).URL+"/v1/collab/accounts/"+a.ID.String()+"/notifications",
		map[string]any{"new_mail": false}, &out); code != 200 || out.Notifications.NewMail {
		t.Fatalf("mute: %d %+v", code, out.Notifications)
	}
	var list []MailAccount
	doJSON(t, http.MethodGet, w.api(w.agent).URL+"/v1/collab/accounts", nil, &list)
	if len(list) != 1 || !list[0].Notifications.NewMail {
		t.Fatalf("someone else's mute shows for the owner: %+v", list)
	}
	// Someone who does not see the account cannot set anything on it.
	if code := doJSON(t, http.MethodPut, w.api(w.viewer).URL+"/v1/collab/accounts/"+a.ID.String()+"/notifications",
		map[string]any{"new_mail": false}, nil); code != 404 {
		t.Fatalf("outsider: %d", code)
	}

	arrive(g, "n1", "tn", []string{"INBOX", "UNREAD"}, testMail{from: "Ivy <ivy@example.com>", to: a.Email, subject: "Order",
		msgID: "n1@example.com", body: "New order"})
	w.sync(a)
	if n := len(w.newMailNotes(w.agent)); n != 1 {
		t.Fatalf("owner of the shared account: %d", n)
	}
	if n := len(w.newMailNotes(w.agent2)); n != 0 {
		t.Fatalf("muted member told: %d", n)
	}
	if n := len(w.newMailNotes(w.owner)) + len(w.newMailNotes(w.viewer)); n != 0 {
		t.Fatalf("outside the audience told: %d", n)
	}

	// Unmuted, Bea hears of the next one.
	doJSON(t, http.MethodPut, w.api(w.agent2).URL+"/v1/collab/accounts/"+a.ID.String()+"/notifications", map[string]any{"new_mail": true}, nil)
	arrive(g, "n2", "tn2", []string{"INBOX", "UNREAD"}, testMail{from: "Ivy <ivy@example.com>", to: a.Email, subject: "Second order",
		msgID: "n2@example.com", body: "Another"})
	w.sync(a)
	if n := len(w.newMailNotes(w.agent2)); n != 1 {
		t.Fatalf("unmuted member: %d", n)
	}

	// Made private, the account tells its owner alone, admins included.
	mustExec(t, w.d, `UPDATE collab_accounts SET visibility = 'private' WHERE id = $1`, a.ID)
	arrive(g, "n3", "tn3", []string{"INBOX", "UNREAD"}, testMail{from: "Ivy <ivy@example.com>", to: a.Email, subject: "Third order",
		msgID: "n3@example.com", body: "Again"})
	w.sync(a)
	if n := len(w.newMailNotes(w.agent2)); n != 1 {
		t.Fatalf("a private account told its former audience: %d", n)
	}
	if n := len(w.newMailNotes(w.agent)); n != 3 {
		t.Fatalf("owner: %d", n)
	}

	// The owner mutes their own account.
	if err := setNewMailPref(w.ctx, w.d.DB.Pool, w.wid, a.ID, w.agent.UserID, false); err != nil {
		t.Fatal(err)
	}
	arrive(g, "n4", "tn4", []string{"INBOX", "UNREAD"}, testMail{from: "Ivy <ivy@example.com>", to: a.Email, subject: "Fourth order",
		msgID: "n4@example.com", body: "Once more"})
	w.sync(a)
	if n := len(w.newMailNotes(w.agent)); n != 3 {
		t.Fatalf("muted owner told: %d", n)
	}
}
