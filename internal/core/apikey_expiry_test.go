package core

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
)

// An admin moves an existing key's expiry (contracts/api.md "API keys"): an earlier date is
// immediate, a later one or none at all asks for the same recent sign-in as making a key, a past
// date or an expired key is refused, every change is audit-logged, and an API key never does it.
func TestAPIKeyExpiryChange(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	c, email := e.signup("keyexpiry")
	c.workspace = c.newWorkspace("Key Expiry Co")

	in := func(d time.Duration) string { return time.Now().Add(d).UTC().Format(time.RFC3339) }
	r := c.do("POST", "/v1/api-keys", map[string]any{"name": "Quotes board", "scopes": []string{"tickets:read"}, "expires_at": in(90 * 24 * time.Hour)})
	if r.Status != 201 {
		t.Fatalf("create: %d %s", r.Status, r.Raw)
	}
	key := r.str("id")
	path := "/v1/api-keys/" + key
	expiresOf := func(r resp) any { return r.Body["expires_at"] }

	// With a fresh sign-in: later, then never.
	if r := c.do("PATCH", path, map[string]any{"expires_at": in(365 * 24 * time.Hour)}); r.Status != 200 || expiresOf(r) == nil {
		t.Fatalf("extend: %d %s", r.Status, r.Raw)
	}
	if r := c.do("PATCH", path, map[string]any{"expires_at": nil}); r.Status != 200 || expiresOf(r) != nil {
		t.Fatalf("never: %d %s", r.Status, r.Raw)
	}
	// Leaving expires_at out leaves it alone.
	if r := c.do("PATCH", path, map[string]any{"name": "Quote board"}); r.Status != 200 || expiresOf(r) != nil {
		t.Fatalf("rename kept the expiry: %d %s", r.Status, r.Raw)
	}
	// A past date is refused: revoking stops a key.
	if r := c.do("PATCH", path, map[string]any{"expires_at": in(-time.Hour)}); r.Status != 422 {
		t.Fatalf("past date: %d %s", r.Status, r.Raw)
	}

	// An API key cannot change a key's expiry.
	other := c.do("POST", "/v1/api-keys", map[string]any{"name": "Admin tool", "scopes": []string{"settings:read", "settings:write"}})
	keyCaller := e.client()
	keyCaller.token = other.str("secret")
	if r := keyCaller.do("PATCH", path, map[string]any{"expires_at": in(30 * 24 * time.Hour)}); r.Status != 403 {
		t.Fatalf("as an API key: %d %s", r.Status, r.Raw)
	}

	// An old sign-in still shortens, but extending or removing the date asks to sign in again.
	if _, err := e.d.DB.Pool.Exec(ctx, `UPDATE sessions SET created_at = now() - interval '13 hours'
		WHERE user_id = (SELECT id FROM users WHERE email = $1)`, email); err != nil {
		t.Fatal(err)
	}
	if r := c.do("PATCH", path, map[string]any{"expires_at": in(30 * 24 * time.Hour)}); r.Status != 200 || expiresOf(r) == nil {
		t.Fatalf("shorten (never to a date) with an old sign-in: %d %s", r.Status, r.Raw)
	}
	if r := c.do("PATCH", path, map[string]any{"expires_at": in(7 * 24 * time.Hour)}); r.Status != 200 {
		t.Fatalf("shorten with an old sign-in: %d %s", r.Status, r.Raw)
	}
	if r := c.do("PATCH", path, map[string]any{"expires_at": in(60 * 24 * time.Hour)}); r.Status != 403 || r.str("code") != "reauth_required" {
		t.Fatalf("extend with an old sign-in: %d %s", r.Status, r.Raw)
	}
	if r := c.do("PATCH", path, map[string]any{"expires_at": nil}); r.Status != 403 || r.str("code") != "reauth_required" {
		t.Fatalf("never with an old sign-in: %d %s", r.Status, r.Raw)
	}

	// The audit log has one row per change, from and to.
	type change struct {
		From, To *time.Time
	}
	rows, err := e.d.DB.Pool.Query(ctx, `SELECT data FROM audit_logs WHERE entity_id = $1 AND action = 'api_key.expiry_changed'
		ORDER BY created_at, id`, uuid.MustParse(key))
	if err != nil {
		t.Fatal(err)
	}
	var got []change
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			t.Fatal(err)
		}
		var ch change
		if err := json.Unmarshal(raw, &ch); err != nil {
			t.Fatal(err)
		}
		got = append(got, ch)
	}
	rows.Close()
	// later, never, a date again, earlier: the refused changes logged nothing.
	if len(got) != 4 || got[0].From == nil || got[0].To == nil || !got[0].To.After(*got[0].From) ||
		got[1].To != nil || got[2].From != nil || got[2].To == nil || got[3].To == nil || !got[3].To.Before(*got[2].To) {
		t.Fatalf("audit = %+v", got)
	}

	// An expired key stays expired.
	if _, err := e.d.DB.Pool.Exec(ctx, `UPDATE api_keys SET expires_at = now() - interval '1 day' WHERE id = $1`, uuid.MustParse(key)); err != nil {
		t.Fatal(err)
	}
	if _, err := e.d.DB.Pool.Exec(ctx, `UPDATE sessions SET created_at = now() WHERE user_id = (SELECT id FROM users WHERE email = $1)`, email); err != nil {
		t.Fatal(err)
	}
	if r := c.do("PATCH", path, map[string]any{"expires_at": nil}); r.Status != 422 {
		t.Fatalf("revive an expired key: %d %s", r.Status, r.Raw)
	}
	// A revoked key is gone.
	if r := c.do("DELETE", path, nil); r.Status != 204 {
		t.Fatalf("revoke: %d %s", r.Status, r.Raw)
	}
	if r := c.do("PATCH", path, map[string]any{"expires_at": in(30 * 24 * time.Hour)}); r.Status != 404 {
		t.Fatalf("revoked key: %d %s", r.Status, r.Raw)
	}
}

func TestExpiryWidens(t *testing.T) {
	now := time.Now()
	later, earlier := now.Add(time.Hour), now.Add(-time.Hour)
	for _, c := range []struct {
		name     string
		from, to *time.Time
		want     bool
	}{
		{"never to never", nil, nil, false},
		{"never to a date", nil, &now, false},
		{"a date to never", &now, nil, true},
		{"later", &now, &later, true},
		{"earlier", &now, &earlier, false},
		{"same", &now, &now, false},
	} {
		if got := expiryWidens(c.from, c.to); got != c.want {
			t.Errorf("%s: %v, want %v", c.name, got, c.want)
		}
	}
}
