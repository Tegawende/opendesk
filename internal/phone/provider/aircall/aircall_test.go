package aircall

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/useopendesk/opendesk/internal/phone/provider"
	"github.com/useopendesk/opendesk/internal/phone/provider/aircall/fake"
)

var creds = map[string]string{"api_id": "id-1", "api_token": "token-1"}

func recorded(t *testing.T, name string) [][]byte {
	t.Helper()
	raw, err := os.ReadFile("testdata/" + name + ".json")
	if err != nil {
		t.Fatal(err)
	}
	var list []json.RawMessage
	if err := json.Unmarshal(raw, &list); err != nil {
		t.Fatal(err)
	}
	out := make([][]byte, len(list))
	for i, m := range list {
		out[i] = m
	}
	return out
}

func TestParseAnsweredCall(t *testing.T) {
	events := recorded(t, "answered")
	var stages []string
	for _, body := range events {
		ev, err := Adapter{}.Parse(body)
		if err != nil {
			t.Fatal(err)
		}
		if ev.Kind != provider.KindCall || ev.CallID != "1290001" || ev.Call.Direction != "inbound" {
			t.Fatalf("event %+v", ev)
		}
		stages = append(stages, ev.Call.Stage)
	}
	if strings.Join(stages, ",") != "ringing,ringing,answered,ended" {
		t.Fatalf("stages %v", stages)
	}
	last, _ := Adapter{}.Parse(events[3])
	c := last.Call
	if !last.Final || c.Number != "+32 470 12 34 56" || c.LineName != "FR - Support" || c.LineID != "501" || c.Duration != 190 {
		t.Fatalf("ended %+v", c)
	}
	if c.Agent == nil || c.Agent.Email != "anna@example.com" || c.AnsweredAt == nil || !c.Recording {
		t.Fatalf("agent or answer %+v", c)
	}
	if c.RecordingURL != "https://short.aircall.io/r/AbC" || c.LinksValid != 3*time.Hour || c.AssetURL == "" {
		t.Fatalf("links %+v", c)
	}
	if len(c.Tags) != 1 || c.Tags[0] != "Order" || len(c.Comments) != 1 || c.Comments[0].Text != "Wants a replacement bulb" || c.Comments[0].Author != "Anna Agent" {
		t.Fatalf("tags and comments %+v %+v", c.Tags, c.Comments)
	}
	// Two different events never share an id; the same body always does.
	a, _ := Adapter{}.Parse(events[0])
	b, _ := Adapter{}.Parse(events[1])
	again, _ := Adapter{}.Parse(events[0])
	if a.ID == b.ID || a.ID != again.ID {
		t.Fatal("event ids do not deduplicate retries only")
	}
}

func TestParseMissedVoicemailOutbound(t *testing.T) {
	missed, _ := Adapter{}.Parse(recorded(t, "missed_out_of_hours")[1])
	if missed.Call.MissedReason != "out_of_opening_hours" || missed.Call.AnsweredAt != nil || !missed.Final {
		t.Fatalf("missed %+v", missed.Call)
	}
	vm := recorded(t, "voicemail")
	left, _ := Adapter{}.Parse(vm[1])
	if !left.Call.Voicemail || left.Call.Stage != provider.StageEnded || left.Final || left.Call.VoicemailURL != "https://short.aircall.io/v/XyZ" {
		t.Fatalf("voicemail %+v", left.Call)
	}
	out, _ := Adapter{}.Parse(recorded(t, "outbound")[0])
	if out.Call.Direction != "outbound" || out.Call.Agent == nil {
		t.Fatalf("outbound %+v", out.Call)
	}
	anon := []byte(`{"resource":"call","event":"call.created","timestamp":1,"token":"t","data":{"id":5,"direction":"inbound","raw_digits":"anonymous"}}`)
	ev, err := Adapter{}.Parse(anon)
	if err != nil || !ev.Call.Anonymous || ev.Call.Number != "" {
		t.Fatalf("anonymous %+v %v", ev.Call, err)
	}
}

func TestParseIntelligenceAndIgnored(t *testing.T) {
	ai := recorded(t, "intelligence")
	sum, err := Adapter{}.Parse(ai[0])
	if err != nil || sum.Kind != provider.KindSummary || sum.CallID != "1290001" || !strings.Contains(sum.Summary, "replacement bulb") {
		t.Fatalf("summary %+v %v", sum, err)
	}
	tr, err := Adapter{}.Parse(ai[1])
	if err != nil || tr.Kind != provider.KindTranscript || tr.CallID != "1290001" ||
		tr.Transcript != "Agent: Bonjour, LampTwist support.\nCustomer: Hello, my bulb arrived broken." {
		t.Fatalf("transcript %+v %v", tr, err)
	}
	for _, body := range []string{
		`{"resource":"user","event":"user.created","token":"t","data":{"id":1}}`,
		`{"resource":"call","event":"call.archived","token":"t","data":{"id":1}}`,
	} {
		if _, err := (Adapter{}).Parse([]byte(body)); !errors.Is(err, provider.ErrIgnored) {
			t.Fatalf("%s: %v", body, err)
		}
	}
}

