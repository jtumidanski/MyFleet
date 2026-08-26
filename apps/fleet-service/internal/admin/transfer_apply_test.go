package admin_test

import (
	"reflect"
	"testing"

	"gorm.io/gorm"

	"github.com/jtumidanski/myfleet/apps/fleet-service/internal/admin"
	"github.com/jtumidanski/myfleet/apps/fleet-service/internal/admin/admintest"
)

// rowSnapshot is every column of one row, keyed by column name, so a test can
// assert byte-identity without enumerating columns it does not care about.
func rowSnapshot(t *testing.T, db *gorm.DB, table, id string) map[string]any {
	t.Helper()
	var out map[string]any
	if err := db.Raw("SELECT * FROM "+table+" WHERE id = ?", id).Scan(&out).Error; err != nil {
		t.Fatalf("snapshot %s/%s: %v", table, id, err)
	}
	if len(out) == 0 {
		t.Fatalf("snapshot %s/%s: no such row", table, id)
	}
	return out
}

// assertSameExcept compares with reflect.DeepEqual rather than !=: the snapshot
// values are `any`, and activity_events.payload scans back as []byte, which
// PANICS ("comparing uncomparable type []uint8") the moment a snapshotted
// payload is non-NULL.
func assertSameExcept(t *testing.T, label string, before, after map[string]any, except ...string) {
	t.Helper()
	skip := map[string]bool{}
	for _, k := range except {
		skip[k] = true
	}
	for k, v := range before {
		if skip[k] {
			continue
		}
		if got := after[k]; !reflect.DeepEqual(got, v) {
			t.Errorf("%s: column %s changed %v -> %v; the transfer must not touch it", label, k, v, got)
		}
	}
}

func applyFixture(t *testing.T) (*gorm.DB, admintest.Fixture, admin.TransferSpec) {
	t.Helper()
	db := admintest.NewDB(t)
	f := admintest.SeedFleet(t, db, "fleet-a")
	admintest.SeedFleet(t, db, "fleet-b")
	return db, f, transferSpec(f.VehicleID, "fleet-a", "fleet-b")
}

// FR-XFER-MOVE-1: the vehicle moves and created_at is preserved. The UPDATE
// never mentions created_at at all, which is strictly stronger than relying on
// GORM's `<-:create` tag (that only protects db.Save).
func TestApplyTransfer_movesTheVehicleAndPreservesCreatedAt(t *testing.T) {
	db, f, spec := applyFixture(t)
	before := rowSnapshot(t, db, "fleet.vehicles", f.VehicleID)

	if _, err := admin.ApplyTransfer(db, spec); err != nil {
		t.Fatalf("apply: %v", err)
	}

	after := rowSnapshot(t, db, "fleet.vehicles", f.VehicleID)
	if after["fleet_id"] != "fleet-b" {
		t.Errorf("fleet_id = %v, want fleet-b", after["fleet_id"])
	}
	assertSameExcept(t, "vehicles", before, after, "fleet_id", "updated_at")
}

// FR-XFER-MOVE-2, narrowed by design D10. Three tables must be BYTE-IDENTICAL,
// plus maintenance_record_documents; records and schedules may differ in
// category_id and nothing else.
func TestApplyTransfer_leavesVehicleScopedHistoryUntouched(t *testing.T) {
	db, f, spec := applyFixture(t)

	type target struct{ table, id string }
	identical := []target{
		{"fleet.mileage_records", f.MileageRecordID},
		{"fleet.fuel_logs", f.FuelLogID},
		{"fleet.vehicle_media", f.VehicleMediaID},
		{"fleet.maintenance_record_documents", f.DocumentID},
	}
	categoryOnly := []target{
		{"fleet.maintenance_records", f.MaintenanceRecordID},
		{"fleet.maintenance_schedules", f.ScheduleID},
	}

	before := map[string]map[string]any{}
	for _, tg := range append(append([]target{}, identical...), categoryOnly...) {
		before[tg.table] = rowSnapshot(t, db, tg.table, tg.id)
	}

	if _, err := admin.ApplyTransfer(db, spec); err != nil {
		t.Fatalf("apply: %v", err)
	}

	for _, tg := range identical {
		assertSameExcept(t, tg.table, before[tg.table], rowSnapshot(t, db, tg.table, tg.id))
	}
	for _, tg := range categoryOnly {
		after := rowSnapshot(t, db, tg.table, tg.id)
		assertSameExcept(t, tg.table, before[tg.table], after, "category_id")
		// SeedFleet uses a SYSTEM category, so even category_id is unchanged
		// here (FR-XFER-CAT-2).
		if after["category_id"] != before[tg.table]["category_id"] {
			t.Errorf("%s: a system category was remapped", tg.table)
		}
	}
}

