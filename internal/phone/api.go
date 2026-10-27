package phone

import (
	"context"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/useopendesk/opendesk/internal/audit"
	"github.com/useopendesk/opendesk/internal/auth"
	"github.com/useopendesk/opendesk/internal/db"
	"github.com/useopendesk/opendesk/internal/displayname"
	"github.com/useopendesk/opendesk/internal/events"
	"github.com/useopendesk/opendesk/internal/httpx"
	"github.com/useopendesk/opendesk/internal/ids"
	"github.com/useopendesk/opendesk/internal/phone/provider"
)

// Health codes the settings page words in the reader's language (health_code).
const (
	HealthReceiving   = "receiving"           // the webhook is registered and active
	HealthNoWebhook   = "webhook_missing"     // the provider has no webhook for opendesk (or disabled it)
	HealthRefused     = "credentials_refused" // the provider refused the credentials
	HealthUnreachable = "provider_unreachable"
)

var healthDetails = map[string]string{
	HealthReceiving:   "Calls are sent to opendesk.",
	HealthNoWebhook:   "The provider is not sending calls to opendesk. Test the connection to register the webhook again.",
	HealthRefused:     "The provider refused the credentials. Replace them with working ones.",
	HealthUnreachable: "Couldn't reach the provider. Try again in a moment.",
}

// PhoneProvider is a provider the instance can connect.
type PhoneProvider struct {
	Key    string               `json:"key"`
	Name   string               `json:"name"`
	Fields []PhoneProviderField `json:"fields"`
}

// PhoneProviderField is a credential the admin pastes.
type PhoneProviderField struct {
	Name   string `json:"name"`
	Label  string `json:"label"`
	Secret bool   `json:"secret"`
}

// PhoneConnectionView is a connection with the code its health detail is worded by.
type PhoneConnectionView struct {
	PhoneConnection
	HealthCode string `json:"health_code" enum:",receiving,webhook_missing,credentials_refused,provider_unreachable"`
}

type phoneConnectionList struct {
	Data []PhoneConnectionView `json:"data"`
}

