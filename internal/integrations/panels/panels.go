// Package panels is the ticket panels of the integrations area (contracts/integrations.md
// "Ticket panels"): read-only data about a ticket's contact, fetched from another product
// when the ticket is open and shown beside it. A panel is a kind of source:
//
//   - shopify: the contact's customer and last orders in a Shopify store, by email
//     (Admin GraphQL API, a custom app's token with read_customers and read_orders);
//   - http: a URL of the workspace's own that OpenDesk posts the contact and ticket to,
//     signed like webhooks, answering sections of labelled values (an ERP, billing, a CRM).
//
// Credentials and signing secrets are sealed with d.Box and never leave the server.
// Panels never write anywhere and never block the ticket: a failure is a quiet state.
package panels

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/useopendesk/opendesk/internal/audit"
	"github.com/useopendesk/opendesk/internal/auth"
	"github.com/useopendesk/opendesk/internal/crypto"
	"github.com/useopendesk/opendesk/internal/db"
	"github.com/useopendesk/opendesk/internal/deps"
	"github.com/useopendesk/opendesk/internal/displayname"
	"github.com/useopendesk/opendesk/internal/domain"
	"github.com/useopendesk/opendesk/internal/events"
	"github.com/useopendesk/opendesk/internal/httpx"
	"github.com/useopendesk/opendesk/internal/ids"
	"github.com/useopendesk/opendesk/internal/netsafe"
)

const (
	// KindShopify and KindHTTP are the panel sources.
	KindShopify = "shopify"
	KindHTTP    = "http"

	// maxPerWorkspace bounds a workspace's panels.
	maxPerWorkspace = 10
	// fetchTimeout is the whole budget of one lookup: the ticket never waits longer.
	fetchTimeout = 3 * time.Second
	// cacheTTL keeps an answer for a ticket opened again soon; errTTL keeps a failure
	// shorter, so a product that is down is not asked again on every open.
	cacheTTL = time.Minute
	errTTL   = 15 * time.Second
	// lookupsPerMinute bounds the lookups one panel makes that the cache did not answer.
	lookupsPerMinute = 120
)

