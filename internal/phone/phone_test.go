package phone_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/useopendesk/opendesk/internal/deps"
	"github.com/useopendesk/opendesk/internal/domain"
	"github.com/useopendesk/opendesk/internal/helpdesk/hdtest"
	"github.com/useopendesk/opendesk/internal/helpdesk/resources"
	"github.com/useopendesk/opendesk/internal/helpdesk/tickets"
	"github.com/useopendesk/opendesk/internal/jobs"
	"github.com/useopendesk/opendesk/internal/phone"
	"github.com/useopendesk/opendesk/internal/phone/provider/aircall"
	"github.com/useopendesk/opendesk/internal/phone/provider/aircall/fake"
)

func TestMain(m *testing.M) { hdtest.Main(m) }

var creds = map[string]string{"api_id": "id-1", "api_token": "token-1"}

type harness struct {
	e    *hdtest.Env
	ws   hdtest.WS
	s    *phone.Service
	f    *fake.Aircall
	conn phone.PhoneConnectionView
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	f := fake.New(t, creds["api_id"], creds["api_token"])
	var s *phone.Service
	e := hdtest.New(t, tickets.Register, resources.Register, func(d *deps.Deps, api huma.API, r chi.Router) {
		s = phone.New(d, aircall.Adapter{BaseURL: f.URL()})
		s.Register(api, r)
	})
	ws := e.NewWorkspace()
	// Open 09:00 to 17:00 in Brussels on weekdays.
	day := []map[string]string{{"start": "09:00", "end": "17:00"}}
	e.SetSettings(ws, map[string]any{"timezone": "Europe/Brussels", "business_hours": map[string]any{"enabled": true, "holidays": []any{},
		"schedule": map[string]any{"mon": day, "tue": day, "wed": day, "thu": day, "fri": day, "sat": []any{}, "sun": []any{}}}})
	h := &harness{e: e, ws: ws, s: s, f: f}
	e.Do("POST", "/v1/phone/connections", ws.Admin.As(ws), map[string]any{"provider": "aircall", "credentials": creds}).
		Want(t, 201).JSON(t, &h.conn)
	return h
}

// token is the secret Aircall puts in our webhook's events.
func (h *harness) token(t *testing.T) string {
	t.Helper()
	for _, w := range h.f.Webhooks() {
		if strings.Contains(w.URL, h.conn.ID.String()) {
			return w.Token
		}
	}
	t.Fatal("no webhook registered for the connection")
	return ""
}

// recorded reads a recorded lifecycle, signed with our token, with the agent's email
// set to the workspace agent's.
func (h *harness) recorded(t *testing.T, name string) [][]byte {
	t.Helper()
	raw, err := os.ReadFile("provider/aircall/testdata/" + name + ".json")
	if err != nil {
		t.Fatal(err)
	}
	var list []json.RawMessage
	if err := json.Unmarshal(raw, &list); err != nil {
		t.Fatal(err)
	}
	tok := h.token(t)
	out := make([][]byte, len(list))
	for i, m := range list {
		s := strings.ReplaceAll(string(m), `"hook-token-1"`, `"`+tok+`"`)
		out[i] = []byte(strings.ReplaceAll(s, "anna@example.com", h.ws.Agent.Email))
	}
	return out
}

