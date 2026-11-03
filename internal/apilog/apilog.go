// Package apilog keeps a record of the requests an API key makes, for the keys an admin turned
// "Record requests" on (contracts/api.md "API keys"): when, what was asked, what was sent, the
// answer to a failed request and the records it touched, so a team can check afterwards what
// an integration did. Records are kept 90 days (core.cleanup).
//
// The record is written after the handler answers, in the background: recording never slows
// a request down and never fails it.
package apilog

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"mime"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/useopendesk/opendesk/internal/auth"
	"github.com/useopendesk/opendesk/internal/ids"
)

const (
	// MaxRequestBody is the largest complete JSON capture.
	MaxRequestBody = 64 << 10
	// MaxErrorBody bounds the complete failure JSON used for safe diagnostics.
	MaxErrorBody = 8 << 10
	// maxScan is how much of a successful answer is read for the records it names.
	maxScan = 256 << 10
	// Retention is how long records are kept (core.cleanup deletes older ones).
	Retention = 90 * 24 * time.Hour
	// Masked replaces the value of a field that looks like a secret.
	Masked = "***"
)

// secretName matches field and query names whose values are never kept.
var secretName = regexp.MustCompile(`(?i)secret|token|passw|api_?key|authorization|credential|signature`)

var pending sync.WaitGroup

// Wait blocks until the records being written are written (tests, shutdown).
func Wait() { pending.Wait() }

// Entry is one recorded request, as written.
type Entry struct {
	ID                uuid.UUID
	WorkspaceID       uuid.UUID
	APIKeyID          uuid.UUID
	CreatedAt         time.Time
	Method            string
	Path              string
	Query             string
	Status            int
	DurationMS        int
	RequestID         string
	IdempotencyKey    string
	Replayed          bool
	IP                string
	UserAgent         string
	ContentType       string
	RequestSize       int64
	RequestBody       *string
	RequestTruncated  bool
	ResponseBody      *string
	ResponseTruncated bool
	TicketRef         string
	TicketID          *uuid.UUID
	MessageID         *uuid.UUID
	ContactID         *uuid.UUID
}

// Middleware records the requests of API keys with "Record requests" on. It goes right after
// the credential resolver, so it sees every answer, rate limits and idempotent replays included.
func Middleware(pool *pgxpool.Pool, log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			p := auth.FromContext(r.Context())
			if p == nil || p.Kind != auth.KindAPIKey || !p.RecordRequests || !strings.HasPrefix(r.URL.Path, "/v1/") {
				next.ServeHTTP(w, r)
				return
			}
			start := time.Now()
			e := Entry{
				ID: ids.New(), WorkspaceID: p.WorkspaceID, APIKeyID: p.APIKeyID, CreatedAt: start,
				Method: r.Method, Path: r.URL.Path,
				IdempotencyKey: truncate(r.Header.Get("Idempotency-Key"), 255),
				IP:             p.IP, UserAgent: p.UserAgent, RequestSize: max(r.ContentLength, 0),
				ContentType: truncate(r.Header.Get("Content-Type"), 200),
			}
			if captureTicketBody(r.Method, r.URL.Path) && r.Body != nil {
				e.RequestBody, e.RequestTruncated = readBody(r, e.ContentType)
			}
			rec := &recorder{ResponseWriter: w}
			defer func() {
				failed := recover()
				e.Status = rec.status
				if e.Status == 0 {
					e.Status = http.StatusOK
				}
				if failed != nil {
					// The handler failed: the recoverer above answers 500.
					e.Status = http.StatusInternalServerError
				}
				e.DurationMS = int(time.Since(start) / time.Millisecond)
				e.RequestID = truncate(r.Header.Get("X-Request-Id"), 100)
				e.Replayed = rec.Header().Get("Idempotent-Replayed") == "true"
				e.Path = safePath(r)
				e.Query = maskQuery(r.URL.RawQuery)
				e.TicketRef = ticketRef(e.Path)
				if e.Status >= 400 {
					e.ResponseTruncated = rec.size > MaxErrorBody
					s := failureBody(rec.buf.Bytes(), e.ResponseTruncated, e.Status)
					e.ResponseBody = &s
				} else if rec.size <= maxScan {
					e.TicketID, e.MessageID, e.ContactID = named(e.Path, rec.buf.Bytes())
				}
				if e.TicketID == nil {
					if id, err := uuid.Parse(e.TicketRef); err == nil {
						e.TicketID = &id
					}
				}
				pending.Add(1)
				go func() {
					defer pending.Done()
					ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
					defer cancel()
					if err := Write(ctx, pool, e); err != nil {
						log.Warn("apilog: request not recorded", "record_id", e.ID, "err", err)
					}
				}()
				if failed != nil {
					panic(failed)
				}
			}()
			next.ServeHTTP(rec, r)
		})
	}
}

