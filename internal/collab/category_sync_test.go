package collab

import (
	"testing"

	"github.com/google/uuid"

	"github.com/useopendesk/opendesk/internal/ids"
	"github.com/useopendesk/opendesk/internal/plan"
)

func TestRecoverCategoryIntentsSkipsUnavailableWorkspaces(t *testing.T) {
	for _, reason := range []string{"workspace_disabled", "plan", "suspended"} {
		t.Run(reason, func(t *testing.T) {
			disabled, enabled := newWorld(t), newWorld(t)
			t.Cleanup(func() {
				// Keep the unavailable fixture paused when another subtest uses a different billing mode.
				mustExec(t, disabled.d, `UPDATE workspaces SET settings = settings || '{"mail_enabled":false}' WHERE id = $1`, disabled.wid)
			})
			paused := disabled.account(disabled.agent, ProviderGoogle, VisibilityPrivate, Audience{})
			active := enabled.account(enabled.agent, ProviderGoogle, VisibilityPrivate, Audience{})
			switch reason {
			case "workspace_disabled":
				mustExec(t, disabled.d, `UPDATE workspaces SET settings = settings || '{"mail_enabled":false}' WHERE id = $1`, disabled.wid)
			case "plan":
				d := *enabled.d
				d.Plan = plan.New(true, 1)
				enabled.s = newService(&d)
				if err := plan.SetCollabAccess(enabled.ctx, d.DB.Pool, enabled.wid, plan.CollabAccess{Access: plan.CollabComplimentary}); err != nil {
					t.Fatal(err)
				}
			case "suspended":
				mustExec(t, disabled.d, `UPDATE workspaces SET suspended_at = now() WHERE id = $1`, disabled.wid)
			}
			seed := func(w *world, a *accountRow, age string) uuid.UUID {
				tid, _ := w.seedThread(a, "Lost category wakeup")
				id := ids.New()
				mustExec(t, w.d, `INSERT INTO collab_category_intents (thread_id, workspace_id, account_id, id, epoch, desired, state, updated_at)
      VALUES ($1, $2, $3, $4, $5, 'forums', 'blocked', now() - $6::interval)`, tid, w.wid, a.ID, id, a.CategoryEpoch, age)
				return id
			}
			for range 100 {
				seed(disabled, paused, "10 minutes")
			}
			intent := seed(enabled, active, "5 minutes")
			if n := enabled.count(`SELECT count(*) FROM collab_category_intents WHERE workspace_id = $1`, disabled.wid); n != 100 {
				t.Fatalf("older paused candidates visible to recovery: %d, want 100", n)
			}
			for _, check := range []struct {
				wid    uuid.UUID
				active bool
			}{{disabled.wid, false}, {enabled.wid, true}} {
				active, err := enabled.s.collabActive(enabled.ctx, check.wid)
				if err != nil || active != check.active {
					t.Fatalf("fixture availability: active %v, want %v, error %v", active, check.active, err)
				}
			}
			for range 2 {
				if err := enabled.s.recoverCategoryIntents(enabled.ctx); err != nil {
					t.Fatal(err)
				}
				if n := enabled.count(`SELECT count(*) FROM river_job WHERE kind = 'collab.apply' AND args->>'category_intent_id' = $1`, intent.String()); n != 1 {
					t.Fatalf("eligible wakeups: %d, want 1", n)
				}
			}

			if n := disabled.count(`SELECT count(*) FROM river_job WHERE kind = 'collab.apply' AND args->>'workspace_id' = $1`, disabled.wid.String()); n != 0 {
				t.Fatalf("unavailable workspace wakeups: %d", n)
			}
			if n := disabled.count(`SELECT count(*) FROM collab_category_intents WHERE workspace_id = $1 AND state = 'blocked' AND updated_at < now() - interval '9 minutes'`, disabled.wid); n != 100 {
				t.Fatalf("unchanged paused intents: %d, want 100", n)
			}
		})
	}
}

func TestRecoverCategoryIntentsKeepsEligibleBatchLimit(t *testing.T) {
	w := newWorld(t)
	a := w.account(w.agent, ProviderGoogle, VisibilityPrivate, Audience{})
	for range 101 {
		tid, _ := w.seedThread(a, "Waiting category recovery")
		mustExec(t, w.d, `INSERT INTO collab_category_intents (thread_id, workspace_id, account_id, id, epoch, desired, state, updated_at)
    VALUES ($1, $2, $3, $4, $5, 'forums', 'pending', now() - interval '5 minutes')`, tid, w.wid, a.ID, ids.New(), a.CategoryEpoch)
	}
	for _, want := range []int{100, 101} {
		if err := w.s.recoverCategoryIntents(w.ctx); err != nil {
			t.Fatal(err)
		}
		if n := w.count(`SELECT count(*) FROM river_job WHERE kind = 'collab.apply' AND args->>'workspace_id' = $1`, w.wid.String()); n != want {
			t.Fatalf("recovered wakeups: %d, want %d", n, want)
		}
	}
}
