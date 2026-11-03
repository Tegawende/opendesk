package apilog

import (
	"github.com/go-chi/chi/v5"
	"net/http"
	"net/http/httptest"
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
	for _, body := range []string{
		`{"subject":"R-1","client_secret":"abc\"def","body_html":"<p>unfinish`,
		`{"password":"sensitive`,
		`{"pass\u0077ord":{"nested":"sensitive"`,
		`{"password":123456`,
		`{"safe":true} {"password":"sensitive"}`,
		`{"safe":true} trailing`,
	} {
		if got := MaskJSON([]byte(body), false); got != Masked {
			t.Errorf("incomplete JSON retained: %q", got)
		}
		if got := MaskJSON([]byte(body), true); got != Masked {
			t.Errorf("cut JSON retained: %q", got)
		}
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

func TestCaptureTicketBody(t *testing.T) {
	id := uuid.NewString()
	for _, tc := range []struct {
		method, path string
		want         bool
	}{
		{"POST", "/v1/tickets", true}, {"PATCH", "/v1/tickets/42", true},
		{"POST", "/v1/tickets/" + id + "/messages", true},
		{"PATCH", "/v1/tickets/42/messages/" + id, true},
		{"PUT", "/v1/tickets/42/draft", false}, {"POST", "/v1/tickets/bulk", false},
		{"POST", "/v1/tickets/42/merge", false}, {"POST", "/v1/tickets/42/threads", false},
		{"POST", "/v1/collab/drafts", false}, {"POST", "/v1/auth/login", false},
		{"PATCH", "/v1/mailboxes/" + id, false}, {"POST", "/v1/integrations", false},
		{"POST", "/v1/webhooks", false}, {"POST", "/v1/widget/messages", false},
		{"POST", "/v1/tickets/arbitrary-secret/messages", false},
	} {
		if got := captureTicketBody(tc.method, tc.path); got != tc.want {
			t.Errorf("%s %s: %v", tc.method, tc.path, got)
		}
	}
}

func TestFailureBody(t *testing.T) {
	for _, body := range []string{
		`{"status":422,"code":"validation_failed","detail":"credential","title":"credential","errors":[{"location":"body.password","message":"credential","value":{"nested":"credential"}}]}`,
		`{"code":"credential","token":"credential","errors":[{"location":"body.credential","value":"credential"}]}`,
		`{"errors":[{"value":"credential`, `not json credential`,
	} {
		for _, cut := range []bool{false, true} {
			got := failureBody([]byte(body), cut, 422)
			if strings.Contains(got, "credential") || !strings.Contains(got, `"status":422`) {
				t.Errorf("unsafe failure: %s", got)
			}
		}
	}
}

func TestQueryCaptureAllowlist(t *testing.T) {
	got := maskQuery("limit=20&page=2&expand=messages&search=PRIVATE_SENTINEL&token=CREDENTIAL_SENTINEL&token_CREDENTIAL_SENTINEL=value")
	if strings.Contains(got, "SENTINEL") || !strings.Contains(got, "limit=20") || !strings.Contains(got, "expand=messages") {
		t.Fatalf("query=%s", got)
	}
	for _, raw := range []string{"limit=10&limit=PRIVATE_SENTINEL", "page=-1", "expand=PRIVATE_SENTINEL", "%zz=PRIVATE_SENTINEL"} {
		if got := maskQuery(raw); got != "" {
			t.Fatalf("unsafe query retained: %s", got)
		}
	}
}

func TestSafePath(t *testing.T) {
	router := chi.NewRouter()
	capture := func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(safePath(r))) }
	router.Post("/v1/phone/hooks/{provider}/{id}/{secret}", capture)
	router.NotFound(capture)
	for _, secret := range []string{uuid.NewString(), "123456", "PATH_CREDENTIAL_SENTINEL"} {
		out := httptest.NewRecorder()
		router.ServeHTTP(out, httptest.NewRequest(http.MethodPost, "/v1/phone/hooks/provider/42/"+secret, nil))
		if got := out.Body.String(); got != "/v1/phone/hooks/{provider}/42/{secret}" {
			t.Fatalf("unsafe path: %s", got)
		}
	}
	out := httptest.NewRecorder()
	router.ServeHTTP(out, httptest.NewRequest(http.MethodGet, "/v1/UNKNOWN_CREDENTIAL_SENTINEL", nil))
	if out.Body.String() != "/v1/[unmatched]" {
		t.Fatal("unmatched path content retained")
	}
}
