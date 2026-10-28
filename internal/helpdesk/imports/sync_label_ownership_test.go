package imports

import (
	"github.com/google/uuid"
	"github.com/useopendesk/opendesk/internal/helpdesk/hdtest"
	"github.com/useopendesk/opendesk/internal/helpdesk/resources"
	"slices"
	"testing"
	"time"
)

func TestReview245RenamedHumanLabel(t *testing.T) {
	e := newEnv(t)
	x := e.start(t, e.zendeskBody())
	if d := e.drive(t, x.ID); d.Status != StatusCompleted {
		t.Fatal(d.Status)
	}
	tid := e.ticketByRef(t, "101")
	e.Do("PATCH", "/v1/tickets/"+tid.String(), e.admin, map[string]any{"remove_tags": []string{"vip"}}).Want(t, 200)
	e.Do("PATCH", "/v1/tickets/"+tid.String(), e.admin, map[string]any{"add_tags": []string{"vip"}}).Want(t, 200)
	var tag uuid.UUID
	e.Scalar(`SELECT id FROM tags WHERE workspace_id=$1 AND name='vip'`, &tag, e.ws.ID)
	labels := hdtest.New(t, resources.Register)
	labels.Do("PATCH", "/v1/tags/"+tag.String(), e.admin, map[string]any{"name": "important"}).Want(t, 200)
	e.f.mu.Lock()
	for i := range e.f.tickets {
		if e.f.tickets[i].ID == "101" {
			e.f.tickets[i].Tags = nil
			e.f.tickets[i].UpdatedAt = time.Now().UTC()
		}
	}
	e.f.mu.Unlock()
	e.Do("POST", "/v1/imports/"+x.ID.String()+"/sync", e.admin, nil).Want(t, 200)
	if d := e.drive(t, x.ID); d.Status != StatusCompleted {
		t.Fatal(d.Status)
	}
	if got := e.labels(t, tid); !slices.Contains(got, "important") {
		t.Fatalf("sync deleted human-applied label after rename: %v", got)
	}
}