// FR-XFER-MOVE-3/4 and design D11: fleet_id is rewritten and NOTHING else is.
func TestApplyTransfer_repointsVehicleActivityAndOnlyItsFleetID(t *testing.T) {
	db, f, spec := applyFixture(t)
	if err := db.Exec(`INSERT INTO fleet.activity_events (id, fleet_id, vehicle_id, actor_user_id, type, created_at)
	                   VALUES ('ae-fleet', ?, NULL, ?, 'membership.joined', ?)`,
		f.FleetID, f.OwnerUserID, seedNow()).Error; err != nil {
		t.Fatalf("seed fleet-level event: %v", err)
	}
	before := rowSnapshot(t, db, "fleet.activity_events", f.ActivityEventID)
	beforeFleetLevel := rowSnapshot(t, db, "fleet.activity_events", "ae-fleet")

	if _, err := admin.ApplyTransfer(db, spec); err != nil {
		t.Fatalf("apply: %v", err)
	}

	after := rowSnapshot(t, db, "fleet.activity_events", f.ActivityEventID)
	if after["fleet_id"] != "fleet-b" {
		t.Errorf("vehicle event fleet_id = %v, want fleet-b", after["fleet_id"])
	}
	assertSameExcept(t, "activity_events", before, after, "fleet_id")

	assertSameExcept(t, "activity_events (fleet-level)", beforeFleetLevel,
		rowSnapshot(t, db, "fleet.activity_events", "ae-fleet"))
}

// FR-XFER-SRC-4, and the ordering that makes it work: both events are inserted
// AFTER the bulk rewrite, so the OUT event keeps the SOURCE fleet id even
// though its vehicle_id matches the rewrite predicate.
func TestApplyTransfer_writesBothTransferEventsWithTheRightFleets(t *testing.T) {
	db, f, spec := applyFixture(t)

	if _, err := admin.ApplyTransfer(db, spec); err != nil {
		t.Fatalf("apply: %v", err)
	}

	out := scanOne[string](t, db,
		`SELECT fleet_id FROM fleet.activity_events WHERE type = ? AND vehicle_id = ?`,
		admin.EventVehicleTransferredOut, f.VehicleID)
	if out != "fleet-a" {
		t.Errorf("transferred_out fleet_id = %q, want fleet-a (it must survive the bulk rewrite)", out)
	}
	in := scanOne[string](t, db,
		`SELECT fleet_id FROM fleet.activity_events WHERE type = ? AND vehicle_id = ?`,
		admin.EventVehicleTransferredIn, f.VehicleID)
	if in != "fleet-b" {
		t.Errorf("transferred_in fleet_id = %q, want fleet-b", in)
	}

	actor := scanOne[string](t, db,
		`SELECT actor_user_id FROM fleet.activity_events WHERE type = ?`,
		admin.EventVehicleTransferredOut)
	if actor != "admin-1" {
		t.Errorf("transferred_out actor = %q, want admin-1", actor)
	}
	payload := scanOne[string](t, db,
		`SELECT payload FROM fleet.activity_events WHERE type = ?`, admin.EventVehicleTransferredOut)
	if payload == "" {
		t.Error("transferred_out payload is empty; it must name the counterpart fleet")
	}
}

// The two transfer events are inserted, so they must NOT inflate the
// activity_events figure the operator was shown. The count is taken before the
// writes for exactly that reason.
func TestApplyTransfer_countsMatchThePreview(t *testing.T) {
	db, f, spec := applyFixture(t)
	preview, err := admin.CountTransfer(db, f.VehicleID)
	if err != nil {
		t.Fatalf("preview count: %v", err)
	}

	applied, err := admin.ApplyTransfer(db, spec)
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	for k, want := range preview {
		if applied[k] != want {
			t.Errorf("%s: preview %d, applied %d", k, want, applied[k])
		}
	}
	if _, ok := applied["categories_created"]; !ok {
		t.Error("affected_counts is missing categories_created")
	}
	if _, ok := applied["widgets_removed"]; !ok {
		t.Error("affected_counts is missing widgets_removed")
	}
}

