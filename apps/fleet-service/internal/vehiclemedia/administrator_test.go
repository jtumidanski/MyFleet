package vehiclemedia

import (
	"errors"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// newTestDB mirrors the harness used across fleet-service (maintenancerecord,
// dashboard, activity): SQLite in memory with a second in-memory database
// attached as "fleet", because TableName is schema-qualified for Postgres and
// SQLite has no schemas. Explicit DDL rather than AutoMigrate, since GORM emits
// CREATE INDEX with the schema prefix stripped for the `index`-tagged columns.
//
// fleet.vehicles is created too: SoftDelete reads and writes
// primary_image_media_id, and stubbing that out would skip the very logic these
// tests exist to cover.
func newTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.Exec("ATTACH DATABASE ':memory:' AS fleet").Error; err != nil {
		t.Fatalf("attach fleet schema: %v", err)
	}
	ddl := []string{
		`CREATE TABLE fleet.vehicle_media (
			id TEXT PRIMARY KEY, vehicle_id TEXT, media_id TEXT,
			is_primary BOOLEAN DEFAULT 0, sort_order INTEGER DEFAULT 0,
			created_at DATETIME, deleted_at DATETIME,
		purge_operation_id TEXT)`,
		`CREATE TABLE fleet.vehicles (
			id TEXT PRIMARY KEY, primary_image_media_id TEXT,
		deleted_at DATETIME, purge_operation_id TEXT)`,
	}
	for _, stmt := range ddl {
		if err := db.Exec(stmt).Error; err != nil {
			t.Fatalf("ddl: %v", err)
		}
	}
	return db
}

// seedVehicle creates the vehicle row and attaches photos in order, returning
// the administrator. The first media id becomes the primary, matching what the
// API does after an upload.
func seedVehicle(t *testing.T, db *gorm.DB, vehicleID string, mediaIDs ...string) Administrator {
	t.Helper()
	admin := NewAdministrator(db)
	if err := db.Exec("INSERT INTO fleet.vehicles (id, primary_image_media_id) VALUES (?, ?)",
		vehicleID, nil).Error; err != nil {
		t.Fatalf("seed vehicle: %v", err)
	}
	for i, mediaID := range mediaIDs {
		m := NewBuilder().SetVehicleID(vehicleID).SetMediaID(mediaID).SetSortOrder(i).Build()
		if _, err := admin.Insert(m); err != nil {
			t.Fatalf("insert %s: %v", mediaID, err)
		}
	}
	return admin
}

func setPrimary(t *testing.T, db *gorm.DB, vehicleID, mediaID string) {
	t.Helper()
	if err := db.Exec("UPDATE fleet.vehicle_media SET is_primary = 1 WHERE vehicle_id = ? AND media_id = ?",
		vehicleID, mediaID).Error; err != nil {
		t.Fatalf("set is_primary: %v", err)
	}
	if err := db.Exec("UPDATE fleet.vehicles SET primary_image_media_id = ? WHERE id = ?",
		mediaID, vehicleID).Error; err != nil {
		t.Fatalf("mirror primary: %v", err)
	}
}

func mirror(t *testing.T, db *gorm.DB, vehicleID string) *string {
	t.Helper()
	var got *string
	if err := db.Raw("SELECT primary_image_media_id FROM fleet.vehicles WHERE id = ?", vehicleID).
		Scan(&got).Error; err != nil {
		t.Fatalf("read mirror: %v", err)
	}
	return got
}

func liveMediaIDs(t *testing.T, db *gorm.DB, vehicleID string) []string {
	t.Helper()
	var ids []string
	if err := db.Raw(
		"SELECT media_id FROM fleet.vehicle_media WHERE vehicle_id = ? AND deleted_at IS NULL ORDER BY sort_order",
		vehicleID).Scan(&ids).Error; err != nil {
		t.Fatalf("list live: %v", err)
	}
	return ids
}

func TestSoftDelete_removesTheReferenceFromTheGallery(t *testing.T) {
	db := newTestDB(t)
	admin := seedVehicle(t, db, "v1", "m1", "m2")

	if err := admin.SoftDelete("v1", "m2"); err != nil {
		t.Fatalf("SoftDelete: %v", err)
	}

	// The read path filters on deleted_at IS NULL; leaving the row live is the
	// original defect (a removed photo kept rendering).
	if got := liveMediaIDs(t, db, "v1"); len(got) != 1 || got[0] != "m1" {
		t.Errorf("live media = %v, want [m1]", got)
	}
}

func TestSoftDelete_promotesASuccessorWhenThePrimaryGoes(t *testing.T) {
	db := newTestDB(t)
	admin := seedVehicle(t, db, "v1", "m1", "m2", "m3")
	setPrimary(t, db, "v1", "m1")

	if err := admin.SoftDelete("v1", "m1"); err != nil {
		t.Fatalf("SoftDelete: %v", err)
	}

	// Lowest sort_order among survivors, matching the gallery's own order.
	got := mirror(t, db, "v1")
	if got == nil || *got != "m2" {
		t.Errorf("primary_image_media_id = %v, want m2", got)
	}
	var primaries []string
	if err := db.Raw(
		"SELECT media_id FROM fleet.vehicle_media WHERE vehicle_id = ? AND is_primary = 1 AND deleted_at IS NULL",
		"v1").Scan(&primaries).Error; err != nil {
		t.Fatalf("count primaries: %v", err)
	}
	if len(primaries) != 1 || primaries[0] != "m2" {
		t.Errorf("primary rows = %v, want exactly [m2]", primaries)
	}
}

