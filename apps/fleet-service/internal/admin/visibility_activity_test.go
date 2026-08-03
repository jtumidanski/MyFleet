package admin_test

import (
	"testing"

	"github.com/jtumidanski/myfleet/apps/fleet-service/internal/activity"
	"github.com/jtumidanski/myfleet/apps/fleet-service/internal/admin/admintest"
	"github.com/jtumidanski/myfleet/packages/shared-go/server"
)

// LastActivityByVehicle feeds vehicle status derivation: a purged event that
// still counts as "recent activity" makes a dormant vehicle look healthy.
func TestActivityProvider_hidesSoftDeleted(t *testing.T) {
	db := admintest.NewDB(t)
	f := admintest.SeedFleet(t, db, "fleet-1")
	prov := activity.NewProvider(db)
	page := server.Page{Number: 1, Size: 25}

	if rows, total, err := prov.ListByFleet(f.FleetID, page); err != nil || len(rows) != 1 || total != 1 {
		t.Fatalf("fixture expected one activity event, got %d/%d err %v", len(rows), total, err)
	}

	if err := db.Exec(`UPDATE fleet.activity_events SET deleted_at = CURRENT_TIMESTAMP,
	                   purge_operation_id = 'op-1' WHERE id = ?`, f.ActivityEventID).Error; err != nil {
		t.Fatalf("soft delete: %v", err)
	}

	if rows, total, err := prov.ListByFleet(f.FleetID, page); err != nil || len(rows) != 0 || total != 0 {
		t.Errorf("ListByFleet must ignore soft-deleted events, got %d/%d err %v", len(rows), total, err)
	}
	if rows, total, err := prov.ListByVehicle(f.VehicleID, page); err != nil || len(rows) != 0 || total != 0 {
		t.Errorf("ListByVehicle must ignore soft-deleted events, got %d/%d err %v", len(rows), total, err)
	}
	last, err := prov.LastActivityByVehicle(f.VehicleID)
	if err != nil {
		t.Fatalf("last activity: %v", err)
	}
	if !last.IsZero() {
		t.Errorf("LastActivityByVehicle must ignore soft-deleted events, got %v", last)
	}
}
