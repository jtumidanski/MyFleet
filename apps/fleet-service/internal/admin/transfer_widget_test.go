package admin_test

import (
	"testing"

	"gorm.io/gorm"

	"github.com/jtumidanski/myfleet/apps/fleet-service/internal/admin"
	"github.com/jtumidanski/myfleet/apps/fleet-service/internal/admin/admintest"
)

func addWidget(t *testing.T, db *gorm.DB, id, dashboardID, config string) {
	t.Helper()
	if err := db.Exec(`INSERT INTO fleet.dashboard_widgets
		(id, dashboard_id, type, position_x, position_y, width, height, config)
		VALUES (?, ?, 'vehicle-summary', 0, 0, 2, 2, ?)`, id, dashboardID, config).Error; err != nil {
		t.Fatalf("seed widget %s: %v", id, err)
	}
}

// FR-XFER-SRC-1/2/3 in one fixture: only the source fleet's widgets pinned to
// the MOVED vehicle are candidates.
func TestWidgetsPinnedToVehicle_scopesToSourceFleetAndVehicle(t *testing.T) {
	db := admintest.NewDB(t)
	a := admintest.SeedFleet(t, db, "fleet-a")
	b := admintest.SeedFleet(t, db, "fleet-b")

	pinned := `{"vehicleId":"` + a.VehicleID + `"}`
	addWidget(t, db, "w-source-pinned", a.DashboardID, pinned)
	addWidget(t, db, "w-source-other-vehicle", a.DashboardID,
		`{"vehicleId":"`+a.SecondVehicleID+`"}`)
	addWidget(t, db, "w-source-no-vehicle", a.DashboardID, `{"range":"90d"}`)
	addWidget(t, db, "w-source-malformed", a.DashboardID, `{not json`)
	// Same vehicle id, but a DIFFERENT fleet's dashboard: never a candidate.
	addWidget(t, db, "w-dest-pinned", b.DashboardID, pinned)

	ids, err := admin.WidgetsPinnedToVehicle(db, a.FleetID, a.VehicleID)
	if err != nil {
		t.Fatalf("pinned widgets: %v", err)
	}
	if len(ids) != 1 || ids[0] != "w-source-pinned" {
		t.Fatalf("pinned = %v, want [w-source-pinned]", ids)
	}
}

func TestPruneWidgets_deletesOnlyTheNamedRows(t *testing.T) {
	db := admintest.NewDB(t)
	a := admintest.SeedFleet(t, db, "fleet-a")
	addWidget(t, db, "w-doomed", a.DashboardID, `{"vehicleId":"`+a.VehicleID+`"}`)
	addWidget(t, db, "w-kept", a.DashboardID, `{"vehicleId":"`+a.SecondVehicleID+`"}`)

	n, err := admin.PruneWidgets(db, []string{"w-doomed"})
	if err != nil {
		t.Fatalf("prune: %v", err)
	}
	if n != 1 {
		t.Errorf("pruned = %d, want 1", n)
	}
	if got := scanOne[int](t, db,
		`SELECT count(*) FROM fleet.dashboard_widgets WHERE id = 'w-doomed'`); got != 0 {
		t.Error("the pinned widget survived; the delete is a HARD delete (design D5)")
	}
	if got := scanOne[int](t, db,
		`SELECT count(*) FROM fleet.dashboard_widgets WHERE id = 'w-kept'`); got != 1 {
		t.Error("an unrelated widget was deleted")
	}
	// The dashboard itself is fleet-scoped and stays.
	if got := scanOne[int](t, db,
		`SELECT count(*) FROM fleet.dashboards WHERE id = ?`, a.DashboardID); got != 1 {
		t.Error("the dashboard was deleted; only widgets are pruned")
	}
}

func TestPruneWidgets_emptyInputIsANoOp(t *testing.T) {
	db := admintest.NewDB(t)
	admintest.SeedFleet(t, db, "fleet-a")
	before := scanOne[int](t, db, `SELECT count(*) FROM fleet.dashboard_widgets`)

	n, err := admin.PruneWidgets(db, nil)
	if err != nil {
		t.Fatalf("prune: %v", err)
	}
	if n != 0 {
		t.Errorf("pruned = %d, want 0", n)
	}
	if after := scanOne[int](t, db, `SELECT count(*) FROM fleet.dashboard_widgets`); after != before {
		t.Errorf("widget count changed: %d -> %d", before, after)
	}
}
