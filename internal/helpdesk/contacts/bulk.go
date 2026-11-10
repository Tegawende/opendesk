package contacts

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	"github.com/danielgtaylor/huma/v2"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/useopendesk/opendesk/internal/audit"
	"github.com/useopendesk/opendesk/internal/auth"
	"github.com/useopendesk/opendesk/internal/deps"
	"github.com/useopendesk/opendesk/internal/domain"
	"github.com/useopendesk/opendesk/internal/events"
	"github.com/useopendesk/opendesk/internal/helpdesk/hd"
	"github.com/useopendesk/opendesk/internal/httpx"
)

// maxBulkQuery bounds how many records one request that names its records by a filter acts on.
// Each record is audited and announced, so a larger selection is split by narrowing the filter.
const maxBulkQuery = 1000

type bulkResult struct {
	Updated int `json:"updated"`
	Deleted int `json:"deleted"`
}

type contactBulkQuery struct {
	Q         string `json:"q,omitempty" maxLength:"200"`
	CompanyID string `json:"company_id,omitempty"`
	Tag       string `json:"tag,omitempty" maxLength:"50"`
	Blocked   string `json:"blocked,omitempty" enum:"true,false,"`
}

type contactBulkPatch struct {
	AddTags    []string         `json:"add_tags,omitempty" maxItems:"50" doc:"Adds these labels, by name"`
	RemoveTags []string         `json:"remove_tags,omitempty" maxItems:"50" doc:"Removes these labels, by name"`
	CompanyID  Nullable[string] `json:"company_id,omitempty" doc:"Company id; null clears it"`
}

type contactBulkBody struct {
	IDs    []string          `json:"ids,omitempty" maxItems:"100"`
	Query  *contactBulkQuery `json:"query,omitempty" doc:"The People list's filters, instead of ids"`
	Patch  *contactBulkPatch `json:"patch,omitempty"`
	Delete bool              `json:"delete,omitempty"`
}

type companyBulkQuery struct {
	Q   string `json:"q,omitempty" maxLength:"200"`
	Tag string `json:"tag,omitempty" maxLength:"50"`
}

type companyBulkPatch struct {
	AddTags    []string `json:"add_tags,omitempty" maxItems:"50" doc:"Adds these labels, by name"`
	RemoveTags []string `json:"remove_tags,omitempty" maxItems:"50" doc:"Removes these labels, by name"`
}

type companyBulkBody struct {
	IDs    []string          `json:"ids,omitempty" maxItems:"100"`
	Query  *companyBulkQuery `json:"query,omitempty" doc:"The Companies list's filters, instead of ids"`
	Patch  *companyBulkPatch `json:"patch,omitempty"`
	Delete bool              `json:"delete,omitempty"`
}

// bulkShape checks the request names its records one way and asks for one thing.
func bulkShape(ids int, query, patch, del bool) error {
	if patch == del {
		return httpx.Validation("send either patch or delete: true")
	}
	if (ids == 0) == !query {
		return httpx.Validation("send either ids or query")
	}
	return nil
}

// workspaceIDs resolves the ids the request named, refusing any that is not in the workspace.
func workspaceIDs(ctx context.Context, tx pgx.Tx, table, what string, wid uuid.UUID, raw []string) ([]uuid.UUID, error) {
	seen := map[uuid.UUID]bool{}
	var want []uuid.UUID
	for _, r := range raw {
		id, err := hd.ParseID(r, what)
		if err != nil {
			return nil, err
		}
		if !seen[id] {
			seen[id] = true
			want = append(want, id)
		}
	}
	rows, err := tx.Query(ctx, `SELECT id FROM `+table+` WHERE workspace_id = $1 AND id = ANY($2) ORDER BY id`, wid, want)
	if err != nil {
		return nil, err
	}
	found, err := pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
	if err != nil {
		return nil, err
	}
	if len(found) != len(want) {
		return nil, httpx.NotFound(what)
	}
	return found, nil
}

func tooMany(n int) error {
	if n > maxBulkQuery {
		return httpx.E(422, "too_many_records", "That filter matches more than 1000 records. Narrow it and try again.")
	}
	return nil
}

