package admin_test

import (
	"sort"
	"strings"
	"testing"

	"github.com/jtumidanski/myfleet/apps/fleet-service/internal/admin"
	"github.com/jtumidanski/myfleet/apps/fleet-service/internal/admin/admintest"
)

// countOnlyKeys are the plan steps that must NEVER carry a Set clause. They are
// FR-XFER-MOVE-2's "these follow the vehicle for free" list, expressed as an
// absence in the plan rather than left to a reviewer's memory.
var countOnlyKeys = map[string]bool{
	"maintenance_records":   true,
	"maintenance_schedules": true,
	"fuel_logs":             true,
	"mileage_records":       true,
	"vehicle_media":         true,
}

func TestTransferPlan_countOnlyStepsHaveNoSetClause(t *testing.T) {
	seen := map[string]bool{}
	for _, s := range admin.TransferPlan {
		if seen[s.Key] {
			t.Errorf("duplicate plan key %q — keys are API surface in affected_counts", s.Key)
		}
		seen[s.Key] = true
		if countOnlyKeys[s.Key] && s.Set != "" {
			t.Errorf("%s must be count-only (FR-XFER-MOVE-2) but has Set = %q", s.Key, s.Set)
		}
		if !countOnlyKeys[s.Key] && s.Set == "" {
			t.Errorf("%s has no Set clause; if that is intended, add it to countOnlyKeys", s.Key)
		}
		// R16: ApplyTransfer's Exec(q, spec.DestFleetID, spec.VehicleID) hardcodes
		// "exactly one ? in Set, exactly one ? in Where, in that order" — pin that
		// contract here so a future step that violates it fails in the test suite
		// rather than at runtime.
		if n := strings.Count(s.Set, "?"); n > 1 {
			t.Errorf("%s: Set has %d placeholders, want at most 1", s.Key, n)
		}
		if n := strings.Count(s.Where, "?"); n != 1 {
			t.Errorf("%s: Where has %d placeholders, want exactly 1", s.Key, n)
		}
	}
	for k := range countOnlyKeys {
		if !seen[k] {
			t.Errorf("plan is missing count-only step %q", k)
		}
	}
}

func TestCountTransfer_countsTheVehiclesOwnRowsOnly(t *testing.T) {
	db := admintest.NewDB(t)
	f := admintest.SeedFleet(t, db, "fleet-a")

	got, err := admin.CountTransfer(db, f.VehicleID)
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	// SeedFleet gives vehicle-1 exactly one of each and vehicle-2 none.
	for _, k := range []string{
		"maintenance_records", "maintenance_schedules", "fuel_logs",
		"mileage_records", "vehicle_media", "activity_events",
	} {
		if got[k] != 1 {
			t.Errorf("%s = %d, want 1", k, got[k])
		}
	}

	second, err := admin.CountTransfer(db, f.SecondVehicleID)
	if err != nil {
		t.Fatalf("count second: %v", err)
	}
	for k, n := range second {
		if n != 0 {
			t.Errorf("second vehicle %s = %d, want 0", k, n)
		}
	}
}

// A fleet-level activity event (vehicle_id IS NULL) describes the fleet, not
// the car, and must never be counted or moved (FR-XFER-MOVE-4).
func TestCountTransfer_ignoresFleetLevelActivityEvents(t *testing.T) {
	db := admintest.NewDB(t)
	f := admintest.SeedFleet(t, db, "fleet-a")
	if err := db.Exec(`INSERT INTO fleet.activity_events (id, fleet_id, vehicle_id, actor_user_id, type)
	                   VALUES ('ae-fleet', ?, NULL, ?, 'membership.joined')`,
		f.FleetID, f.OwnerUserID).Error; err != nil {
		t.Fatalf("seed fleet-level event: %v", err)
	}
	got, err := admin.CountTransfer(db, f.VehicleID)
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if got["activity_events"] != 1 {
		t.Errorf("activity_events = %d, want 1 (the fleet-level row must not be counted)", got["activity_events"])
	}
}

func TestVehicleMediaIDs_unionsPhotosPrimaryImageAndReceipts(t *testing.T) {
	db := admintest.NewDB(t)
	f := admintest.SeedFleet(t, db, "fleet-a")
	// SeedFleet already gives vehicle_media -> MediaID and a record document
	// -> MediaID (the same id). Add a distinct primary image and a distinct
	// receipt so the union has three members.
	if err := db.Exec(`UPDATE fleet.vehicles SET primary_image_media_id = 'media-primary' WHERE id = ?`,
		f.VehicleID).Error; err != nil {
		t.Fatalf("set primary image: %v", err)
	}
	if err := db.Exec(`INSERT INTO fleet.maintenance_record_documents (id, maintenance_record_id, media_id)
	                   VALUES ('doc-2', ?, 'media-receipt')`, f.MaintenanceRecordID).Error; err != nil {
		t.Fatalf("seed second document: %v", err)
	}

	ids, err := admin.VehicleMediaIDs(db, f.VehicleID)
	if err != nil {
		t.Fatalf("media ids: %v", err)
	}
	sort.Strings(ids)
	want := []string{f.MediaID, "media-primary", "media-receipt"}
	sort.Strings(want)
	if len(ids) != len(want) {
		t.Fatalf("media ids = %v, want %v", ids, want)
	}
	for i := range want {
		if ids[i] != want[i] {
			t.Fatalf("media ids = %v, want %v", ids, want)
		}
	}
}

// primary_image_media_id is a plain NOT NULL string column: "none" is the empty
// string, not NULL. Filtering it with IS NOT NULL would send media-service an
// empty id.
func TestVehicleMediaIDs_skipsEmptyPrimaryImage(t *testing.T) {
	db := admintest.NewDB(t)
	f := admintest.SeedFleet(t, db, "fleet-a")
	if err := db.Exec(`UPDATE fleet.vehicles SET primary_image_media_id = '' WHERE id = ?`,
		f.VehicleID).Error; err != nil {
		t.Fatalf("clear primary image: %v", err)
	}
	ids, err := admin.VehicleMediaIDs(db, f.VehicleID)
	if err != nil {
		t.Fatalf("media ids: %v", err)
	}
	for _, id := range ids {
		if id == "" {
			t.Fatalf("media ids contains the empty string: %v", ids)
		}
	}
}
