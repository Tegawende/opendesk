package tickets_test

import (
	"testing"

	"github.com/useopendesk/opendesk/internal/auth"
	"github.com/useopendesk/opendesk/internal/crypto"
	"github.com/useopendesk/opendesk/internal/domain"
	"github.com/useopendesk/opendesk/internal/helpdesk/hdtest"
	"github.com/useopendesk/opendesk/internal/ids"
)

// A change an API key makes names the key in the ticket's timeline; a member's names nobody else.
func TestActivityNamesTheAPIKey(t *testing.T) {
	e, ws := newEnv(t)
	secret := "od_live_" + crypto.RandomToken(32)
	keyID := ids.New()
	e.Exec(`INSERT INTO api_keys (id, workspace_id, name, prefix, key_hash, scopes) VALUES ($1, $2, $3, $4, $5, $6)`,
		keyID, ws.ID, "Sorting routines", secret[:12], crypto.HashToken(secret), []string{auth.ScopeTicketsRead, auth.ScopeTicketsWrite})
	tk := createTicket(t, e, ws, map[string]any{"contact": map[string]any{"email": "carla@example.com", "name": "Carla"}})
	path := "/v1/tickets/" + tk.ID.String()

	e.Do("PATCH", path, hdtest.Auth{Bearer: secret}, map[string]any{"status": "solved", "priority": "high"}).Want(t, 200)
	e.Do("PATCH", path, ws.Agent.As(ws), map[string]any{"status": "open"}).Want(t, 200)

	var acts struct {
		Data []domain.Activity `json:"data"`
	}
	e.Do("GET", path+"/activities", ws.Agent.As(ws), nil).Want(t, 200).JSON(t, &acts)
	byKey, byMember := 0, 0
	for _, a := range acts.Data {
		switch a.Actor.Type {
		case "api":
			if a.Actor.APIKey == nil || a.Actor.APIKey.ID != keyID || a.Actor.APIKey.Name != "Sorting routines" {
				t.Fatalf("%s by the key: api_key %+v", a.Type, a.Actor.APIKey)
			}
			byKey++
		case "user":
			if a.Actor.APIKey != nil {
				t.Fatalf("%s by a member names a key: %+v", a.Type, a.Actor.APIKey)
			}
			byMember++
		}
	}
	if byKey != 2 || byMember != 1 {
		t.Fatalf("activities by the key %d, by the member %d, want 2 and 1: %+v", byKey, byMember, acts.Data)
	}

	// A deleted key leaves its changes unnamed, not broken.
	e.Exec(`DELETE FROM api_keys WHERE id = $1`, keyID)
	e.Do("GET", path+"/activities", ws.Agent.As(ws), nil).Want(t, 200).JSON(t, &acts)
	for _, a := range acts.Data {
		if a.Actor.Type == "api" && a.Actor.APIKey != nil {
			t.Fatalf("deleted key still named: %+v", a.Actor.APIKey)
		}
	}
}