// Panel is a workspace's ticket panel as the API shows it. The token or signing secret
// never leaves the server, and an http panel shows its URL's host only, since a path can
// carry a secret.
type Panel struct {
	ID        uuid.UUID `json:"id"`
	Kind      string    `json:"kind" enum:"shopify,http"`
	Name      string    `json:"name"`
	Host      string    `json:"host" doc:"shopify: the shop's myshopify.com domain; http: the URL's host"`
	Enabled   bool      `json:"enabled"`
	Position  int       `json:"position"`
	Status    string    `json:"status" enum:"connected,error"`
	LastError string    `json:"last_error"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// row is a stored panel; target is the shop domain or the URL.
type row struct {
	ID          uuid.UUID
	WorkspaceID uuid.UUID
	Kind        string
	Name        string
	Target      string
	SecretEnc   []byte
	Enabled     bool
	Position    int
	Status      string
	LastError   string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

func (x *row) api() Panel {
	host := x.Target
	if x.Kind == KindHTTP {
		host = hostOf(x.Target)
	}
	return Panel{ID: x.ID, Kind: x.Kind, Name: x.Name, Host: host, Enabled: x.Enabled, Position: x.Position,
		Status: x.Status, LastError: x.LastError, CreatedAt: x.CreatedAt, UpdatedAt: x.UpdatedAt}
}

const rowSelect = `SELECT id, workspace_id, kind, name, target, secret_enc, enabled, position, status, last_error,
	created_at, updated_at FROM ticket_panels`

func scanRow(r pgx.Row) (*row, error) {
	var x row
	err := r.Scan(&x.ID, &x.WorkspaceID, &x.Kind, &x.Name, &x.Target, &x.SecretEnc, &x.Enabled, &x.Position, &x.Status,
		&x.LastError, &x.CreatedAt, &x.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &x, nil
}

func loadRow(ctx context.Context, q db.Querier, wid, id uuid.UUID) (*row, error) {
	return scanRow(q.QueryRow(ctx, rowSelect+` WHERE workspace_id = $1 AND id = $2`, wid, id))
}

func listRows(ctx context.Context, q db.Querier, wid uuid.UUID) ([]*row, error) {
	rows, err := q.Query(ctx, rowSelect+` WHERE workspace_id = $1 ORDER BY position, created_at`, wid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*row{}
	for rows.Next() {
		x, err := scanRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}

// Service holds the outbound client and the answers cache (one per process).
type Service struct {
	d     *deps.Deps
	http  *http.Client
	guard netsafe.Guard
	cache *answerCache
	// shopifyEndpoint is the Admin GraphQL URL of a shop; tests point it at a fake.
	shopifyEndpoint func(shop string) string
}

var (
	servicesMu sync.Mutex
	services   = map[*deps.Deps]*Service{}
)

// For returns the service for d (one per process).
func For(d *deps.Deps) *Service {
	servicesMu.Lock()
	defer servicesMu.Unlock()
	if s := services[d]; s != nil {
		return s
	}
	dev := !d.Cfg.IsProduction()
	s := &Service{d: d, http: netsafe.Client(dev, fetchTimeout), guard: netsafe.Guard{Dev: dev}, cache: newAnswerCache(1000),
		shopifyEndpoint: shopifyGraphQL}
	services[d] = s
	return s
}

// Register adds the ticket panel routes.
func Register(d *deps.Deps, api huma.API, _ chi.Router) {
	For(d).registerRoutes(api)
}

func hostOf(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	return u.Hostname()
}

func clip(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n-1]) + "…"
	}
	return s
}

func requireAdminUser(ctx context.Context) (*auth.Principal, error) {
	p, err := auth.RequireWorkspace(ctx, auth.RoleAdmin, auth.ScopeSettingsWrite)
	if err != nil {
		return nil, httpx.Wrap(err)
	}
	if p.Kind != auth.KindUser {
		return nil, httpx.Forbidden() // a person connects another product's credentials
	}
	return p, nil
}

func parseID(raw, what string) (uuid.UUID, error) {
	id, ok := ids.Parse(raw)
	if !ok {
		return uuid.Nil, httpx.NotFound(what)
	}
	return id, nil
}

func wrapLoad(err error) error {
	if db.IsNotFound(err) {
		return httpx.NotFound("Ticket panel")
	}
	return httpx.Wrap(err)
}

func (s *Service) publish(ctx context.Context, tx pgx.Tx, typ string, wid uuid.UUID, data map[string]any) error {
	_, err := events.Publish(ctx, tx, events.Event{WorkspaceID: wid, Type: typ, Data: data})
	return err
}

// newSecret is an http panel's signing secret, shown once.
func newSecret() string { return "pnsec_" + crypto.RandomToken(32) }

type panelBody struct {
	Body Panel
}

type createdBody struct {
	CacheControl string `header:"Cache-Control"`
	Body         struct {
		Panel  Panel  `json:"panel"`
		Secret string `json:"secret,omitempty" doc:"http panels: the signing secret, shown once"`
	}
}

func (s *Service) registerRoutes(api huma.API) {
	tags := []string{"Ticket panels"}

	huma.Register(api, huma.Operation{OperationID: "list-ticket-panels", Method: http.MethodGet, Path: "/v1/ticket-panels",
		Summary: "List the workspace's ticket panels", Tags: tags},
		func(ctx context.Context, _ *struct{}) (*struct {
			Body struct {
				Data []Panel `json:"data"`
			}
		}, error) {
			p, err := auth.RequireWorkspace(ctx, auth.RoleViewer, auth.ScopeTicketsRead)
			if err != nil {
				return nil, httpx.Wrap(err)
			}
			rows, err := listRows(ctx, s.d.DB.Pool, p.WorkspaceID)
			if err != nil {
				return nil, httpx.Wrap(err)
			}
			out := &struct {
				Body struct {
					Data []Panel `json:"data"`
				}
			}{}
			out.Body.Data = make([]Panel, 0, len(rows))
			for _, x := range rows {
				out.Body.Data = append(out.Body.Data, x.api())
			}
			return out, nil
		})

	huma.Register(api, huma.Operation{OperationID: "create-ticket-panel", Method: http.MethodPost, Path: "/v1/ticket-panels",
		Summary: "Add a ticket panel", Tags: tags, DefaultStatus: http.StatusCreated,
		Description: "`shopify` needs `shop` and `token` (checked with the shop first); `http` needs `url` and answers its signing `secret` once."},
		func(ctx context.Context, in *struct {
			Body struct {
				Kind  string `json:"kind" enum:"shopify,http"`
				Name  string `json:"name,omitempty" maxLength:"64"`
				Shop  string `json:"shop,omitempty" maxLength:"255" doc:"shopify: the shop's myshopify.com domain or handle"`
				Token string `json:"token,omitempty" maxLength:"4096" doc:"shopify: a custom app's Admin API access token"`
				URL   string `json:"url,omitempty" maxLength:"2000" doc:"http: the https URL OpenDesk posts to"`
			}
		}) (*createdBody, error) {
			p, err := requireAdminUser(ctx)
			if err != nil {
				return nil, err
			}
			if !s.d.Limiter.Allow("ticket-panel:create:"+p.WorkspaceID.String(), 30, time.Hour) {
				return nil, httpx.E(http.StatusTooManyRequests, "rate_limited", "Too many attempts. Try again later.")
			}
			b := in.Body
			x := &row{ID: ids.New(), WorkspaceID: p.WorkspaceID, Kind: b.Kind, Enabled: true, Status: "connected"}
			out := &createdBody{CacheControl: "no-store"}
			switch b.Kind {
			case KindShopify:
				if x.Target, err = normalizeShop(b.Shop); err != nil {
					return nil, httpx.Validation(err.Error())
				}
				token := strings.TrimSpace(b.Token)
				if token == "" || strings.ContainsAny(token, " \r\n\t") {
					return nil, httpx.Validation("Paste the Admin API access token.")
				}
				shopName, err := s.checkShopify(ctx, x.Target, token)
				if err != nil {
					return nil, httpx.E(422, "panel_failed", "Couldn't connect to Shopify: "+clip(err.Error(), 300))
				}
				x.Name = shopName
				if x.SecretEnc, err = s.d.Box.Seal([]byte(token)); err != nil {
					return nil, httpx.Wrap(err)
				}
			case KindHTTP:
				if x.Target, err = s.guard.ValidateURL(ctx, b.URL); err != nil {
					return nil, httpx.E(422, "invalid_url", err.Error())
				}
				x.Name = hostOf(x.Target)
				out.Body.Secret = newSecret()
				if x.SecretEnc, err = s.d.Box.Seal([]byte(out.Body.Secret)); err != nil {
					return nil, httpx.Wrap(err)
				}
			default:
				return nil, httpx.Validation("A panel is shopify or http.")
			}
			if strings.TrimSpace(b.Name) != "" {
				if x.Name, err = displayname.Validate("Name", b.Name, displayname.Workspace, false); err != nil {
					return nil, httpx.Wrap(err)
				}
			}
			if x.Name == "" {
				x.Name = map[string]string{KindShopify: "Shopify", KindHTTP: "Panel"}[x.Kind]
			}
			err = s.d.DB.Tx(ctx, func(tx pgx.Tx) error {
				var n int
				if err := tx.QueryRow(ctx, `SELECT count(*), COALESCE(max(position) + 1, 0) FROM ticket_panels WHERE workspace_id = $1`,
					p.WorkspaceID).Scan(&n, &x.Position); err != nil {
					return err
				}
				if n >= maxPerWorkspace {
					return httpx.Conflict("A workspace can have at most 10 ticket panels.")
				}
				if _, err := tx.Exec(ctx, `INSERT INTO ticket_panels (id, workspace_id, kind, name, target, secret_enc, position, created_by)
					VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`, x.ID, p.WorkspaceID, x.Kind, x.Name, x.Target, x.SecretEnc, x.Position,
					p.ActorUserID()); err != nil {
					return err
				}
				saved, err := loadRow(ctx, tx, p.WorkspaceID, x.ID)
				if err != nil {
					return err
				}
				*x = *saved
				if err := s.publish(ctx, tx, "ticket_panel.created", p.WorkspaceID, map[string]any{"panel": x.api()}); err != nil {
					return err
				}
				return audit.Log(ctx, tx, p, p.WorkspaceID, "ticket_panel.connected", "ticket_panel", &x.ID,
					map[string]any{"kind": x.Kind, "host": x.api().Host})
			})
			if err != nil {
				return nil, httpx.Wrap(err)
			}
			out.Body.Panel = x.api()
			return out, nil
		})

	huma.Register(api, huma.Operation{OperationID: "update-ticket-panel", Method: http.MethodPatch, Path: "/v1/ticket-panels/{id}",
		Summary: "Change a ticket panel", Tags: tags},
		func(ctx context.Context, in *struct {
			ID   string `path:"id"`
			Body struct {
				Name     *string `json:"name,omitempty" maxLength:"64"`
				Enabled  *bool   `json:"enabled,omitempty"`
				Position *int    `json:"position,omitempty" minimum:"0" maximum:"100"`
				Token    *string `json:"token,omitempty" maxLength:"4096" doc:"shopify: a new access token, checked with the shop first"`
				URL      *string `json:"url,omitempty" maxLength:"2000" doc:"http: a new URL"`
			}
		}) (*panelBody, error) {
			p, err := requireAdminUser(ctx)
			if err != nil {
				return nil, err
			}
			id, err := parseID(in.ID, "Ticket panel")
			if err != nil {
				return nil, err
			}
			x, err := loadRow(ctx, s.d.DB.Pool, p.WorkspaceID, id)
			if err != nil {
				return nil, wrapLoad(err)
			}
			b := in.Body
			changes := []string{}
			if b.Name != nil {
				if x.Name, err = displayname.Validate("Name", *b.Name, displayname.Workspace, false); err != nil {
					return nil, httpx.Wrap(err)
				}
				changes = append(changes, "name")
			}
			if b.Enabled != nil {
				x.Enabled = *b.Enabled
				changes = append(changes, "enabled")
			}
			if b.Position != nil {
				x.Position = *b.Position
				changes = append(changes, "position")
			}
			if b.Token != nil {
				if x.Kind != KindShopify {
					return nil, httpx.Validation("Only a Shopify panel has a token.")
				}
				token := strings.TrimSpace(*b.Token)
				if token == "" || strings.ContainsAny(token, " \r\n\t") {
					return nil, httpx.Validation("Paste the Admin API access token.")
				}
				if _, err := s.checkShopify(ctx, x.Target, token); err != nil {
					return nil, httpx.E(422, "panel_failed", "Couldn't connect to Shopify: "+clip(err.Error(), 300))
				}
				if x.SecretEnc, err = s.d.Box.Seal([]byte(token)); err != nil {
					return nil, httpx.Wrap(err)
				}
				x.Status, x.LastError = "connected", ""
				changes = append(changes, "token")
			}
			if b.URL != nil {
				if x.Kind != KindHTTP {
					return nil, httpx.Validation("Only an http panel has a URL.")
				}
				if x.Target, err = s.guard.ValidateURL(ctx, *b.URL); err != nil {
					return nil, httpx.E(422, "invalid_url", err.Error())
				}
				x.Status, x.LastError = "connected", ""
				changes = append(changes, "url")
			}
			err = s.d.DB.Tx(ctx, func(tx pgx.Tx) error {
				if _, err := tx.Exec(ctx, `UPDATE ticket_panels SET name = $3, enabled = $4, position = $5, target = $6,
					secret_enc = $7, status = $8, last_error = $9, updated_at = now() WHERE workspace_id = $1 AND id = $2`,
					p.WorkspaceID, id, x.Name, x.Enabled, x.Position, x.Target, x.SecretEnc, x.Status, x.LastError); err != nil {
					return err
				}
				saved, err := loadRow(ctx, tx, p.WorkspaceID, id)
				if err != nil {
					return err
				}
				*x = *saved
				if err := s.publish(ctx, tx, "ticket_panel.updated", p.WorkspaceID, map[string]any{"panel": x.api()}); err != nil {
					return err
				}
				return audit.Log(ctx, tx, p, p.WorkspaceID, "ticket_panel.updated", "ticket_panel", &x.ID,
					map[string]any{"kind": x.Kind, "host": x.api().Host, "changes": changes})
			})
			if err != nil {
				return nil, httpx.Wrap(err)
			}
			return &panelBody{x.api()}, nil
		})

	huma.Register(api, huma.Operation{OperationID: "rotate-ticket-panel-secret", Method: http.MethodPost,
		Path: "/v1/ticket-panels/{id}/rotate-secret", Summary: "Replace an http panel's signing secret", Tags: tags},
		func(ctx context.Context, in *struct {
			ID string `path:"id"`
		}) (*createdBody, error) {
			p, err := requireAdminUser(ctx)
			if err != nil {
				return nil, err
			}
			id, err := parseID(in.ID, "Ticket panel")
			if err != nil {
				return nil, err
			}
			x, err := loadRow(ctx, s.d.DB.Pool, p.WorkspaceID, id)
			if err != nil {
				return nil, wrapLoad(err)
			}
			if x.Kind != KindHTTP {
				return nil, httpx.Validation("Only an http panel has a signing secret.")
			}
			out := &createdBody{CacheControl: "no-store"}
			out.Body.Secret = newSecret()
			sealed, err := s.d.Box.Seal([]byte(out.Body.Secret))
			if err != nil {
				return nil, httpx.Wrap(err)
			}
			err = s.d.DB.Tx(ctx, func(tx pgx.Tx) error {
				if _, err := tx.Exec(ctx, `UPDATE ticket_panels SET secret_enc = $3, updated_at = now() WHERE workspace_id = $1 AND id = $2`,
					p.WorkspaceID, id, sealed); err != nil {
					return err
				}
				saved, err := loadRow(ctx, tx, p.WorkspaceID, id)
				if err != nil {
					return err
				}
				*x = *saved
				if err := s.publish(ctx, tx, "ticket_panel.updated", p.WorkspaceID, map[string]any{"panel": x.api()}); err != nil {
					return err
				}
				return audit.Log(ctx, tx, p, p.WorkspaceID, "ticket_panel.updated", "ticket_panel", &x.ID,
					map[string]any{"kind": x.Kind, "host": x.api().Host, "changes": []string{"secret"}})
			})
			if err != nil {
				return nil, httpx.Wrap(err)
			}
			out.Body.Panel = x.api()
			return out, nil
		})

	huma.Register(api, huma.Operation{OperationID: "delete-ticket-panel", Method: http.MethodDelete, Path: "/v1/ticket-panels/{id}",
		Summary: "Remove a ticket panel", Tags: tags, DefaultStatus: http.StatusNoContent},
		func(ctx context.Context, in *struct {
			ID string `path:"id"`
		}) (*struct{}, error) {
			p, err := requireAdminUser(ctx)
			if err != nil {
				return nil, err
			}
			id, err := parseID(in.ID, "Ticket panel")
			if err != nil {
				return nil, err
			}
			err = s.d.DB.Tx(ctx, func(tx pgx.Tx) error {
				x, err := loadRow(ctx, tx, p.WorkspaceID, id)
				if err != nil {
					return wrapLoad(err)
				}
				if _, err := tx.Exec(ctx, `DELETE FROM ticket_panels WHERE workspace_id = $1 AND id = $2`, p.WorkspaceID, id); err != nil {
					return err
				}
				if err := s.publish(ctx, tx, "ticket_panel.deleted", p.WorkspaceID, map[string]any{"panel_id": id}); err != nil {
					return err
				}
				return audit.Log(ctx, tx, p, p.WorkspaceID, "ticket_panel.disconnected", "ticket_panel", &id,
					map[string]any{"kind": x.Kind, "host": x.api().Host})
			})
			if err != nil {
				return nil, httpx.Wrap(err)
			}
			s.cache.forget(id)
			return nil, nil
		})

	huma.Register(api, huma.Operation{OperationID: "get-ticket-panel", Method: http.MethodGet, Path: "/v1/tickets/{id}/panels/{panelId}",
		Summary: "Read a ticket panel for a ticket", Tags: []string{"Tickets"},
		Description: "What the panel's product knows about the ticket's contact. A failure is a `state`, never an error status: the ticket never waits on another product."},
		func(ctx context.Context, in *struct {
			ID      string `path:"id"`
			PanelID string `path:"panelId"`
			Refresh bool   `query:"refresh" doc:"Ask the product again rather than answer from the last minute's copy"`
		}) (*struct{ Body Data }, error) {
			p, err := auth.RequireWorkspace(ctx, auth.RoleViewer, auth.ScopeTicketsRead)
			if err != nil {
				return nil, httpx.Wrap(err)
			}
			tid, err := parseID(in.ID, "Ticket")
			if err != nil {
				return nil, err
			}
			pid, err := parseID(in.PanelID, "Ticket panel")
			if err != nil {
				return nil, err
			}
			t, err := domain.LoadVisibleTicket(ctx, s.d.DB.Pool, p, tid)
			if db.IsNotFound(err) {
				return nil, httpx.NotFound("Ticket")
			}
			if err != nil {
				return nil, httpx.Wrap(err)
			}
			x, err := loadRow(ctx, s.d.DB.Pool, p.WorkspaceID, pid)
			if err != nil {
				return nil, wrapLoad(err)
			}
			if !x.Enabled {
				return nil, httpx.NotFound("Ticket panel")
			}
			data := s.read(ctx, x, &t, in.Refresh)
			return &struct{ Body Data }{data}, nil
		})
}

// errAuth marks a refusal of the panel's credentials: the panel shows as needing attention
// in settings until a new token or URL is saved, or a lookup succeeds again.
var errAuth = errors.New("credentials refused")

// authError is a refusal of the stored credentials with the product's words.
type authError struct{ msg string }

func (e *authError) Error() string { return e.msg }
func (e *authError) Is(target error) bool {
	return target == errAuth
}

// markStatus records a refused credential, or a lookup that works again, publishing the
// change. Other failures (a timeout, an outage) leave the status as it is.
func (s *Service) markStatus(ctx context.Context, x *row, lookupErr error) {
	status, last := "connected", ""
	switch {
	case errors.Is(lookupErr, errAuth):
		status, last = "error", clip(lookupErr.Error(), 300)
	case lookupErr != nil:
		return
	}
	if status == x.Status && last == x.LastError {
		return
	}
	err := s.d.DB.Tx(ctx, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `UPDATE ticket_panels SET status = $3, last_error = $4 WHERE workspace_id = $1 AND id = $2
			AND (status <> $3 OR last_error <> $4)`, x.WorkspaceID, x.ID, status, last)
		if err != nil || tag.RowsAffected() == 0 {
			return err
		}
		x.Status, x.LastError = status, last
		return s.publish(ctx, tx, "ticket_panel.updated", x.WorkspaceID, map[string]any{"panel": x.api()})
	})
	if err != nil {
		s.d.Log.Warn("ticket panel status", "panel", x.ID, "err", err)
	}
}