// Register mounts the phone API and the providers' webhook.
func (s *Service) Register(api huma.API, r chi.Router) {
	tags := []string{"Phone"}
	huma.Register(api, huma.Operation{OperationID: "list-phone-providers", Method: http.MethodGet, Path: "/v1/phone/providers", Tags: tags, Summary: "List phone providers"},
		func(ctx context.Context, _ *struct{}) (*struct {
			Body struct {
				Data []PhoneProvider `json:"data"`
			}
		}, error) {
			if _, err := auth.RequireWorkspace(ctx, auth.RoleViewer, auth.ScopeSettingsRead); err != nil {
				return nil, httpx.Wrap(err)
			}
			out := &struct {
				Body struct {
					Data []PhoneProvider `json:"data"`
				}
			}{}
			out.Body.Data = []PhoneProvider{}
			for _, d := range s.descriptors() {
				p := PhoneProvider{Key: d.Key, Name: d.Name, Fields: []PhoneProviderField{}}
				for _, f := range d.Fields {
					p.Fields = append(p.Fields, PhoneProviderField{Name: f.Name, Label: f.Label, Secret: f.Secret})
				}
				out.Body.Data = append(out.Body.Data, p)
			}
			return out, nil
		})

	huma.Register(api, huma.Operation{OperationID: "list-phone-connections", Method: http.MethodGet, Path: "/v1/phone/connections", Tags: tags, Summary: "List phone connections"},
		func(ctx context.Context, _ *struct{}) (*struct{ Body phoneConnectionList }, error) {
			p, err := auth.RequireWorkspace(ctx, auth.RoleViewer, auth.ScopeSettingsRead)
			if err != nil {
				return nil, httpx.Wrap(err)
			}
			list, err := s.listConnections(ctx, p.WorkspaceID)
			if err != nil {
				return nil, httpx.Wrap(err)
			}
			out := &struct{ Body phoneConnectionList }{phoneConnectionList{Data: []PhoneConnectionView{}}}
			for _, c := range list {
				out.Body.Data = append(out.Body.Data, s.view(c, seesWebhook(p)))
			}
			return out, nil
		})

	huma.Register(api, huma.Operation{OperationID: "create-phone-connection", Method: http.MethodPost, Path: "/v1/phone/connections", Tags: tags, Summary: "Connect a phone provider", DefaultStatus: http.StatusCreated},
		func(ctx context.Context, in *struct {
			Body struct {
				Provider    string            `json:"provider"`
				Name        string            `json:"name,omitempty" maxLength:"80"`
				Credentials map[string]string `json:"credentials"`
			}
		}) (*struct{ Body PhoneConnectionView }, error) {
			p, err := requireAdminUser(ctx)
			if err != nil {
				return nil, err
			}
			a, ok := s.adapters[strings.TrimSpace(in.Body.Provider)]
			if !ok {
				return nil, httpx.Validation("Unknown phone provider.")
			}
			creds := cleanCredentials(a.Describe(), in.Body.Credentials)
			if creds == nil {
				return nil, httpx.Validation("Provide every required credential.")
			}
			account, err := a.Validate(ctx, s.http, creds)
			if err != nil {
				s.d.Log.Info("phone connect refused", "provider", a.Describe().Key, "err", err)
				return nil, httpx.E(422, "phone_credentials_refused", "The provider refused these credentials.")
			}
			sealed, err := s.d.Box.SealJSON(creds)
			if err != nil {
				return nil, httpx.Wrap(err)
			}
			name := strings.TrimSpace(in.Body.Name)
			if name == "" {
				name = displayname.CleanOr(account.Name, displayname.Workspace, a.Describe().Name)
			} else if name, err = displayname.Validate("Name", name, displayname.Workspace, false); err != nil {
				return nil, httpx.Wrap(err)
			}
			c := PhoneConnection{ID: ids.New(), WorkspaceID: p.WorkspaceID, Provider: a.Describe().Key, Name: name,
				AccountName: displayname.CleanOr(account.Name, displayname.Workspace, ""), Status: "connected", HealthStatus: "unknown",
				ReuseDays: 7, InsightCards: true, CreatedAt: time.Now().UTC(), credentialsEnc: sealed}
			err = s.d.DB.Tx(ctx, func(tx pgx.Tx) error {
				if _, err := tx.Exec(ctx, `INSERT INTO phone_connections (id, workspace_id, provider, name, account_name, credentials_enc, created_by, created_at)
					VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`, c.ID, c.WorkspaceID, c.Provider, c.Name, c.AccountName, c.credentialsEnc, p.UserID, c.CreatedAt); err != nil {
					return err
				}
				if _, err := events.Publish(ctx, tx, events.Event{WorkspaceID: p.WorkspaceID, Type: "phone_connection.created", Admins: true,
					Data: map[string]any{"phone_connection": s.view(c, true)}}); err != nil {
					return err
				}
				return audit.Log(ctx, tx, p, p.WorkspaceID, "phone.connected", "phone_connection", &c.ID, map[string]any{"provider": c.Provider})
			})
			if db.IsUniqueViolation(err, "") {
				return nil, httpx.Conflict("This workspace is already connected to that provider.")
			}
			if err != nil {
				return nil, httpx.Wrap(err)
			}
			c = s.reconcile(ctx, c, creds)
			return &struct{ Body PhoneConnectionView }{s.view(c, true)}, nil
		})

	huma.Register(api, huma.Operation{OperationID: "update-phone-connection", Method: http.MethodPatch, Path: "/v1/phone/connections/{id}", Tags: tags, Summary: "Update a phone connection"},
		func(ctx context.Context, in *struct {
			ID   string `path:"id"`
			Body struct {
				Name           *string           `json:"name,omitempty" maxLength:"80"`
				ReuseDays      *int              `json:"reuse_days,omitempty" minimum:"0" maximum:"90" doc:"A call goes on the caller's latest solved ticket updated within this many days when they have no open one; 0 opens a new ticket instead"`
				AssignAnswered *bool             `json:"assign_answered,omitempty" doc:"An answered call assigns an unassigned ticket to the member who took it"`
				InsightCards   *bool             `json:"insight_cards,omitempty" doc:"A ringing call shows the caller and their tickets in the provider's app"`
				Credentials    map[string]string `json:"credentials,omitempty" doc:"Replacement credentials; the webhook is checked with them"`
			}
		}) (*struct{ Body PhoneConnectionView }, error) {
			p, err := requireAdminUser(ctx)
			if err != nil {
				return nil, err
			}
			c, err := s.workspaceConnection(ctx, p.WorkspaceID, in.ID)
			if err != nil {
				return nil, err
			}
			b := in.Body
			changes := map[string]any{}
			if b.Name != nil {
				n, err := displayname.Validate("Name", strings.TrimSpace(*b.Name), displayname.Workspace, false)
				if err != nil {
					return nil, httpx.Wrap(err)
				}
				c.Name, changes["name"] = n, n
			}
			if b.ReuseDays != nil {
				c.ReuseDays, changes["reuse_days"] = *b.ReuseDays, *b.ReuseDays
			}
			if b.AssignAnswered != nil {
				c.AssignAnswered, changes["assign_answered"] = *b.AssignAnswered, *b.AssignAnswered
			}
			if b.InsightCards != nil {
				c.InsightCards, changes["insight_cards"] = *b.InsightCards, *b.InsightCards
			}
			var creds map[string]string
			if len(b.Credentials) > 0 {
				a, ok := s.adapters[c.Provider]
				if !ok {
					return nil, httpx.Validation("Unknown phone provider.")
				}
				if creds = cleanCredentials(a.Describe(), b.Credentials); creds == nil {
					return nil, httpx.Validation("Provide every required credential.")
				}
				if _, err := a.Validate(ctx, s.http, creds); err != nil {
					return nil, httpx.E(422, "phone_credentials_refused", "The provider refused these credentials.")
				}
				if c.credentialsEnc, err = s.d.Box.SealJSON(creds); err != nil {
					return nil, httpx.Wrap(err)
				}
				changes["credentials"] = "replaced"
			}
			err = s.d.DB.Tx(ctx, func(tx pgx.Tx) error {
				tag, err := tx.Exec(ctx, `UPDATE phone_connections SET name = $3, reuse_days = $4, assign_answered = $5, insight_cards = $6,
					credentials_enc = $7, updated_at = now() WHERE workspace_id = $1 AND id = $2`,
					p.WorkspaceID, c.ID, c.Name, c.ReuseDays, c.AssignAnswered, c.InsightCards, c.credentialsEnc)
				if err != nil {
					return err
				}
				if tag.RowsAffected() == 0 {
					return pgx.ErrNoRows
				}
				if _, err := events.Publish(ctx, tx, events.Event{WorkspaceID: p.WorkspaceID, Type: "phone_connection.updated", Admins: true,
					Data: map[string]any{"phone_connection": s.view(c, true)}}); err != nil {
					return err
				}
				action := "phone.updated"
				if creds != nil {
					action = "phone.credentials_rotated"
				}
				return audit.Log(ctx, tx, p, p.WorkspaceID, action, "phone_connection", &c.ID, changes)
			})
			if db.IsNotFound(err) {
				return nil, httpx.NotFound("Phone connection")
			}
			if err != nil {
				return nil, httpx.Wrap(err)
			}
			if creds != nil {
				c = s.reconcile(ctx, c, creds)
			}
			return &struct{ Body PhoneConnectionView }{s.view(c, true)}, nil
		})

	huma.Register(api, huma.Operation{OperationID: "test-phone-connection", Method: http.MethodPost, Path: "/v1/phone/connections/{id}/test", Tags: tags,
		Summary: "Test and repair a phone connection", Description: "Checks the credentials and the provider's webhook, and registers the webhook again when the provider no longer has it."},
		func(ctx context.Context, in *struct {
			ID string `path:"id"`
		}) (*struct{ Body PhoneConnectionView }, error) {
			p, err := requireAdminUser(ctx)
			if err != nil {
				return nil, err
			}
			c, err := s.workspaceConnection(ctx, p.WorkspaceID, in.ID)
			if err != nil {
				return nil, err
			}
			creds, err := s.credentials(c)
			if err != nil {
				return nil, httpx.Wrap(err)
			}
			c = s.reconcile(ctx, c, creds)
			return &struct{ Body PhoneConnectionView }{s.view(c, true)}, nil
		})

	huma.Register(api, huma.Operation{OperationID: "delete-phone-connection", Method: http.MethodDelete, Path: "/v1/phone/connections/{id}", Tags: tags,
		Summary: "Disconnect a phone provider", Description: "Removes opendesk's webhook from the provider and the connection. Calls already on tickets stay.", DefaultStatus: http.StatusNoContent},
		func(ctx context.Context, in *struct {
			ID string `path:"id"`
		}) (*struct{}, error) {
			p, err := requireAdminUser(ctx)
			if err != nil {
				return nil, err
			}
			c, err := s.workspaceConnection(ctx, p.WorkspaceID, in.ID)
			if err != nil {
				return nil, err
			}
			if a, ok := s.adapters[c.Provider]; ok && c.webhookID != "" {
				if creds, err := s.credentials(c); err == nil {
					if err := a.DeleteWebhook(ctx, s.http, creds, c.webhookID); err != nil {
						// The webhook stays at the provider, which disables it once its events are refused.
						s.d.Log.Warn("phone webhook not removed", "provider", c.Provider, "connection_id", c.ID, "err", err)
					}
				}
			}
			err = s.d.DB.Tx(ctx, func(tx pgx.Tx) error {
				tag, err := tx.Exec(ctx, `DELETE FROM phone_connections WHERE workspace_id = $1 AND id = $2`, p.WorkspaceID, c.ID)
				if err != nil {
					return err
				}
				if tag.RowsAffected() == 0 {
					return pgx.ErrNoRows
				}
				if _, err := events.Publish(ctx, tx, events.Event{WorkspaceID: p.WorkspaceID, Type: "phone_connection.deleted", Admins: true,
					Data: map[string]any{"phone_connection_id": c.ID}}); err != nil {
					return err
				}
				return audit.Log(ctx, tx, p, p.WorkspaceID, "phone.disconnected", "phone_connection", &c.ID, map[string]any{"provider": c.Provider})
			})
			if db.IsNotFound(err) {
				return nil, httpx.NotFound("Phone connection")
			}
			if err != nil {
				return nil, httpx.Wrap(err)
			}
			return &struct{}{}, nil
		})

	huma.Register(api, huma.Operation{OperationID: "list-phone-lines", Method: http.MethodGet, Path: "/v1/phone/lines", Tags: tags,
		Summary: "List phone lines", Description: "The numbers of every connected provider account, with the lines calls already came in on, for automation conditions."},
		func(ctx context.Context, _ *struct{}) (*struct {
			Body struct {
				Data []PhoneLine `json:"data"`
			}
		}, error) {
			p, err := auth.RequireWorkspace(ctx, auth.RoleAdmin, auth.ScopeSettingsRead)
			if err != nil {
				return nil, httpx.Wrap(err)
			}
			lines, err := s.lines(ctx, p.WorkspaceID)
			if err != nil {
				return nil, httpx.Wrap(err)
			}
			out := &struct {
				Body struct {
					Data []PhoneLine `json:"data"`
				}
			}{}
			out.Body.Data = lines
			return out, nil
		})

	r.Post("/v1/phone/hooks/{provider}/{id}", s.webhook)
}

