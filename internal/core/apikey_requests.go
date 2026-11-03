package core

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/useopendesk/opendesk/internal/auth"
	"github.com/useopendesk/opendesk/internal/db"
	"github.com/useopendesk/opendesk/internal/httpx"
)

// APIKeyRequest is one request an API key with "Record requests" on made (internal/apilog).
type APIKeyRequest struct {
	ID             uuid.UUID `json:"id"`
	CreatedAt      time.Time `json:"created_at"`
	Method         string    `json:"method"`
	Path           string    `json:"path"`
	Query          string    `json:"query" doc:"The query string, secret-looking parameters masked"`
	Status         int       `json:"status"`
	DurationMS     int       `json:"duration_ms"`
	IdempotencyKey string    `json:"idempotency_key"`
	Replayed       bool      `json:"replayed" doc:"The answer was an idempotent replay of an earlier request"`
	// Ticket is the ticket the request named in its path or that its answer named.
	Ticket    *APIKeyRequestTicket `json:"ticket"`
	TicketRef string               `json:"ticket_ref" doc:"The ticket as the path named it: an id or a number, as written"`
	MessageID *uuid.UUID           `json:"message_id"`
	ContactID *uuid.UUID           `json:"contact_id"`
}

type APIKeyRequestTicket struct {
	ID     uuid.UUID `json:"id"`
	Number *int64    `json:"number" doc:"Null once the ticket is gone"`
}

// APIKeyRequestDetail is a recorded request with what was sent and the answer to a failure.
type APIKeyRequestDetail struct {
	APIKeyRequest
	RequestID         string  `json:"request_id" doc:"X-Request-Id of the answer"`
	IP                string  `json:"ip"`
	UserAgent         string  `json:"user_agent"`
	ContentType       string  `json:"content_type"`
	RequestSize       int64   `json:"request_size"`
	RequestBody       *string `json:"request_body" doc:"The JSON a write sent, secret-looking fields masked, up to 64 KB; null for reads and other bodies"`
	RequestTruncated  bool    `json:"request_truncated"`
	ResponseBody      *string `json:"response_body" doc:"The answer to a failed request (status 400 and up), up to 8 KB"`
	ResponseTruncated bool    `json:"response_truncated"`
}

const apiKeyRequestCols = `r.id, r.created_at, r.method, r.path, r.query, r.status, r.duration_ms, r.idempotency_key, r.replayed,
	r.ticket_id, t.number, r.ticket_ref, r.message_id, r.contact_id`

const apiKeyRequestFrom = ` FROM api_key_requests r LEFT JOIN tickets t ON t.id = r.ticket_id AND t.workspace_id = r.workspace_id`

func scanAPIKeyRequest(r pgx.Row, extra ...any) (APIKeyRequest, error) {
	var x APIKeyRequest
	var tid *uuid.UUID
	var number *int64
	err := r.Scan(append([]any{&x.ID, &x.CreatedAt, &x.Method, &x.Path, &x.Query, &x.Status, &x.DurationMS, &x.IdempotencyKey, &x.Replayed,
		&tid, &number, &x.TicketRef, &x.MessageID, &x.ContactID}, extra...)...)
	if tid != nil {
		x.Ticket = &APIKeyRequestTicket{ID: *tid, Number: number}
	}
	return x, err
}

