package admin_test

import (
	"testing"
	"time"

	"gorm.io/gorm"

	"github.com/jtumidanski/myfleet/apps/fleet-service/internal/admin"
	"github.com/jtumidanski/myfleet/apps/fleet-service/internal/admin/admintest"
)

// seedNow is the timestamp admintest.SeedFleet stamps on every row it inserts.
const seedYear = 2026

func seedNow() time.Time { return time.Date(seedYear, 1, 1, 12, 0, 0, 0, time.UTC) }

func transferSpec(vehicleID, src, dst string) admin.TransferSpec {
	return admin.TransferSpec{
		VehicleID:     vehicleID,
		SourceFleetID: src,
		DestFleetID:   dst,
		Label:         "The Green Bean",
		ActorUserID:   "admin-1",
		Now:           time.Date(2026, 8, 25, 18, 0, 0, 0, time.UTC),
	}
}

func scanOne[T any](t *testing.T, db *gorm.DB, q string, args ...any) T {
	t.Helper()
	var out T
	if err := db.Raw(q, args...).Scan(&out).Error; err != nil {
		t.Fatalf("query %.60s: %v", q, err)
	}
	return out
}

// seedCustomCategory attaches a fleet-scoped category to the seeded vehicle's
// record and schedule, replacing the system category SeedFleet used.
func seedCustomCategory(t *testing.T, db *gorm.DB, f admintest.Fixture, id, name, kind string) {
	t.Helper()
	if err := db.Exec(`INSERT INTO fleet.maintenance_categories
		(id, name, description, system_defined, kind, fleet_id)
		VALUES (?, ?, 'Seasonal swap', 0, ?, ?)`, id, name, kind, f.FleetID).Error; err != nil {
		t.Fatalf("seed category: %v", err)
	}
	if err := db.Exec(`UPDATE fleet.maintenance_records SET category_id = ? WHERE id = ?`,
		id, f.MaintenanceRecordID).Error; err != nil {
		t.Fatalf("point record at category: %v", err)
	}
	if err := db.Exec(`UPDATE fleet.maintenance_schedules SET category_id = ? WHERE id = ?`,
		id, f.ScheduleID).Error; err != nil {
		t.Fatalf("point schedule at category: %v", err)
	}
}

func TestResolveCategories_createsDestinationEquivalent(t *testing.T) {
	db := admintest.NewDB(t)
	f := admintest.SeedFleet(t, db, "fleet-a")
	admintest.SeedFleet(t, db, "fleet-b")
	seedCustomCategory(t, db, f, "cat-winter", "Winter Tires", "maintenance")

	created, err := admin.ResolveCategories(db, transferSpec(f.VehicleID, "fleet-a", "fleet-b"))
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if created != 1 {
		t.Fatalf("created = %d, want 1", created)
	}

	newID := scanOne[string](t, db,
		`SELECT category_id FROM fleet.maintenance_records WHERE id = ?`, f.MaintenanceRecordID)
	if newID == "cat-winter" {
		t.Fatal("record still points at the source-fleet category")
	}
	if got := scanOne[string](t, db,
		`SELECT category_id FROM fleet.maintenance_schedules WHERE id = ?`, f.ScheduleID); got != newID {
		t.Errorf("schedule category = %q, want the same resolved id %q", got, newID)
	}

	// FR-XFER-CAT-4: name, description and kind copied; fleet_id destination;
	// system_defined false.
	var row struct {
		Name          string
		Description   string
		Kind          string
		FleetID       string
		SystemDefined bool
	}
	if err := db.Raw(`SELECT name, description, kind, fleet_id, system_defined
	                  FROM fleet.maintenance_categories WHERE id = ?`, newID).Scan(&row).Error; err != nil {
		t.Fatalf("read new category: %v", err)
	}
	if row.Name != "Winter Tires" || row.Description != "Seasonal swap" || row.Kind != "maintenance" {
		t.Errorf("new category = %+v, want the source's name/description/kind", row)
	}
	if row.FleetID != "fleet-b" {
		t.Errorf("new category fleet_id = %q, want fleet-b", row.FleetID)
	}
	if row.SystemDefined {
		t.Error("new category is system_defined; a copied fleet category never is")
	}

	// FR-XFER-CAT-6: the source category survives untouched.
	if n := scanOne[int](t, db,
		`SELECT count(*) FROM fleet.maintenance_categories
		  WHERE id = 'cat-winter' AND fleet_id = 'fleet-a'`); n != 1 {
		t.Error("the source-fleet category was deleted or re-scoped")
	}
}

