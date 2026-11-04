package supportchannels

import (
	"errors"
	"testing"

	"github.com/useopendesk/opendesk/internal/jobs"
	"github.com/useopendesk/opendesk/internal/products"
	"github.com/useopendesk/opendesk/internal/supportchannels/channel"
)

// A command that arrives while Tickets is off stays stored and unapplied; turning Tickets
// on wakes its job, and it then changes the ticket once.
func TestTicketsOffHoldsChannelCommands(t *testing.T) {
	w := newWorkflow(t)
	w.e.SetSettings(w.ws, map[string]any{"tickets_enabled": false})
	in := channel.Inbound{EventID: "HELD1", ExternalChannelID: "C1", ExternalThreadID: "10.1", ExternalUserID: "U1", Command: "status", Value: "pending", Respond: "callback"}
	if err := w.s.persistInbound(w.e.Ctx, w.c, []channel.Inbound{in}); err != nil {
		t.Fatal(err)
	}
	a := jobs.SupportChannelInboundArgs{ConnectionID: w.c.ID, ExternalEventID: in.EventID}
	if err := w.s.processInbound(w.e.Ctx, a); !errors.Is(err, products.ErrHeld) {
		t.Fatalf("want held, got %v", err)
	}
	var open string
	w.e.Scalar(`SELECT status FROM tickets WHERE id = $1`, &open, w.ticket.ID)
	if open == "pending" {
		t.Fatal("command applied while off")
	}

	w.e.Exec(`UPDATE river_job SET state = 'scheduled', scheduled_at = now() + interval '1 hour' WHERE kind = 'support_channels.inbound' AND args->>'external_event_id' = 'HELD1'`)
	w.e.SetSettings(w.ws, map[string]any{"tickets_enabled": true})
	if err := products.Reactivate(w.e.Ctx, w.e.D.DB.Pool, w.ws.ID); err != nil {
		t.Fatal(err)
	}
	var woken int
	w.e.Scalar(`SELECT count(*) FROM river_job WHERE kind = 'support_channels.inbound' AND state = 'scheduled' AND scheduled_at <= now() + interval '1 minute' AND args->>'external_event_id' = 'HELD1'`, &woken)
	if woken != 1 {
		t.Fatalf("held job not woken: %d", woken)
	}
	if err := w.s.processInbound(w.e.Ctx, a); err != nil {
		t.Fatal(err)
	}
	if status, _ := w.status(t); status != "pending" {
		t.Fatalf("status %s after replay", status)
	}
}
