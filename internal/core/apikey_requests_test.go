package core

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/useopendesk/opendesk/internal/apilog"
	"github.com/useopendesk/opendesk/internal/auth"
	"github.com/useopendesk/opendesk/internal/httpx"
)

// "Record requests" on an API key (contracts/api.md "API keys"): off, nothing is kept; on, every
// request is, with what a write sent (secrets masked, omitted past 64 KB), the answer to a failure and
// the records it touched; only admin users read the record, and it goes after 90 days.
func TestAPIKeyRequestLog(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	c, _ := e.signup("keylog")
	wid := c.newWorkspace("Key Log Co")

	r := c.do("POST", "/v1/api-keys", map[string]any{"name": "Returny", "scopes": []string{"tickets:read", "tickets:write"}})
	if r.Status != 201 || r.Body["record_requests"] != false {
		t.Fatalf("create: %d %s", r.Status, r.Raw)
	}
	key, secret := r.str("id"), r.str("secret")

	// A stand-in for the API behind the recorder, shaped like its answers.
	ticketID, contactID, messageID := uuid.New(), uuid.New(), uuid.New()
	var got int
	router := chi.NewRouter()
	resolver := &auth.Resolver{Pool: e.d.DB.Pool, Tokens: e.d.Tokens, ClientIP: httpx.ClientIP(false, 0)}
	router.Use(resolver.Middleware, apilog.Middleware(e.d.DB.Pool, slog.New(slog.NewTextHandler(io.Discard, nil))))
	answer := func(w http.ResponseWriter, status int, v any) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(v)
	}
	router.Post("/v1/tickets", func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		got = len(b)
		answer(w, 201, map[string]any{"id": ticketID, "number": 4242, "contact": map[string]any{"id": contactID}})
	})
	router.Post("/v1/tickets/{ref}/messages", func(w http.ResponseWriter, r *http.Request) {
		if chi.URLParam(r, "ref") == "4242" {
			answer(w, 201, map[string]any{"id": messageID, "ticket_id": ticketID})
			return
		}
		answer(w, 422, map[string]any{"status": 422, "code": "validation_failed", "detail": "body_html is empty"})
	})
	router.Get("/v1/tickets/{ref}", func(w http.ResponseWriter, r *http.Request) {
		answer(w, 200, map[string]any{"id": ticketID, "number": 4242})
	})
	api := httptest.NewServer(router)
	defer api.Close()
	call := func(method, path string, body []byte, hdr ...string) int {
		t.Helper()
		req, _ := http.NewRequest(method, api.URL+path, bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+secret)
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		for i := 0; i+1 < len(hdr); i += 2 {
			req.Header.Set(hdr[i], hdr[i+1])
		}
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = io.Copy(io.Discard, res.Body)
		res.Body.Close()
		apilog.Wait()
		return res.StatusCode
	}
	count := func() int {
		var n int
		if err := e.d.DB.Pool.QueryRow(ctx, `SELECT count(*) FROM api_key_requests WHERE api_key_id = $1`, key).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}

	// Off: nothing is kept.
	if s := call("GET", "/v1/tickets/1", nil); s != 200 || count() != 0 {
		t.Fatalf("recorded with the setting off: %d, %d rows", s, count())
	}

	// On, from the key's next request; the change is audited.
	if r := c.do("PATCH", "/v1/api-keys/"+key, map[string]any{"record_requests": true}); r.Status != 200 || r.Body["record_requests"] != true {
		t.Fatalf("turn on: %d %s", r.Status, r.Raw)
	}
	var audited bool
	if err := e.d.DB.Pool.QueryRow(ctx, `SELECT (data->>'record_requests')::boolean FROM audit_logs
		WHERE entity_id = $1 AND action = 'api_key.updated' ORDER BY created_at DESC LIMIT 1`, key).Scan(&audited); err != nil || !audited {
		t.Fatalf("audit: %v %v", audited, err)
	}

	// A write: its JSON, secrets masked, the idempotency key and the records its answer names.
	if _, err := e.d.DB.Pool.Exec(ctx, `INSERT INTO tickets (id, workspace_id, number, subject, channel, brand_id) VALUES ($1, $2, 4242, 'Return R-1', 'api',
		(SELECT id FROM brands WHERE workspace_id = $2 AND is_default))`, ticketID, wid); err != nil {
		t.Fatal(err)
	}
	create := []byte(`{"subject":"Return R-1","password":"hunter2","meta":{"api_token":"abc","order":"#1042"}}`)
	if s := call("POST", "/v1/tickets", create, "Idempotency-Key", "returny-R-1"); s != 201 {
		t.Fatalf("create: %d", s)
	}
	if got != len(create) {
		t.Fatalf("the handler got %d bytes of %d", got, len(create))
	}
	// A failure keeps its answer; a ticket named by its number is found.

	if s := call("POST", "/v1/tickets/4241/messages", []byte(`{"kind":"note","body_html":""}`)); s != 422 {
		t.Fatalf("refused note: %d", s)
	}
	if s := call("POST", "/v1/tickets/4242/messages", []byte(`{"kind":"note","body_html":"<p>Return details</p>"}`)); s != 201 {
		t.Fatalf("note: %d", s)
	}
	// A read: no body, the query masked, the reference as the path wrote it.
	if s := call("GET", "/v1/tickets/ext:555?token=s3cr3t&expand=messages", nil); s != 200 {
		t.Fatalf("read: %d", s)
	}
	// A body past 64 KB reaches the handler whole and is kept cut.
	big := []byte(`{"body_html":"` + strings.Repeat("é", 40<<10) + `","secret":"x"}`)
	if s := call("POST", "/v1/tickets", big); s != 201 || got != len(big) {
		t.Fatalf("big: %d, the handler got %d of %d", s, got, len(big))
	}

	list := func(q string) []map[string]any {
		t.Helper()
		r := c.do("GET", "/v1/api-keys/"+key+"/requests"+q, nil)
		if r.Status != 200 {
			t.Fatalf("list %s: %d %s", q, r.Status, r.Raw)
		}
		var out []map[string]any
		for _, x := range r.Body["data"].([]any) {
			out = append(out, x.(map[string]any))
		}
		return out
	}
	all := list("")
	if len(all) != 5 {
		t.Fatalf("listed %d, want 5: %v", len(all), all)
	}
	byPath := map[string]map[string]any{}
	for _, x := range all {
		byPath[x["method"].(string)+" "+x["path"].(string)+" "+strconv.Itoa(int(x["status"].(float64)))] = x
	}
	detail := func(x map[string]any) map[string]any {
		t.Helper()
		r := c.do("GET", "/v1/api-keys/"+key+"/requests/"+x["id"].(string), nil)
		if r.Status != 200 {
			t.Fatalf("detail: %d %s", r.Status, r.Raw)
		}
		return r.Body
	}

	var w, cut map[string]any
	for _, x := range all {
		if x["method"] == "POST" && x["path"] == "/v1/tickets" {
			if x["idempotency_key"] == "returny-R-1" {
				w = detail(x)
			} else {
				cut = detail(x)
			}
		}
	}
	body, _ := w["request_body"].(string)
	if strings.Contains(body, "hunter2") || strings.Contains(body, "abc") || !strings.Contains(body, `"password":"***"`) ||
		!strings.Contains(body, `"order":"#1042"`) || w["idempotency_key"] != "returny-R-1" || w["request_truncated"] != false {
		t.Fatalf("write: %v", w)
	}
	if tk, _ := w["ticket"].(map[string]any); tk == nil || tk["id"] != ticketID.String() || tk["number"] != float64(4242) || w["contact_id"] != contactID.String() {
		t.Fatalf("write's records: %v", w)
	}
	f := detail(byPath["POST /v1/tickets/4241/messages 422"])
	if rb, _ := f["response_body"].(string); !strings.Contains(rb, "validation_failed") || f["ticket_ref"] != "4241" || f["ticket"] != nil {
		t.Fatalf("failure: %v", f)
	}
	n := detail(byPath["POST /v1/tickets/4242/messages 201"])
	if tk, _ := n["ticket"].(map[string]any); tk == nil || tk["id"] != ticketID.String() || n["message_id"] != messageID.String() || n["response_body"] != nil {
		t.Fatalf("note: %v", n)
	}
	g := detail(byPath["GET /v1/tickets/ext:555 200"])
	if g["request_body"] != nil || strings.Contains(g["query"].(string), "s3cr3t") || !strings.Contains(g["query"].(string), "expand=messages") || g["ticket_ref"] != "ext:555" {
		t.Fatalf("read: %v", g)
	}
	if b, _ := cut["request_body"].(string); cut["request_truncated"] != true || b != apilog.Masked {
		t.Fatalf("cut body: %v, %d bytes", cut["request_truncated"], len(b))
	}

	// Filters and pages.
	for q, want := range map[string]int{"?status=4xx": 1, "?status=2xx": 4, "?method=GET": 1, "?ticket=ext:555": 1,
		"?ticket=4242": 4, "?ticket=" + ticketID.String(): 4, "?ticket=4241": 1, "?since=2000-01-01T00:00:00Z&until=2000-01-02T00:00:00Z": 0} {
		if got := list(q); len(got) != want {
			t.Fatalf("%s: %d, want %d", q, len(got), want)
		}
	}
	if r := c.do("GET", "/v1/api-keys/"+key+"/requests?since=yesterday", nil); r.Status != 422 {
		t.Fatalf("bad since: %d", r.Status)
	}
	p1 := c.do("GET", "/v1/api-keys/"+key+"/requests?limit=3", nil)
	cur, _ := p1.Body["next_cursor"].(string)
	p2 := c.do("GET", "/v1/api-keys/"+key+"/requests?limit=3&cursor="+cur, nil)
	if len(p1.Body["data"].([]any)) != 3 || cur == "" || len(p2.Body["data"].([]any)) != 2 {
		t.Fatalf("pages: %s / %s", p1.Raw, p2.Raw)
	}

	// Admin users only: not an API key (even one that may write settings), not an agent.
	other := c.do("POST", "/v1/api-keys", map[string]any{"name": "Settings tool", "scopes": []string{"settings:read", "settings:write"}})
	keyCaller := e.client()
	keyCaller.token, keyCaller.workspace = other.str("secret"), wid
	if r := keyCaller.do("GET", "/v1/api-keys/"+key+"/requests", nil); r.Status != 403 {
		t.Fatalf("as an API key: %d %s", r.Status, r.Raw)
	}
	agent, agentEmail := e.signup("keylogagent")
	if _, err := e.d.DB.Pool.Exec(ctx, `INSERT INTO members (workspace_id, user_id, role) SELECT $1, id, 'agent' FROM users WHERE email = $2`, wid, agentEmail); err != nil {
		t.Fatal(err)
	}
	agent.workspace = wid
	if r := agent.do("GET", "/v1/api-keys/"+key+"/requests", nil); r.Status != 403 {
		t.Fatalf("as an agent: %d %s", r.Status, r.Raw)
	}
	// Another workspace's admin does not find the key.
	stranger, _ := e.signup("keylogstranger")
	stranger.newWorkspace("Elsewhere")
	if r := stranger.do("GET", "/v1/api-keys/"+key+"/requests", nil); r.Status != 404 {
		t.Fatalf("another workspace: %d %s", r.Status, r.Raw)
	}

	// Off again: the next request is not kept. A revoked key's record stays readable.
	if r := c.do("PATCH", "/v1/api-keys/"+key, map[string]any{"record_requests": false}); r.Status != 200 || r.Body["record_requests"] != false {
		t.Fatalf("turn off: %d %s", r.Status, r.Raw)
	}
	if call("GET", "/v1/tickets/1", nil); count() != 5 {
		t.Fatalf("recorded once off: %d", count())
	}
	if r := c.do("DELETE", "/v1/api-keys/"+key, nil); r.Status != 204 {
		t.Fatalf("revoke: %d", r.Status)
	}
	if got := list(""); len(got) != 5 {
		t.Fatalf("after revoking: %d", len(got))
	}

	// Kept 90 days.
	if _, err := e.d.DB.Pool.Exec(ctx, `UPDATE api_key_requests SET created_at = now() - interval '91 days'
		WHERE id = (SELECT id FROM api_key_requests WHERE api_key_id = $1 ORDER BY created_at LIMIT 1)`, key); err != nil {
		t.Fatal(err)
	}
	if err := (&cleanupWorker{s: e.s}).Work(ctx, nil); err != nil {
		t.Fatal(err)
	}
	if count() != 4 {
		t.Fatalf("after cleanup: %d", count())
	}
}

// A key made with "Record requests" on records from its first request.
func TestAPIKeyRequestLogOnCreate(t *testing.T) {
	e := newEnv(t)
	c, _ := e.signup("keylogcreate")
	c.newWorkspace("Key Log Create Co")
	r := c.do("POST", "/v1/api-keys", map[string]any{"name": "Claimy", "scopes": []string{"tickets:read"}, "record_requests": true})
	if r.Status != 201 || r.Body["record_requests"] != true {
		t.Fatalf("create: %d %s", r.Status, r.Raw)
	}
	if g := c.do("GET", "/v1/api-keys/"+r.str("id"), nil); g.Body["record_requests"] != true {
		t.Fatalf("get: %s", g.Raw)
	}
}