// FR-XFER-CAT-3: the lookup is case-INSENSITIVE, so a destination category that
// differs only in case is reused rather than duplicated.
func TestResolveCategories_reusesCaseInsensitiveMatch(t *testing.T) {
	db := admintest.NewDB(t)
	f := admintest.SeedFleet(t, db, "fleet-a")
	admintest.SeedFleet(t, db, "fleet-b")
	seedCustomCategory(t, db, f, "cat-winter", "Winter Tires", "maintenance")
	if err := db.Exec(`INSERT INTO fleet.maintenance_categories
		(id, name, description, system_defined, kind, fleet_id)
		VALUES ('cat-dest', 'winter tires', '', 0, 'maintenance', 'fleet-b')`).Error; err != nil {
		t.Fatalf("seed destination category: %v", err)
	}

	created, err := admin.ResolveCategories(db, transferSpec(f.VehicleID, "fleet-a", "fleet-b"))
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if created != 0 {
		t.Errorf("created = %d, want 0 — the differing-case destination row must be reused", created)
	}
	if got := scanOne[string](t, db,
		`SELECT category_id FROM fleet.maintenance_records WHERE id = ?`,
		f.MaintenanceRecordID); got != "cat-dest" {
		t.Errorf("record category = %q, want cat-dest", got)
	}
}

// Kind is matched EXACTLY. A modification category named the same as a
// maintenance one is a different thing and must not be collapsed into it.
func TestResolveCategories_doesNotMatchAcrossKind(t *testing.T) {
	db := admintest.NewDB(t)
	f := admintest.SeedFleet(t, db, "fleet-a")
	admintest.SeedFleet(t, db, "fleet-b")
	seedCustomCategory(t, db, f, "cat-winter", "Winter Tires", "modification")
	if err := db.Exec(`INSERT INTO fleet.maintenance_categories
		(id, name, description, system_defined, kind, fleet_id)
		VALUES ('cat-dest', 'Winter Tires', '', 0, 'maintenance', 'fleet-b')`).Error; err != nil {
		t.Fatalf("seed destination category: %v", err)
	}

	created, err := admin.ResolveCategories(db, transferSpec(f.VehicleID, "fleet-a", "fleet-b"))
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if created != 1 {
		t.Errorf("created = %d, want 1 — a different kind is a different category", created)
	}
}

// FR-XFER-CAT-2: system categories (fleet_id IS NULL) are globally visible and
// must never be remapped. SeedFleet already points the record and schedule at
// 'category-1', a system row.
func TestResolveCategories_leavesSystemCategoriesAlone(t *testing.T) {
	db := admintest.NewDB(t)
	f := admintest.SeedFleet(t, db, "fleet-a")
	admintest.SeedFleet(t, db, "fleet-b")

	created, err := admin.ResolveCategories(db, transferSpec(f.VehicleID, "fleet-a", "fleet-b"))
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if created != 0 {
		t.Errorf("created = %d, want 0", created)
	}
	if got := scanOne[string](t, db,
		`SELECT category_id FROM fleet.maintenance_records WHERE id = ?`,
		f.MaintenanceRecordID); got != "category-1" {
		t.Errorf("record category = %q, want the untouched system category-1", got)
	}
}

// Another vehicle in the source fleet keeps pointing at the source category:
// the remap is scoped to the MOVED vehicle's rows only.
func TestResolveCategories_doesNotRemapOtherVehicles(t *testing.T) {
	db := admintest.NewDB(t)
	f := admintest.SeedFleet(t, db, "fleet-a")
	admintest.SeedFleet(t, db, "fleet-b")
	seedCustomCategory(t, db, f, "cat-winter", "Winter Tires", "maintenance")
	if err := db.Exec(`INSERT INTO fleet.maintenance_records
		(id, vehicle_id, category_id, description, performed_at, mileage, cost, created_at)
		VALUES ('rec-other', ?, 'cat-winter', 'Tires', ?, 1, 1.0, ?)`,
		f.SecondVehicleID, seedNow(), seedNow()).Error; err != nil {
		t.Fatalf("seed other vehicle record: %v", err)
	}

	if _, err := admin.ResolveCategories(db, transferSpec(f.VehicleID, "fleet-a", "fleet-b")); err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if got := scanOne[string](t, db,
		`SELECT category_id FROM fleet.maintenance_records WHERE id = 'rec-other'`); got != "cat-winter" {
		t.Errorf("other vehicle's record category = %q, want the untouched cat-winter", got)
	}
}

