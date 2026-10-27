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
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// Webhook is a webhook the fake holds.
type Webhook struct {
	ID     string
	URL    string
	Events []string
	Token  string
	Active bool
}

// Aircall is the fake API. Set RefuseAIEvents to answer 400 to a webhook asking for
// Conversation Intelligence events, and Limit to answer 429 to everything.
type Aircall struct {
	RefuseAIEvents bool
	Limit          bool

	mu       sync.Mutex
	srv      *httptest.Server
	id, tok  string
	webhooks map[string]*Webhook
	cards    map[string][]map[string]any
	requests []string
}

// New starts a fake answering to the API ID and token.
func New(t testing.TB, apiID, apiToken string) *Aircall {
	f := &Aircall{id: apiID, tok: apiToken, webhooks: map[string]*Webhook{}, cards: map[string][]map[string]any{}}
	f.srv = httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.srv.Close)
	return f
}

// URL is the API base, the Adapter's BaseURL.
func (f *Aircall) URL() string { return f.srv.URL + "/v1" }

// AddWebhook adds a webhook another integration made.
func (f *Aircall) AddWebhook(id, url, token string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.webhooks[id] = &Webhook{ID: id, URL: url, Token: token, Active: true}
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
	if id, tok, ok := r.BasicAuth(); !ok || id != f.id || tok != f.tok {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"Unauthorized"}`))
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/v1")
	switch {
	case r.Method == http.MethodGet && path == "/company":
		reply(w, 200, map[string]any{"company": map[string]any{"name": "LampTwist", "users_count": 8, "numbers_count": 10}})
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
		hook := &Webhook{ID: random(), URL: body.URL, Events: body.Events, Token: random(), Active: true}
		f.webhooks[hook.ID] = hook
		reply(w, 201, map[string]any{"webhook": webhookJSON(hook)})
	case strings.HasPrefix(path, "/webhooks/"):
		id := strings.TrimPrefix(path, "/webhooks/")
		hook, ok := f.webhooks[id]
		if !ok {
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
