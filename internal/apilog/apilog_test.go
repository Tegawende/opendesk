package apilog

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/google/uuid"
)

func TestMaskJSON(t *testing.T) {
	got := MaskJSON([]byte(`{"name":"Claimy","password":"hunter2","nested":{"api_key":"k","list":[{"refresh_token":1}]},"credentials":{"user":"u"},"note":"<b>tokenless</b>"}`), false)
	for _, leak := range []string{"hunter2", `"k"`, `"refresh_token":1`, `"user"`} {
		if strings.Contains(got, leak) {
			t.Fatalf("kept %s: %s", leak, got)
		}
	}
	if !strings.Contains(got, `"name":"Claimy"`) || !strings.Contains(got, `"note":"<b>tokenless</b>"`) || !strings.Contains(got, `"credentials":"***"`) {
		t.Fatalf("masked too much: %s", got)
	}
	// Cut, it no longer parses: string members are masked where they stand.
	cut := MaskJSON([]byte(`{"subject":"R-1","client_secret":"abc\"def","body_html":"<p>unfinish`), true)
	if strings.Contains(cut, "abc") || !strings.HasPrefix(cut, `{"subject":"R-1","client_secret":"***","body_html":"<p>unfinish`) {
		t.Fatalf("cut: %s", cut)
	}
	// Numbers keep their digits.
	if got := MaskJSON([]byte(`{"amount":12345678901234567890}`), false); got != `{"amount":12345678901234567890}` {
		t.Fatalf("number: %s", got)
	}
}

func TestMaskQuery(t *testing.T) {
	if got := maskQuery("expand=messages&access_token=s3cr3t"); strings.Contains(got, "s3cr3t") || !strings.Contains(got, "expand=messages") {
		t.Fatalf("query: %s", got)
	}
}

func TestTicketRef(t *testing.T) {
	for path, want := range map[string]string{
		"/v1/tickets/4242/messages": "4242",
		"/v1/tickets/ext:555":       "ext:555",
		"/v1/tickets/ext%3A555/":    "ext:555",
		"/v1/tickets":               "",
		"/v1/contacts/x":            "",
	} {
		if got := ticketRef(path); got != want {
			t.Errorf("%s: %q, want %q", path, got, want)
		}
	}
}

func TestNamed(t *testing.T) {
	tid, mid, cid := uuid.New(), uuid.New(), uuid.New()
	ticket, message, contact := named("/v1/tickets", []byte(`{"id":"`+tid.String()+`","number":7,"contact":{"id":"`+cid.String()+`"}}`))
	if ticket == nil || *ticket != tid || message != nil || contact == nil || *contact != cid {
		t.Fatalf("ticket: %v %v %v", ticket, message, contact)
	}
	ticket, message, _ = named("/v1/tickets/7/messages", []byte(`{"id":"`+mid.String()+`","ticket_id":"`+tid.String()+`"}`))
	if ticket == nil || *ticket != tid || message == nil || *message != mid {
		t.Fatalf("message: %v %v", ticket, message)
	}
	if _, _, contact = named("/v1/contacts/"+cid.String(), []byte(`{"id":"`+cid.String()+`"}`)); contact == nil || *contact != cid {
		t.Fatalf("contact: %v", contact)
	}
	if a, b, c := named("/v1/tickets", []byte(`{"data":[]}`)); a != nil || b != nil || c != nil {
		t.Fatal("a list names nothing")
	}
}

func TestTruncateKeepsText(t *testing.T) {
	s := truncate(strings.Repeat("é", 10), 5)
	if !utf8.ValidString(s) || s != "éé" {
		t.Fatalf("%q", s)
	}
	if got := clean("a\x00b\xffc"); got != "ab�c" {
		t.Fatalf("%q", got)
	}
}
