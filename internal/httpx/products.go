package httpx

import (
	"encoding/json"

	"github.com/danielgtaylor/huma/v2"
	"github.com/google/uuid"

	"github.com/useopendesk/opendesk/internal/auth"
	"github.com/useopendesk/opendesk/internal/db"
	"github.com/useopendesk/opendesk/internal/products"
)

// TicketsGate refuses the operations applies selects with feature_disabled while the
// workspace has Tickets off. The workspace is the caller's (member, API key, visitor or
// customer); a public route names it by its workspace_slug path parameter. A request with
// neither passes through, so authentication answers it as before.
func TicketsGate(q db.Querier, applies func(*huma.Operation) bool) func(huma.Context, func(huma.Context)) {
	return func(ctx huma.Context, next func(huma.Context)) {
		if op := ctx.Operation(); op == nil || !applies(op) {
			next(ctx)
			return
		}
		wid := uuid.Nil
		if p := auth.FromContext(ctx.Context()); p != nil {
			wid = p.WorkspaceID
		}
		if slug := ctx.Param("workspace_slug"); wid == uuid.Nil && slug != "" {
			if err := q.QueryRow(ctx.Context(), `SELECT id FROM workspaces WHERE slug = $1`, slug).Scan(&wid); err != nil {
				next(ctx)
				return
			}
		}
		if wid == uuid.Nil {
			next(ctx)
			return
		}
		err := products.RequireTickets(ctx.Context(), q, wid)
		if err == nil {
			next(ctx)
			return
		}
		e, ok := Wrap(err).(*Error)
		if !ok {
			e = E(500, "internal", "Something went wrong.")
		}
		e.RequestID = ctx.Header("X-Request-Id")
		if e.RequestID == "" {
			e.RequestID = RequestIDFrom(ctx.Context())
		}
		ctx.SetHeader("Content-Type", "application/problem+json")
		ctx.SetStatus(e.Status)
		_ = json.NewEncoder(ctx.BodyWriter()).Encode(e)
	}
}

// HasTag reports whether the operation carries tag.
func HasTag(op *huma.Operation, tag string) bool {
	for _, t := range op.Tags {
		if t == tag {
			return true
		}
	}
	return false
}
