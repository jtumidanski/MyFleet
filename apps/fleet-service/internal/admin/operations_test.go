package admin_test

import (
	"testing"
	"time"

	"gorm.io/gorm"

	"github.com/jtumidanski/myfleet/apps/fleet-service/internal/admin"
	"github.com/jtumidanski/myfleet/apps/fleet-service/internal/admin/admintest"
)

var testNow = time.Date(2026, 8, 2, 14, 3, 11, 0, time.UTC)

// everyPurgeableTable is the assertion surface for "zero orphans". It is
// spelled out rather than derived from the manifest on purpose: deriving it
// would make the test agree with the manifest by construction, which is exactly
// the property under test.
var everyPurgeableTable = []string{
	"fleet.fleets", "fleet.vehicles", "fleet.fleet_memberships", "fleet.fleet_invites",
	"fleet.mileage_records", "fleet.fuel_logs", "fleet.maintenance_records",
	"fleet.maintenance_record_documents", "fleet.maintenance_schedules",
	"fleet.vehicle_media", "fleet.activity_events", "fleet.dashboards",
	"fleet.dashboard_widgets",
}

// FR-ADMIN-PURGE-4: a fleet purge leaves ZERO live rows anywhere beneath the
// fleet, and leaves every other fleet completely untouched.
func TestStamp_fleetScope_cascadesWithNoOrphans(t *testing.T) {
	db := admintest.NewDB(t)

	// The bystander fleet is seeded FIRST so its contribution to every table can
	// be recorded. Both halves of this test's claim then reduce to one equality:
	// after purging fleet-1, every table's live count is back to exactly the
	// bystander's baseline — no orphan of fleet-1 survives, and no row of
	// fleet-2 was taken.
	admintest.SeedFleet(t, db, "fleet-2")
	baseline := make(map[string]int, len(everyPurgeableTable))
	for _, table := range everyPurgeableTable {
		baseline[table] = admintest.CountLive(t, db, table)
	}

	target := admintest.SeedFleet(t, db, "fleet-1")

	root := admin.Root{Scope: admin.ScopeFleet, TargetID: target.FleetID}

	counts, err := admin.Count(db, root)
	if err != nil {
		t.Fatalf("count: %v", err)
	}

	var stamped map[string]int
	if err := db.Transaction(func(tx *gorm.DB) error {
		var serr error
		stamped, serr = admin.Stamp(tx, root, "op-1", testNow)
		return serr
	}); err != nil {
		t.Fatalf("stamp: %v", err)
	}

	// FR-ADMIN-UI-9: the blast-radius numbers and the purge's affected rows are
	// the same query, so they must be equal for every key.
	for key, want := range counts {
		if got := stamped[key]; got != want {
			t.Errorf("%s: blast radius said %d, purge stamped %d", key, want, got)
		}
	}

	for _, table := range everyPurgeableTable {
		if got := admintest.CountLive(t, db, table); got != baseline[table] {
			t.Errorf("%s has %d live rows after a fleet purge, want %d (the bystander fleet's rows and nothing else)",
				table, got, baseline[table])
		}
	}
	// The other fleet is untouched. Counting total rows in the second fleet is
	// the cross-tenant assertion: a predicate that resolved too widely would
	// stamp them.
	var liveElsewhere int64
	if err := db.Raw(`SELECT count(*) FROM fleet.vehicles
	                  WHERE fleet_id = 'fleet-2' AND deleted_at IS NULL`).Scan(&liveElsewhere).Error; err != nil {
		t.Fatalf("count other fleet: %v", err)
	}
	if liveElsewhere != 2 {
		t.Errorf("a fleet purge touched another fleet: %d of 2 vehicles remain live", liveElsewhere)
	}
	// Seeded reference data is a PRD non-goal and must survive.
	if got := admintest.CountRows(t, db, "fleet.maintenance_categories"); got != 1 {
		t.Errorf("maintenance categories must survive a purge, got %d rows", got)
	}
}

