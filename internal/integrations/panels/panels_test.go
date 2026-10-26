package panels

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/useopendesk/opendesk/internal/core"
	"github.com/useopendesk/opendesk/internal/crypto"
	"github.com/useopendesk/opendesk/internal/helpdesk"
	"github.com/useopendesk/opendesk/internal/helpdesk/hdtest"
)

func TestMain(m *testing.M) {
	if os.Getenv("TEST_DATABASE_URL") == "" {
		os.Setenv("TEST_DATABASE_URL", "postgres://opendesk:opendesk@localhost:55432/opendesk_panels?sslmode=disable")
	}
	hdtest.Main(m)
}

func apiEnv(t *testing.T) (*hdtest.Env, hdtest.WS) {
	t.Helper()
	e := hdtest.New(t, core.Register, helpdesk.Register, Register)
	return e, e.NewWorkspace()
}

// newTicket opens a ticket from contact (no contact when email is empty) and answers its id.
func newTicket(t *testing.T, e *hdtest.Env, a hdtest.Auth, email string, extra map[string]any) string {
	t.Helper()
	body := map[string]any{"subject": "Where is my order?", "body_html": "Hi"}
	if email != "" {
		body["contact"] = map[string]any{"email": email, "name": "Ada Buyer"}
	}
	for k, v := range extra {
		body[k] = v
	}
	var tk struct {
		ID string `json:"id"`
	}
	e.Do("POST", "/v1/tickets", a, body).Want(t, 201).JSON(t, &tk)
	return tk.ID
}

// readPanel reads a panel for a ticket into a fresh value.
func readPanel(t *testing.T, e *hdtest.Env, path string, a hdtest.Auth) Data {
	t.Helper()
	var d Data
	e.Do("GET", path, a, nil).Want(t, 200).JSON(t, &d)
	return d
}

// fakeShopify answers a shop's token endpoint (the client credentials grant, for the app
// "app-id") and its Admin GraphQL API, which knows one customer.
type fakeShopify struct {
	mu      sync.Mutex
	valid   map[string]bool // the Admin API tokens the shop accepts
	secret  string          // the app's client secret
	scope   string          // what the app's version grants
	issued  atomic.Int32
	lookups atomic.Int32
}

func newFakeShopify(tokens ...string) *fakeShopify {
	f := &fakeShopify{secret: "app-secret", scope: "read_customers,read_orders"}
	f.only(tokens...)
	return f
}

// only makes the shop accept these tokens alone, revoking the others.
func (f *fakeShopify) only(tokens ...string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.valid = map[string]bool{}
	for _, t := range tokens {
		f.valid[t] = true
	}
}

func (f *fakeShopify) set(secret, scope string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.secret, f.scope = secret, scope
}

