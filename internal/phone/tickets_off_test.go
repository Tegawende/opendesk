package phone_test

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/useopendesk/opendesk/internal/jobs"
	"github.com/useopendesk/opendesk/internal/products"
)

// A call event that arrives while Tickets is off stays stored and unapplied, and is
// applied once Tickets is on.
func TestTicketsOffHoldsCallEvents(t *testing.T) {
	h := newHarness(t)
	h.e.SetSettings(h.ws, map[string]any{"tickets_enabled": false})
	for _, b := range h.recorded(t, "answered") {
		if got := h.send(t, b); got != 200 {
			t.Fatalf("webhook answered %d", got)
		}
	}
	queued := h.e.Jobs(jobs.PhoneInboundArgs{}.Kind(), h.ws.ID)
	if len(queued) == 0 {
		t.Fatal("event not queued")
	}
	var args []jobs.PhoneInboundArgs
	for _, q := range queued {
		var a jobs.PhoneInboundArgs
		_ = json.Unmarshal(q, &a)
		args = append(args, a)
		if err := h.s.ProcessInbound(h.e.Ctx, a); !errors.Is(err, products.ErrHeld) {
			t.Fatalf("want held, got %v", err)
		}
	}
	var held, tickets int
	h.e.Scalar(`SELECT count(*) FROM phone_inbound_events WHERE workspace_id = $1 AND processed_at IS NULL`, &held, h.ws.ID)
	h.e.Scalar(`SELECT count(*) FROM tickets WHERE workspace_id = $1`, &tickets, h.ws.ID)
	if held != len(args) || tickets != 0 {
		t.Fatalf("held %d of %d, tickets %d", held, len(args), tickets)
	}
	h.e.SetSettings(h.ws, map[string]any{"tickets_enabled": true})
	for _, a := range args {
		if err := h.s.ProcessInbound(h.e.Ctx, a); err != nil {
			t.Fatal(err)
		}
	}
	h.e.Scalar(`SELECT count(*) FROM tickets WHERE workspace_id = $1`, &tickets, h.ws.ID)
	if tickets != 1 {
		t.Fatalf("%d tickets after replay", tickets)
	}
}
