package email

import (
	"errors"
	"strings"
	"testing"

	"github.com/useopendesk/opendesk/internal/crypto"
	"github.com/useopendesk/opendesk/internal/products"
)

// Mail that arrives while Tickets is off is stored and stays pending, creates nothing,
// and becomes its ticket once Tickets is on and the held job is woken.
func TestTicketsOffHoldsInboundMail(t *testing.T) {
	w := newWorld(t)
	m := w.mailbox(InboundForwarding, OutboundSystem, nil)
	mustExec(t, w.d, `UPDATE workspaces SET settings = settings || '{"tickets_enabled": false}' WHERE id = $1`, w.wid)

	raw := strings.ReplaceAll(fixture(t, "new_plain.eml", "{{FROM}}", customer(), "{{TO}}", w.forwardAddr(m, ""), "{{ID}}", crypto.RandomString(8)), "\n", "\r\n")
	id, dup, err := w.s.ingest(w.ctx, m, []byte(raw), w.forwardAddr(m, ""), nil, viaForward)
	if err != nil || dup {
		t.Fatal(err, dup)
	}
	if err := w.s.processInbound(w.ctx, w.wid, id); !errors.Is(err, products.ErrHeld) {
		t.Fatalf("want held, got %v", err)
	}
	if st, _ := w.inbound(id); st != "pending" {
		t.Fatalf("status %s", st)
	}
	if n := w.count(`SELECT count(*) FROM tickets WHERE workspace_id = $1`, w.wid); n != 0 {
		t.Fatalf("%d tickets while off", n)
	}

	// The held job waits; turning Tickets on wakes it ahead of its snooze.
	mustExec(t, w.d, `UPDATE river_job SET state = 'scheduled', scheduled_at = now() + interval '1 hour' WHERE kind = 'email.process_inbound' AND args->>'workspace_id' = $1`, w.wid.String())
	mustExec(t, w.d, `UPDATE workspaces SET settings = settings || '{"tickets_enabled": true}' WHERE id = $1`, w.wid)
	if err := products.Reactivate(w.ctx, w.d.DB.Pool, w.wid); err != nil {
		t.Fatal(err)
	}
	if n := w.count(`SELECT count(*) FROM river_job WHERE kind = 'email.process_inbound' AND state = 'scheduled' AND scheduled_at <= now() + interval '1 minute' AND args->>'workspace_id' = $1`, w.wid.String()); n != 1 {
		t.Fatalf("held job not woken: %d", n)
	}
	if err := w.s.processInbound(w.ctx, w.wid, id); err != nil {
		t.Fatal(err)
	}
	if st, _ := w.inbound(id); st != "processed" {
		t.Fatalf("status %s", st)
	}
	if n := w.count(`SELECT count(*) FROM tickets WHERE workspace_id = $1`, w.wid); n != 1 {
		t.Fatalf("%d tickets after replay", n)
	}
}