// Write stores a record. A ticket the path names by its number is looked up.
func Write(ctx context.Context, pool *pgxpool.Pool, e Entry) error {
	var number *int64
	if n, err := strconv.ParseInt(e.TicketRef, 10, 64); err == nil && e.TicketID == nil {
		number = &n
	}
	if e.TicketID == nil && number != nil {
		var id uuid.UUID
		err := pool.QueryRow(ctx, `SELECT id FROM tickets WHERE workspace_id=$1 AND number=$2`, e.WorkspaceID, *number).Scan(&id)
		if err == nil {
			e.TicketID = &id
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
	}
	var live bool
	if e.TicketID != nil {
		if err := pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM tickets WHERE workspace_id=$1 AND id=$2)`, e.WorkspaceID, *e.TicketID).Scan(&live); err != nil {
			return err
		}
	}
	if !live {
		e.RequestBody = nil
	}
	_, err := pool.Exec(ctx, `INSERT INTO api_key_requests (id, workspace_id, api_key_id, created_at, method, path, query, status,
		duration_ms, request_id, idempotency_key, replayed, ip, user_agent, content_type, request_size, request_body, request_truncated,
		response_body, response_truncated, ticket_ref, ticket_id, message_id, contact_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21,
			COALESCE($22, (SELECT t.id FROM tickets t WHERE t.workspace_id = $2 AND t.number = $25)), $23, $24)`,
		e.ID, e.WorkspaceID, e.APIKeyID, e.CreatedAt, e.Method, truncate(e.Path, 2000), truncate(e.Query, 2000), e.Status,
		e.DurationMS, e.RequestID, e.IdempotencyKey, e.Replayed, e.IP, e.UserAgent, e.ContentType, e.RequestSize, e.RequestBody, e.RequestTruncated,
		e.ResponseBody, e.ResponseTruncated, truncate(e.TicketRef, 200), e.TicketID, e.MessageID, e.ContactID, number)
	return err
}

// readBody keeps a write's JSON body, secrets masked, and hands the handler the whole body.
func readBody(r *http.Request, contentType string) (*string, bool) {
	mt, _, _ := mime.ParseMediaType(contentType)
	if contentType != "" && mt != "application/json" && !strings.HasSuffix(mt, "+json") {
		return nil, false // a file upload or a form: its size and type say enough
	}
	head, err := io.ReadAll(io.LimitReader(r.Body, MaxRequestBody+1))
	r.Body = readCloser{io.MultiReader(bytes.NewReader(head), r.Body), r.Body}
	if err != nil || len(head) == 0 {
		return nil, false
	}
	cut := len(head) > MaxRequestBody
	if cut {
		head = []byte(truncate(string(head), MaxRequestBody))
	}
	s := clean(MaskJSON(head, cut))
	return &s, cut
}

type readCloser struct {
	io.Reader
	io.Closer
}

// MaskJSON keeps only complete JSON, with secret fields masked. An incomplete capture is
// omitted as a whole, including when a byte limit falls inside a field name or value.
func MaskJSON(body []byte, cut bool) string {
	if cut || !json.Valid(body) {
		return Masked
	}
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()
	var v any
	if dec.Decode(&v) != nil {
		return Masked
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if enc.Encode(mask(v)) != nil {
		return Masked
	}
	result := strings.TrimRight(buf.String(), "\n")
	if len(result) > MaxRequestBody {
		return Masked
	}
	return result
}

// captureTicketBody is deliberately limited to writes with one ticket's shared content.
// Drafts, bulk operations, private mail and account/provider settings keep metadata only.
func captureTicketBody(method, path string) bool {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if path == "/v1/tickets" {
		return method == http.MethodPost
	}
	if len(parts) < 3 || parts[0] != "v1" || parts[1] != "tickets" || !safeRef(parts[2]) {
		return false
	}
	if len(parts) == 3 {
		return method == http.MethodPatch
	}
	if len(parts) == 4 && parts[3] == "messages" {
		return method == http.MethodPost
	}
	return len(parts) == 5 && parts[3] == "messages" && safeRef(parts[4]) && method == http.MethodPatch
}

func safeRef(s string) bool {
	if _, err := uuid.Parse(s); err == nil {
		return true
	}
	for _, prefix := range []string{"ext:", "gorgias:", "zendesk:"} {
		s = strings.TrimPrefix(s, prefix)
	}
	n, err := strconv.ParseInt(s, 10, 64)
	return err == nil && n > 0
}

// Route templates keep arbitrary path parameters, including credentials, out of records.
func safePath(r *http.Request) string {
	rc := chi.RouteContext(r.Context())
	if rc == nil || rc.RoutePattern() == "" {
		return "/v1/[unmatched]"
	}
	path := rc.RoutePattern()
	for i, key := range rc.URLParams.Keys {
		value := rc.URLParams.Values[i]
		if !secretName.MatchString(key) && safeRef(value) {
			path = strings.ReplaceAll(path, "{"+key+"}", value)
		}
	}
	return path
}

// Failure diagnostics contain no free-form strings supplied by a caller or provider.
func failureBody(body []byte, cut bool, status int) string {
	out := map[string]any{"status": status, "title": http.StatusText(status)}
	var problem struct {
		Code   string            `json:"code"`
		Errors []json.RawMessage `json:"errors"`
	}
	if !cut && json.Unmarshal(body, &problem) == nil {
		switch problem.Code {
		case "validation_failed", "not_found", "forbidden", "unauthenticated", "rate_limited", "conflict", "bad_request", "internal", "mfa_required", "reauth_required", "plan_limit":
			out["code"] = problem.Code
		}
		if len(problem.Errors) > 0 {
			out["validation_errors"] = len(problem.Errors)
		}
	}
	result, _ := json.Marshal(out)
	return string(result)
}

func mask(v any) any {
	switch t := v.(type) {
	case map[string]any:
		for k, x := range t {
			if secretName.MatchString(k) {
				t[k] = Masked
			} else {
				t[k] = mask(x)
			}
		}
	case []any:
		for i, x := range t {
			t[i] = mask(x)
		}
	}
	return v
}

func maskQuery(raw string) string {
	if raw == "" {
		return ""
	}
	q, err := url.ParseQuery(raw)
	if err != nil {
		return ""
	}
	for k, values := range q {
		switch k {
		case "limit", "page":
			if len(values) != 1 {
				delete(q, k)
				continue
			}
			n, err := strconv.Atoi(values[0])
			if err != nil || n < 1 || n > 1000 {
				delete(q, k)
			}
		case "expand":
			if len(values) != 1 || values[0] != "messages" {
				delete(q, k)
			}
		default:
			switch strings.ToLower(k) {
			case "secret", "token", "access_token", "refresh_token", "password", "api_key", "apikey", "authorization", "credential", "credentials", "signature":
				q[k] = []string{Masked}
			default:
				delete(q, k)
			}
		}
	}
	return q.Encode()
}

// ticketRef is the ticket a /v1/tickets/{ref}/… path names, as written.
func ticketRef(path string) string {
	rest, ok := strings.CutPrefix(path, "/v1/tickets/")
	if !ok {
		return ""
	}
	ref, _, _ := strings.Cut(rest, "/")
	if s, err := url.PathUnescape(ref); err == nil {
		return s
	}
	return ref
}

// named reads the records a successful answer names: a ticket (an object with a number), a
// message (one with a ticket_id, on a messages path), a contact.
func named(path string, body []byte) (ticket, message, contact *uuid.UUID) {
	var obj struct {
		ID       *uuid.UUID `json:"id"`
		Number   *int64     `json:"number"`
		TicketID *uuid.UUID `json:"ticket_id"`
		Contact  *struct {
			ID *uuid.UUID `json:"id"`
		} `json:"contact"`
	}
	if len(body) == 0 || json.Unmarshal(body, &obj) != nil {
		return nil, nil, nil
	}
	switch {
	case strings.HasPrefix(path, "/v1/tickets"):
		if obj.Number != nil && obj.ID != nil {
			ticket = obj.ID
			if obj.Contact != nil {
				contact = obj.Contact.ID
			}
		} else if obj.TicketID != nil {
			ticket = obj.TicketID
			if strings.Contains(path, "/messages") {
				message = obj.ID
			}
		}
	case strings.HasPrefix(path, "/v1/contacts/") || path == "/v1/contacts":
		contact = obj.ID
	}
	return ticket, message, contact
}

// recorder passes the answer through, keeping its status and its first bytes.
type recorder struct {
	http.ResponseWriter
	status int
	buf    bytes.Buffer
	size   int
}

func (w *recorder) WriteHeader(code int) {
	if w.status == 0 {
		w.status = code
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *recorder) Write(b []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	if room := maxScan - w.buf.Len(); room > 0 {
		w.buf.Write(b[:min(len(b), room)])
	}
	w.size += len(b)
	return w.ResponseWriter.Write(b)
}

func (w *recorder) Unwrap() http.ResponseWriter { return w.ResponseWriter }

func (w *recorder) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (w *recorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if h, ok := w.ResponseWriter.(http.Hijacker); ok {
		return h.Hijack()
	}
	return nil, nil, errors.New("apilog: the response cannot be hijacked")
}

// truncate cuts s to at most n bytes, on a character boundary.
func truncate(s string, n int) string {
	if len(s) <= n {
		return clean(s)
	}
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return clean(s[:n])
}

// clean makes s storable as Postgres text: valid UTF-8 (a body cut inside a character, a
// binary answer) and no NUL.
func clean(s string) string {
	return strings.ReplaceAll(strings.ToValidUTF8(s, "�"), "\x00", "")
}