// PhoneLine is a provider number calls come in on.
type PhoneLine struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Provider string `json:"provider"`
}

// lines lists each connection's numbers from the provider, and the lines seen on calls,
// so the list still shows them when the provider can't be reached.
func (s *Service) lines(ctx context.Context, wid uuid.UUID) ([]PhoneLine, error) {
	out := []PhoneLine{}
	seen := map[string]bool{}
	add := func(l PhoneLine) {
		key := l.Provider + "\x00" + strings.ToLower(l.Name)
		if l.Name != "" && !seen[key] {
			seen[key] = true
			out = append(out, l)
		}
	}
	conns, err := s.listConnections(ctx, wid)
	if err != nil {
		return nil, err
	}
	for _, c := range conns {
		a, ok := s.adapters[c.Provider]
		if !ok {
			continue
		}
		creds, err := s.credentials(c)
		if err != nil {
			return nil, err
		}
		list, err := a.Lines(ctx, s.http, creds)
		if err != nil {
			s.d.Log.Info("phone lines not listed", "provider", c.Provider, "connection_id", c.ID, "err", err)
			continue
		}
		for _, l := range list {
			add(PhoneLine{ID: l.ID, Name: displayname.CleanOr(l.Name, displayname.Workspace, l.Name), Provider: c.Provider})
		}
	}
	rows, err := s.d.DB.Pool.Query(ctx, `SELECT DISTINCT ON (provider, lower(line_name)) line_id, line_name, provider FROM phone_calls
		WHERE workspace_id = $1 AND line_name <> '' ORDER BY provider, lower(line_name), created_at DESC LIMIT 200`, wid)
	if err != nil {
		return nil, err
	}
	seenRows, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (PhoneLine, error) {
		var l PhoneLine
		return l, r.Scan(&l.ID, &l.Name, &l.Provider)
	})
	if err != nil {
		return nil, err
	}
	for _, l := range seenRows {
		add(l)
	}
	sort.Slice(out, func(i, j int) bool { return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name) })
	return out, nil
}

