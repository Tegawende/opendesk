package imports

import (
	"github.com/useopendesk/opendesk/internal/helpdesk/imports/source"
	"slices"
	"testing"
	"time"
)

func TestReviewCombinedHumanLabelAfterMerge(t *testing.T) {
	for _, mode := range []string{"source", "manual"} {
		t.Run(mode, func(t *testing.T) {
			e := newEnv(t)
			// Both source tickets initially carry vip.
			e.f.tickets[1].Tags = []string{"vip"}
			x := e.start(t, e.zendeskBody())
			if d := e.drive(t, x.ID); d.Status != StatusCompleted {
				t.Fatal(d.Status)
			}
			src, dst := e.ticketByRef(t, "101"), e.ticketByRef(t, "102")
			e.Do("PATCH", "/v1/tickets/"+src.String(), e.admin, map[string]any{"remove_tags": []string{"vip"}}).Want(t, 200)
			e.Do("PATCH", "/v1/tickets/"+src.String(), e.admin, map[string]any{"add_tags": []string{"vip"}}).Want(t, 200)
			if mode == "manual" {
				e.Do("POST", "/v1/tickets/"+src.String()+"/merge", e.admin, map[string]any{"into_id": dst.String()}).Want(t, 200)
			}
			// Source merges 101 into 102. The importer moves its labels and messages.
			now := time.Now().UTC().Add(-time.Minute)
			merged := e.f.tickets[1]
			merged.Loaded = true
			merged.Messages = append(slices.Clone(e.f.loads["102"]), e.f.tickets[0].Messages...)
			merged.UpdatedAt = now
			e.f.mu.Lock()
			e.f.tickets = []source.Ticket{merged}
			e.f.mu.Unlock()
			sync := func() {
				e.Do("POST", "/v1/imports/"+x.ID.String()+"/sync", e.admin, nil).Want(t, 200)
				if d := e.drive(t, x.ID); d.Status != StatusCompleted {
					t.Fatal(d.Status)
				}
			}
			sync()
			var didMerge bool
			e.Scalar(`SELECT merged_into_id=$2 FROM tickets WHERE id=$1`, &didMerge, src, dst)
			if !didMerge {
				t.Fatal("setup did not merge")
			}
			e.f.mu.Lock()
			e.f.tickets[0].Tags = nil
			e.f.tickets[0].UpdatedAt = now.Add(time.Second)
			e.f.mu.Unlock()
			sync()
			if got := e.labels(t, dst); !slices.Contains(got, "vip") {
				t.Fatalf("sync deleted the human label carried from merged ticket: %v", got)
			}

			var sourceAudit, targetAudit int
			e.Scalar(`SELECT count(*) FROM activities WHERE ticket_id=$1 AND type='tagged'`, &sourceAudit, src)
			e.Scalar(`SELECT count(*) FROM activities WHERE ticket_id=$1 AND type='tagged'`, &targetAudit, dst)
			if sourceAudit != 1 || targetAudit != 0 {
				t.Fatalf("original audit changed: source=%d target=%d", sourceAudit, targetAudit)
			}
		})
	}
}
