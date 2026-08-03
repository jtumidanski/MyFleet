package admin_test

import (
	"testing"

	"github.com/jtumidanski/myfleet/apps/fleet-service/internal/admin/admintest"
	"github.com/jtumidanski/myfleet/apps/fleet-service/internal/dashboard"
)

func TestDashboardProvider_hidesSoftDeleted(t *testing.T) {
	db := admintest.NewDB(t)
	f := admintest.SeedFleet(t, db, "fleet-1")
	prov := dashboard.NewProvider(db)

	d, err := prov.GetDashboard(f.FleetID, f.OwnerUserID)
	if err != nil {
		t.Fatalf("fixture dashboard should be readable: %v", err)
	}
	if len(d.Widgets()) != 1 {
		t.Fatalf("fixture expected one widget, got %d", len(d.Widgets()))
	}

	if err := db.Exec(`UPDATE fleet.dashboards SET deleted_at = CURRENT_TIMESTAMP,
	                   purge_operation_id = 'op-1' WHERE id = ?`, f.DashboardID).Error; err != nil {
		t.Fatalf("soft delete: %v", err)
	}

	d, err = prov.GetDashboard(f.FleetID, f.OwnerUserID)
	if err != nil {
		t.Fatalf("get after soft delete: %v", err)
	}
	if len(d.Widgets()) != 0 {
		t.Errorf("a soft-deleted dashboard must read as an empty layout, got %d widgets", len(d.Widgets()))
	}
}

// design §6.4: saving a layout while the dashboard row is soft-deleted must
// REVIVE that row, not insert a second one. Two live rows for one (fleet, user)
// makes the read non-deterministic and survives a later cancel.
func TestDashboardAdministrator_revivesSoftDeletedLayout(t *testing.T) {
	db := admintest.NewDB(t)
	f := admintest.SeedFleet(t, db, "fleet-1")
	adm := dashboard.NewAdministrator(db)

	if err := db.Exec(`UPDATE fleet.dashboards SET deleted_at = CURRENT_TIMESTAMP,
	                   purge_operation_id = 'op-1' WHERE id = ?`, f.DashboardID).Error; err != nil {
		t.Fatalf("soft delete: %v", err)
	}

	saved, err := adm.Replace(f.FleetID, f.OwnerUserID, []dashboard.WidgetInput{
		{Type: "vehicle-status", PositionX: 0, PositionY: 0, Width: 2, Height: 2},
	})
	if err != nil {
		t.Fatalf("replace after soft delete: %v", err)
	}
	if saved.ID() != f.DashboardID {
		t.Errorf("Replace inserted a new dashboard row %q instead of reviving %q", saved.ID(), f.DashboardID)
	}
	if got := admintest.CountRows(t, db, "fleet.dashboards"); got != 1 {
		t.Errorf("expected exactly one dashboard row after revive, got %d", got)
	}
	if got := admintest.CountLive(t, db, "fleet.dashboards"); got != 1 {
		t.Errorf("the revived dashboard must be live, got %d live rows", got)
	}

	var opID *string
	if err := db.Raw(`SELECT purge_operation_id FROM fleet.dashboards WHERE id = ?`, f.DashboardID).
		Scan(&opID).Error; err != nil {
		t.Fatalf("read purge_operation_id: %v", err)
	}
	if opID != nil {
		t.Errorf("a revived dashboard must leave its purge operation, got %v", *opID)
	}
}