func TestVerify(t *testing.T) {
	body := recorded(t, "answered")[0]
	if err := (Adapter{}).Verify(body, "hook-token-1"); err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"", "hook-token-2", "hook-token-1x"} {
		if err := (Adapter{}).Verify(body, secret); !errors.Is(err, provider.ErrUnauthorized) {
			t.Fatalf("secret %q verified", secret)
		}
	}
	if err := (Adapter{}).Verify([]byte(`{"data":{}}`), "hook-token-1"); err == nil {
		t.Fatal("a body without a token verified")
	}
}

func TestAPIAgainstFakeAircall(t *testing.T) {
	ctx := context.Background()
	f := fake.New(t, creds["api_id"], creds["api_token"])
	a := Adapter{BaseURL: f.URL()}
	client := http.DefaultClient

	acct, err := a.Validate(ctx, client, creds)
	if err != nil || acct.Name != "LampTwist" {
		t.Fatalf("validate %+v %v", acct, err)
	}
	if _, err := a.Validate(ctx, client, map[string]string{"api_id": "id-1", "api_token": "wrong"}); !provider.Refused(err) {
		t.Fatalf("wrong token: %v", err)
	}

	// Another integration's webhook is there before ours, and stays.
	f.AddWebhook("gorgias-1", "https://gorgias.example.com/hook", "their-token")
	hook, err := a.RegisterWebhook(ctx, client, creds, "https://desk.example.com/v1/phone/hooks/aircall/x")
	if err != nil || hook.ID == "" || hook.Secret == "" {
		t.Fatalf("register %+v %v", hook, err)
	}
	w := f.Webhook(hook.ID)
	if w == nil || w.URL != "https://desk.example.com/v1/phone/hooks/aircall/x" || !contains(w.Events, "call.ended") || !contains(w.Events, "summary.created") {
		t.Fatalf("registered %+v", w)
	}
	if ok, err := a.WebhookActive(ctx, client, creds, hook.ID); err != nil || !ok {
		t.Fatalf("active %v %v", ok, err)
	}
	f.Disable(hook.ID)
	if ok, _ := a.WebhookActive(ctx, client, creds, hook.ID); ok {
		t.Fatal("a disabled webhook reads active")
	}
	if err := a.DeleteWebhook(ctx, client, creds, hook.ID); err != nil {
		t.Fatal(err)
	}
	if err := a.DeleteWebhook(ctx, client, creds, hook.ID); err != nil {
		t.Fatalf("deleting a gone webhook: %v", err)
	}
	if ok, _ := a.WebhookActive(ctx, client, creds, hook.ID); ok {
		t.Fatal("a deleted webhook reads active")
	}
	if f.Webhook("gorgias-1") == nil {
		t.Fatal("another integration's webhook was removed")
	}

	// A company without the AI add-on gets the call events alone.
	f.RefuseAIEvents = true
	hook, err = a.RegisterWebhook(ctx, client, creds, "https://desk.example.com/h")
	if err != nil || contains(f.Webhook(hook.ID).Events, "summary.created") {
		t.Fatalf("fallback %v %+v", err, f.Webhook(hook.ID))
	}

	lines, err := a.Lines(ctx, client, creds)
	if err != nil || len(lines) != 2 || lines[0].Name != "FR - Support" {
		t.Fatalf("lines %+v %v", lines, err)
	}

	card := provider.Card{Title: "Jane Doe", TitleLink: "https://desk.example.com/contacts/1",
		Lines: []provider.CardLine{{Label: "Ticket", Text: "#12 My bulb arrived broken and the box was open", Link: "http://insecure.example.com"}}}
	if err := a.PushCard(ctx, client, creds, "1290001", card); err != nil {
		t.Fatal(err)
	}
	got := f.Cards("1290001")
	if len(got) != 1 {
		t.Fatalf("cards %+v", got)
	}
	contents := got[0]["contents"].([]any)
	title := contents[0].(map[string]any)
	line := contents[1].(map[string]any)
	if title["type"] != "title" || title["text"] != "Jane Doe" || title["link"] != "https://desk.example.com/contacts/1" {
		t.Fatalf("title %+v", title)
	}
	if line["type"] != "shortText" || line["label"] != "Ticket" || line["link"] != nil {
		t.Fatalf("line %+v (an http link must be dropped)", line)
	}

	f.Limit = true
	err = a.PushCard(ctx, client, creds, "1290001", card)
	var limited *provider.RateLimited
	if !errors.As(err, &limited) || limited.RetryAfter <= 0 {
		t.Fatalf("rate limit: %v", err)
	}
}

func contains(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}