func (f *fakeShopify) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	if r.URL.Path == "/admin/oauth/access_token" {
		_ = r.ParseForm()
		switch {
		case r.PostForm.Get("grant_type") != "client_credentials" || !strings.HasPrefix(r.Header.Get("Content-Type"), "application/x-www-form-urlencoded"):
			http.Error(w, `{"error":"invalid_request"}`, http.StatusBadRequest)
		case r.PostForm.Get("client_id") == "other-org":
			http.Error(w, `{"error":"shop_not_permitted","error_description":"Client credentials cannot be performed on this shop."}`, http.StatusBadRequest)
		case r.PostForm.Get("client_id") != "app-id" || r.PostForm.Get("client_secret") != f.secret:
			http.Error(w, `{"error":"invalid_client"}`, http.StatusBadRequest)
		default:
			tok := fmt.Sprintf("shpat_app_%d", f.issued.Add(1))
			f.valid[tok] = true
			_ = json.NewEncoder(w).Encode(map[string]any{"access_token": tok, "scope": f.scope, "expires_in": 86399})
		}
		return
	}
	if r.URL.Path != "/admin/api/"+shopifyVersion+"/graphql.json" {
		http.NotFound(w, r)
		return
	}
	if !f.valid[r.Header.Get("X-Shopify-Access-Token")] {
		http.Error(w, `{"errors":"[API] Invalid API key or access token"}`, http.StatusUnauthorized)
		return
	}
	var req struct {
		Query     string         `json:"query"`
		Variables map[string]any `json:"variables"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	if strings.Contains(req.Query, "shop {") {
		_, _ = io.WriteString(w, `{"data":{"shop":{"name":"Lumen Store"}}}`)
		return
	}
	f.lookups.Add(1)
	if req.Variables["q"] != `email:"ada@example.com"` {
		// A phrase search can still return a near match: it must not be taken.
		_, _ = io.WriteString(w, `{"data":{"customers":{"nodes":[{"id":"gid://shopify/Customer/9","displayName":"Ada Other",
			"defaultEmailAddress":{"emailAddress":"ada.other@example.com"},"numberOfOrders":"1","amountSpent":{"amount":"5.00","currencyCode":"EUR"},
			"tags":[],"orders":{"nodes":[]}}]}}}`)
		return
	}
	_, _ = io.WriteString(w, `{"data":{"customers":{"nodes":[{"id":"gid://shopify/Customer/42","displayName":"Ada Buyer",
		"defaultEmailAddress":{"emailAddress":"Ada@Example.com"},"numberOfOrders":"3","amountSpent":{"amount":"412.50","currencyCode":"EUR"},
		"tags":["VIP"],"orders":{"nodes":[{"id":"gid://shopify/Order/1001","name":"#1042","createdAt":"2026-10-20T09:00:00Z","cancelledAt":null,
		"displayFinancialStatus":"PAID","displayFulfillmentStatus":"FULFILLED",
		"currentTotalPriceSet":{"shopMoney":{"amount":"129.00","currencyCode":"EUR"}},
		"lineItems":{"nodes":[{"title":"Arco lamp","variantTitle":"Black","quantity":1,"originalUnitPriceSet":{"shopMoney":{"amount":"129.00","currencyCode":"EUR"}}}]},
		"fulfillments":[{"trackingInfo":[{"company":"DHL","number":"JD0123","url":"https://dhl.example/track/JD0123"},{"company":null,"number":null,"url":"javascript:alert(1)"}]}]}]}}]}}}`)
}

func TestShopifyPanel(t *testing.T) {
	e, ws := apiEnv(t)
	shop := newFakeShopify("shpat_good")
	srv := httptest.NewServer(shop)
	defer srv.Close()
	s := For(e.D)
	s.shopifyBase = func(string) string { return srv.URL }
	admin, agent, viewer := ws.Admin.As(ws), ws.Agent.As(ws), ws.Viewer.As(ws)

	body := map[string]any{"kind": "shopify", "shop": "https://Lumen-Store.myshopify.com/admin", "token": "shpat_good"}
	e.Do("POST", "/v1/ticket-panels", agent, body).Want(t, 403)
	e.Do("POST", "/v1/ticket-panels", admin, map[string]any{"kind": "shopify", "shop": "evil.example.com", "token": "x"}).Want(t, 422)
	e.Do("POST", "/v1/ticket-panels", admin, map[string]any{"kind": "shopify", "shop": "lumen-store", "token": "wrong"}).Want(t, 422)
	_, seq := e.Events(ws.ID, 0)
	var created struct {
		Panel  Panel  `json:"panel"`
		Secret string `json:"secret"`
	}
	e.Do("POST", "/v1/ticket-panels", admin, body).Want(t, 201).JSON(t, &created)
	p := created.Panel
	if p.Kind != "shopify" || p.Name != "Lumen Store" || p.Host != "lumen-store.myshopify.com" || p.Status != "connected" || created.Secret != "" ||
		p.Auth != "access_token" || p.ClientID != "" {
		t.Fatalf("panel = %+v, secret %q", p, created.Secret)
	}
	if types, _ := e.Events(ws.ID, seq); !slices.Contains(types, "ticket_panel.created") {
		t.Fatalf("events = %v", types)
	}
	var sealed string
	e.Scalar(`SELECT encode(secret_enc, 'escape') FROM ticket_panels WHERE id = $1`, &sealed, p.ID)
	if strings.Contains(sealed, "shpat_good") {
		t.Fatal("token stored in the clear")
	}
	var auditN int
	e.Scalar(`SELECT count(*) FROM audit_logs WHERE workspace_id = $1 AND action = 'ticket_panel.connected' AND NOT (data::text LIKE '%shpat%')`, &auditN, ws.ID)
	if auditN != 1 {
		t.Fatalf("audit entries = %d", auditN)
	}
	list := e.Do("GET", "/v1/ticket-panels", viewer, nil).Want(t, 200)
	if strings.Contains(string(list.Body), "shpat") {
		t.Fatalf("list leaks: %s", list.Body)
	}

	// The ticket's contact: the customer, the last order, tracking, links into the admin.
	tid := newTicket(t, e, agent, "ada@example.com", nil)
	path := "/v1/tickets/" + tid + "/panels/" + p.ID.String()
	var d Data
	d = readPanel(t, e, path, viewer)
	if d.State != "ok" || d.Customer == nil || d.Customer.Name != "Ada Buyer" || d.Customer.OrdersCount != 3 || d.Customer.TotalSpent != "412.50" ||
		d.Customer.URL != "https://admin.shopify.com/store/lumen-store/customers/42" || !slices.Equal(d.Customer.Tags, []string{"VIP"}) {
		t.Fatalf("customer = %+v (%s %s)", d.Customer, d.State, d.Message)
	}
	if len(d.Orders) != 1 {
		t.Fatalf("orders = %+v", d.Orders)
	}
	o := d.Orders[0]
	if o.Number != "#1042" || o.Total != "129.00" || o.Currency != "EUR" || o.FinancialStatus != "paid" || o.FulfillmentStatus != "fulfilled" ||
		o.URL != "https://admin.shopify.com/store/lumen-store/orders/1001" || len(o.Items) != 1 || o.Items[0].Variant != "Black" {
		t.Fatalf("order = %+v", o)
	}
	// A parcel whose only detail is a link that isn't https shows nothing, so it is left out.
	if len(o.Tracking) != 1 || o.Tracking[0].URL != "https://dhl.example/track/JD0123" || o.Tracking[0].Company != "DHL" {
		t.Fatalf("tracking = %+v", o.Tracking)
	}
	// The next open is answered from the cache; refresh asks again.
	n := shop.lookups.Load()
	e.Do("GET", path, viewer, nil).Want(t, 200)
	if shop.lookups.Load() != n {
		t.Fatal("second open was not cached")
	}
	e.Do("GET", path+"?refresh=true", viewer, nil).Want(t, 200)
	if shop.lookups.Load() != n+1 {
		t.Fatal("refresh did not ask again")
	}

	// Another contact the store doesn't know (a near match aside), and a ticket without one.
	d = readPanel(t, e, "/v1/tickets/"+newTicket(t, e, agent, "ada.o@example.com", nil)+"/panels/"+p.ID.String(), viewer)
	if d.State != "empty" || d.Customer != nil {
		t.Fatalf("unknown contact = %+v", d)
	}
	d = readPanel(t, e, "/v1/tickets/"+newTicket(t, e, agent, "", nil)+"/panels/"+p.ID.String(), viewer)
	if d.State != "no_contact" {
		t.Fatalf("no contact = %+v", d)
	}

	// A revoked token: a quiet state on the ticket, and the panel needs attention in settings.
	shop.only("shpat_new")
	d = readPanel(t, e, path+"?refresh=true", viewer)
	if d.State != "error" || !strings.Contains(d.Message, "access token") {
		t.Fatalf("revoked = %+v", d)
	}
	var status, lastErr string
	e.Scalar(`SELECT status || '|' || last_error FROM ticket_panels WHERE id = $1`, &status, p.ID)
	if status, lastErr, _ = strings.Cut(status, "|"); status != "error" || lastErr == "" {
		t.Fatalf("status = %q %q", status, lastErr)
	}
	// A new token is checked, then the panel is connected again.
	e.Do("PATCH", "/v1/ticket-panels/"+p.ID.String(), admin, map[string]any{"token": "still-wrong"}).Want(t, 422)
	upd := e.Do("PATCH", "/v1/ticket-panels/"+p.ID.String(), admin, map[string]any{"token": "shpat_new", "name": "Shop"}).Want(t, 200).Map(t)
	if upd["status"] != "connected" || upd["name"] != "Shop" {
		t.Fatalf("updated = %v", upd)
	}
	d = readPanel(t, e, path, viewer)
	if d.State != "ok" {
		t.Fatalf("after the new token = %+v", d)
	}

	// Turned off, it is not read; removed, it is gone.
	e.Do("PATCH", "/v1/ticket-panels/"+p.ID.String(), admin, map[string]any{"enabled": false}).Want(t, 200)
	e.Do("GET", path, viewer, nil).Want(t, 404)
	e.Do("DELETE", "/v1/ticket-panels/"+p.ID.String(), agent, nil).Want(t, 403)
	e.Do("DELETE", "/v1/ticket-panels/"+p.ID.String(), admin, nil).Want(t, 204)
	e.Do("GET", path, viewer, nil).Want(t, 404)
	e.Scalar(`SELECT count(*) FROM audit_logs WHERE workspace_id = $1 AND action = 'ticket_panel.disconnected'`, &auditN, ws.ID)
	if auditN != 1 {
		t.Fatalf("disconnect audit = %d", auditN)
	}
}

// TestShopifyAppPanel: an app made in Shopify's Dev Dashboard connects with its client ID
// and secret; OpenDesk asks the shop for a token, keeps it until shortly before its end
// and asks again when the shop revokes it.
func TestShopifyAppPanel(t *testing.T) {
	e, ws := apiEnv(t)
	shop := newFakeShopify()
	srv := httptest.NewServer(shop)
	defer srv.Close()
	s := For(e.D)
	s.shopifyBase = func(string) string { return srv.URL }
	admin, viewer := ws.Admin.As(ws), ws.Viewer.As(ws)
	app := func(extra map[string]any) map[string]any {
		b := map[string]any{"kind": "shopify", "shop": "lumen-store", "client_id": "app-id", "client_secret": "app-secret"}
		for k, v := range extra {
			b[k] = v
		}
		return b
	}
	refused := func(body map[string]any, want string) {
		t.Helper()
		if r := e.Do("POST", "/v1/ticket-panels", admin, body).Want(t, 422); !strings.Contains(string(r.Body), want) {
			t.Fatalf("refusal %s: %s", want, r.Body)
		}
	}

	// What the shop would refuse is caught before saving.
	refused(app(map[string]any{"client_secret": "wrong"}), "client ID or secret")
	refused(app(map[string]any{"client_id": "other-org"}), "organization")
	refused(app(map[string]any{"client_secret": ""}), "client ID and client secret")
	refused(app(map[string]any{"token": "shpat_x"}), "not both")
	shop.set("app-secret", "read_customers")
	refused(app(nil), "read_orders")
	shop.set("app-secret", "read_customers,write_orders") // a write scope includes its read one
	var created struct {
		Panel Panel `json:"panel"`
	}
	e.Do("POST", "/v1/ticket-panels", admin, app(nil)).Want(t, 201).JSON(t, &created)
	p := created.Panel
	if p.Auth != "client_credentials" || p.ClientID != "app-id" || p.Status != "connected" || p.Name != "Lumen Store" {
		t.Fatalf("panel = %+v", p)
	}
	var sealed string
	e.Scalar(`SELECT encode(secret_enc, 'escape') FROM ticket_panels WHERE id = $1`, &sealed, p.ID)
	if strings.Contains(sealed, "app-secret") {
		t.Fatal("client secret stored in the clear")
	}
	if list := string(e.Do("GET", "/v1/ticket-panels", viewer, nil).Want(t, 200).Body); strings.Contains(list, "app-secret") || strings.Contains(list, "shpat_") {
		t.Fatalf("list leaks: %s", list)
	}

	// One token serves the lookups until shortly before its end.
	path := "/v1/tickets/" + newTicket(t, e, ws.Agent.As(ws), "ada@example.com", nil) + "/panels/" + p.ID.String()
	issued := shop.issued.Load()
	if d := readPanel(t, e, path, viewer); d.State != "ok" || d.Customer == nil || d.Customer.Name != "Ada Buyer" {
		t.Fatalf("data = %+v", d)
	}
	readPanel(t, e, path+"?refresh=true", viewer)
	if n := shop.issued.Load(); n != issued+1 {
		t.Fatalf("tokens asked for: %d", n-issued)
	}
	s.tokens.now = func() time.Time { return time.Now().Add(24*time.Hour - time.Minute) }
	if d := readPanel(t, e, path+"?refresh=true", viewer); d.State != "ok" || shop.issued.Load() != issued+2 {
		t.Fatalf("near its end: %+v, %d tokens", d, shop.issued.Load()-issued)
	}

	// A token revoked before its end (the app reinstalled): once more with a new one.
	shop.only()
	if d := readPanel(t, e, path+"?refresh=true", viewer); d.State != "ok" || shop.issued.Load() != issued+3 {
		t.Fatalf("revoked: %+v, %d tokens", d, shop.issued.Load()-issued)
	}

	// A rotated secret: the panel needs attention until it is given the new one, which keeps
	// the client ID.
	shop.set("app-secret-2", "read_customers,read_orders")
	shop.only()
	if d := readPanel(t, e, path+"?refresh=true", viewer); d.State != "error" || !strings.Contains(d.Message, "client ID or secret") {
		t.Fatalf("rotated = %+v", d)
	}
	var status string
	e.Scalar(`SELECT status FROM ticket_panels WHERE id = $1`, &status, p.ID)
	if status != "error" {
		t.Fatalf("status = %q", status)
	}
	id := "/v1/ticket-panels/" + p.ID.String()
	e.Do("PATCH", id, admin, map[string]any{"client_secret": "app-secret"}).Want(t, 422)
	upd := e.Do("PATCH", id, admin, map[string]any{"client_secret": "app-secret-2"}).Want(t, 200).Map(t)
	if upd["status"] != "connected" || upd["client_id"] != "app-id" {
		t.Fatalf("new secret = %v", upd)
	}
	if d := readPanel(t, e, path, viewer); d.State != "ok" {
		t.Fatalf("after the new secret = %+v", d)
	}

	// A pasted token in place of the app, and back: a secret alone needs a client ID.
	shop.only("shpat_old")
	if upd = e.Do("PATCH", id, admin, map[string]any{"token": "shpat_old"}).Want(t, 200).Map(t); upd["auth"] != "access_token" || upd["client_id"] != nil {
		t.Fatalf("token = %v", upd)
	}
	if d := readPanel(t, e, path, viewer); d.State != "ok" {
		t.Fatalf("with the token = %+v", d)
	}
	e.Do("PATCH", id, admin, map[string]any{"client_secret": "app-secret-2"}).Want(t, 422)
	e.Do("PATCH", id, admin, map[string]any{"client_id": "app-id", "client_secret": "app-secret-2"}).Want(t, 200)
	var leaks int
	e.Scalar(`SELECT count(*) FROM audit_logs WHERE workspace_id = $1 AND (data::text LIKE '%app-secret%' OR data::text LIKE '%shpat%')`, &leaks, ws.ID)
	if leaks != 0 {
		t.Fatalf("audit leaks credentials: %d", leaks)
	}
}

func TestPanelFollowsTicketVisibility(t *testing.T) {
	e, ws := apiEnv(t)
	shop := newFakeShopify("shpat_good")
	srv := httptest.NewServer(shop)
	defer srv.Close()
	For(e.D).shopifyBase = func(string) string { return srv.URL }
	var created struct {
		Panel Panel `json:"panel"`
	}
	e.Do("POST", "/v1/ticket-panels", ws.Admin.As(ws), map[string]any{"kind": "shopify", "shop": "lumen-store", "token": "shpat_good"}).Want(t, 201).JSON(t, &created)
	lead := e.AddUser(ws, "agent", "Lena Lead")
	var team string
	e.Scalar(`INSERT INTO teams (id, workspace_id, name, private) VALUES (gen_random_uuid(), $1, 'Security', true) RETURNING id::text`, &team, ws.ID)
	e.Exec(`INSERT INTO team_members (team_id, user_id, role) VALUES ($1::uuid, $2, 'lead')`, team, lead.ID)
	tid := newTicket(t, e, lead.As(ws), "ada@example.com", map[string]any{"team_id": team})
	path := "/v1/tickets/" + tid + "/panels/" + created.Panel.ID.String()
	n := shop.lookups.Load()
	e.Do("GET", path, ws.Agent.As(ws), nil).Want(t, 404)
	if shop.lookups.Load() != n {
		t.Fatal("a hidden ticket's contact was looked up")
	}
	e.Do("GET", path, lead.As(ws), nil).Want(t, 200)
	// Another workspace's panel is not found.
	other := e.NewWorkspace()
	e.Do("GET", "/v1/tickets/"+newTicket(t, e, other.Agent.As(other), "ada@example.com", nil)+"/panels/"+created.Panel.ID.String(),
		other.Agent.As(other), nil).Want(t, 404)
}

func TestHTTPPanel(t *testing.T) {
	e, ws := apiEnv(t)
	var secret atomic.Value
	secret.Store("")
	var got atomic.Value
	mux := http.NewServeMux()
	mux.HandleFunc("/panel", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		sig := r.Header.Get("OD-Signature")
		ts, mac, _ := strings.Cut(strings.TrimPrefix(sig, "t="), ",v1=")
		if !crypto.EqualHex(mac, crypto.HMACHex([]byte(secret.Load().(string)), ts+"."+string(body))) {
			http.Error(w, "bad signature", http.StatusUnauthorized)
			return
		}
		got.Store(string(body))
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"sections":[{"title":"Odoo\norder","items":[
			{"label":"Confirmed delivery","value":"2026-11-04","url":"https://erp.example/so/42"},
			{"label":"Lines","value":3},{"label":"Paid","value":true},
			{"label":"Script","value":"x","url":"javascript:alert(1)"},{"label":"","value":""}]},
			{"title":"Empty","items":[]}]}`)
	})
	mux.HandleFunc("/gone", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNotFound) })
	mux.HandleFunc("/huge", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"sections":"`+strings.Repeat("x", 70<<10)+`"}`)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	admin, viewer := ws.Admin.As(ws), ws.Viewer.As(ws)

	e.Do("POST", "/v1/ticket-panels", admin, map[string]any{"kind": "http", "url": "ftp://erp.example/panel"}).Want(t, 422)
	var created struct {
		Panel  Panel  `json:"panel"`
		Secret string `json:"secret"`
	}
	resp := e.Do("POST", "/v1/ticket-panels", admin, map[string]any{"kind": "http", "url": srv.URL + "/panel?key=s3cret", "name": "Odoo"}).Want(t, 201)
	if resp.Header.Get("Cache-Control") != "no-store" {
		t.Fatal("the secret's answer is cacheable")
	}
	resp.JSON(t, &created)
	if !strings.HasPrefix(created.Secret, "pnsec_") || created.Panel.Name != "Odoo" || strings.Contains(created.Panel.Host, "s3cret") {
		t.Fatalf("created = %+v", created)
	}
	if strings.Contains(string(e.Do("GET", "/v1/ticket-panels", viewer, nil).Want(t, 200).Body), "s3cret") {
		t.Fatal("the URL's query is listed")
	}
	secret.Store(created.Secret)

	tid := newTicket(t, e, ws.Agent.As(ws), "ada@example.com", nil)
	path := "/v1/tickets/" + tid + "/panels/" + created.Panel.ID.String()
	var d Data
	d = readPanel(t, e, path, viewer)
	if d.State != "ok" || len(d.Sections) != 1 || d.Sections[0].Title != "Odoo order" {
		t.Fatalf("data = %+v", d)
	}
	items := d.Sections[0].Items
	if len(items) != 4 || items[0].URL != "https://erp.example/so/42" || items[1].Value != "3" || items[2].Value != "true" || items[3].URL != "" {
		t.Fatalf("items = %+v", items)
	}
	var sent lookupRequest
	_ = json.Unmarshal([]byte(got.Load().(string)), &sent)
	if sent.Contact.Email != "ada@example.com" || sent.Ticket.ID.String() != tid || sent.WorkspaceID != ws.ID || sent.PanelID != created.Panel.ID {
		t.Fatalf("request = %+v", sent)
	}

	// A new secret: the old one is refused until the URL knows the new one.
	var rotated struct {
		Secret string `json:"secret"`
	}
	e.Do("POST", "/v1/ticket-panels/"+created.Panel.ID.String()+"/rotate-secret", admin, nil).Want(t, 200).JSON(t, &rotated)
	if rotated.Secret == "" || rotated.Secret == created.Secret {
		t.Fatalf("rotated = %q", rotated.Secret)
	}
	d = readPanel(t, e, path, viewer)
	if d.State != "error" || !strings.Contains(d.Message, "signing secret") {
		t.Fatalf("old secret = %+v", d)
	}
	secret.Store(rotated.Secret)
	d = readPanel(t, e, path+"?refresh=1", viewer)
	if d.State != "ok" {
		t.Fatalf("new secret = %+v", d)
	}

	// Nothing for this contact, and an answer too large.
	e.Do("PATCH", "/v1/ticket-panels/"+created.Panel.ID.String(), admin, map[string]any{"url": srv.URL + "/gone"}).Want(t, 200)
	d = readPanel(t, e, path, viewer)
	if d.State != "empty" {
		t.Fatalf("404 = %+v", d)
	}
	e.Do("PATCH", "/v1/ticket-panels/"+created.Panel.ID.String(), admin, map[string]any{"url": srv.URL + "/huge"}).Want(t, 200)
	d = readPanel(t, e, path, viewer)
	if d.State != "error" || !strings.Contains(d.Message, "64 KB") {
		t.Fatalf("huge = %+v", d)
	}
}

func TestPanelLimitAndKinds(t *testing.T) {
	e, ws := apiEnv(t)
	admin := ws.Admin.As(ws)
	e.Do("POST", "/v1/ticket-panels", admin, map[string]any{"kind": "zendesk"}).Want(t, 422)
	srv := httptest.NewServer(http.NotFoundHandler())
	defer srv.Close()
	for i := range maxPerWorkspace {
		e.Do("POST", "/v1/ticket-panels", admin, map[string]any{"kind": "http", "url": srv.URL + "/p" + string(rune('a'+i))}).Want(t, 201)
	}
	e.Do("POST", "/v1/ticket-panels", admin, map[string]any{"kind": "http", "url": srv.URL + "/more"}).Want(t, 409)
	key := e.APIKey(ws, "settings:write", "tickets:read")
	e.Do("POST", "/v1/ticket-panels", hdtest.Auth{Bearer: key}, map[string]any{"kind": "http", "url": srv.URL + "/key"}).Want(t, 403)
	e.Do("GET", "/v1/ticket-panels", hdtest.Auth{Bearer: key}, nil).Want(t, 200)
}

func TestNormalizeShop(t *testing.T) {
	for in, want := range map[string]string{
		"lumen-store":               "lumen-store.myshopify.com",
		"Lumen-Store.myshopify.com": "lumen-store.myshopify.com",
		"https://lumen-store.myshopify.com/admin/orders": "lumen-store.myshopify.com",
	} {
		if got, err := normalizeShop(in); err != nil || got != want {
			t.Errorf("normalizeShop(%q) = %q, %v", in, got, err)
		}
	}
	for _, in := range []string{"", "evil.example.com", "a.myshopify.com.evil.example", "lumen store"} {
		if _, err := normalizeShop(in); err == nil {
			t.Errorf("normalizeShop(%q) accepted", in)
		}
	}
	if q := searchPhrase(`a"b\c@x.com`); q != `email:"a\"b\\c@x.com"` {
		t.Errorf("searchPhrase = %s", q)
	}
}

func TestShopifyCredentialsAndScopes(t *testing.T) {
	if c, err := shopifyCredentials(" app-id ", "secret\n", ""); err != nil || c.clientID != "app-id" || c.secret != "secret" {
		t.Errorf("app = %+v, %v", c, err)
	}
	if c, err := shopifyCredentials("", "", " shpat_x "); err != nil || c.clientID != "" || c.secret != "shpat_x" {
		t.Errorf("token = %+v, %v", c, err)
	}
	for _, in := range [][3]string{{"", "", ""}, {"app-id", "", ""}, {"", "secret", ""}, {"app id", "secret", ""}, {"app-id", "s", "shpat_x"}, {"", "", "shpat x"}} {
		if _, err := shopifyCredentials(in[0], in[1], in[2]); err == nil {
			t.Errorf("shopifyCredentials(%q) accepted", in)
		}
	}
	for scope, want := range map[string]string{
		"read_customers,read_orders":                   "",
		"write_customers, read_all_orders,read_orders": "",
		"read_customers":                               "read_orders",
		"":                                             "read_customers read_orders",
	} {
		if got := strings.Join(missingScopes(scope), " "); got != want {
			t.Errorf("missingScopes(%q) = %q", scope, got)
		}
	}
	if err := tokenRefusal(400, []byte(`<p>Oauth error shop_not_permitted: Client credentials cannot be performed on this shop.</p>`)); !strings.Contains(err.Error(), "organization") {
		t.Errorf("shop_not_permitted = %v", err)
	}
	if err := tokenRefusal(503, nil); err.Error() != "Shopify answered 503." {
		t.Errorf("503 = %v", err)
	}
}