func (h *harness) send(t *testing.T, body []byte) int {
	t.Helper()
	res, err := http.Post(h.e.Srv.URL+"/v1/phone/hooks/aircall/"+h.conn.ID.String(), "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	return res.StatusCode
}

// drain runs the queued phone events, insight cards and ticket triggers until none are left.
func (h *harness) drain(t *testing.T) {
	t.Helper()
	for i := 0; i < 20; i++ {
		n := 0
		for _, raw := range h.e.Jobs(jobs.PhoneInboundArgs{}.Kind(), h.ws.ID) {
			var a jobs.PhoneInboundArgs
			_ = json.Unmarshal(raw, &a)
			if err := h.s.ProcessInbound(h.e.Ctx, a); err != nil {
				t.Fatalf("inbound: %v", err)
			}
			n++
		}
		for _, raw := range h.e.Jobs(jobs.PhoneInsightCardArgs{}.Kind(), h.ws.ID) {
			var a jobs.PhoneInsightCardArgs
			_ = json.Unmarshal(raw, &a)
			if err := h.s.PushCard(h.e.Ctx, a); err != nil {
				t.Fatalf("card: %v", err)
			}
			n++
		}
		for _, raw := range h.e.Jobs(jobs.TicketTriggerArgs{}.Kind(), h.ws.ID) {
			var a jobs.TicketTriggerArgs
			_ = json.Unmarshal(raw, &a)
			if err := tickets.RunTrigger(h.e.Ctx, h.e.D, a); err != nil {
				t.Fatalf("trigger %s: %v", a.Trigger, err)
			}
			n++
		}
		if n == 0 {
			return
		}
	}
	t.Fatal("jobs did not settle")
}

func (h *harness) sendAll(t *testing.T, bodies [][]byte) {
	t.Helper()
	for _, b := range bodies {
		if got := h.send(t, b); got != 200 {
			t.Fatalf("webhook answered %d", got)
		}
		h.drain(t)
	}
}

func (h *harness) callEntries(t *testing.T, ticketID uuid.UUID) []domain.Activity {
	t.Helper()
	var out struct {
		Data []domain.Activity `json:"data"`
	}
	h.e.Do("GET", "/v1/tickets/"+ticketID.String()+"/activities", h.ws.Agent.As(h.ws), nil).Want(t, 200).JSON(t, &out)
	var calls []domain.Activity
	for _, a := range out.Data {
		if a.Type == phone.ActivityCall {
			calls = append(calls, a)
		}
	}
	return calls
}

func (h *harness) ticketOfCall(t *testing.T, external string) domain.Ticket {
	t.Helper()
	var tid uuid.UUID
	h.e.Scalar(`SELECT ticket_id FROM phone_calls WHERE workspace_id = $1 AND external_call_id = $2`, &tid, h.ws.ID, external)
	tk, err := domain.LoadTicket(h.e.Ctx, h.e.D.DB.Pool, h.ws.ID, tid)
	if err != nil {
		t.Fatal(err)
	}
	return tk
}

func TestConnectManagesOnlyItsOwnWebhook(t *testing.T) {
	f := fake.New(t, creds["api_id"], creds["api_token"])
	f.AddWebhook("gorgias-1", "https://gorgias.example.com/aircall", "theirs")
	var s *phone.Service
	e := hdtest.New(t, func(d *deps.Deps, api huma.API, r chi.Router) {
		s = phone.New(d, aircall.Adapter{BaseURL: f.URL()})
		s.Register(api, r)
	})
	_ = s
	ws := e.NewWorkspace()
	body := map[string]any{"provider": "aircall", "credentials": creds}

	e.Do("POST", "/v1/phone/connections", ws.Agent.As(ws), body).Want(t, 403)
	e.Do("POST", "/v1/phone/connections", ws.Admin.As(ws), map[string]any{"provider": "aircall",
		"credentials": map[string]string{"api_id": "id-1", "api_token": "wrong"}}).Want(t, 422)

	var c phone.PhoneConnectionView
	r := e.Do("POST", "/v1/phone/connections", ws.Admin.As(ws), body).Want(t, 201)
	r.JSON(t, &c)
	if c.Name != "LampTwist" || c.HealthStatus != "healthy" || c.HealthCode != phone.HealthReceiving || c.ReuseDays != 7 || !c.InsightCards {
		t.Fatalf("connection %+v", c)
	}
	if strings.Contains(string(r.Body), "token-1") || !strings.HasSuffix(c.WebhookURL, "/v1/phone/hooks/aircall/"+c.ID.String()) {
		t.Fatalf("credentials or webhook URL: %s", r.Body)
	}
	e.Do("POST", "/v1/phone/connections", ws.Admin.As(ws), body).Want(t, 409)
	var list struct {
		Data []phone.PhoneConnectionView `json:"data"`
	}
	e.Do("GET", "/v1/phone/connections", ws.Viewer.As(ws), nil).Want(t, 200).JSON(t, &list)
	if len(list.Data) != 1 {
		t.Fatalf("list %+v", list)
	}
	ours := func() []fake.Webhook {
		var out []fake.Webhook
		for _, w := range f.Webhooks() {
			if w.ID != "gorgias-1" {
				out = append(out, w)
			}
		}
		return out
	}
	if len(ours()) != 1 || f.Webhook("gorgias-1") == nil {
		t.Fatalf("webhooks %+v", f.Webhooks())
	}
	first := ours()[0].ID

	// Aircall disabled ours after failed deliveries: a test registers a new one.
	f.Disable(first)
	e.Do("POST", "/v1/phone/connections/"+c.ID.String()+"/test", ws.Admin.As(ws), nil).Want(t, 200).JSON(t, &c)
	if now := ours(); c.HealthStatus != "healthy" || len(now) != 1 || now[0].ID == first || !now[0].Active {
		t.Fatalf("repair %+v %+v", c, now)
	}
	var lines struct {
		Data []phone.PhoneLine `json:"data"`
	}
	e.Do("GET", "/v1/phone/lines", ws.Admin.As(ws), nil).Want(t, 200).JSON(t, &lines)
	if len(lines.Data) != 2 || lines.Data[0].Name != "FR - B2B" {
		t.Fatalf("lines %+v", lines)
	}

	e.Do("PATCH", "/v1/phone/connections/"+c.ID.String(), ws.Admin.As(ws), map[string]any{"reuse_days": 0, "assign_answered": true}).
		Want(t, 200).JSON(t, &c)
	if c.ReuseDays != 0 || !c.AssignAnswered {
		t.Fatalf("patch %+v", c)
	}

	e.Do("DELETE", "/v1/phone/connections/"+c.ID.String(), ws.Admin.As(ws), nil).Want(t, 204)
	if len(ours()) != 0 || f.Webhook("gorgias-1") == nil {
		t.Fatalf("after disconnect %+v", f.Webhooks())
	}
	for _, req := range f.Requests() {
		if req == "GET /v1/webhooks" || req == "DELETE /v1/webhooks/gorgias-1" {
			t.Fatalf("touched webhooks that are not ours: %s", req)
		}
	}
	var audits int
	e.Scalar(`SELECT count(*) FROM audit_logs WHERE workspace_id = $1 AND action IN ('phone.connected', 'phone.disconnected')`, &audits, ws.ID)
	if audits != 2 {
		t.Fatalf("audit entries %d", audits)
	}
}

func TestWebhookVerification(t *testing.T) {
	h := newHarness(t)
	body := h.recorded(t, "answered")[0]
	if got := h.send(t, bytes.ReplaceAll(body, []byte(h.token(t)), []byte("forged"))); got != 401 {
		t.Fatalf("forged token: %d", got)
	}
	res, _ := http.Post(h.e.Srv.URL+"/v1/phone/hooks/aircall/"+uuid.NewString(), "application/json", bytes.NewReader(body))
	if res.StatusCode != 404 {
		t.Fatalf("unknown connection: %d", res.StatusCode)
	}
	ignored := []byte(`{"resource":"user","event":"user.opened","timestamp":1,"token":"` + h.token(t) + `","data":{"id":1}}`)
	if got := h.send(t, ignored); got != 200 {
		t.Fatalf("ignored event: %d", got)
	}
	if n := len(h.e.Jobs(jobs.PhoneInboundArgs{}.Kind(), h.ws.ID)); n != 0 {
		t.Fatalf("an ignored or forged event was queued: %d", n)
	}
	// A retry of the same event is stored and queued once.
	h.send(t, body)
	h.send(t, body)
	if n := len(h.e.Jobs(jobs.PhoneInboundArgs{}.Kind(), h.ws.ID)); n != 1 {
		t.Fatalf("retries queued %d jobs", n)
	}
}

func TestAnsweredCallIsOneEntryFromRingingToSummary(t *testing.T) {
	h := newHarness(t)
	h.e.Do("PATCH", "/v1/phone/connections/"+h.conn.ID.String(), h.ws.Admin.As(h.ws), map[string]any{"assign_answered": true}).Want(t, 200)
	_, seq := h.e.Events(h.ws.ID, 0)
	bodies := h.recorded(t, "answered")
	h.sendAll(t, bodies)

	tk := h.ticketOfCall(t, "1290001")
	if tk.Channel != domain.ChannelPhone || tk.Subject != "Call from +32470123456" || tk.Contact == nil {
		t.Fatalf("ticket %+v", tk)
	}
	contact, err := domain.LoadContact(h.e.Ctx, h.e.D.DB.Pool, h.ws.ID, tk.Contact.ID)
	if err != nil || contact.Phone != "+32470123456" {
		t.Fatalf("contact %+v %v", contact, err)
	}
	if tk.Assignee == nil || tk.Assignee.ID != h.ws.Agent.ID {
		t.Fatalf("an answered call assigns its ticket to who took it: %+v", tk.Assignee)
	}
	entries := h.callEntries(t, tk.ID)
	if len(entries) != 1 {
		t.Fatalf("%d call entries", len(entries))
	}
	d := entries[0].Data
	agent, _ := d["agent"].(map[string]any)
	if d["outcome"] != "answered" || d["stage"] != "ended" || d["direction"] != "inbound" || d["duration"] != float64(190) ||
		agent["id"] != h.ws.Agent.ID.String() || d["line"] != "FR - Support" || d["outside_hours"] != false {
		t.Fatalf("entry %+v", d)
	}
	if d["recording_url"] != "https://short.aircall.io/r/AbC" || d["asset_url"] != "https://assets.aircall.io/calls/1290001/recording" ||
		d["link"] != "https://api.aircall.io/v1/calls/1290001" || d["links_expire_at"] == nil {
		t.Fatalf("links %+v", d)
	}
	if tags, _ := d["tags"].([]any); len(tags) != 1 || tags[0] != "Order" {
		t.Fatalf("tags %+v", d["tags"])
	}
	if comments, _ := d["comments"].([]any); len(comments) != 1 {
		t.Fatalf("comments %+v", d["comments"])
	}
	types, _ := h.e.Events(h.ws.ID, seq)
	if !slices.Contains(types, "activity.created") || !slices.Contains(types, "activity.updated") || !slices.Contains(types, "contact.created") {
		t.Fatalf("events %v", types)
	}
	// The ringing call showed the caller in Aircall.
	if cards := h.f.Cards("1290001"); len(cards) != 1 {
		t.Fatalf("insight cards %+v", cards)
	} else if title := cards[0]["contents"].([]any)[0].(map[string]any); title["text"] != "+32470123456" {
		t.Fatalf("card title %+v", title)
	}

	// Aircall's retry of the final event changes nothing and runs no automation again.
	h.send(t, bodies[len(bodies)-1])
	if n := len(h.e.Jobs(jobs.PhoneInboundArgs{}.Kind(), h.ws.ID)); n != 0 {
		t.Fatalf("a retried event was queued again: %d", n)
	}
	var triggered int
	h.e.Scalar(`SELECT count(*) FROM phone_calls WHERE workspace_id = $1 AND ended_trigger_at IS NOT NULL`, &triggered, h.ws.ID)
	if triggered != 1 {
		t.Fatalf("ended triggers %d", triggered)
	}

	// The AI summary and transcript land on the same entry.
	h.sendAll(t, h.recorded(t, "intelligence"))
	d = h.callEntries(t, tk.ID)[0].Data
	if !strings.Contains(d["summary"].(string), "replacement bulb") || !strings.HasPrefix(d["transcript"].(string), "Agent: Bonjour") {
		t.Fatalf("intelligence %+v", d)
	}
}

func TestEventsOutOfOrderSettleTheSame(t *testing.T) {
	h := newHarness(t)
	bodies := h.recorded(t, "answered")
	for i := len(bodies) - 1; i >= 0; i-- {
		if got := h.send(t, bodies[i]); got != 200 {
			t.Fatal(got)
		}
	}
	h.drain(t)
	tk := h.ticketOfCall(t, "1290001")
	entries := h.callEntries(t, tk.ID)
	if len(entries) != 1 || entries[0].Data["outcome"] != "answered" || entries[0].Data["stage"] != "ended" {
		t.Fatalf("entries %+v", entries)
	}
	// The ring came after the end: too late for a card.
	if cards := h.f.Cards("1290001"); len(cards) != 0 {
		t.Fatalf("cards %+v", cards)
	}
}

func TestMissedCallOutsideHoursReopensTheTicket(t *testing.T) {
	h := newHarness(t)
	// The caller is known, with a number written another way, and has a solved ticket.
	var tk domain.Ticket
	h.e.Do("POST", "/v1/tickets", h.ws.Agent.As(h.ws), map[string]any{"subject": "Broken bulb", "channel": "api", "kind": "note",
		"body_html": "<p>Earlier</p>", "status": "solved"}).Want(t, 201).JSON(t, &tk)
	cid := uuid.New()
	h.e.Exec(`INSERT INTO contacts (id, workspace_id, name, phone) VALUES ($1, $2, 'Jane Doe', '0032 470 12 34 56')`, cid, h.ws.ID)
	h.e.Exec(`UPDATE tickets SET contact_id = $2, status = 'solved' WHERE id = $1`, tk.ID, cid)

	// Validation: call conditions need the call_ended trigger.
	h.e.Do("POST", "/v1/automations", h.ws.Admin.As(h.ws), map[string]any{"name": "Wrong", "trigger": "ticket_created",
		"conditions": []map[string]any{{"field": "call_outcome", "op": "equals", "value": "missed"}},
		"actions":    []map[string]any{{"type": "set_priority", "value": "high"}}}).Want(t, 422)
	h.e.Do("POST", "/v1/automations", h.ws.Admin.As(h.ws), map[string]any{"name": "Wrong value", "trigger": "call_ended",
		"conditions": []map[string]any{{"field": "call_time", "op": "equals", "value": "lunch"}},
		"actions":    []map[string]any{{"type": "set_priority", "value": "high"}}}).Want(t, 422)

	h.e.Do("POST", "/v1/automations", h.ws.Admin.As(h.ws), map[string]any{"name": "Missed call outside opening hours", "trigger": "call_ended",
		"conditions": []map[string]any{
			{"field": "call_outcome", "op": "equals", "value": "missed"},
			{"field": "call_time", "op": "equals", "value": "outside_business_hours"},
			{"field": "call_direction", "op": "equals", "value": "inbound"},
		},
		"actions": []map[string]any{
			{"type": "set_status", "value": "open"},
			{"type": "add_note", "value": "<p>Missed call outside opening hours: call back.</p>"},
			{"type": "set_priority", "value": "high"},
		}}).Want(t, 201)
	h.e.Do("POST", "/v1/automations", h.ws.Admin.As(h.ws), map[string]any{"name": "Tag the reason", "trigger": "call_ended",
		"conditions": []map[string]any{{"field": "call_missed_reason", "op": "equals", "value": "out_of_opening_hours"},
			{"field": "call_line", "op": "contains", "value": "support"}},
		"actions": []map[string]any{{"type": "add_tag", "value": "after-hours"}}}).Want(t, 201)
	// An answered-call rule must not fire.
	h.e.Do("POST", "/v1/automations", h.ws.Admin.As(h.ws), map[string]any{"name": "Answered", "trigger": "call_ended",
		"conditions": []map[string]any{{"field": "call_outcome", "op": "equals", "value": "answered"}},
		"actions":    []map[string]any{{"type": "add_tag", "value": "answered"}}}).Want(t, 201)

	h.sendAll(t, h.recorded(t, "missed_out_of_hours"))

	got := h.ticketOfCall(t, "1290002")
	if got.ID != tk.ID {
		t.Fatalf("the call went on %s, not the caller's recent ticket %s", got.ID, tk.ID)
	}
	if got.Status != "open" || got.Priority != "high" {
		t.Fatalf("ticket %s %s", got.Status, got.Priority)
	}
	var tags []string
	for _, tg := range got.Tags {
		tags = append(tags, tg.Name)
	}
	if !slices.Contains(tags, "after-hours") || slices.Contains(tags, "answered") {
		t.Fatalf("tags %v", tags)
	}
	var notes int
	h.e.Scalar(`SELECT count(*) FROM messages WHERE ticket_id = $1 AND kind = 'note' AND body_text LIKE 'Missed call outside%'`, &notes, tk.ID)
	if notes != 1 {
		t.Fatalf("notes %d", notes)
	}
	d := h.callEntries(t, tk.ID)[0].Data
	if d["outcome"] != "missed" || d["missed_reason"] != "out_of_opening_hours" || d["outside_hours"] != true || d["caller_name"] != "" {
		t.Fatalf("entry %+v", d)
	}
	var contacts int
	h.e.Scalar(`SELECT count(*) FROM contacts WHERE workspace_id = $1`, &contacts, h.ws.ID)
	if contacts != 1 {
		t.Fatalf("a known caller got a second contact: %d", contacts)
	}
}

func TestVoicemailOutboundAndPlacement(t *testing.T) {
	h := newHarness(t)
	h.sendAll(t, h.recorded(t, "voicemail"))
	vm := h.ticketOfCall(t, "1290003")
	d := h.callEntries(t, vm.ID)[0].Data
	if d["outcome"] != "voicemail" || d["voicemail"] != true || d["missed_reason"] != "no_available_agent" || vm.Subject != "Call from +33612345678" {
		t.Fatalf("voicemail %+v %q", d, vm.Subject)
	}

	// The first call opens a ticket; the outbound call to the same number goes on it while it is open.
	h.sendAll(t, h.recorded(t, "answered"))
	in := h.ticketOfCall(t, "1290001")
	h.sendAll(t, h.recorded(t, "outbound"))
	out := h.ticketOfCall(t, "1290004")
	if out.ID != in.ID || len(h.callEntries(t, in.ID)) != 2 {
		t.Fatalf("outbound call on %s, open ticket %s", out.ID, in.ID)
	}
	d = h.callEntries(t, in.ID)[1].Data
	if d["direction"] != "outbound" || d["outcome"] != "answered" {
		t.Fatalf("outbound %+v", d)
	}

	// Solved, and with reuse off, the next call opens a new ticket.
	h.e.Exec(`UPDATE tickets SET status = 'solved' WHERE id = $1`, in.ID)
	h.e.Do("PATCH", "/v1/phone/connections/"+h.conn.ID.String(), h.ws.Admin.As(h.ws), map[string]any{"reuse_days": 0}).Want(t, 200)
	next := h.recorded(t, "outbound")
	for i := range next {
		next[i] = bytes.ReplaceAll(next[i], []byte("1290004"), []byte("1290005"))
	}
	h.sendAll(t, next)
	if again := h.ticketOfCall(t, "1290005"); again.ID == in.ID || again.Subject != "Call to +32470123456" {
		t.Fatalf("after solving: %s %q", again.ID, again.Subject)
	}

	// A withheld number opens a ticket without a contact.
	anon := []byte(`{"resource":"call","event":"call.ended","timestamp":1793095200,"token":"` + h.token(t) +
		`","data":{"id":1290006,"direction":"inbound","raw_digits":"anonymous","started_at":1793095200,"ended_at":1793095210}}`)
	h.sendAll(t, [][]byte{anon})
	if tk := h.ticketOfCall(t, "1290006"); tk.Contact != nil || tk.Subject != "Call from a withheld number" {
		t.Fatalf("anonymous %+v", tk)
	}
}