func registerBulk(d *deps.Deps, api huma.API) {
	huma.Register(api, op("bulk-contacts", http.MethodPost, "/v1/contacts/bulk", "Update or delete many contacts", "Contacts", 0),
		func(ctx context.Context, in *struct{ Body contactBulkBody }) (*struct{ Body bulkResult }, error) {
			p, err := auth.RequireWorkspace(ctx, auth.RoleAgent, auth.ScopeContactsWrite)
			if err != nil {
				return nil, httpx.Wrap(err)
			}
			b := in.Body
			if err := bulkShape(len(b.IDs), b.Query != nil, b.Patch != nil, b.Delete); err != nil {
				return nil, err
			}
			var out bulkResult
			err = d.DB.Tx(ctx, func(tx pgx.Tx) error {
				var ids []uuid.UUID
				var err error
				if b.Query != nil {
					ids, err = contactIDsByQuery(ctx, tx, p, *b.Query)
				} else {
					ids, err = workspaceIDs(ctx, tx, "contacts", "Contact", p.WorkspaceID, b.IDs)
				}
				if err != nil {
					return err
				}
				var company *uuid.UUID
				if b.Patch != nil && b.Patch.CompanyID.Set && !b.Patch.CompanyID.Null {
					id, err := hd.ParseRef(b.Patch.CompanyID.Value, "company_id")
					if err != nil {
						return err
					}
					if err := hd.CheckRef(ctx, tx, p.WorkspaceID, "company", id); err != nil {
						return err
					}
					company = &id
				}
				for _, id := range ids {
					if b.Delete {
						if _, err := tx.Exec(ctx, `DELETE FROM contacts WHERE workspace_id = $1 AND id = $2`, p.WorkspaceID, id); err != nil {
							return err
						}
						if err := audit.Log(ctx, tx, p, p.WorkspaceID, "contact.deleted", "contact", &id, map[string]any{"bulk": true}); err != nil {
							return err
						}
						if _, err := events.Publish(ctx, tx, events.Event{WorkspaceID: p.WorkspaceID, Type: "contact.deleted", Data: map[string]any{"contact_id": id}}); err != nil {
							return err
						}
						out.Deleted++
						continue
					}
					if err := applyLabels(ctx, tx, p, domain.ContactLabels, id, nil, b.Patch.AddTags, b.Patch.RemoveTags); err != nil {
						return err
					}
					if b.Patch.CompanyID.Set {
						if _, err := tx.Exec(ctx, `UPDATE contacts SET company_id = $3 WHERE workspace_id = $1 AND id = $2`, p.WorkspaceID, id, company); err != nil {
							return err
						}
					}
					if _, err := tx.Exec(ctx, `UPDATE contacts SET updated_at = now() WHERE workspace_id = $1 AND id = $2`, p.WorkspaceID, id); err != nil {
						return err
					}
					c, err := domain.LoadContact(ctx, tx, p.WorkspaceID, id)
					if err != nil {
						return err
					}
					if err := audit.Log(ctx, tx, p, p.WorkspaceID, "contact.updated", "contact", &id, map[string]any{"bulk": true}); err != nil {
						return err
					}
					if _, err := events.Publish(ctx, tx, events.Event{WorkspaceID: p.WorkspaceID, Type: "contact.updated", Data: map[string]any{"contact": c}}); err != nil {
						return err
					}
					out.Updated++
				}
				return nil
			})
			if err != nil {
				return nil, httpx.Wrap(err)
			}
			return &struct{ Body bulkResult }{out}, nil
		})

	huma.Register(api, op("bulk-companies", http.MethodPost, "/v1/companies/bulk", "Update or delete many companies", "Companies", 0),
		func(ctx context.Context, in *struct{ Body companyBulkBody }) (*struct{ Body bulkResult }, error) {
			p, err := auth.RequireWorkspace(ctx, auth.RoleAgent, auth.ScopeContactsWrite)
			if err != nil {
				return nil, httpx.Wrap(err)
			}
			b := in.Body
			if err := bulkShape(len(b.IDs), b.Query != nil, b.Patch != nil, b.Delete); err != nil {
				return nil, err
			}
			var out bulkResult
			err = d.DB.Tx(ctx, func(tx pgx.Tx) error {
				var ids []uuid.UUID
				var err error
				if b.Query != nil {
					ids, err = companyIDsByQuery(ctx, tx, p, *b.Query)
				} else {
					ids, err = workspaceIDs(ctx, tx, "companies", "Company", p.WorkspaceID, b.IDs)
				}
				if err != nil {
					return err
				}
				for _, id := range ids {
					if b.Delete {
						if _, err := tx.Exec(ctx, `DELETE FROM companies WHERE workspace_id = $1 AND id = $2`, p.WorkspaceID, id); err != nil {
							return err
						}
						if err := audit.Log(ctx, tx, p, p.WorkspaceID, "company.deleted", "company", &id, map[string]any{"bulk": true}); err != nil {
							return err
						}
						if _, err := events.Publish(ctx, tx, events.Event{WorkspaceID: p.WorkspaceID, Type: "company.deleted", Data: map[string]any{"company_id": id}}); err != nil {
							return err
						}
						out.Deleted++
						continue
					}
					if err := applyLabels(ctx, tx, p, domain.CompanyLabels, id, nil, b.Patch.AddTags, b.Patch.RemoveTags); err != nil {
						return err
					}
					if _, err := tx.Exec(ctx, `UPDATE companies SET updated_at = now() WHERE workspace_id = $1 AND id = $2`, p.WorkspaceID, id); err != nil {
						return err
					}
					c, err := loadCompany(ctx, tx, p.WorkspaceID, id)
					if err != nil {
						return err
					}
					if err := audit.Log(ctx, tx, p, p.WorkspaceID, "company.updated", "company", &id, map[string]any{"bulk": true}); err != nil {
						return err
					}
					if _, err := events.Publish(ctx, tx, events.Event{WorkspaceID: p.WorkspaceID, Type: "company.updated", Data: map[string]any{"company": c}}); err != nil {
						return err
					}
					out.Updated++
				}
				return nil
			})
			if err != nil {
				return nil, httpx.Wrap(err)
			}
			return &struct{ Body bulkResult }{out}, nil
		})
}

