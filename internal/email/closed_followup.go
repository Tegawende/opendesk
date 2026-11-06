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
// workspace keeps it, while they are an owner, admin or agent who is not suspended (whom
// auto-assignment and skills accept) and, when team (the follow-up's team) is private, on that
// team. nil leaves the follow-up unassigned, for auto-assignment.
func followUpAssignee(ctx context.Context, tx pgx.Tx, wid, prev uuid.UUID, team *uuid.UUID) (*uuid.UUID, error) {
	st, err := hd.LoadSettings(ctx, tx, wid)
	if err != nil || !st.FollowUpKeepsAssignee {
		return nil, err
	}
	var uid uuid.UUID
	err = tx.QueryRow(ctx, `SELECT t.assignee_id FROM tickets t
		JOIN members m ON m.workspace_id = t.workspace_id AND m.user_id = t.assignee_id
		JOIN users u ON u.id = m.user_id
		WHERE t.workspace_id = $1 AND t.id = $2 AND m.role IN ('owner','admin','agent') AND u.suspended_at IS NULL`, wid, prev).Scan(&uid)
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

// followUpAssigned records that the follow-up tid (of team) went to the assignee of prev (number
// prevNumber): an assigned activity by system, via previous_ticket, and their assigned
// notification. The activity names prev only when everyone who sees the follow-up may see prev
// too: a prev in another, private team stays unnamed, as a merge's target does.
func followUpAssigned(ctx context.Context, tx pgx.Tx, wid, tid uuid.UUID, team *uuid.UUID, prev uuid.UUID, prevNumber int64, assignee uuid.UUID) error {
	data := map[string]any{"from": nil, "to": assignee.String(), "via": "previous_ticket",
		"previous_ticket_id": prev.String(), "previous_ticket_number": prevNumber}
	var prevTeam *uuid.UUID
	if err := tx.QueryRow(ctx, `SELECT team_id FROM tickets WHERE workspace_id = $1 AND id = $2`, wid, prev).Scan(&prevTeam); err != nil {
		return err
	}
	if prevTeam != nil && (team == nil || *team != *prevTeam) {
		private, err := hd.TeamPrivate(ctx, tx, wid, prevTeam)
		if err != nil {
			return err
		}
		if private {
			data["previous_ticket_id"], data["previous_ticket_number"] = nil, nil
		}
	}
	if _, err := domain.LogActivity(ctx, tx, wid, tid, "system", nil, "assigned", data); err != nil {
		return err
	}
	return hd.Notify(ctx, tx, wid, assignee, hd.NotifyAssigned, &tid, map[string]any{"by": nil, "actor_type": "system"})
}
