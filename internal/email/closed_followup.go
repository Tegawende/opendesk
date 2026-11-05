package email

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/useopendesk/opendesk/internal/db"
	"github.com/useopendesk/opendesk/internal/domain"
	"github.com/useopendesk/opendesk/internal/helpdesk/hd"
)

// followUpAssignee is who a customer's follow-up of the closed ticket prev is created assigned
// to (contracts/api.md "Assignment", followup_keeps_assignee): prev's assignee, when the
// workspace keeps it, while they are a member who is not a viewer and, when team (the follow-up's
// team) is private, on that team. nil leaves the follow-up unassigned, for auto-assignment.
func followUpAssignee(ctx context.Context, tx pgx.Tx, wid, prev uuid.UUID, team *uuid.UUID) (*uuid.UUID, error) {
	st, err := hd.LoadSettings(ctx, tx, wid)
	if err != nil || !st.FollowUpKeepsAssignee {
		return nil, err
	}
	var uid uuid.UUID
	err = tx.QueryRow(ctx, `SELECT t.assignee_id FROM tickets t
		JOIN members m ON m.workspace_id = t.workspace_id AND m.user_id = t.assignee_id
		WHERE t.workspace_id = $1 AND t.id = $2 AND m.role <> 'viewer'`, wid, prev).Scan(&uid)
	if db.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if team != nil {
		private, err := hd.TeamPrivate(ctx, tx, wid, team)
		if err != nil {
			return nil, err
		}
		if private {
			role, err := hd.TeamRole(ctx, tx, wid, *team, uid)
			if err != nil || role == "" {
				return nil, err
			}
		}
	}
	return &uid, nil
}

// followUpAssigned records that the follow-up tid went to the assignee of prev (number
// prevNumber): an assigned activity by system, via previous_ticket, naming prev, and their
// assigned notification.
func followUpAssigned(ctx context.Context, tx pgx.Tx, wid, tid, prev uuid.UUID, prevNumber int64, assignee uuid.UUID) error {
	if _, err := domain.LogActivity(ctx, tx, wid, tid, "system", nil, "assigned", map[string]any{"from": nil, "to": assignee.String(),
		"via": "previous_ticket", "previous_ticket_id": prev.String(), "previous_ticket_number": prevNumber}); err != nil {
		return err
	}
	return hd.Notify(ctx, tx, wid, assignee, hd.NotifyAssigned, &tid, map[string]any{"by": nil, "actor_type": "system"})
}