func TestApplyTransfer_reportsCategoriesCreatedAndWidgetsRemoved(t *testing.T) {
	db, f, spec := applyFixture(t)
	seedCustomCategory(t, db, f, "cat-winter", "Winter Tires", "maintenance")
	addWidget(t, db, "w-pinned", f.DashboardID, `{"vehicleId":"`+f.VehicleID+`"}`)

	applied, err := admin.ApplyTransfer(db, spec)
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if applied["categories_created"] != 1 {
		t.Errorf("categories_created = %d, want 1", applied["categories_created"])
	}
	if applied["widgets_removed"] != 1 {
		t.Errorf("widgets_removed = %d, want 1", applied["widgets_removed"])
	}
}

// An empty id is not a no-op, it is a WILDCARD: WidgetsPinnedToVehicle matches
// a `{}` config against an empty vehicle id, so an unvalidated spec would hard
// delete unrelated widgets. Every id is therefore checked before the first
// statement runs.
func TestApplyTransfer_rejectsAnEmptyIDBeforeTouchingAnything(t *testing.T) {
	cases := map[string]admin.TransferSpec{
		"vehicle":           transferSpec("", "fleet-a", "fleet-b"),
		"source fleet":      transferSpec("fleet-a-vehicle-1", "", "fleet-b"),
		"destination fleet": transferSpec("fleet-a-vehicle-1", "fleet-a", ""),
	}
	for name, spec := range cases {
		t.Run(name, func(t *testing.T) {
			db, f, _ := applyFixture(t)
			addWidget(t, db, "w-unpinned", f.DashboardID, `{}`)

			// Errorf, not Fatalf: when the guard is missing the assertions
			// below are the ones that show what the empty id actually did.
			if _, err := admin.ApplyTransfer(db, spec); err == nil {
				t.Errorf("apply with an empty %s id returned no error", name)
			}
			if n := scanOne[int](t, db,
				`SELECT count(*) FROM fleet.dashboard_widgets WHERE id = ?`, "w-unpinned"); n != 1 {
				t.Error("an unrelated widget was pruned by a spec that should have been rejected")
			}
			if got := scanOne[string](t, db,
				`SELECT fleet_id FROM fleet.vehicles WHERE id = ?`, f.VehicleID); got != "fleet-a" {
				t.Errorf("fleet_id = %q, want fleet-a: nothing may be written", got)
			}
			if n := scanOne[int](t, db, `SELECT count(*) FROM fleet.activity_events WHERE type = ?`,
				admin.EventVehicleTransferredOut); n != 0 {
				t.Error("a transfer activity event was written for a rejected spec")
			}
		})
	}
}

// The whole operation must be atomic. A failure anywhere inside the caller's
// transaction leaves the vehicle where it was.
func TestApplyTransfer_rollsBackWithTheCallersTransaction(t *testing.T) {
	db, f, spec := applyFixture(t)
	sentinel := errForTest{}

	err := db.Transaction(func(tx *gorm.DB) error {
		if _, aerr := admin.ApplyTransfer(tx, spec); aerr != nil {
			return aerr
		}
		return sentinel
	})
	if err != sentinel {
		t.Fatalf("transaction err = %v, want the sentinel", err)
	}
	if got := scanOne[string](t, db, `SELECT fleet_id FROM fleet.vehicles WHERE id = ?`,
		f.VehicleID); got != "fleet-a" {
		t.Errorf("fleet_id = %q after rollback, want fleet-a", got)
	}
	if n := scanOne[int](t, db, `SELECT count(*) FROM fleet.activity_events WHERE type = ?`,
		admin.EventVehicleTransferredIn); n != 0 {
		t.Error("a transfer activity event survived the rollback")
	}
}

type errForTest struct{}

func (errForTest) Error() string { return "rollback sentinel" }
