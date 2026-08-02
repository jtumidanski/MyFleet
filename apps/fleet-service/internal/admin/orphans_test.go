package admin_test

import (
	"testing"
	"time"

	"github.com/jtumidanski/myfleet/apps/fleet-service/internal/admin"
	"github.com/jtumidanski/myfleet/apps/fleet-service/internal/admin/admintest"
	"github.com/jtumidanski/myfleet/apps/fleet-service/internal/vehicle"
)

// PRD §11: hard-deleting a vehicle must take its children with it. Before this
// task the DELETE cascaded to nothing and the children lived forever.
func TestPurgeExpired_cascadesToChildren(t *testing.T) {
	db := admintest.NewDB(t)
	f := admintest.SeedFleet(t, db, "fleet-1")

	past := time.Now().UTC().Add(-time.Hour)
	if err := db.Exec(`UPDATE fleet.vehicles SET deleted_at = ?, purge_after = ? WHERE id = ?`,
		past, past, f.VehicleID).Error; err != nil {
		t.Fatalf("expire vehicle: %v", err)
	}

	if err := vehicle.PurgeExpired(db, admin.DeleteVehicleChildren); err != nil {
		t.Fatalf("purge expired: %v", err)
	}

	if got := admintest.CountRows(t, db, "fleet.vehicles"); got != 1 {
		t.Errorf("expected only the un-expired vehicle to remain, got %d rows", got)
	}
	for _, table := range []string{
		"fleet.mileage_records", "fleet.fuel_logs", "fleet.maintenance_records",
		"fleet.maintenance_record_documents", "fleet.maintenance_schedules", "fleet.vehicle_media",
	} {
		if got := admintest.CountRows(t, db, table); got != 0 {
			t.Errorf("%s left %d orphaned rows behind the purged vehicle", table, got)
		}
	}
}

// The cascade must reach ONLY the expired vehicle's own children. SeedFleet
// gives every fleet exactly one row per child table, so a single-fleet fixture
// cannot distinguish a correctly id-scoped DELETE from a bare
// `DELETE FROM <table>` with the `WHERE ... IN ?` accidentally dropped — both
// would leave that fixture's child tables at zero. A second, untouched fleet
// is what makes the difference observable.
func TestPurgeExpired_cascadeIsScopedToExpiredVehicle(t *testing.T) {
	db := admintest.NewDB(t)
	f1 := admintest.SeedFleet(t, db, "fleet-1")
	f2 := admintest.SeedFleet(t, db, "fleet-2")

	past := time.Now().UTC().Add(-time.Hour)
	if err := db.Exec(`UPDATE fleet.vehicles SET deleted_at = ?, purge_after = ? WHERE id = ?`,
		past, past, f1.VehicleID).Error; err != nil {
		t.Fatalf("expire vehicle: %v", err)
	}

	if err := vehicle.PurgeExpired(db, admin.DeleteVehicleChildren); err != nil {
		t.Fatalf("purge expired: %v", err)
	}

	rowExists := func(table, id string) bool {
		t.Helper()
		var n int64
		if err := db.Raw("SELECT count(*) FROM "+table+" WHERE id = ?", id).Scan(&n).Error; err != nil {
			t.Fatalf("count %s: %v", table, err)
		}
		return n == 1
	}

	// fleet-2's vehicle was never expired: every one of its child rows must
	// survive the cascade untouched.
	survivors := map[string]string{
		"fleet.mileage_records":              f2.MileageRecordID,
		"fleet.fuel_logs":                    f2.FuelLogID,
		"fleet.maintenance_records":          f2.MaintenanceRecordID,
		"fleet.maintenance_record_documents": f2.DocumentID,
		"fleet.maintenance_schedules":        f2.ScheduleID,
		"fleet.vehicle_media":                f2.VehicleMediaID,
	}
	for table, id := range survivors {
		if !rowExists(table, id) {
			t.Errorf("%s: fleet-2's row %s was deleted by a cascade meant to be scoped to fleet-1's vehicle", table, id)
		}
	}

	// fleet-1's own children must be gone.
	purged := map[string]string{
		"fleet.mileage_records":              f1.MileageRecordID,
		"fleet.fuel_logs":                    f1.FuelLogID,
		"fleet.maintenance_records":          f1.MaintenanceRecordID,
		"fleet.maintenance_record_documents": f1.DocumentID,
		"fleet.maintenance_schedules":        f1.ScheduleID,
		"fleet.vehicle_media":                f1.VehicleMediaID,
	}
	for table, id := range purged {
		if rowExists(table, id) {
			t.Errorf("%s: fleet-1's row %s survived its own vehicle's purge", table, id)
		}
	}
}