// requestKey checks the key is the workspace's, revoked ones included: their record stays.
func requestKey(ctx context.Context, q db.Querier, wid uuid.UUID, raw string) (uuid.UUID, error) {
	id, err := parseID(raw, "API key")
	if err != nil {
		return id, err
	}
	var ok bool
	if err := q.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM api_keys WHERE id = $1 AND workspace_id = $2)`, id, wid).Scan(&ok); err != nil {
		return id, httpx.Wrap(err)
	}
	if !ok {
		return id, httpx.NotFound("API key")
	}
	return id, nil
}

func (s *svc) registerAPIKeyRequests(api huma.API) {
	tags := []string{"API keys"}

	register(s, api, huma.Operation{OperationID: "list-api-key-requests", Method: http.MethodGet, Path: "/v1/api-keys/{id}/requests",
		Summary: "List an API key's recorded requests", Tags: tags,
		Description: "The requests the key made while Record requests was on, newest first, kept 90 days. Admin users only."},
		func(ctx context.Context, in *struct {
			ID string `path:"id"`
			httpx.ListParams
			Status string `query:"status" enum:"2xx,3xx,4xx,5xx,errors," doc:"Only answers of this class; errors: every failure, 400 and up"`
			Method string `query:"method" enum:"GET,POST,PUT,PATCH,DELETE," doc:"Only this method"`
			Since  string `query:"since" doc:"RFC 3339: made at or after"`
			Until  string `query:"until" doc:"RFC 3339: made before"`
			Ticket string `query:"ticket" maxLength:"200" doc:"Only requests about this ticket: its id, its number, or the reference the path used"`
		}) (*struct{ Body httpx.Page[APIKeyRequest] }, error) {
			p, err := requireUserWorkspace(ctx, auth.RoleAdmin)
			if err != nil {
				return nil, httpx.Wrap(err)
			}
			id, err := requestKey(ctx, s.d.DB.Pool, p.WorkspaceID, in.ID)
			if err != nil {
				return nil, err
			}
			since, until, err := timeRange(in.Since, in.Until)
			if err != nil {
				return nil, err
			}
			var cur struct {
				T  time.Time `json:"t"`
				ID uuid.UUID `json:"id"`
			}
			if err := httpx.DecodeCursor(in.Cursor, &cur); err != nil {
				return nil, err
			}
			if cur.T.IsZero() {
				cur.T = time.Now().Add(time.Hour)
			}
			lo, hi := 0, 1000
			if in.Status == "errors" {
				lo = 400
			} else if in.Status != "" {
				lo = int(in.Status[0]-'0') * 100
				hi = lo + 100
			}
			ticket := strings.TrimSpace(in.Ticket)
			rows, err := s.d.DB.Pool.Query(ctx, `SELECT `+apiKeyRequestCols+apiKeyRequestFrom+`
				WHERE r.workspace_id = $1 AND r.api_key_id = $2 AND (r.created_at, r.id) < ($3, $4)
				AND r.status >= $5 AND r.status < $6 AND ($7 = '' OR r.method = $7)
				AND ($8::timestamptz IS NULL OR r.created_at >= $8) AND ($9::timestamptz IS NULL OR r.created_at < $9)
				AND ($10 = '' OR r.ticket_ref = $10 OR r.ticket_id::text = $10
					OR r.ticket_id IN (SELECT id FROM tickets WHERE workspace_id = $1 AND number::text = $10))
				ORDER BY r.created_at DESC, r.id DESC LIMIT $11`,
				p.WorkspaceID, id, cur.T, cur.ID, lo, hi, in.Method, since, until, ticket, in.Limit+1)
			if err != nil {
				return nil, httpx.Wrap(err)
			}
			list, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (APIKeyRequest, error) { return scanAPIKeyRequest(r) })
			if err != nil {
				return nil, httpx.Wrap(err)
			}
			return &struct{ Body httpx.Page[APIKeyRequest] }{httpx.NewPage(list, in.Limit, func(x APIKeyRequest) any {
				return map[string]any{"t": x.CreatedAt, "id": x.ID}
			})}, nil
		})

	register(s, api, huma.Operation{OperationID: "get-api-key-request", Method: http.MethodGet, Path: "/v1/api-keys/{id}/requests/{requestId}",
		Summary: "Get an API key's recorded request", Tags: tags,
		Description: "One recorded request with the JSON it sent and the answer to a failure. Admin users only."},
		func(ctx context.Context, in *struct {
			ID        string `path:"id"`
			RequestID string `path:"requestId"`
		}) (*struct{ Body APIKeyRequestDetail }, error) {
			p, err := requireUserWorkspace(ctx, auth.RoleAdmin)
			if err != nil {
				return nil, httpx.Wrap(err)
			}
			id, err := requestKey(ctx, s.d.DB.Pool, p.WorkspaceID, in.ID)
			if err != nil {
				return nil, err
			}
			rid, err := parseID(in.RequestID, "Request")
			if err != nil {
				return nil, err
			}
			var d APIKeyRequestDetail
			d.APIKeyRequest, err = scanAPIKeyRequest(s.d.DB.Pool.QueryRow(ctx, `SELECT `+apiKeyRequestCols+`,
				r.request_id, r.ip, r.user_agent, r.content_type, r.request_size, r.request_body, r.request_truncated, r.response_body, r.response_truncated`+
				apiKeyRequestFrom+` WHERE r.workspace_id = $1 AND r.api_key_id = $2 AND r.id = $3`, p.WorkspaceID, id, rid),
				&d.RequestID, &d.IP, &d.UserAgent, &d.ContentType, &d.RequestSize, &d.RequestBody, &d.RequestTruncated, &d.ResponseBody, &d.ResponseTruncated)
			if db.IsNotFound(err) {
				return nil, httpx.NotFound("Request")
			}
			if err != nil {
				return nil, httpx.Wrap(err)
			}
			return &struct{ Body APIKeyRequestDetail }{d}, nil
		})
}

// timeRange parses optional RFC 3339 bounds.
func timeRange(since, until string) (*time.Time, *time.Time, error) {
	var out [2]*time.Time
	for i, v := range []string{since, until} {
		if v == "" {
			continue
		}
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			return nil, nil, httpx.Validation("since and until are RFC 3339 times, such as 2026-11-02T09:00:00Z.")
		}
		out[i] = &t
	}
	return out[0], out[1], nil
}
