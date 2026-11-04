package products

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestEvaluate(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, settings                       string
		instance, entitlement, tickets, mail bool
		reason                               string
	}{
		{"legacy defaults", `{}`, true, true, true, true, ""},
		{"explicit enabled", `{"tickets_enabled":true,"mail_enabled":true}`, true, true, true, true, ""},
		{"tickets disabled", `{"tickets_enabled":false}`, true, true, false, true, ""},
		{"mail disabled", `{"mail_enabled":false}`, true, true, true, false, WorkspaceDisabled},
		{"plan precedes workspace", `{"mail_enabled":false}`, true, false, true, false, Plan},
		{"instance precedes plan", `{"mail_enabled":false}`, false, false, true, false, InstanceDisabled},
		{"instance cannot be overridden", `{"mail_enabled":true}`, false, true, true, false, InstanceDisabled},
		{"plan cannot be overridden", `{"mail_enabled":true}`, true, false, true, false, Plan},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var settings Settings
			if err := json.Unmarshal([]byte(tc.settings), &settings); err != nil {
				t.Fatal(err)
			}
			state := Evaluate(settings, tc.instance, tc.entitlement)
			if state.Tickets != tc.tickets || state.Mail != tc.mail || state.MailDisabledReason != tc.reason {
				t.Fatalf("state = %+v", state)
			}
			for _, check := range []struct {
				product string
				enabled bool
				err     error
			}{{Tickets, tc.tickets, state.RequireTickets()}, {Mail, tc.mail, state.RequireMail()}} {
				if check.enabled {
					if check.err != nil {
						t.Fatal(check.err)
					}
					continue
				}
				var disabled *DisabledError
				if !errors.As(check.err, &disabled) || disabled.Product != check.product || disabled.Reason == "" {
					t.Fatalf("disabled error = %#v", check.err)
				}
			}
		})
	}
}