// FR-ADMIN-RESTORE-7 / design F3: the legacy sweep must not eat a vehicle that
// belongs to a pending, still-cancellable admin operation.
func TestPurgeExpired_skipsAdminStampedVehicles(t *testing.T) {
	db := admintest.NewDB(t)
	f := admintest.SeedFleet(t, db, "fleet-1")

	past := time.Now().UTC().Add(-time.Hour)
	// purge_after is set here deliberately: an admin stamp never writes it, so
	// this row could only exist if someone later "helpfully" did. The explicit
	// purge_operation_id IS NULL narrowing is what still saves it.
	if err := db.Exec(`UPDATE fleet.vehicles SET deleted_at = ?, purge_after = ?,
	                   purge_operation_id = 'op-1' WHERE id = ?`, past, past, f.VehicleID).Error; err != nil {
		t.Fatalf("stamp vehicle: %v", err)
	}

	if err := vehicle.PurgeExpired(db, admin.DeleteVehicleChildren); err != nil {
		t.Fatalf("purge expired: %v", err)
	}
	if got := admintest.CountRows(t, db, "fleet.vehicles"); got != 2 {
		t.Errorf("the legacy sweep hard-deleted an admin-stamped vehicle: %d of 2 rows remain", got)
	}
	if got := admintest.CountRows(t, db, "fleet.mileage_records"); got != 1 {
		t.Errorf("the legacy sweep cascaded into an admin-stamped vehicle: %d of 1 rows remain", got)
	}
}

// The one-time cleanup for rows the old sweep already orphaned (PRD §11b).
func TestDeleteOrphans_removesRowsWithNoParentAndIsANoOpWhenClean(t *testing.T) {
	db := admintest.NewDB(t)
	f := admintest.SeedFleet(t, db, "fleet-1")

	if removed, err := admin.DeleteOrphans(db); err != nil {
		t.Fatalf("clean-database cleanup: %v", err)
	} else {
		for key, n := range removed {
			if n != 0 {
				t.Errorf("cleanup removed %d %s rows from a clean database", n, key)
			}
		}
	}

	// Simulate the historical defect: the vehicle row is gone, its children stay.
	if err := db.Exec(`DELETE FROM fleet.vehicles WHERE id = ?`, f.VehicleID).Error; err != nil {
		t.Fatalf("orphan the children: %v", err)
	}

	removed, err := admin.DeleteOrphans(db)
	if err != nil {
		t.Fatalf("cleanup: %v", err)
	}
	if removed["mileage_records"] != 1 || removed["fuel_logs"] != 1 || removed["maintenance_records"] != 1 {
		t.Errorf("cleanup under-reported: %+v", removed)
	}
	for _, table := range []string{
		"fleet.mileage_records", "fleet.fuel_logs", "fleet.maintenance_records",
		"fleet.maintenance_record_documents", "fleet.maintenance_schedules", "fleet.vehicle_media",
	} {
		if got := admintest.CountRows(t, db, table); got != 0 {
			t.Errorf("%s still has %d orphaned rows", table, got)
		}
	}
	// activity_events has a nullable vehicle_id and belongs to the FLEET, so it
	// is deliberately not orphan-swept by vehicle deletion.
	if got := admintest.CountRows(t, db, "fleet.activity_events"); got != 1 {
		t.Errorf("activity events must survive a vehicle's deletion, got %d rows", got)
	}
	// The surviving fleet's own rows are untouched.
	if got := admintest.CountRows(t, db, "fleet.fleet_memberships"); got != 1 {
		t.Errorf("cleanup removed a live membership, %d rows remain", got)
	}
}
