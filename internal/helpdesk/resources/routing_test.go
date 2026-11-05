package resources_test

import (
	"testing"

	"github.com/useopendesk/opendesk/internal/helpdesk/hdtest"
	"github.com/useopendesk/opendesk/internal/helpdesk/resources"
	"github.com/useopendesk/opendesk/internal/ids"
)

func TestAssignmentSettings(t *testing.T) {
	e := hdtest.New(t, resources.Register)
	ws := e.NewWorkspace()
	other := e.NewWorkspace()
	team, foreign := ids.New(), ids.New()
	e.Exec(`INSERT INTO teams (id, workspace_id, name) VALUES ($1, $2, 'Tier 1'), ($3, $4, 'Theirs')`, team, ws.ID, foreign, other.ID)

	var a resources.Assignment
	e.Do("GET", "/v1/assignment", ws.Viewer.As(ws), nil).Want(t, 200).JSON(t, &a)
	if a.Mode != "none" || len(a.Rules) != 0 || a.AutoAwayMinutes != 0 || a.NewTicketNotify != "auto" || a.FollowUpKeepsAssignee {
		t.Fatalf("defaults %+v", a)
	}
	body := map[string]any{"mode": "load_balanced", "auto_away_minutes": 30, "new_ticket_notify": "leads", "followup_keeps_assignee": true,
		"rules": []map[string]any{{"target_type": "team", "target_id": team.String(), "mode": "round_robin"}}}
	e.Do("PATCH", "/v1/assignment", ws.Agent.As(ws), body).Want(t, 403)
	e.Do("PATCH", "/v1/assignment", ws.Admin.As(ws), body).Want(t, 200).JSON(t, &a)
	if a.Mode != "load_balanced" || len(a.Rules) != 1 || a.Rules[0].Mode != "round_robin" || a.AutoAwayMinutes != 30 ||
		a.NewTicketNotify != "leads" || !a.FollowUpKeepsAssignee {
		t.Fatalf("saved %+v", a)
	}
	// Another workspace's team, duplicates and out-of-range values are refused.
	for _, b := range []map[string]any{
		{"rules": []map[string]any{{"target_type": "team", "target_id": foreign.String(), "mode": "round_robin"}}},
		{"rules": []map[string]any{{"target_type": "team", "target_id": team.String(), "mode": "none"}, {"target_type": "team", "target_id": team.String(), "mode": "none"}}},
		{"auto_away_minutes": 5},
		{"mode": "random"},
		{"new_ticket_notify": "nobody"},
	} {
		e.Do("PATCH", "/v1/assignment", ws.Admin.As(ws), b).Want(t, 422)
	}
	e.Do("PATCH", "/v1/assignment", ws.Admin.As(ws), map[string]any{"rules": []any{}}).Want(t, 200).JSON(t, &a)
	if len(a.Rules) != 0 || a.Mode != "load_balanced" || a.NewTicketNotify != "leads" || !a.FollowUpKeepsAssignee {
		t.Fatalf("rules not replaced %+v", a)
	}
	var n int
	e.Scalar(`SELECT count(*) FROM audit_logs WHERE workspace_id = $1 AND action = 'assignment.updated'`, &n, ws.ID)
	if n != 2 {
		t.Fatalf("audit %d", n)
	}
	var notify string
	e.Scalar(`SELECT data->>'new_ticket_notify' FROM audit_logs WHERE workspace_id = $1 AND action = 'assignment.updated'
		ORDER BY created_at DESC LIMIT 1`, &notify, ws.ID)
	if notify != "leads" {
		t.Fatalf("audit new_ticket_notify %q", notify)
	}
	var keeps string
	e.Scalar(`SELECT data->>'followup_keeps_assignee' FROM audit_logs WHERE workspace_id = $1 AND action = 'assignment.updated'
		ORDER BY created_at DESC LIMIT 1`, &keeps, ws.ID)
	if keeps != "true" {
		t.Fatalf("audit followup_keeps_assignee %q", keeps)
	}
	// Turned off again, it is stored false.
	e.Do("PATCH", "/v1/assignment", ws.Admin.As(ws), map[string]any{"followup_keeps_assignee": false}).Want(t, 200).JSON(t, &a)
	if a.FollowUpKeepsAssignee {
		t.Fatal("followup_keeps_assignee still on")
	}
	e.Do("PATCH", "/v1/assignment", ws.Admin.As(ws), map[string]any{"followup_keeps_assignee": "yes"}).Want(t, 422)
	// An unknown stored value reads as auto.
	e.SetSettings(ws, map[string]any{"new_ticket_notify": "sometimes"})
	e.Do("GET", "/v1/assignment", ws.Viewer.As(ws), nil).Want(t, 200).JSON(t, &a)
	if a.NewTicketNotify != "auto" {
		t.Fatalf("unknown value read as %q", a.NewTicketNotify)
	}
}