// contactIDsByQuery is the People list's filters as a selection: the same matching as the list.
func contactIDsByQuery(ctx context.Context, tx pgx.Tx, p *auth.Principal, f contactBulkQuery) ([]uuid.UUID, error) {
	extra, args, err := recordFilters(ctx, tx, p, domain.ContactLabels, domain.FieldEntityContact, "ct", f.Tag, nil, nil, 4)
	if err != nil {
		return nil, err
	}
	switch f.Blocked {
	case "true":
		extra += " AND ct.blocked"
	case "false":
		extra += " AND NOT ct.blocked"
	}
	var companyID *uuid.UUID
	if f.CompanyID != "" {
		id, err := hd.ParseRef(f.CompanyID, "company_id")
		if err != nil {
			return nil, err
		}
		companyID = &id
	}
	q := strings.TrimSpace(f.Q)
	rows, err := tx.Query(ctx, `SELECT ct.id FROM contacts ct WHERE ct.workspace_id = $1
		AND ($2 = '' OR ct.search @@ websearch_to_tsquery('simple', $2) OR ct.fields_search @@ websearch_to_tsquery('simple', $2)
			OR ct.email::text ILIKE $3 OR ct.name ILIKE $3 OR `+domain.ExtraEmailLikeSQL("lower($3)")+`)
		AND ($4::uuid IS NULL OR ct.company_id = $4)`+extra+` ORDER BY ct.id LIMIT `+strconv.Itoa(maxBulkQuery+1),
		append([]any{p.WorkspaceID, q, likePrefix(q), companyID}, args...)...)
	if err != nil {
		return nil, err
	}
	ids, err := pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
	if err != nil {
		return nil, err
	}
	return ids, tooMany(len(ids))
}

func companyIDsByQuery(ctx context.Context, tx pgx.Tx, p *auth.Principal, f companyBulkQuery) ([]uuid.UUID, error) {
	extra, args, err := recordFilters(ctx, tx, p, domain.CompanyLabels, domain.FieldEntityCompany, "co", f.Tag, nil, nil, 3)
	if err != nil {
		return nil, err
	}
	q := strings.TrimSpace(f.Q)
	like := "%" + strings.TrimSuffix(likePrefix(q), "%") + "%"
	rows, err := tx.Query(ctx, `SELECT co.id FROM companies co WHERE co.workspace_id = $1
		AND ($2 = '' OR co.name ILIKE $3 OR co.domain::text ILIKE $3 OR co.fields_search @@ websearch_to_tsquery('simple', $2))`+extra+` ORDER BY co.id LIMIT `+strconv.Itoa(maxBulkQuery+1),
		append([]any{p.WorkspaceID, q, like}, args...)...)
	if err != nil {
		return nil, err
	}
	ids, err := pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
	if err != nil {
		return nil, err
	}
	return ids, tooMany(len(ids))
}
