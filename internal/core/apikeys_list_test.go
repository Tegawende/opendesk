package core

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/useopendesk/opendesk/internal/apilog"
	"github.com/useopendesk/opendesk/internal/ids"
)

// The API keys list counts each recording key's failed requests of the last 24 hours
// (error_count_24h), for the list's Journal column; null for a key that does not record.
func TestAPIKeyErrorCount(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	c, _ := e.signup("keyerrors")
	wid := c.newWorkspace("Key Errors Co")

	rec := c.do("POST", "/v1/api-keys", map[string]any{"name": "Returns portal", "scopes": []string{"tickets:read"}, "record_requests": true})
	quiet := c.do("POST", "/v1/api-keys", map[string]any{"name": "Zapier", "scopes": []string{"tickets:read"}})
	if rec.Status != 201 || quiet.Status != 201 {
		t.Fatalf("create: %d %s / %d %s", rec.Status, rec.Raw, quiet.Status, quiet.Raw)
	}
	if rec.Body["error_count_24h"] != float64(0) || quiet.Body["error_count_24h"] != nil {
		t.Fatalf("new keys: %v / %v", rec.Body["error_count_24h"], quiet.Body["error_count_24h"])
	}
	key, err := uuid.Parse(rec.str("id"))
	if err != nil {
		t.Fatal(err)
	}
	ws, err := uuid.Parse(wid)
	if err != nil {
		t.Fatal(err)
	}
	// Two failures today, one success, and a failure two days ago that no longer counts.
	for _, r := range []struct {
		status int
		ago    time.Duration
	}{{422, time.Minute}, {500, time.Hour}, {201, time.Minute}, {404, 48 * time.Hour}} {
		if err := apilog.Write(ctx, e.d.DB.Pool, apilog.Entry{ID: ids.New(), WorkspaceID: ws, APIKeyID: key,
			CreatedAt: time.Now().Add(-r.ago), Method: "POST", Path: "/v1/tickets", Status: r.status}); err != nil {
			t.Fatal(err)
		}
	}

	list := c.do("GET", "/v1/api-keys", nil)
	if list.Status != 200 {
		t.Fatalf("list: %d %s", list.Status, list.Raw)
	}
	counts := map[string]any{}
	for _, k := range list.Body["data"].([]any) {
		m := k.(map[string]any)
		counts[m["name"].(string)] = m["error_count_24h"]
	}
	if counts["Returns portal"] != float64(2) || counts["Zapier"] != nil {
		t.Fatalf("counts: %v", counts)
	}
	if g := c.do("GET", "/v1/api-keys/"+key.String(), nil); g.Body["error_count_24h"] != float64(2) {
		t.Fatalf("get: %s", g.Raw)
	}
	// Recording off: the count goes back to null.
	if r := c.do("PATCH", "/v1/api-keys/"+key.String(), map[string]any{"record_requests": false}); r.Status != 200 || r.Body["error_count_24h"] != nil {
		t.Fatalf("off: %d %s", r.Status, r.Raw)
	}
}
