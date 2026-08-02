package admin_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jtumidanski/myfleet/apps/fleet-service/internal/admin"
	"github.com/jtumidanski/myfleet/apps/fleet-service/internal/admin/admintest"
)

func TestReapDue_hardDeletesPastTheWindowAndMarksReaped(t *testing.T) {
	db := admintest.NewDB(t)
	admintest.SeedFleet(t, db, "fleet-1")
	media := &stubDownstream{name: "media"}
	proc := newProcessor(t, db, stubAuth{admin: true}, media)
	op, err := proc.Create(context.Background(), fleetInput())
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	// Wind the clock past the recovery window.
	later := admin.NewProcessorClockForTest(proc, testNow.Add(admin.DefaultRecoveryWindow+time.Hour))
	if err := later.ReapDue(context.Background()); err != nil {
		t.Fatalf("reap: %v", err)
	}

	got, err := admin.NewProvider(db).GetOperation(op.ID())
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Status() != admin.StatusReaped {
		t.Errorf("status = %q, want reaped", got.Status())
	}
	if got.ReapedAt() == nil {
		t.Error("reaped_at must be stamped")
	}
	for _, table := range []string{"fleet.fleets", "fleet.vehicles", "fleet.mileage_records"} {
		if n := admintest.CountRows(t, db, table); n != 0 {
			t.Errorf("%s still has %d rows after the reap", table, n)
		}
	}
	if media.reaped != 1 {
		t.Errorf("media reap called %d times, want 1", media.reaped)
	}
	// FR-ADMIN-AUDIT-1 + FR-ADMIN-UI-13: the reaper's row is attributed to the
	// system, not to the admin who requested the purge days earlier.
	var actor string
	db.Raw(`SELECT actor_user_id FROM fleet.admin_audit_events WHERE action = ?`,
		admin.ActionPurgeReaped).Scan(&actor)
	if actor != admin.ActorSystem {
		t.Errorf("reaper audit actor = %q, want %q", actor, admin.ActorSystem)
	}
	// FR-ADMIN-AUDIT-2: the audit trail survives its own purge.
	if n := admintest.CountRows(t, db, "fleet.admin_audit_events"); n < 2 {
		t.Errorf("audit rows must survive the reap, got %d", n)
	}
}

func TestReapDue_leavesOperationsInsideTheWindowAlone(t *testing.T) {
	db := admintest.NewDB(t)
	admintest.SeedFleet(t, db, "fleet-1")
	proc := newProcessor(t, db, stubAuth{admin: true}, &stubDownstream{name: "media"})
	op, _ := proc.Create(context.Background(), fleetInput())

	if err := proc.ReapDue(context.Background()); err != nil {
		t.Fatalf("reap: %v", err)
	}
	got, _ := admin.NewProvider(db).GetOperation(op.ID())
	if got.Status() != admin.StatusPending {
		t.Errorf("status = %q, want the operation still recoverable", got.Status())
	}
	if admintest.CountRows(t, db, "fleet.vehicles") != 2 {
		t.Error("the reaper hard-deleted rows still inside their recovery window")
	}
}

// design §8.4: downstream BEFORE local. A downstream failure must leave the
// local rows — and therefore the purge_operation_id the next tick needs — in
// place.
func TestReapDue_downstreamFailureLeavesTheOperationRetryable(t *testing.T) {
	db := admintest.NewDB(t)
	admintest.SeedFleet(t, db, "fleet-1")
	media := &failingReap{stubDownstream: stubDownstream{name: "media"}, failReap: true}
	proc := newProcessor(t, db, stubAuth{admin: true}, media)
	op, _ := proc.Create(context.Background(), fleetInput())

	later := admin.NewProcessorClockForTest(proc, testNow.Add(admin.DefaultRecoveryWindow+time.Hour))
	if err := later.ReapDue(context.Background()); err != nil {
		t.Fatalf("a downstream failure must not abort the whole run: %v", err)
	}

	got, _ := admin.NewProvider(db).GetOperation(op.ID())
	if got.Status() == admin.StatusReaped {
		t.Error("an operation whose downstream reap failed must not be marked reaped")
	}
	if admintest.CountRows(t, db, "fleet.vehicles") == 0 {
		t.Error("local rows were destroyed before the downstream reap succeeded — the next tick has nothing to retry")
	}

	// The next tick completes it.
	media.failReap = false
	if err := later.ReapDue(context.Background()); err != nil {
		t.Fatalf("second run: %v", err)
	}
	got, _ = admin.NewProvider(db).GetOperation(op.ID())
	if got.Status() != admin.StatusReaped {
		t.Errorf("the retry tick must complete the reap, got %q", got.Status())
	}
}

// FR-ADMIN-RESTORE-6: running the reaper twice is harmless.
func TestReapDue_isIdempotent(t *testing.T) {
	db := admintest.NewDB(t)
	admintest.SeedFleet(t, db, "fleet-1")
	proc := newProcessor(t, db, stubAuth{admin: true}, &stubDownstream{name: "media"})
	if _, err := proc.Create(context.Background(), fleetInput()); err != nil {
		t.Fatalf("create: %v", err)
	}

	later := admin.NewProcessorClockForTest(proc, testNow.Add(admin.DefaultRecoveryWindow+time.Hour))
	for i := 0; i < 2; i++ {
		if err := later.ReapDue(context.Background()); err != nil {
			t.Fatalf("run %d: %v", i+1, err)
		}
	}
}

type failingReap struct {
	stubDownstream
	failReap bool
}

func (f *failingReap) Reap(ctx context.Context, opID string) (map[string]int, error) {
	if f.failReap {
		return nil, errors.New("connection refused")
	}
	return f.stubDownstream.Reap(ctx, opID)
}
