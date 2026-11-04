package collab

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"

	"github.com/useopendesk/opendesk/internal/auth"
	"github.com/useopendesk/opendesk/internal/jobs"
	"github.com/useopendesk/opendesk/internal/plan"
	"github.com/useopendesk/opendesk/internal/products"
)

func TestWorkspaceMailDisabledAPIs(t *testing.T) {
	w := newWorld(t)
	a := w.account(w.agent, ProviderGoogle, VisibilityPrivate, Audience{})
	w.seedThread(a, "Kept mail", w.folder(a, RoleInbox))
	srv := w.api(w.agent)
	mustExec(t, w.d, `UPDATE workspaces SET settings = settings || '{"mail_enabled":false}' WHERE id = $1`, w.wid)
	var status CollabStatus
	if st := doJSON(t, "GET", srv.URL+"/v1/collab", nil, &status); st != 200 || status.Enabled || status.Reason == nil || *status.Reason != products.WorkspaceDisabled || status.Accounts != 1 {
		t.Fatalf("status: %d %+v", st, status)
	}
	for _, tc := range []struct {
		method, path string
		body         any
	}{
		{"GET", "/v1/collab/accounts", nil},
		{"GET", "/v1/collab/threads", nil},
		{"GET", "/v1/collab/counts", nil},
		{"PATCH", "/v1/collab/accounts/" + a.ID.String(), map[string]any{"name": "Changed"}},
		{"POST", "/v1/collab/accounts/oauth/google/start", map[string]any{}},
	} {
		t.Run(tc.method+tc.path, func(t *testing.T) {
			var body struct{ Code, Product, Reason string }
			if st := doJSON(t, tc.method, srv.URL+tc.path, tc.body, &body); st != 403 || body.Code != "feature_disabled" || body.Product != products.Mail || body.Reason != products.WorkspaceDisabled {
				t.Fatalf("disabled: %d %+v", st, body)
			}
		})
	}
	ai := w.api(w.key(w.agent, "Drafter", auth.ScopeCollabDraft))
	var e apiError
	if st := doJSON(t, "GET", ai.URL+"/v1/collab/threads", nil, &e); st != 403 || e.Code != "feature_disabled" {
		t.Fatalf("AI client: %d %+v", st, e)
	}
	if got, err := w.s.overviewCounts(w.ctx, w.agent); err != nil || got != nil {
		t.Fatalf("overview: %#v %v", got, err)
	}
	var prefs SidebarPrefs
	if st := doJSON(t, "GET", srv.URL+"/v1/collab/sidebar", nil, &prefs); st != 200 {
		t.Fatalf("sidebar: %d", st)
	}
	mustExec(t, w.d, `UPDATE workspaces SET settings = settings || '{"mail_enabled":true}' WHERE id = $1`, w.wid)
	if st := doJSON(t, "GET", srv.URL+"/v1/collab/counts", nil, nil); st != 200 {
		t.Fatalf("restored counts: %d", st)
	}
	if w.count(`SELECT count(*) FROM collab_threads WHERE workspace_id=$1`, w.wid) != 1 {
		t.Fatal("stored mail changed")
	}
}

func TestWorkspaceMailAvailabilityPrecedence(t *testing.T) {
	for _, tc := range []struct {
		name                       string
		instance, planOff, billing bool
		reason                     string
	}{
		{"instance", false, false, false, "disabled"},
		{"plan instance switch", true, true, false, "disabled"},
		{"plan", true, false, true, "plan"},
		{"workspace", true, false, false, products.WorkspaceDisabled},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := newWorld(t)
			cfg := *w.d.Cfg
			cfg.Collab = tc.instance
			d := *w.d
			d.Cfg = &cfg
			d.Plan = plan.New(tc.billing, 1)
			d.Plan.CollabOff = tc.planOff
			w.s = newService(&d)
			mustExec(t, w.d, `UPDATE workspaces SET settings=settings || '{"mail_enabled":false}' WHERE id=$1`, w.wid)
			srv := w.api(w.agent)
			var status CollabStatus
			if st := doJSON(t, "GET", srv.URL+"/v1/collab", nil, &status); st != 200 || status.Enabled || status.Reason == nil || *status.Reason != tc.reason {
				t.Fatalf("status: %d %+v", st, status)
			}
			var body struct{ Code, Product, Reason string }
			if st := doJSON(t, "GET", srv.URL+"/v1/collab/accounts", nil, &body); st != 403 || body.Code != "feature_disabled" {
				t.Fatalf("disabled API: %d %+v", st, body)
			}
		})
	}
}

