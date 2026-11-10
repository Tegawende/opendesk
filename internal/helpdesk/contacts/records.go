package contacts

import (
	"context"
	"strconv"
	"strings"

	"github.com/danielgtaylor/huma/v2"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/useopendesk/opendesk/internal/auth"
	"github.com/useopendesk/opendesk/internal/db"
	"github.com/useopendesk/opendesk/internal/domain"
	"github.com/useopendesk/opendesk/internal/helpdesk/hd"
	"github.com/useopendesk/opendesk/internal/httpx"
)

// fieldParams reads field.<key> filters from the query (see hd.FieldFiltersFromQuery).
type fieldParams struct {
	fields    []hd.FieldFilter
	fieldsErr error
}

func (f *fieldParams) Resolve(ctx huma.Context) []error {
	u := ctx.URL()
	f.fields, f.fieldsErr = hd.FieldFiltersFromQuery(u.Query())
	return nil
}

type contactListIn struct {
	httpx.ListParams
	Q         string `query:"q" maxLength:"200" doc:"Search name, email, phone and custom field values"`
	CompanyID string `query:"company_id" doc:"Only contacts of this company"`
	Tag       string `query:"tag" maxLength:"50" doc:"Label name"`
	Blocked   string `query:"blocked" enum:"true,false," doc:"Only blocked contacts, or only those who are not"`
	Sort      string `query:"sort" enum:"-created,created,name,-name,-tickets,tickets," doc:"Newest first by default"`
	fieldParams
}

type companyListIn struct {
	httpx.ListParams
	Q    string `query:"q" maxLength:"200" doc:"Search name, domain and custom field values"`
	Tag  string `query:"tag" maxLength:"50" doc:"Label name"`
	Sort string `query:"sort" enum:"name,-name,-created,created,-contacts,contacts," doc:"By name, A to Z, by default"`
	fieldParams
}

// offsetCursor pages a list sorted by something other than its default order. The default
// orders page by key; these are for a person looking through a list, so a position is enough.
type offsetCursor struct {
	O int `json:"o"`
}

// listOrder is the ORDER BY of a requested sort ("" or the default: the caller's own keyset
// order), with a tie-break on the id so every page of it is stable. Only fixed text from
// the table below reaches the query.
func listOrder(sort string, orders map[string]string, id string) string {
	order, ok := orders[sort]
	if !ok {
		return ""
	}
	dir := " ASC"
	if strings.HasPrefix(sort, "-") {
		dir = " DESC"
	}
	return order + dir + ", " + id + dir
}

// recordFilters builds the extra " AND ..." clauses for label and custom field filters on
// alias, binding parameters from $next.
func recordFilters(ctx context.Context, q db.Querier, p *auth.Principal, kind domain.LabelKind, entity, alias, tag string,
	ff []hd.FieldFilter, ffErr error, last int) (string, []any, error) {
	if ffErr != nil {
		return "", nil, ffErr
	}
	var args []any
	arg := func(v any) string {
		args = append(args, v)
		return "$" + strconv.Itoa(last+len(args))
	}
	sql := ""
	if tag != "" {
		sql += ` AND EXISTS (SELECT 1 FROM ` + kind.Table + ` flt JOIN tags fg ON fg.id = flt.tag_id
			WHERE flt.` + kind.Col + ` = ` + alias + `.id AND fg.workspace_id = ` + alias + `.workspace_id AND lower(fg.name) = lower(` + arg(tag) + `))`
	}
	if len(ff) > 0 {
		defs, err := domain.LoadFieldDefs(ctx, q, p.WorkspaceID, entity)
		if err != nil {
			return "", nil, err
		}
		norm, err := hd.NormalizeFieldFilters(defs, ff)
		if err != nil {
			return "", nil, err
		}
		for _, f := range norm {
			sql += " AND " + hd.FieldClause(defs[f.Key], f, alias+".fields", arg, p.ActorUserID())
		}
	}
	return sql, args, nil
}

// applyFields merges a fields patch into *cur after validating it against the entity's definitions.
func applyFields(ctx context.Context, tx pgx.Tx, wid uuid.UUID, entity string, cur *map[string]any, patch map[string]any) error {
	if *cur == nil {
		*cur = map[string]any{}
	}
	if patch == nil {
		return nil
	}
	defs, err := domain.LoadFieldDefs(ctx, tx, wid, entity)
	if err != nil {
		return err
	}
	next, _, err := domain.MergeFields(defs, *cur, patch, domain.MemberChecker(ctx, tx, wid))
	if err != nil {
		return err
	}
	*cur = next
	return nil
}

// applyLabels applies label changes by name to a contact or company.
func applyLabels(ctx context.Context, tx pgx.Tx, p *auth.Principal, kind domain.LabelKind, id uuid.UUID, replace *[]string, add, remove []string) error {
	ops := domain.TagOps{Replace: replace, Add: add, Remove: remove}
	if ops.Empty() {
		return nil
	}
	create, err := hd.CanCreateLabels(ctx, tx, p)
	if err != nil {
		return err
	}
	ops.Create = create
	_, _, err = domain.ApplyTags(ctx, tx, p.WorkspaceID, kind, id, ops)
	return err
}
