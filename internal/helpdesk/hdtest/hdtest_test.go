package hdtest_test

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/useopendesk/opendesk/internal/helpdesk/hdtest"
)

func TestMain(m *testing.M) { hdtest.Main(m) }

func TestJobsReturnsInsertionOrderWithTiedTimes(t *testing.T) {
	e := hdtest.New(t)
	wid, other := uuid.New(), uuid.New()
	// Allocate increasing job IDs, but store the rows in the opposite order. All
	// timestamps tie, as they can when jobs are inserted in the same transaction.
	e.Exec(`WITH queued AS MATERIALIZED (
		SELECT nextval(pg_get_serial_sequence('river_job', 'id')) AS id, n
		FROM generate_series(1, 3) AS n
	) INSERT INTO river_job (id, kind, args, created_at, scheduled_at)
		SELECT id, 'hdtest_order', jsonb_build_object('workspace_id', $1::text, 'n', n), now(), now()
		FROM queued ORDER BY n DESC`, wid.String())
	e.Exec(`INSERT INTO river_job (kind, args) VALUES ('hdtest_order', jsonb_build_object('workspace_id', $1::text))`, other.String())
	queued := e.Jobs("hdtest_order", wid)
	if len(queued) != 3 {
		t.Fatalf("got %d jobs, want 3", len(queued))
	}
	for i, raw := range queued {
		var a struct{ N int }
		if err := json.Unmarshal(raw, &a); err != nil {
			t.Fatal(err)
		}
		if a.N != i+1 {
			t.Fatalf("job %d: got %d, want %d", i, a.N, i+1)
		}
	}
	if left := e.Jobs("hdtest_order", wid); len(left) != 0 {
		t.Fatalf("returned jobs remain queued: %s", left)
	}
	if left := e.Jobs("hdtest_order", other); len(left) != 1 {
		t.Fatalf("other workspace has %d jobs, want 1", len(left))
	}
}
