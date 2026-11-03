package core

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/useopendesk/opendesk/internal/apilog"
	"github.com/useopendesk/opendesk/internal/auth"
	"github.com/useopendesk/opendesk/internal/collab"
	"github.com/useopendesk/opendesk/internal/domain"
	"github.com/useopendesk/opendesk/internal/helpdesk/tickets"
	"github.com/useopendesk/opendesk/internal/httpx"
	"github.com/useopendesk/opendesk/internal/ids"
)

func reviewLoggedEnv(t *testing.T) (*testEnv, *client, string, string) {
	t.Helper()
	e := newEnv(t)
	c, _ := e.signup("reviewlog")
	c.newWorkspace("Review logging")
	k := c.do("POST", "/v1/api-keys", map[string]any{"name": "Review key", "scopes": []string{"tickets:read", "tickets:write"}, "record_requests": true})
	if k.Status != 201 {
		t.Fatalf("create key: %d %s", k.Status, k.Raw)
	}
	router := chi.NewRouter()
	resolver := &auth.Resolver{Pool: e.d.DB.Pool, Tokens: e.d.Tokens, ClientIP: httpx.ClientIP(false, 0)}
	router.Use(httpx.RequestID, resolver.Middleware, apilog.Middleware(e.d.DB.Pool, e.d.Log))
	api := humachi.New(router, huma.DefaultConfig("review", "0"))
	Register(e.d, api, router)
	tickets.Register(e.d, api, router)
	collab.Register(e.d, api, router)
	srv := httptest.NewServer(router)
	e.srv = srv
	t.Cleanup(srv.Close)
	t.Cleanup(apilog.Wait)
	return e, c, k.str("id"), k.str("secret")
}
func TestReview327TruncatedSecret(t *testing.T) {
	e, c, key, secret := reviewLoggedEnv(t)
	caller := e.client()
	caller.token = secret
	body := map[string]any{"padding": strings.Repeat("p", apilog.MaxRequestBody-80), "password": "REVIEW_SECRET_MARKER" + strings.Repeat("s", 90)}
	r := caller.do("POST", "/v1/tickets", body)
	apilog.Wait()
	var kept *string
	if err := e.d.DB.Pool.QueryRow(context.Background(), `SELECT request_body FROM api_key_requests WHERE api_key_id=$1 ORDER BY created_at DESC LIMIT 1`, key).Scan(&kept); err != nil {
		t.Fatal(err)
	}
	t.Logf("real ticket POST status=%d; stored body bytes=%d; secret marker retained=%v", r.Status, lenValue(kept), containsValue(kept, "REVIEW_SECRET_MARKER"))
	rows := c.do("GET", "/v1/api-keys/"+key+"/requests", nil)
	id := rows.Body["data"].([]any)[0].(map[string]any)["id"].(string)
	detail := c.do("GET", "/v1/api-keys/"+key+"/requests/"+id, nil)
	if detail.Status != 200 {
		t.Fatalf("detail=%d %s", detail.Status, detail.Raw)
	}
	if containsValue(kept, "REVIEW_SECRET_MARKER") || strings.Contains(string(detail.Raw), "REVIEW_SECRET_MARKER") {
		t.Fatal("admin request detail retained the truncated password in plaintext")
	}
}
func TestReview327HiddenTicketLog(t *testing.T) {
	e, c, key, secret := reviewLoggedEnv(t)
	ctx := context.Background()
	wid := uuid.MustParse(c.workspace)
	team := ids.New()
	if _, err := e.d.DB.Pool.Exec(ctx, `INSERT INTO teams(id,workspace_id,name,private) VALUES($1,$2,'Private review',true)`, team, wid); err != nil {
		t.Fatal(err)
	}
	if _, err := e.d.DB.Pool.Exec(ctx, `UPDATE api_keys SET private_team_ids=ARRAY[$2::uuid] WHERE id=$1`, key, team); err != nil {
		t.Fatal(err)
	}
	var ticket domain.Ticket
	if err := e.d.DB.Tx(ctx, func(tx pgx.Tx) error {
		var err error
		ticket, err = domain.CreateTicket(ctx, tx, domain.NewTicket{WorkspaceID: wid, TeamID: &team, Subject: "Private incident", Channel: domain.ChannelEmail})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	path := "/v1/tickets/" + ticket.ID.String()
	if r := c.do("GET", path, nil); r.Status != 404 {
		t.Fatalf("owner should not see private ticket: %d", r.Status)
	}
	caller := e.client()
	caller.token = secret
	r := caller.do("POST", path+"/messages", map[string]any{"kind": "note", "body_html": "<p>REVIEW_PRIVATE_NOTE_MARKER</p>"})
	apilog.Wait()
	if r.Status != 201 {
		t.Fatalf("authorized key note=%d %s", r.Status, r.Raw)
	}
	rows := c.do("GET", "/v1/api-keys/"+key+"/requests", nil)
	if rows.Status != 200 {
		t.Fatalf("list=%d %s", rows.Status, rows.Raw)
	}
	if len(rows.Body["data"].([]any)) != 0 {
		t.Fatal("hidden ticket appears in request list")
	}
	var rid string
	if err := e.d.DB.Pool.QueryRow(ctx, `SELECT id::text FROM api_key_requests WHERE api_key_id=$1 AND method='POST' ORDER BY created_at DESC LIMIT 1`, key).Scan(&rid); err != nil {
		t.Fatal(err)
	}
	detail := c.do("GET", "/v1/api-keys/"+key+"/requests/"+rid, nil)
	t.Logf("private ticket GET=404; recorded note status=%d; admin request detail=%d; private note retained=%v", r.Status, detail.Status, strings.Contains(string(detail.Raw), "REVIEW_PRIVATE_NOTE_MARKER"))
	if detail.Status != 404 || strings.Contains(string(detail.Raw), "REVIEW_PRIVATE_NOTE_MARKER") {
		t.Fatal("admin outside private team can read its internal note through request log")
	}
	var uid uuid.UUID
	if err := e.d.DB.Pool.QueryRow(ctx, `SELECT created_by FROM api_keys WHERE id=$1`, key).Scan(&uid); err != nil {
		t.Fatal(err)
	}
	if _, err := e.d.DB.Pool.Exec(ctx, `INSERT INTO team_members(team_id,user_id) VALUES($1,$2)`, team, uid); err != nil {
		t.Fatal(err)
	}
	if got := c.do("GET", "/v1/api-keys/"+key+"/requests/"+rid, nil); got.Status != 200 || !strings.Contains(string(got.Raw), "REVIEW_PRIVATE_NOTE_MARKER") {
		t.Fatal("member cannot inspect shared ticket log")
	}
	if _, err := e.d.DB.Pool.Exec(ctx, `DELETE FROM team_members WHERE team_id=$1 AND user_id=$2`, team, uid); err != nil {
		t.Fatal(err)
	}
	if got := c.do("GET", "/v1/api-keys/"+key+"/requests/"+rid, nil); got.Status != 404 {
		t.Fatal("former member retains log access")
	}
	if _, err := e.d.DB.Pool.Exec(ctx, `UPDATE tickets SET assignee_id=$1 WHERE id=$2`, uid, ticket.ID); err != nil {
		t.Fatal(err)
	}
	if got := c.do("GET", "/v1/api-keys/"+key+"/requests/"+rid, nil); got.Status != 200 {
		t.Fatal("assignee cannot inspect ticket log")
	}
	if _, err := e.d.DB.Pool.Exec(ctx, `DELETE FROM tickets WHERE id=$1`, ticket.ID); err != nil {
		t.Fatal(err)
	}
	if got := c.do("GET", "/v1/api-keys/"+key+"/requests/"+rid, nil); got.Status != 404 {
		t.Fatal("deleted ticket content retained in log API")
	}
}

func TestReview327FailureSecret(t *testing.T) {
	e, c, key, secret := reviewLoggedEnv(t)
	caller := e.client()
	caller.token = secret
	r := caller.do("POST", "/v1/auth/login", map[string]any{"email": "review@example.com", "password": "REVIEW_FAILURE_SECRET_MARKER" + strings.Repeat("x", 1100)})
	apilog.Wait()
	if r.Status != 422 {
		t.Fatalf("expected validation response: %d %s", r.Status, r.Raw)
	}
	rows := c.do("GET", "/v1/api-keys/"+key+"/requests", nil)
	id := rows.Body["data"].([]any)[0].(map[string]any)["id"].(string)
	detail := c.do("GET", "/v1/api-keys/"+key+"/requests/"+id, nil)
	if detail.Status != 200 {
		t.Fatalf("detail=%d %s", detail.Status, detail.Raw)
	}
	t.Logf("failed login status=%d; masked request=%v; failed response retains password=%v", r.Status, !strings.Contains(string(detail.Raw), "REVIEW_FAILURE_SECRET_MARKER"), strings.Contains(detail.Body["response_body"].(string), "REVIEW_FAILURE_SECRET_MARKER"))
	if strings.Contains(detail.Body["response_body"].(string), "REVIEW_FAILURE_SECRET_MARKER") {
		t.Fatal("failure response stores password as errors[].value despite masked request")
	}
}

func lenValue(v *string) int {
	if v == nil {
		return 0
	}
	return len(*v)
}
func containsValue(v *string, s string) bool { return v != nil && strings.Contains(*v, s) }

func TestReview327VisibleTicketCapture(t *testing.T) {
	e, c, key, secret := reviewLoggedEnv(t)
	caller := e.client()
	caller.token = secret
	created := caller.do("POST", "/v1/tickets", map[string]any{"subject": "Safe recorded ticket"})
	if created.Status != 201 {
		t.Fatalf("ticket=%d %s", created.Status, created.Raw)
	}
	path := "/v1/tickets/" + created.str("id") + "/messages"
	body := map[string]any{"kind": "note", "body_html": "<p>Shared diagnostic note</p>"}
	if r := caller.do("POST", path, body); r.Status != 201 {
		t.Fatalf("note=%d %s", r.Status, r.Raw)
	}
	apilog.Wait()
	var rid, kept string
	if err := e.d.DB.Pool.QueryRow(context.Background(), `SELECT id::text,request_body FROM api_key_requests WHERE api_key_id=$1 AND path=$2 ORDER BY created_at DESC LIMIT 1`, key, path).Scan(&rid, &kept); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(kept, "Shared diagnostic note") || strings.Contains(kept, "credential-sentinel") {
		t.Fatal("safe content missing or credential retained")
	}
	if r := c.do("GET", "/v1/api-keys/"+key+"/requests/"+rid, nil); r.Status != 200 || !strings.Contains(string(r.Raw), "Shared diagnostic note") {
		t.Fatal("visible admin cannot inspect safe note")
	}
	body["password"] = "credential-sentinel"
	if r := caller.do("POST", path, body); r.Status != 422 {
		t.Fatalf("extra field status=%d", r.Status)
	}
	apilog.Wait()
	if err := e.d.DB.Pool.QueryRow(context.Background(), `SELECT request_body FROM api_key_requests WHERE api_key_id=$1 AND path=$2 ORDER BY created_at DESC LIMIT 1`, key, path).Scan(&kept); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(kept, "credential-sentinel") || !strings.Contains(kept, `"password":"***"`) {
		t.Fatal("named credential was not masked")
	}
	body["password"] = "CROSS_BOUNDARY_SENTINEL" + strings.Repeat("s", 90)
	body["padding"] = ""
	encoded, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	value := body["password"].(string)
	at := strings.Index(string(encoded), value)
	body["padding"] = strings.Repeat("p", apilog.MaxRequestBody-at-len(value)/2)
	caller.do("POST", path, body)
	apilog.Wait()
	if err := e.d.DB.Pool.QueryRow(context.Background(), `SELECT request_body FROM api_key_requests WHERE api_key_id=$1 AND path=$2 ORDER BY created_at DESC LIMIT 1`, key, path).Scan(&kept); err != nil {
		t.Fatal(err)
	}
	if kept != apilog.Masked {
		t.Fatal("incomplete body retained on live ticket")
	}
	t.Log("visible ticket content retained; named credential masked; incomplete live-ticket capture omitted")
}

func TestReview327PrivateRoutesMetadata(t *testing.T) {
	e, c, key, secret := reviewLoggedEnv(t)
	caller := e.client()
	caller.token = secret
	for _, path := range []string{"/v1/collab/drafts", "/v1/auth/password/reset", "/v1/auth/mfa", "/v1/me/password", "/v1/webhooks", "/v1/tickets/bulk", "/v1/tickets/PRIVATE_ROUTE_SENTINEL/messages"} {
		r := caller.do("POST", path+"?search=PRIVATE_QUERY_SENTINEL&token=CREDENTIAL_QUERY_SENTINEL", map[string]any{"body_html": "PRIVATE_BODY_SENTINEL", "password": "CREDENTIAL_BODY_SENTINEL", "mode": "PRIVATE_MODE_SENTINEL", "token": "CREDENTIAL_TOKEN_SENTINEL"})
		if r.Status < 400 {
			t.Fatalf("invalid request unexpectedly accepted: %s=%d", path, r.Status)
		}
		apilog.Wait()
		var rid string
		var body *string
		if err := e.d.DB.Pool.QueryRow(context.Background(), `SELECT id::text, request_body FROM api_key_requests WHERE api_key_id=$1 ORDER BY created_at DESC LIMIT 1`, key).Scan(&rid, &body); err != nil {
			t.Fatal(err)
		}
		if body != nil {
			t.Fatalf("private route retained request body: %s", path)
		}
		detail := c.do("GET", "/v1/api-keys/"+key+"/requests/"+rid, nil)
		if detail.Status != 200 {
			t.Fatalf("metadata not readable: %s=%d", path, detail.Status)
		}
		if strings.Contains(string(detail.Raw), "SENTINEL") {
			t.Fatalf("request data leaked through metadata or failure diagnostics: %s", path)
		}
	}
	t.Log("real Collab, password, MFA, webhook, bulk and unresolved-ticket failures retain safe diagnostics without private bodies, queries or path parameters")
}
