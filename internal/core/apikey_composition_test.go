package core

import (
	"context"
	"testing"

	"github.com/useopendesk/opendesk/internal/ids"
)

func TestAPIKeyRecordingWithBotAndScopes(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	c, _ := e.signup("keyrecordbot")
	wid := c.newWorkspace("Recording Bot Co")
	bot := ids.New()
	if _, err := e.d.DB.Pool.Exec(ctx, `INSERT INTO bots (id, workspace_id, name, look) VALUES ($1, $2, 'Helper', '{}')`, bot, wid); err != nil {
		t.Fatal(err)
	}
	created := c.do("POST", "/v1/api-keys", map[string]any{"name": "Recorded helper", "scopes": []string{"tickets:read", "tickets:write"}, "bot_id": bot.String(), "record_requests": true})
	if created.Status != 201 || created.str("bot_id") != bot.String() || created.Body["record_requests"] != true {
		t.Fatalf("create: %d %s", created.Status, created.Raw)
	}
	path := "/v1/api-keys/" + created.str("id")
	off := c.do("PATCH", path, map[string]any{"record_requests": false})
	if off.Status != 200 || off.str("bot_id") != bot.String() || off.Body["record_requests"] != false || len(off.Body["scopes"].([]any)) != 2 {
		t.Fatalf("record-only update: %d %s", off.Status, off.Raw)
	}
	var audited bool
	if err := e.d.DB.Pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM audit_logs WHERE workspace_id = $1 AND entity_id = $2 AND action = 'api_key.updated' AND data->>'record_requests' = 'false' AND data->>'bot_id' = $3)`, wid, created.str("id"), bot.String()).Scan(&audited); err != nil || !audited {
		t.Fatalf("recording audit: %v, %v", audited, err)
	}
	combined := c.do("PATCH", path, map[string]any{"record_requests": true, "bot_id": nil, "scopes": []string{"tickets:read"}})
	if combined.Status != 200 || combined.Body["bot_id"] != nil || combined.Body["record_requests"] != true || len(combined.Body["scopes"].([]any)) != 1 {
		t.Fatalf("combined update: %d %s", combined.Status, combined.Raw)
	}
	current := c.do("GET", path, nil)
	if current.Status != 200 || current.Body["bot_id"] != nil || current.Body["record_requests"] != true || current.Body["scopes"].([]any)[0] != "tickets:read" {
		t.Fatalf("read-back: %d %s", current.Status, current.Raw)
	}
}