// requireAdminUser admits a workspace admin signed in as a person: connecting a phone
// system hands it a way into tickets, so API keys can't.
func requireAdminUser(ctx context.Context) (*auth.Principal, error) {
	p, err := auth.RequireWorkspace(ctx, auth.RoleAdmin, auth.ScopeSettingsWrite)
	if err != nil {
		return nil, httpx.Wrap(err)
	}
	if p.Kind != auth.KindUser {
		return nil, httpx.Forbidden()
	}
	return p, nil
}

func cleanCredentials(d provider.Descriptor, in map[string]string) map[string]string {
	out := map[string]string{}
	for _, f := range d.Fields {
		v := strings.TrimSpace(in[f.Name])
		if v == "" || len(v) > 500 {
			return nil
		}
		out[f.Name] = v
	}
	return out
}

func (s *Service) webhookURL(c PhoneConnection) string {
	return strings.TrimRight(s.d.Cfg.APIURL, "/") + "/v1/phone/hooks/" + c.Provider + "/" + c.ID.String()
}

// seesWebhook reports whether p may read a connection's webhook URL: an admin, or an API
// key or AI client that may change settings.
func seesWebhook(p *auth.Principal) bool {
	switch p.Kind {
	case auth.KindAPIKey:
		return p.HasScope(auth.ScopeSettingsWrite)
	case auth.KindUser:
		return p.Role.AtLeast(auth.RoleAdmin) && (p.GrantID == uuid.Nil || p.HasScope(auth.ScopeSettingsWrite))
	}
	return false
}