func TestSoftDelete_clearsTheMirrorWhenTheLastPhotoGoes(t *testing.T) {
	db := newTestDB(t)
	admin := seedVehicle(t, db, "v1", "m1")
	setPrimary(t, db, "v1", "m1")

	if err := admin.SoftDelete("v1", "m1"); err != nil {
		t.Fatalf("SoftDelete: %v", err)
	}

	// A mirror still pointing at the removed reference is what makes a vehicle
	// card render a photo the gallery no longer lists.
	if got := mirror(t, db, "v1"); got != nil {
		t.Errorf("primary_image_media_id = %q, want NULL", *got)
	}
}

func TestSoftDelete_leavesThePrimaryAloneWhenANonPrimaryGoes(t *testing.T) {
	db := newTestDB(t)
	admin := seedVehicle(t, db, "v1", "m1", "m2")
	setPrimary(t, db, "v1", "m1")

	if err := admin.SoftDelete("v1", "m2"); err != nil {
		t.Fatalf("SoftDelete: %v", err)
	}

	got := mirror(t, db, "v1")
	if got == nil || *got != "m1" {
		t.Errorf("primary_image_media_id = %v, want it untouched at m1", got)
	}
}

// The primary decision is derived from fleet.vehicles inside the transaction
// rather than from a flag the caller read earlier. A stale is_primary column
// must therefore NOT trigger a promotion — this is the concurrent-SetPrimary
// case, where trusting the caller would clobber the choice just made.
func TestSoftDelete_ignoresAStaleIsPrimaryFlag(t *testing.T) {
	db := newTestDB(t)
	admin := seedVehicle(t, db, "v1", "m1", "m2", "m3")
	// Row m1 still claims primary, but the vehicle now points at m3 — the state
	// left behind by a SetPrimary that raced this delete.
	if err := db.Exec("UPDATE fleet.vehicle_media SET is_primary = 1 WHERE media_id = 'm1'").Error; err != nil {
		t.Fatalf("seed stale flag: %v", err)
	}
	if err := db.Exec("UPDATE fleet.vehicles SET primary_image_media_id = 'm3' WHERE id = 'v1'").Error; err != nil {
		t.Fatalf("seed mirror: %v", err)
	}

	if err := admin.SoftDelete("v1", "m1"); err != nil {
		t.Fatalf("SoftDelete: %v", err)
	}

	got := mirror(t, db, "v1")
	if got == nil || *got != "m3" {
		t.Errorf("primary_image_media_id = %v, want m3 (the concurrent choice preserved)", got)
	}
}

func TestSoftDelete_refusesAnotherVehiclesMedia(t *testing.T) {
	db := newTestDB(t)
	admin := seedVehicle(t, db, "v1", "m1")
	seedVehicle(t, db, "v2", "m2")

	// v1 must not be able to remove v2's reference: the route authorizes the
	// vehicle, so the scope has to be enforced on the row too.
	if err := admin.SoftDelete("v1", "m2"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-vehicle delete err = %v, want ErrNotFound", err)
	}
	if got := liveMediaIDs(t, db, "v2"); len(got) != 1 {
		t.Errorf("v2 media = %v, want its row untouched", got)
	}
}

func TestSoftDelete_alreadyRemovedIsNotFound(t *testing.T) {
	db := newTestDB(t)
	admin := seedVehicle(t, db, "v1", "m1", "m2")
	setPrimary(t, db, "v1", "m1")

	if err := admin.SoftDelete("v1", "m2"); err != nil {
		t.Fatalf("first delete: %v", err)
	}
	if err := admin.SoftDelete("v1", "m2"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("second delete err = %v, want ErrNotFound", err)
	}
	// A no-op delete must not disturb the primary.
	if got := mirror(t, db, "v1"); got == nil || *got != "m1" {
		t.Errorf("primary_image_media_id = %v, want m1", got)
	}
}

func TestSoftDelete_unknownMediaIsNotFound(t *testing.T) {
	db := newTestDB(t)
	admin := seedVehicle(t, db, "v1", "m1")

	if err := admin.SoftDelete("v1", "nope"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown media err = %v, want ErrNotFound", err)
	}
}

// is_primary is cleared with deleted_at so a dead row can never be read as the
// primary by a query that forgets the deleted_at predicate.
func TestSoftDelete_clearsIsPrimaryOnTheRemovedRow(t *testing.T) {
	db := newTestDB(t)
	admin := seedVehicle(t, db, "v1", "m1", "m2")
	setPrimary(t, db, "v1", "m1")

	if err := admin.SoftDelete("v1", "m1"); err != nil {
		t.Fatalf("SoftDelete: %v", err)
	}

	var stillPrimary int64
	if err := db.Raw("SELECT COUNT(*) FROM fleet.vehicle_media WHERE media_id = 'm1' AND is_primary = 1").
		Scan(&stillPrimary).Error; err != nil {
		t.Fatalf("count: %v", err)
	}
	if stillPrimary != 0 {
		t.Errorf("removed row still flagged primary")
	}
}
