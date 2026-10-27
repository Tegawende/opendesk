package phone

import (
	"testing"
	"time"

	"github.com/useopendesk/opendesk/internal/phone/provider"
)

func TestNormalizeNumber(t *testing.T) {
	for in, want := range map[string]string{
		"+32 470 12 34 56":   "+32470123456",
		"0032 470/12.34.56":  "+32470123456",
		"+33 (0)6 12-34-56":  "+336123456",
		"0470 12 34 56":      "0470123456",
		"anonymous":          "",
		"":                   "",
		"12":                 "",
		"+1234567890123456":  "",
		"+32 470 12 34 ext9": "",
	} {
		if got := NormalizeNumber(in); got != want {
			t.Errorf("NormalizeNumber(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSubject(t *testing.T) {
	if s := Subject("inbound", "Jane Doe", "+32470"); s != "Call from Jane Doe" {
		t.Fatal(s)
	}
	if s := Subject("outbound", "", "+32470"); s != "Call to +32470" {
		t.Fatal(s)
	}
	if s := Subject("inbound", "", ""); s != "Call from a withheld number" {
		t.Fatal(s)
	}
}

func at(sec int64) *time.Time { t := time.Unix(sec, 0).UTC(); return &t }

// The same events in any order leave the call in the same state.
func TestMergeIsOrderIndependent(t *testing.T) {
	events := []provider.Event{
		{CallID: "1", At: *at(100), Call: &provider.Call{Direction: "inbound", Stage: provider.StageRinging, Number: "+32 470 12 34 56", StartedAt: at(100), LineName: "FR - Support"}},
		{CallID: "1", At: *at(110), Call: &provider.Call{Direction: "inbound", Stage: provider.StageAnswered, AnsweredAt: at(110), Agent: &provider.Person{Name: "Anna Agent"}}},
		{CallID: "1", At: *at(200), Final: true, Call: &provider.Call{Direction: "inbound", Stage: provider.StageEnded, AnsweredAt: at(110), EndedAt: at(190), Duration: 90,
			Recording: true, RecordingURL: "https://short/r", LinksValid: time.Hour, Tags: []string{"Order"}}},
		{CallID: "1", At: *at(210), Call: &provider.Call{Direction: "inbound", AssetURL: "https://asset"}},
	}
	orders := [][]int{{0, 1, 2, 3}, {3, 2, 1, 0}, {2, 0, 3, 1}}
	var first phoneCall
	for i, order := range orders {
		c := phoneCall{Direction: "inbound"}
		for _, j := range order {
			merge(&c, events[j])
		}
		if c.Stage != provider.StageEnded || c.Outcome != OutcomeAnswered || c.Number != "+32470123456" || c.Duration != 90 ||
			c.AgentName != "Anna Agent" || c.AssetURL != "https://asset" || c.LineName != "FR - Support" || !c.HasRecording || len(c.Tags) != 1 {
			t.Fatalf("order %v: %+v", order, c)
		}
		if i == 0 {
			first = c
			continue
		}
		if !c.StartedAt.Equal(*first.StartedAt) || !c.AnsweredAt.Equal(*first.AnsweredAt) || !c.EndedAt.Equal(*first.EndedAt) {
			t.Fatalf("order %v: times differ %+v", order, c)
		}
	}
}

func TestOutcome(t *testing.T) {
	c := phoneCall{Stage: provider.StageAnswered, AnsweredAt: at(1)}
	if outcome(c) != "" {
		t.Fatal("a call still going has no outcome")
	}
	c.Stage = provider.StageEnded
	if outcome(c) != OutcomeAnswered {
		t.Fatal("answered")
	}
	c.AnsweredAt = nil
	if outcome(c) != OutcomeMissed {
		t.Fatal("missed")
	}
	c.HasVoicemail = true
	if outcome(c) != OutcomeVoicemail {
		t.Fatal("voicemail")
	}
}