// view is a connection as a response or an event carries it, with its webhook URL only
// for admins.
func (s *Service) view(c PhoneConnection, admin bool) PhoneConnectionView {
	c.WebhookURL = ""
	if admin {
		c.WebhookURL = s.webhookURL(c)
	}
	v := PhoneConnectionView{PhoneConnection: c}
	for code, detail := range healthDetails {
		if detail == c.HealthDetail {
			v.HealthCode = code
		}
	}
	return v
}

const connectionSelect = `SELECT id, workspace_id, provider, name, account_name, credentials_enc, webhook_id, webhook_secret_enc,
	status, last_error, health_status, health_detail, health_checked_at, reuse_days, assign_answered, insight_cards, last_event_at, created_at
	FROM phone_connections`

func scanConnection(row pgx.Row) (PhoneConnection, error) {
	var c PhoneConnection
	err := row.Scan(&c.ID, &c.WorkspaceID, &c.Provider, &c.Name, &c.AccountName, &c.credentialsEnc, &c.webhookID, &c.webhookSecretEnc,
		&c.Status, &c.LastError, &c.HealthStatus, &c.HealthDetail, &c.HealthCheckedAt, &c.ReuseDays, &c.AssignAnswered, &c.InsightCards,
		&c.LastEventAt, &c.CreatedAt)
	return c, err
}

func (s *Service) listConnections(ctx context.Context, wid uuid.UUID) ([]PhoneConnection, error) {
	rows, err := s.d.DB.Pool.Query(ctx, connectionSelect+` WHERE workspace_id = $1 ORDER BY created_at`, wid)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (PhoneConnection, error) { return scanConnection(r) })
}

func (s *Service) workspaceConnection(ctx context.Context, wid uuid.UUID, raw string) (PhoneConnection, error) {
	id, err := uuid.Parse(raw)
	if err != nil {
		return PhoneConnection{}, httpx.NotFound("Phone connection")
	}
	c, err := scanConnection(s.d.DB.Pool.QueryRow(ctx, connectionSelect+` WHERE workspace_id = $1 AND id = $2`, wid, id))
	if db.IsNotFound(err) {
		return c, httpx.NotFound("Phone connection")
	}
	if err != nil {
		return c, httpx.Wrap(err)
	}
	return c, nil
}

func (s *Service) credentials(c PhoneConnection) (map[string]string, error) {
	out := map[string]string{}
	return out, s.d.Box.OpenJSON(c.credentialsEnc, &out)
}

