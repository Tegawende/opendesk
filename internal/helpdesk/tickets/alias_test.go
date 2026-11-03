package tickets_test

import (
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/useopendesk/opendesk/internal/domain"
)

func TestTicketAliasUpdateSearchAndLookup(t *testing.T) {
	e, ws := newEnv(t)
	agent := ws.Agent.As(ws)
	first := createTicket(t, e, ws, map[string]any{"subject": "Shipping update"})
	second := createTicket(t, e, ws, map[string]any{"subject": "Another ticket"})

	e.Do("PATCH", "/v1/tickets/"+first.ID.String(), agent, map[string]any{"alias": " order-42 "}).Want(t, 200)
	if got := getTicket(t, e, ws, first.ID).Alias; got == nil || *got != "ORDER-42" {
		t.Fatalf("alias = %v, want ORDER-42", got)
	}
	e.Do("PATCH", "/v1/tickets/"+second.ID.String(), agent, map[string]any{"alias": "order-42"}).Want(t, 409)
	e.Do("PATCH", "/v1/tickets/"+second.ID.String(), agent, map[string]any{"alias": "42"}).Want(t, 422)
	e.Do("PATCH", "/v1/tickets/"+second.ID.String(), agent, map[string]any{"alias": "order 42"}).Want(t, 422)

	if got := list(t, e, agent, "q=order-42"); len(got.Data) != 1 || got.Data[0].ID != first.ID {
		t.Fatalf("alias search returned %v", idsOf(got))
	}
	var found domain.Ticket
	e.Do("GET", "/v1/tickets/order-42", ws.Viewer.As(ws), nil).Want(t, 200).JSON(t, &found)
	if found.ID != first.ID {
		t.Fatalf("alias lookup returned %s, want %s", found.ID, first.ID)
	}

	e.Do("PATCH", "/v1/tickets/order-42", agent, map[string]any{"subject": "Updated through reference"}).Want(t, 200)
	if getTicket(t, e, ws, first.ID).Subject != "Updated through reference" || getTicket(t, e, ws, second.ID).Subject != "Another ticket" {
		t.Fatal("alias patch selected wrong ticket")
	}
	e.Do("PATCH", "/v1/tickets/"+first.ID.String(), agent, map[string]any{"alias": nil}).Want(t, 200)
	if got := getTicket(t, e, ws, first.ID).Alias; got != nil {
		t.Fatalf("cleared alias = %v", *got)
	}
}

func TestTicketAliasLookupKeepsPrivateTeamVisibility(t *testing.T) {
	e, ws := newFullEnv(t)
	private := addTeam(t, e, ws, ws.Agent.ID)
	e.Exec(`UPDATE teams SET private = true WHERE id = $1`, private)
	secret := createTicket(t, e, ws, map[string]any{"subject": "Private case", "team_id": private.String()})
	e.Do("PATCH", "/v1/tickets/"+secret.ID.String(), ws.Agent.As(ws), map[string]any{"alias": "PRIVATE-42"}).Want(t, 200)
	e.Do("GET", "/v1/tickets/private-42", ws.Viewer.As(ws), nil).Want(t, 404)
	e.Do("PATCH", "/v1/tickets/private-42", ws.Owner.As(ws), map[string]any{"subject": "Hidden"}).Want(t, 404)
	page := list(t, e, ws.Viewer.As(ws), "q=private-42")
	if len(page.Data) != 0 {
		t.Fatalf("hidden alias appeared in search: %v", idsOf(page))
	}
}

func TestTicketAliasReservesUUIDReferences(t *testing.T) {
	for _, form := range []string{"canonical", "compact", "wrapped", "braced", "urn", "nil", "non-rfc"} {
		t.Run(form, func(t *testing.T) {
			e, ws := newEnv(t)
			first := createTicket(t, e, ws, map[string]any{"subject": "Alias owner"})
			second := createTicket(t, e, ws, map[string]any{"subject": "UUID owner"})
			alias := strings.ToUpper(second.ID.String())
			switch form {
			case "compact":
				alias = strings.ReplaceAll(alias, "-", "")
			case "wrapped":
				alias = "A" + alias + "Z"
			case "braced":
				alias = "{" + alias + "}"
			case "urn":
				alias = "urn:uuid:" + alias
			case "nil":
				alias = "00000000-0000-0000-0000-000000000000"
			case "non-rfc":
				alias = "ABCDEFAB-CDEF-0000-0000-ABCDEFABCDEF"
			}
			if _, err := uuid.Parse(alias); err != nil {
				t.Fatal(err)
			}
			e.Do("PATCH", "/v1/tickets/"+first.ID.String(), ws.Agent.As(ws), map[string]any{"alias": alias, "subject": "Changed"}).Want(t, 422)
			if got := getTicket(t, e, ws, first.ID); got.Alias != nil || got.Subject != "Alias owner" {
				t.Fatal("rejected patch changed alias owner")
			}
			if got := getTicket(t, e, ws, second.ID); got.Subject != "UUID owner" {
				t.Fatal("rejected patch changed UUID owner")
			}
			if form == "canonical" || form == "compact" || form == "wrapped" {
				var found domain.Ticket
				e.Do("GET", "/v1/tickets/"+alias, ws.Agent.As(ws), nil).Want(t, 200).JSON(t, &found)
				if found.ID != second.ID {
					t.Fatal("UUID precedence changed")
				}
			}
		})
	}
}
