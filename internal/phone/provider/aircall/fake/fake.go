// Package fake is an in-memory Aircall Public API for tests: credential checks,
// webhooks (create, read, disable, delete), numbers and insight cards. Tests never call
// the real Aircall.
package fake

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// Webhook is a webhook the fake holds, in the account (API ID) that made it.
type Webhook struct {
	ID      string
	Account string
	URL     string
	Events  []string
	Token   string
	Active  bool
}

// Aircall is the fake API. Set RefuseAIEvents to answer 400 to a webhook asking for
// Conversation Intelligence events, and Limit to answer 429 to everything.
type Aircall struct {
	RefuseAIEvents      bool
	Limit               bool
	TranscriptionStatus int

	mu       sync.Mutex
	srv      *httptest.Server
	id       string
	accounts map[string]account // by API ID
	webhooks map[string]*Webhook
	cards    map[string][]map[string]any
	requests []string
}

type account struct{ token, company string }

// New starts a fake answering to the API ID and token, the company "LampTwist".
func New(t testing.TB, apiID, apiToken string) *Aircall {
	f := &Aircall{id: apiID, accounts: map[string]account{apiID: {apiToken, "LampTwist"}}, webhooks: map[string]*Webhook{},
		cards: map[string][]map[string]any{}}
	f.srv = httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.srv.Close)
	return f
}

// URL is the API base, the Adapter's BaseURL.
func (f *Aircall) URL() string { return f.srv.URL + "/v1" }

// AddAccount adds another Aircall company, with its own key and webhooks: a key of one
// company never sees the other's webhooks.
func (f *Aircall) AddAccount(apiID, apiToken, company string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.accounts[apiID] = account{apiToken, company}
}

// AddWebhook adds a webhook another integration made in the first account.
func (f *Aircall) AddWebhook(id, url, token string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.webhooks[id] = &Webhook{ID: id, Account: f.id, URL: url, Token: token, Active: true}
}

// Webhook returns a copy of a webhook, nil when there is none.
func (f *Aircall) Webhook(id string) *Webhook {
	f.mu.Lock()
	defer f.mu.Unlock()
	if w, ok := f.webhooks[id]; ok {
		c := *w
		return &c
	}
	return nil
}

// Webhooks returns copies of every webhook.
func (f *Aircall) Webhooks() []Webhook {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []Webhook
	for _, w := range f.webhooks {
		out = append(out, *w)
	}
	return out
}

// Disable turns a webhook off, as Aircall does after failed deliveries.
func (f *Aircall) Disable(id string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if w, ok := f.webhooks[id]; ok {
		w.Active = false
	}
}

// Cards returns the insight cards pushed to a call.
func (f *Aircall) Cards(callID string) []map[string]any {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]map[string]any{}, f.cards[callID]...)
}

// Requests returns "METHOD /path" of every request so far.
func (f *Aircall) Requests() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string{}, f.requests...)
}

func (f *Aircall) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.requests = append(f.requests, r.Method+" "+r.URL.Path)
	if f.Limit {
		w.Header().Set("X-AircallApi-Reset", strconv.FormatInt(time.Now().Add(30*time.Second).Unix(), 10))
		w.WriteHeader(http.StatusTooManyRequests)
		return
	}
	caller, tok, ok := r.BasicAuth()
	if acc, known := f.accounts[caller]; !ok || !known || tok != acc.token {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"Unauthorized"}`))
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/v1")
	switch {
	case r.Method == http.MethodGet && strings.HasPrefix(path, "/calls/") && strings.HasSuffix(path, "/transcription"):
		if f.TranscriptionStatus != 0 {
			w.WriteHeader(f.TranscriptionStatus)
			return
		}
		_, file, _, _ := runtime.Caller(0)
		raw, err := os.ReadFile(filepath.Join(filepath.Dir(file), "../testdata/transcription.json"))
		if err != nil {
			w.WriteHeader(500)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(raw)
	case r.Method == http.MethodGet && path == "/company":
		reply(w, 200, map[string]any{"company": map[string]any{"name": f.accounts[caller].company, "users_count": 8, "numbers_count": 10}})
	case r.Method == http.MethodGet && path == "/numbers":
		reply(w, 200, map[string]any{"numbers": []map[string]any{{"id": 501, "name": "FR - Support"}, {"id": 502, "name": "FR - B2B"}},
			"meta": map[string]any{"next_page_link": nil}})
	case r.Method == http.MethodPost && path == "/webhooks":
		var body struct {
			URL    string   `json:"url"`
			Events []string `json:"events"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		for _, e := range body.Events {
			if f.RefuseAIEvents && (e == "summary.created" || e == "transcription.created") {
				reply(w, 400, map[string]any{"error": "Invalid event"})
				return
			}
		}
		hook := &Webhook{ID: random(), Account: caller, URL: body.URL, Events: body.Events, Token: random(), Active: true}
		f.webhooks[hook.ID] = hook
		reply(w, 201, map[string]any{"webhook": webhookJSON(hook)})
	case strings.HasPrefix(path, "/webhooks/"):
		id := strings.TrimPrefix(path, "/webhooks/")
		hook, ok := f.webhooks[id]
		if !ok || hook.Account != caller {
			reply(w, 404, map[string]any{"error": "Not found"})
			return
		}
		switch r.Method {
		case http.MethodGet:
			reply(w, 200, map[string]any{"webhook": webhookJSON(hook)})
		case http.MethodDelete:
			delete(f.webhooks, id)
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	case r.Method == http.MethodPost && strings.HasPrefix(path, "/calls/") && strings.HasSuffix(path, "/insight_cards"):
		id := strings.TrimSuffix(strings.TrimPrefix(path, "/calls/"), "/insight_cards")
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			reply(w, 400, map[string]any{"error": "Bad request"})
			return
		}
		f.cards[id] = append(f.cards[id], body)
		w.WriteHeader(http.StatusCreated)
	default:
		reply(w, 404, map[string]any{"error": "Not found"})
	}
}

func webhookJSON(h *Webhook) map[string]any {
	return map[string]any{"webhook_id": h.ID, "direct_link": "https://api.aircall.io/v1/webhooks/" + h.ID, "custom_name": "opendesk",
		"url": h.URL, "active": h.Active, "token": h.Token, "events": h.Events}
}

func reply(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func random() string {
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