// FR-ADMIN-PURGE-3: rows already soft-deleted by ordinary product flows carry a
// NULL purge_operation_id and must be left alone by the stamp — and therefore
// must NOT come back when the operation is cancelled (FR-ADMIN-RESTORE-3).
func TestStampAndRestore_leaveIndependentlyDeletedRowsDeleted(t *testing.T) {
	db := admintest.NewDB(t)
	f := admintest.SeedFleet(t, db, "fleet-1")

	if err := db.Exec(`UPDATE fleet.fuel_logs SET deleted_at = ? WHERE id = ?`,
		testNow.Add(-24*time.Hour), f.FuelLogID).Error; err != nil {
		t.Fatalf("pre-existing user delete: %v", err)
	}

	root := admin.Root{Scope: admin.ScopeFleet, TargetID: f.FleetID}
	if err := db.Transaction(func(tx *gorm.DB) error {
		_, serr := admin.Stamp(tx, root, "op-1", testNow)
		return serr
	}); err != nil {
		t.Fatalf("stamp: %v", err)
	}

	var opID *string
	if err := db.Raw(`SELECT purge_operation_id FROM fleet.fuel_logs WHERE id = ?`, f.FuelLogID).
		Scan(&opID).Error; err != nil {
		t.Fatalf("read purge_operation_id: %v", err)
	}
	if opID != nil {
		t.Fatalf("the stamp claimed a row the user had already deleted (op %v)", *opID)
	}

	if err := db.Transaction(func(tx *gorm.DB) error { return admin.Restore(tx, "op-1") }); err != nil {
		t.Fatalf("restore: %v", err)
	}
	if got := admintest.CountLive(t, db, "fleet.fuel_logs"); got != 0 {
		t.Errorf("cancelling a purge resurrected a row the user had deleted: %d live", got)
	}
	// Everything the operation DID stamp comes back.
	if got := admintest.CountLive(t, db, "fleet.vehicles"); got != 2 {
		t.Errorf("restore did not return the vehicles: %d live of 2", got)
	}
	if got := admintest.CountLive(t, db, "fleet.fleets"); got != 1 {
		t.Errorf("restore did not return the fleet: %d live of 1", got)
	}
}

// FR-ADMIN-PURGE-10: a replayed stamp changes nothing and returns the SAME
// counts as the first call, not zeros. That is what makes the retry endpoint
// safe to press repeatedly AND leaves affected_counts correct afterwards.
func TestStamp_isIdempotent(t *testing.T) {
	db := admintest.NewDB(t)
	f := admintest.SeedFleet(t, db, "fleet-1")
	root := admin.Root{Scope: admin.ScopeFleet, TargetID: f.FleetID}

	run := func(now time.Time) map[string]int {
		var out map[string]int
		if err := db.Transaction(func(tx *gorm.DB) error {
			var serr error
			out, serr = admin.Stamp(tx, root, "op-1", now)
			return serr
		}); err != nil {
			t.Fatalf("stamp: %v", err)
		}
		return out
	}
	first := run(testNow)
	second := run(testNow.Add(time.Hour))

	for key, want := range first {
		if got := second[key]; got != want {
			t.Errorf("%s: replay returned %d, first call returned %d", key, got, want)
		}
	}
	var stampedAt time.Time
	if err := db.Raw(`SELECT deleted_at FROM fleet.vehicles WHERE id = ?`, f.VehicleID).
		Scan(&stampedAt).Error; err != nil {
		t.Fatalf("read deleted_at: %v", err)
	}
	if !stampedAt.UTC().Equal(testNow) {
		t.Errorf("a replayed stamp rewrote deleted_at: %v, want %v", stampedAt.UTC(), testNow)
	}
}

