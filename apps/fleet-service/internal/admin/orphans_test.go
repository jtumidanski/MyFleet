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