func TestWorkspaceMailPausesAndResumesWork(t *testing.T) {
	w := newWorld(t)
	g := newFakeGmail(t)
	w.useGmail(g)
	a := w.account(w.agent, ProviderGoogle, VisibilityPrivate, Audience{})
	g.add("m1", "t1", []string{"INBOX", "UNREAD"}, testMail{from: "sender@example.com", to: a.Email, subject: "Retained", msgID: "retained@example.com", body: "Mail waits"})
	mustExec(t, w.d, `UPDATE workspaces SET settings=settings || '{"mail_enabled":false}' WHERE id=$1`, w.wid)
	snoozes := func(name string, err error) {
		t.Helper()
		var snooze *river.JobSnoozeError
		if !errors.As(err, &snooze) || snooze.Duration < time.Minute {
			t.Fatalf("%s should snooze: %v", name, err)
		}
	}
	snoozes("sync", w.s.syncAccount(w.ctx, a.ID))
	snoozes("categorize", w.s.categorize(w.ctx, a.ID, 1))
	intentID := uuid.New()
	snoozes("category delivery", w.s.applyRemote(w.ctx, &river.Job[jobs.CollabApplyArgs]{JobRow: &rivertype.JobRow{Attempt: 1, MaxAttempts: 3}, Args: jobs.CollabApplyArgs{WorkspaceID: w.wid, AccountID: a.ID, Action: ActionCategorize, CategoryIntentID: &intentID}}))
	snoozes("apply", w.s.applyRemote(w.ctx, &river.Job[jobs.CollabApplyArgs]{JobRow: &rivertype.JobRow{Attempt: 1, MaxAttempts: 3}, Args: jobs.CollabApplyArgs{WorkspaceID: w.wid, AccountID: a.ID, Action: ActionRead}}))
	snoozes("ticket send", w.s.sendTicketReply(w.ctx, w.wid, uuid.New(), false))
	snoozes("ticket answer", w.s.takeInAnswer(w.ctx, jobs.CollabTicketAnswerArgs{WorkspaceID: w.wid, MessageID: uuid.New()}))
	if err := w.s.enqueueDue(w.ctx); err != nil {
		t.Fatal(err)
	}
	if w.count(`SELECT count(*) FROM river_job WHERE kind='collab.sync_account' AND args->>'account_id'=$1`, a.ID.String()) != 0 {
		t.Fatal("disabled account was scheduled")
	}
	kept, err := loadAccount(w.ctx, w.d.DB.Pool, w.wid, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	if kept.Reconnect || kept.Status != a.Status || string(kept.CredentialsEnc) != string(a.CredentialsEnc) || string(kept.SyncState) != string(a.SyncState) || kept.LastSyncedAt != nil {
		t.Fatal("paused account changed")
	}
	if w.count(`SELECT count(*) FROM collab_messages WHERE account_id=$1`, a.ID) != 0 {
		t.Fatal("disabled sync imported mail")
	}
	mustExec(t, w.d, `UPDATE workspaces SET settings=settings || '{"mail_enabled":true}' WHERE id=$1`, w.wid)
	if err := w.s.enqueueDue(w.ctx); err != nil {
		t.Fatal(err)
	}
	if w.count(`SELECT count(*) FROM river_job WHERE kind='collab.sync_account' AND args->>'account_id'=$1`, a.ID.String()) != 1 {
		t.Fatal("re-enabled account was not scheduled")
	}
	if err := w.s.syncAccount(w.ctx, a.ID); err != nil {
		t.Fatal(err)
	}
	if err := w.s.syncAccount(w.ctx, a.ID); err != nil {
		t.Fatal(err)
	}
	if w.count(`SELECT count(*) FROM collab_messages WHERE account_id=$1 AND remote_id='m1'`, a.ID) != 1 {
		t.Fatal("resumed sync lost or duplicated mail")
	}
}

func TestWorkspaceMailProviderActionResumes(t *testing.T) {
	w := newWorld(t)
	a := w.account(w.agent, ProviderGoogle, VisibilityPrivate, Audience{})
	tid, _ := w.seedThread(a, "Pending action", w.folder(a, RoleInbox))
	fake := &fakeProvider{}
	w.useFake(fake)
	job := &river.Job[jobs.CollabApplyArgs]{JobRow: &rivertype.JobRow{Attempt: 3, MaxAttempts: 3}, Args: jobs.CollabApplyArgs{WorkspaceID: w.wid, AccountID: a.ID, ThreadIDs: []uuid.UUID{tid}, Action: ActionRead}}
	mustExec(t, w.d, `UPDATE workspaces SET settings=settings || '{"mail_enabled":false}' WHERE id=$1`, w.wid)
	var snooze *river.JobSnoozeError
	if err := w.s.applyRemote(w.ctx, job); !errors.As(err, &snooze) {
		t.Fatalf("pending action: %v", err)
	}
	if len(fake.applied) != 0 {
		t.Fatal("disabled provider action ran")
	}
	mustExec(t, w.d, `UPDATE workspaces SET settings=settings || '{"mail_enabled":true}' WHERE id=$1`, w.wid)
	if err := w.s.applyRemote(w.ctx, job); err != nil {
		t.Fatal(err)
	}
	if len(fake.applied) != 1 || fake.applied[0].Action != ActionRead {
		t.Fatalf("resumed actions: %+v", fake.applied)
	}
}