func TestTriageSettings(t *testing.T) {
	e := hdtest.New(t, resources.Register)
	ws := e.NewWorkspace()
	var ts resources.TriageSettings
	e.Do("GET", "/v1/triage", ws.Agent.As(ws), nil).Want(t, 200).JSON(t, &ts)
	if ts.AutoTriage || len(ts.Fields) != 3 || ts.AssignAfterTriage || ts.Available || ts.Threshold != 0.7 || ts.Engine.Provider != "none" {
		t.Fatalf("defaults %+v", ts)
	}
	e.Do("PATCH", "/v1/triage", ws.Agent.As(ws), map[string]any{"auto_triage": true}).Want(t, 403)
	e.Do("PATCH", "/v1/triage", ws.Admin.As(ws), map[string]any{"auto_triage": true, "fields": []string{"labels"},
		"assign_after_triage": true}).Want(t, 200).JSON(t, &ts)
	if !ts.AutoTriage || len(ts.Fields) != 1 || ts.Fields[0] != "labels" || !ts.AssignAfterTriage {
		t.Fatalf("saved %+v", ts)
	}
	// Other AI keys are kept.
	var tone string
	e.Exec(`UPDATE workspaces SET settings = jsonb_set(settings, '{ai,tone}', '"concise"') WHERE id = $1`, ws.ID)
	e.Do("PATCH", "/v1/triage", ws.Admin.As(ws), map[string]any{"fields": []string{}}).Want(t, 200).JSON(t, &ts)
	e.Scalar(`SELECT settings -> 'ai' ->> 'tone' FROM workspaces WHERE id = $1`, &tone, ws.ID)
	if tone != "concise" || len(ts.Fields) != 0 || !ts.AutoTriage {
		t.Fatalf("merge lost keys: tone %q %+v", tone, ts)
	}
	e.Do("PATCH", "/v1/triage", ws.Admin.As(ws), map[string]any{"fields": []string{"status"}}).Want(t, 422)
	e.Do("PATCH", "/v1/triage", ws.Admin.As(ws), map[string]any{"threshold": 0.3}).Want(t, 422)
	e.Do("PATCH", "/v1/triage", ws.Admin.As(ws), map[string]any{"threshold": 0.856, "fields": []string{"severity", "team"}}).
		Want(t, 200).JSON(t, &ts)
	if ts.Threshold != 0.86 || len(ts.Fields) != 2 || ts.Fields[1] != "team" {
		t.Fatalf("threshold %+v", ts)
	}
}

func TestBusinessHoursAPI(t *testing.T) {
	e := hdtest.New(t, resources.Register)
	ws := e.NewWorkspace()
	var bh resources.BusinessHoursBody
	e.Do("GET", "/v1/business-hours", ws.Viewer.As(ws), nil).Want(t, 200).JSON(t, &bh)
	if bh.Enabled || len(bh.Schedule["mon"]) != 1 || bh.Timezone != "UTC" {
		t.Fatalf("defaults %+v", bh)
	}
	body := map[string]any{"enabled": true, "timezone": "Europe/Berlin",
		"schedule": map[string]any{"mon": []map[string]string{{"start": "08:00", "end": "12:00"}, {"start": "13:00", "end": "18:00"}}},
		"holidays": []map[string]string{{"date": "2026-12-25", "name": "Christmas"}}}
	e.Do("PUT", "/v1/business-hours", ws.Agent.As(ws), body).Want(t, 403)
	e.Do("PUT", "/v1/business-hours", ws.Admin.As(ws), body).Want(t, 200).JSON(t, &bh)
	if !bh.Enabled || bh.Timezone != "Europe/Berlin" || len(bh.Schedule["mon"]) != 2 || len(bh.Schedule["tue"]) != 0 || len(bh.Holidays) != 1 {
		t.Fatalf("saved %+v", bh)
	}
	body["timezone"] = "Mars/Olympus"
	e.Do("PUT", "/v1/business-hours", ws.Admin.As(ws), body).Want(t, 422)
	body["timezone"] = "UTC"
	body["schedule"] = map[string]any{"mon": []map[string]string{{"start": "18:00", "end": "08:00"}}}
	e.Do("PUT", "/v1/business-hours", ws.Admin.As(ws), body).Want(t, 422)

	// SLA policies take the flag.
	var p resources.SlaPolicy
	e.Do("POST", "/v1/sla-policies", ws.Admin.As(ws), map[string]any{"name": "Hours", "first_response_minutes": 60,
		"resolution_minutes": 600, "business_hours": true}).Want(t, 201).JSON(t, &p)
	if !p.BusinessHours {
		t.Fatal("business_hours not saved")
	}
}
