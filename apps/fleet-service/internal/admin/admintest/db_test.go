package admintest_test

import (
	"testing"

	"github.com/jtumidanski/myfleet/apps/fleet-service/internal/admin/admintest"
)

// TestNewDB_createsEveryPurgeableTable is the harness's own regression test: a
// table added to the manifest but forgotten here would otherwise surface as a
// confusing "no such table" deep inside a cascade test.
func TestNewDB_createsEveryPurgeableTable(t *testing.T) {
	db := admintest.NewDB(t)
	for _, table := range []string{
		"fleet.fleets", "fleet.vehicles", "fleet.fleet_memberships", "fleet.fleet_invites",
		"fleet.mileage_records", "fleet.fuel_logs", "fleet.maintenance_records",
		"fleet.maintenance_record_documents", "fleet.maintenance_schedules",
		"fleet.vehicle_media", "fleet.activity_events", "fleet.dashboards",
		"fleet.dashboard_widgets", "fleet.maintenance_categories",
		"fleet.purge_operations", "fleet.admin_audit_events",
	} {
		var n int64
		if err := db.Raw("SELECT count(*) FROM " + table).Scan(&n).Error; err != nil {
			t.Errorf("%s is missing from admintest.NewDB: %v", table, err)
		}
	}
}

// TestSeedFleet_populatesEveryTable makes the fixture usable as a cascade
// oracle: every purgeable table must have at least one row, or a cascade test
// can pass by simply never touching an empty table.
func TestSeedFleet_populatesEveryTable(t *testing.T) {
	db := admintest.NewDB(t)
	f := admintest.SeedFleet(t, db, "fleet-1")
	if f.VehicleID == "" || f.MaintenanceRecordID == "" || f.DashboardID == "" {
		t.Fatalf("fixture is incomplete: %+v", f)
	}
	for _, table := range []string{
		"fleet.vehicles", "fleet.fleet_memberships", "fleet.fleet_invites",
		"fleet.mileage_records", "fleet.fuel_logs", "fleet.maintenance_records",
		"fleet.maintenance_record_documents", "fleet.maintenance_schedules",
		"fleet.vehicle_media", "fleet.activity_events", "fleet.dashboards",
		"fleet.dashboard_widgets",
	} {
		var n int64
		if err := db.Raw("SELECT count(*) FROM " + table).Scan(&n).Error; err != nil {
			t.Fatalf("count %s: %v", table, err)
		}
		if n == 0 {
			t.Errorf("%s has no seeded rows — a cascade test over this fixture would pass vacuously", table)
		}
	}
}
