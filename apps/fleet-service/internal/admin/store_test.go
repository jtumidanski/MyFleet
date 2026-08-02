package admin_test

import (
	"testing"
	"time"

	"gorm.io/gorm"

	"github.com/jtumidanski/myfleet/apps/fleet-service/internal/admin"
	"github.com/jtumidanski/myfleet/apps/fleet-service/internal/admin/admintest"
	"github.com/jtumidanski/myfleet/packages/shared-go/server"
)

func newOperation(t *testing.T, scope admin.Scope, label string) admin.Operation {
	t.Helper()
	b := admin.NewOperationBuilder().
		SetScope(scope).
		SetTargetLabel(label).
		SetRequestedBy("admin-1", "admin@example.com").
		SetPurgeAfter(testNow.Add(admin.DefaultRecoveryWindow))
	if scope == admin.ScopeFleet {
		b = b.SetTarget("fleet", "fleet-1")
	}
	o, err := b.Build()
	if err != nil {
		t.Fatalf("build operation: %v", err)
	}
	return o
}

func TestOperationRoundTrip(t *testing.T) {
	db := admintest.NewDB(t)
	adm := admin.NewAdministrator(db)
	prov := admin.NewProvider(db)

	o := newOperation(t, admin.ScopeFleet, "Fleet fleet-1")
	if err := db.Transaction(func(tx *gorm.DB) error { return adm.InsertOperation(tx, o) }); err != nil {
		t.Fatalf("insert: %v", err)
	}

	got, err := prov.GetOperation(o.ID())
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Status() != admin.StatusPending {
		t.Errorf("a new operation must be pending, got %q", got.Status())
	}
	if got.TargetLabel() != "Fleet fleet-1" {
		t.Errorf("target label = %q", got.TargetLabel())
	}
	if got.RequestedByEmail() != "admin@example.com" {
		t.Errorf("requested_by_email = %q", got.RequestedByEmail())
	}
}

// A system-scope operation has no target at all. target_id must be NULL, not
// the empty string, which Postgres rejects for a uuid column.
func TestSystemOperation_hasANullTarget(t *testing.T) {
	db := admintest.NewDB(t)
	adm := admin.NewAdministrator(db)
	o := newOperation(t, admin.ScopeSystem, "the entire platform")
	if err := db.Transaction(func(tx *gorm.DB) error { return adm.InsertOperation(tx, o) }); err != nil {
		t.Fatalf("insert: %v", err)
	}
	var targetID *string
	if err := db.Raw(`SELECT target_id FROM fleet.purge_operations WHERE id = ?`, o.ID()).
		Scan(&targetID).Error; err != nil {
		t.Fatalf("read target_id: %v", err)
	}
	if targetID != nil {
		t.Errorf("system-scope target_id must be NULL, got %q", *targetID)
	}
}

func TestSetStatusAndAffected(t *testing.T) {
	db := admintest.NewDB(t)
	adm := admin.NewAdministrator(db)
	prov := admin.NewProvider(db)
	o := newOperation(t, admin.ScopeFleet, "Fleet fleet-1")
	if err := db.Transaction(func(tx *gorm.DB) error { return adm.InsertOperation(tx, o) }); err != nil {
		t.Fatalf("insert: %v", err)
	}

	if err := adm.SetAffected(db, o.ID(), map[string]int{"vehicles": 4, "fuel_logs": 130}); err != nil {
		t.Fatalf("set affected: %v", err)
	}
	if err := adm.SetStatus(db, o.ID(), admin.StatusPartial, []string{"media"}, testNow); err != nil {
		t.Fatalf("set status: %v", err)
	}

	got, err := prov.GetOperation(o.ID())
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Status() != admin.StatusPartial {
		t.Errorf("status = %q, want partial", got.Status())
	}
	if got.AffectedCounts()["vehicles"] != 4 {
		t.Errorf("affected = %v", got.AffectedCounts())
	}
	if len(got.FailedServices()) != 1 || got.FailedServices()[0] != "media" {
		t.Errorf("failed services = %v", got.FailedServices())
	}
}

// FR-ADMIN-RESTORE-4: the reaper's candidate set.
func TestListDue_selectsOnlyPendingAndPartialPastTheWindow(t *testing.T) {
	db := admintest.NewDB(t)
	adm := admin.NewAdministrator(db)
	prov := admin.NewProvider(db)

	mk := func(id string, status admin.Status, purgeAfter time.Time) {
		t.Helper()
		o, err := admin.NewOperationBuilder().
			SetID(id).
			SetScope(admin.ScopeSystem).
			SetTargetLabel("the entire platform").
			SetRequestedBy("admin-1", "admin@example.com").
			SetPurgeAfter(purgeAfter).
			Build()
		if err != nil {
			t.Fatalf("build: %v", err)
		}
		if err := db.Transaction(func(tx *gorm.DB) error { return adm.InsertOperation(tx, o) }); err != nil {
			t.Fatalf("insert: %v", err)
		}
		if status != admin.StatusPending {
			if err := adm.SetStatus(db, id, status, nil, testNow); err != nil {
				t.Fatalf("set status: %v", err)
			}
		}
	}
	past, future := testNow.Add(-time.Hour), testNow.Add(time.Hour)
	mk("due-pending", admin.StatusPending, past)
	mk("due-partial", admin.StatusPartial, past)
	mk("not-yet", admin.StatusPending, future)
	mk("cancelled", admin.StatusCancelled, past)
	mk("reaped", admin.StatusReaped, past)

	due, err := prov.ListDue(testNow)
	if err != nil {
		t.Fatalf("list due: %v", err)
	}
	got := map[string]bool{}
	for _, o := range due {
		got[o.ID()] = true
	}
	if !got["due-pending"] || !got["due-partial"] {
		t.Errorf("pending and partial operations past purge_after must be due, got %v", got)
	}
	if got["not-yet"] || got["cancelled"] || got["reaped"] {
		t.Errorf("a not-yet / cancelled / reaped operation must not be due, got %v", got)
	}
}

func TestGetOperation_missingIsNotFound(t *testing.T) {
	db := admintest.NewDB(t)
	if _, err := admin.NewProvider(db).GetOperation("nope"); err != admin.ErrOperationNotFound {
		t.Errorf("want ErrOperationNotFound, got %v", err)
	}
	_ = server.ErrNotFound // the resource layer maps the sentinel; the store returns its own
}
