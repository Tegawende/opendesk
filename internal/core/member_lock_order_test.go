package core

import (
	"context"
	"testing"
	"time"
)

func TestMemberWritesWaitBeforeLockingMembers(t *testing.T) {
	for _, kind := range []string{"availability", "self availability", "profile", "role", "permissions", "remove"} {
		t.Run(kind, func(t *testing.T) {
			e := newEnv(t)
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			owner, _ := e.signup("lock-owner")
			wid := owner.newWorkspace("Lock order")
			agent, _ := e.signup("lock-agent")
			uid := agent.do("GET", "/v1/me", nil).Body["user"].(map[string]any)["id"].(string)
			if _, err := e.d.DB.Pool.Exec(ctx, `INSERT INTO members(workspace_id,user_id,role) VALUES($1,$2,'agent')`, wid, uid); err != nil {
				t.Fatal(err)
			}
			method, path, body, want := "PATCH", "/v1/members/"+uid+"/"+kind, map[string]any{}, 200
			switch kind {
			case "availability":
				body["availability"] = "online"
			case "self availability":
				uid = owner.do("GET", "/v1/me", nil).Body["user"].(map[string]any)["id"].(string)
				path = "/v1/members/me/availability"
				body["availability"] = "online"
			case "profile":
				body["title"] = "Support"
			case "role":
				path = "/v1/members/" + uid
				body["role"] = "viewer"
			case "permissions":
				body["message_delete"] = "off"
			case "remove":
				method = "DELETE"
				path = "/v1/members/" + uid
				want = 204
			}
			gate, err := e.d.DB.Pool.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer gate.Rollback(context.Background())
			var pid int32
			if err := gate.QueryRow(ctx, `SELECT pg_backend_pid() FROM workspaces WHERE id=$1 FOR UPDATE`, wid).Scan(&pid); err != nil {
				t.Fatal(err)
			}
			done := make(chan resp, 1)
			go func() { done <- owner.do(method, path, body) }()
			for {
				var blocked bool
				if err := e.d.DB.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE $1=ANY(pg_blocking_pids(pid)))`, pid).Scan(&blocked); err != nil {
					t.Fatal(err)
				}
				if blocked {
					break
				}
				select {
				case r := <-done:
					t.Fatalf("request returned before lock wait: %d %s", r.Status, r.Raw)
				case <-ctx.Done():
					t.Fatal(ctx.Err())
				case <-time.After(10 * time.Millisecond):
				}
			}
			_, lockErr := gate.Exec(ctx, `SELECT 1 FROM members WHERE workspace_id=$1 AND user_id=$2 FOR UPDATE NOWAIT`, wid, uid)
			gate.Rollback(ctx)
			var r resp
			select {
			case r = <-done:
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}
			if r.Status != want {
				t.Fatalf("%s: %d %s", kind, r.Status, r.Raw)
			}
			if lockErr != nil {
				t.Fatalf("workspace-first operation could not lock member: %v", lockErr)
			}
		})
	}
}