// scope:record with target_type:vehicle cascades to that vehicle's children and
// to nothing else (FR-ADMIN-PURGE-5).
func TestStamp_recordScopeVehicle_cascadesToChildrenOnly(t *testing.T) {
	db := admintest.NewDB(t)
	f := admintest.SeedFleet(t, db, "fleet-1")

	root := admin.Root{Scope: admin.ScopeRecord, TargetType: admin.TargetVehicle, TargetID: f.VehicleID}
	if err := db.Transaction(func(tx *gorm.DB) error {
		_, serr := admin.Stamp(tx, root, "op-1", testNow)
		return serr
	}); err != nil {
		t.Fatalf("stamp: %v", err)
	}

	for _, table := range []string{
		"fleet.mileage_records", "fleet.fuel_logs", "fleet.maintenance_records",
		"fleet.maintenance_record_documents", "fleet.maintenance_schedules", "fleet.vehicle_media",
	} {
		if got := admintest.CountLive(t, db, table); got != 0 {
			t.Errorf("%s still has %d live rows beneath the purged vehicle", table, got)
		}
	}
	if got := admintest.CountLive(t, db, "fleet.vehicles"); got != 1 {
		t.Errorf("expected the sibling vehicle to survive, %d live of 1", got)
	}
	if got := admintest.CountLive(t, db, "fleet.fleets"); got != 1 {
		t.Errorf("a record purge must not touch the fleet, %d live", got)
	}
	if got := admintest.CountLive(t, db, "fleet.fleet_memberships"); got != 1 {
		t.Errorf("a record purge must not touch memberships, %d live", got)
	}
}

// Reap is keyed purely on purge_operation_id, so it is idempotent and order
// independent (FR-ADMIN-RESTORE-6).
func TestReap_hardDeletesAndIsIdempotent(t *testing.T) {
	db := admintest.NewDB(t)
	f := admintest.SeedFleet(t, db, "fleet-1")
	root := admin.Root{Scope: admin.ScopeFleet, TargetID: f.FleetID}

	if err := db.Transaction(func(tx *gorm.DB) error {
		_, serr := admin.Stamp(tx, root, "op-1", testNow)
		return serr
	}); err != nil {
		t.Fatalf("stamp: %v", err)
	}

	deleted, err := admin.Reap(db, "op-1")
	if err != nil {
		t.Fatalf("reap: %v", err)
	}
	if deleted["vehicles"] != 2 {
		t.Errorf("reap reported %d vehicles deleted, want 2", deleted["vehicles"])
	}
	for _, table := range everyPurgeableTable {
		if got := admintest.CountRows(t, db, table); got != 0 {
			t.Errorf("%s still has %d rows after reap", table, got)
		}
	}

	again, err := admin.Reap(db, "op-1")
	if err != nil {
		t.Fatalf("second reap must succeed, got %v", err)
	}
	for key, n := range again {
		if n != 0 {
			t.Errorf("second reap deleted %d more %s rows", n, key)
		}
	}
}

// design §3.4: the ordering rule is a property, not a convention. Stamping in
// the exact reverse of the manifest's order must produce the same result.
func TestStamp_isOrderIndependent(t *testing.T) {
	forward := admintest.NewDB(t)
	f1 := admintest.SeedFleet(t, forward, "fleet-1")
	if err := forward.Transaction(func(tx *gorm.DB) error {
		_, serr := admin.Stamp(tx, admin.Root{Scope: admin.ScopeFleet, TargetID: f1.FleetID}, "op-1", testNow)
		return serr
	}); err != nil {
		t.Fatalf("forward stamp: %v", err)
	}

	reverse := admintest.NewDB(t)
	f2 := admintest.SeedFleet(t, reverse, "fleet-1")
	if err := reverse.Transaction(func(tx *gorm.DB) error {
		return admin.StampReversedForTest(tx, admin.Root{Scope: admin.ScopeFleet, TargetID: f2.FleetID}, "op-1", testNow)
	}); err != nil {
		t.Fatalf("reverse stamp: %v", err)
	}

	for _, table := range everyPurgeableTable {
		if a, b := admintest.CountLive(t, forward, table), admintest.CountLive(t, reverse, table); a != b {
			t.Errorf("%s: forward order left %d live, reverse order left %d — the cascade depends on manifest order", table, a, b)
		}
	}
}