// Ruling R14 (task-031 fix round 1): findDestinationCategory intentionally
// matches only fleet_id = destFleetID and does NOT consider system rows
// (fleet_id IS NULL), even though maintenancecategory.Provider.FindByName
// treats system rows as visible/preferred matches for the picker. This is a
// deliberate divergence, not an oversight: the transfer is reproducing the
// state the SOURCE fleet was already in — a fleet-scoped category shadowing a
// same-named system one — and must carry over that category's own
// description. Reusing the destination's system row instead would discard
// that description and silently lose user data. A duplicate picker entry in
// the destination fleet is judged the lesser harm. Do not "fix" this by
// making findDestinationCategory consider system rows.
func TestResolveCategories_sourceCategoryShadowingASystemOneIsCopiedNotReused(t *testing.T) {
	db := admintest.NewDB(t)
	f := admintest.SeedFleet(t, db, "fleet-a")
	admintest.SeedFleet(t, db, "fleet-b")
	// SeedFleet's shared system category is ('category-1', 'Oil Change', NULL
	// description, system_defined, 'maintenance', fleet_id NULL). Give fleet-a
	// its own fleet-scoped category with the SAME name and kind, carrying a
	// description the system row does not have.
	seedCustomCategory(t, db, f, "cat-oil-fleet-a", "Oil Change", "maintenance")

	created, err := admin.ResolveCategories(db, transferSpec(f.VehicleID, "fleet-a", "fleet-b"))
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if created != 1 {
		t.Fatalf("created = %d, want 1 — the system row must not be treated as an existing destination match", created)
	}

	newID := scanOne[string](t, db,
		`SELECT category_id FROM fleet.maintenance_records WHERE id = ?`, f.MaintenanceRecordID)
	if newID == "category-1" {
		t.Fatal("record was remapped onto the system category rather than a new fleet-scoped one")
	}
	if newID == "cat-oil-fleet-a" {
		t.Fatal("record still points at the source-fleet category")
	}

	var row struct {
		Name          string
		Description   string
		Kind          string
		FleetID       string
		SystemDefined bool
	}
	if err := db.Raw(`SELECT name, description, kind, fleet_id, system_defined
	                  FROM fleet.maintenance_categories WHERE id = ?`, newID).Scan(&row).Error; err != nil {
		t.Fatalf("read new category: %v", err)
	}
	if row.FleetID != "fleet-b" {
		t.Errorf("new category fleet_id = %q, want fleet-b", row.FleetID)
	}
	if row.SystemDefined {
		t.Error("new category is system_defined; a copied fleet category never is")
	}
	if row.Description != "Seasonal swap" {
		t.Errorf("new category description = %q, want the source category's own description (proves the system row was not reused)", row.Description)
	}

	// Exactly one row named "Oil Change"/maintenance now exists in fleet-b
	// besides the untouched global system row: the copy, not a reuse.
	if n := scanOne[int](t, db,
		`SELECT count(*) FROM fleet.maintenance_categories
		  WHERE fleet_id = 'fleet-b' AND name = 'Oil Change' AND kind = 'maintenance'`); n != 1 {
		t.Errorf("fleet-b Oil Change/maintenance rows = %d, want 1 new fleet-scoped copy", n)
	}
}

// PreviewCategories must report exactly what ResolveCategories would create,
// without writing anything (FR-XFER-UI-4).
func TestPreviewCategories_namesWhatWouldBeCreatedAndWritesNothing(t *testing.T) {
	db := admintest.NewDB(t)
	f := admintest.SeedFleet(t, db, "fleet-a")
	admintest.SeedFleet(t, db, "fleet-b")
	seedCustomCategory(t, db, f, "cat-winter", "Winter Tires", "maintenance")

	before := scanOne[int](t, db, `SELECT count(*) FROM fleet.maintenance_categories`)
	got, err := admin.PreviewCategories(db, transferSpec(f.VehicleID, "fleet-a", "fleet-b"))
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	if len(got) != 1 || got[0].Name != "Winter Tires" || got[0].Kind != "maintenance" {
		t.Fatalf("preview = %+v, want one Winter Tires/maintenance entry", got)
	}
	if after := scanOne[int](t, db, `SELECT count(*) FROM fleet.maintenance_categories`); after != before {
		t.Errorf("preview inserted rows: %d -> %d", before, after)
	}
	if got := scanOne[string](t, db,
		`SELECT category_id FROM fleet.maintenance_records WHERE id = ?`,
		f.MaintenanceRecordID); got != "cat-winter" {
		t.Error("preview remapped a record")
	}
}