// reconcile checks the credentials and the provider's webhook, registers the webhook when
// the provider has none for this connection, and stores the outcome as the health.
func (s *Service) reconcile(ctx context.Context, c PhoneConnection, creds map[string]string) PhoneConnection {
	a, ok := s.adapters[c.Provider]
	if !ok {
		return s.storeHealth(ctx, c, "error", HealthUnreachable)
	}
	account, err := a.Validate(ctx, s.http, creds)
	if err != nil {
		s.d.Log.Info("phone credentials check failed", "provider", c.Provider, "connection_id", c.ID, "err", err)
		if provider.Refused(err) {
			return s.storeHealth(ctx, c, "error", HealthRefused)
		}
		return s.storeHealth(ctx, c, "error", HealthUnreachable)
	}
	if name := displayname.CleanOr(account.Name, displayname.Workspace, ""); name != "" && name != c.AccountName {
		c.AccountName = name
		if _, err := s.d.DB.Pool.Exec(ctx, `UPDATE phone_connections SET account_name = $3 WHERE workspace_id = $1 AND id = $2`, c.WorkspaceID, c.ID, name); err != nil {
			s.d.Log.Warn("phone account name not stored", "connection_id", c.ID, "err", err)
		}
	}
	if c.webhookID != "" {
		active, err := a.WebhookActive(ctx, s.http, creds, c.webhookID)
		if err != nil {
			s.d.Log.Info("phone webhook check failed", "provider", c.Provider, "connection_id", c.ID, "err", err)
			return s.storeHealth(ctx, c, "error", HealthUnreachable)
		}
		if active {
			return s.storeHealth(ctx, c, "healthy", HealthReceiving)
		}
		// Gone or disabled by the provider: replace it.
		if err := a.DeleteWebhook(ctx, s.http, creds, c.webhookID); err != nil {
			s.d.Log.Info("phone webhook not removed", "provider", c.Provider, "connection_id", c.ID, "err", err)
		}
	}
	hook, err := a.RegisterWebhook(ctx, s.http, creds, s.webhookURL(c))
	if err != nil {
		s.d.Log.Warn("phone webhook not registered", "provider", c.Provider, "connection_id", c.ID, "err", err)
		return s.storeHealth(ctx, c, "action_required", HealthNoWebhook)
	}
	secret, err := s.d.Box.Seal([]byte(hook.Secret))
	if err != nil {
		s.d.Log.Error("phone webhook secret not sealed", "connection_id", c.ID, "err", err)
		return s.storeHealth(ctx, c, "error", HealthUnreachable)
	}
	if _, err := s.d.DB.Pool.Exec(ctx, `UPDATE phone_connections SET webhook_id = $3, webhook_secret_enc = $4, updated_at = now()
		WHERE workspace_id = $1 AND id = $2`, c.WorkspaceID, c.ID, hook.ID, secret); err != nil {
		s.d.Log.Error("phone webhook not stored", "connection_id", c.ID, "err", err)
		return s.storeHealth(ctx, c, "error", HealthUnreachable)
	}
	c.webhookID, c.webhookSecretEnc = hook.ID, secret
	return s.storeHealth(ctx, c, "healthy", HealthReceiving)
}

func (s *Service) storeHealth(ctx context.Context, c PhoneConnection, status, code string) PhoneConnection {
	now := time.Now().UTC()
	c.HealthStatus, c.HealthDetail, c.HealthCheckedAt = status, healthDetails[code], &now
	c.Status, c.LastError = "connected", ""
	if status == "error" {
		c.Status = "error"
	}
	err := s.d.DB.Tx(ctx, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `UPDATE phone_connections SET status = $3, last_error = '', health_status = $4, health_detail = $5,
			health_checked_at = $6, updated_at = now() WHERE workspace_id = $1 AND id = $2`, c.WorkspaceID, c.ID, c.Status, status, c.HealthDetail, now)
		if err != nil || tag.RowsAffected() == 0 {
			return err
		}
		_, err = events.Publish(ctx, tx, events.Event{WorkspaceID: c.WorkspaceID, Type: "phone_connection.updated", Admins: true,
			Data: map[string]any{"phone_connection": s.view(c, true)}})
		return err
	})
	if err != nil {
		s.d.Log.Warn("phone health not stored", "connection_id", c.ID, "err", err)
	}
	return c
}
