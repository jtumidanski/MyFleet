package admin_test

import (
	"testing"

	"github.com/jtumidanski/myfleet/apps/fleet-service/internal/admin/admintest"
	"github.com/jtumidanski/myfleet/apps/fleet-service/internal/maintenanceschedule"
	"github.com/jtumidanski/myfleet/packages/shared-go/server"
)

// The ListActive path backs /internal/maintenance/due, which drives the
// notification-service reminder job. A purged schedule that stays visible here
// keeps generating notifications for a fleet that no longer exists — the single
// worst miss in the whole visibility sweep (design §9).
func TestMaintenanceScheduleProvider_hidesSoftDeleted(t *testing.T) {
	db := admintest.NewDB(t)
	f := admintest.SeedFleet(t, db, "fleet-1")
	prov := maintenanceschedule.NewProvider(db)

	if _, err := prov.GetByID(f.ScheduleID); err != nil {
		t.Fatalf("fixture schedule should be readable: %v", err)
	}

	if err := db.Exec(`UPDATE fleet.maintenance_schedules SET deleted_at = CURRENT_TIMESTAMP,
	                   purge_operation_id = 'op-1' WHERE id = ?`, f.ScheduleID).Error; err != nil {
		t.Fatalf("soft delete: %v", err)
	}

	if _, err := prov.GetByID(f.ScheduleID); err != maintenanceschedule.ErrNotFound {
		t.Errorf("GetByID must report a soft-deleted schedule as not found, got %v", err)
	}

	rows, total, err := prov.ListByVehicle(f.VehicleID, server.Page{Number: 1, Size: 25})
	if err != nil {
		t.Fatalf("list by vehicle: %v", err)
	}
	if len(rows) != 0 || total != 0 {
		t.Errorf("soft-deleted schedule still listed: %d rows / total %d", len(rows), total)
	}

	active, err := prov.ListActive()
	if err != nil {
		t.Fatalf("list active: %v", err)
	}
	if len(active) != 0 {
		t.Errorf("soft-deleted schedule still reaches the reminder feed: %d rows", len(active))
	}

	byFleet, err := prov.ListActiveByFleet(f.FleetID)
	if err != nil {
		t.Fatalf("list active by fleet: %v", err)
	}
	if len(byFleet) != 0 {
		t.Errorf("soft-deleted schedule still in the fleet queue: %d rows", len(byFleet))
	}

	byVehicle, err := prov.ListActiveByVehicle(f.VehicleID)
	if err != nil {
		t.Fatalf("list active by vehicle: %v", err)
	}
	if len(byVehicle) != 0 {
		t.Errorf("soft-deleted schedule still in the vehicle queue: %d rows", len(byVehicle))
	}
}
