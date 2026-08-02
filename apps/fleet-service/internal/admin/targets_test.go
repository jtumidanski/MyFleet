package admin_test

import (
	"errors"
	"testing"

	"github.com/jtumidanski/myfleet/apps/fleet-service/internal/admin"
	"github.com/jtumidanski/myfleet/apps/fleet-service/internal/admin/admintest"
	"github.com/jtumidanski/myfleet/packages/shared-go/server"
)

// The label is captured while the target still has a name — after the purge the
// rows are gone and the audit log would otherwise read as a bare uuid.
func TestTargetResolver_labelsEachScope(t *testing.T) {
	db := admintest.NewDB(t)
	f := admintest.SeedFleet(t, db, "fleet-1")
	r := admin.NewTargetResolver(db)

	label, _, err := r.Resolve(admin.Root{Scope: admin.ScopeSystem})
	if err != nil || label == "" {
		t.Errorf("system label = %q, err %v", label, err)
	}

	label, _, err = r.Resolve(admin.Root{Scope: admin.ScopeFleet, TargetID: f.FleetID})
	if err != nil {
		t.Fatalf("fleet: %v", err)
	}
	if label == "" {
		t.Error("a fleet must resolve to its name")
	}
}

// design OQ-1: a vehicle purge names the media that vehicle owns — its own
// images AND the documents on its maintenance records.
func TestTargetResolver_vehicleCarriesItsMediaIDs(t *testing.T) {
	db := admintest.NewDB(t)
	f := admintest.SeedFleet(t, db, "fleet-1")
	r := admin.NewTargetResolver(db)

	_, mediaIDs, err := r.Resolve(admin.Root{
		Scope: admin.ScopeRecord, TargetType: admin.TargetVehicle, TargetID: f.VehicleID,
	})
	if err != nil {
		t.Fatalf("vehicle: %v", err)
	}
	if len(mediaIDs) == 0 {
		t.Error("a vehicle with media must name its media ids")
	}
}

// An unknown id is a 404 BEFORE anything is written — not a purge of nothing
// that still leaves an operation row behind.
func TestTargetResolver_unknownTargetIsNotFound(t *testing.T) {
	db := admintest.NewDB(t)
	r := admin.NewTargetResolver(db)

	if _, _, err := r.Resolve(admin.Root{Scope: admin.ScopeFleet, TargetID: "nope"}); !errors.Is(err, server.ErrNotFound) {
		t.Errorf("unknown fleet: want 404, got %v", err)
	}
	if _, _, err := r.Resolve(admin.Root{
		Scope: admin.ScopeRecord, TargetType: admin.TargetVehicle, TargetID: "nope",
	}); !errors.Is(err, server.ErrNotFound) {
		t.Errorf("unknown vehicle: want 404, got %v", err)
	}
}
