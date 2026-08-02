package fleet

import (
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// newFleetDB builds the in-memory harness. Schema-qualified TableNames
// ("fleet.fleets") target Postgres; SQLite has no schemas, so attach an
// in-memory database aliased "fleet". Explicit DDL rather than AutoMigrate:
// Entity carries an `index` tag on DeletedAt and GORM emits CREATE INDEX with
// the schema prefix stripped under SQLite, which cannot resolve against an
// attached schema.
func newFleetDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.Exec("ATTACH DATABASE ':memory:' AS fleet").Error; err != nil {
		t.Fatalf("attach fleet schema: %v", err)
	}
	// KEEP IN SYNC WITH entity.go.
	if err := db.Exec(`CREATE TABLE fleet.fleets (
		id TEXT PRIMARY KEY, name TEXT, created_by_user_id TEXT,
		created_at DATETIME, updated_at DATETIME, deleted_at DATETIME)`).Error; err != nil {
		t.Fatalf("create fleet.fleets: %v", err)
	}
	return db
}

func seedFleet(t *testing.T, db *gorm.DB) Model {
	t.Helper()
	m, err := NewBuilder().SetName("before").SetCreatedByUserID("u1").Build()
	if err != nil {
		t.Fatalf("build fleet: %v", err)
	}
	created, err := NewAdministrator(db).Insert(m)
	if err != nil {
		t.Fatalf("insert fleet: %v", err)
	}
	if created.CreatedAt().IsZero() {
		t.Fatal("Insert left created_at zero; this harness cannot detect the defect it exists for")
	}
	return created
}

func readFleetCreatedAt(t *testing.T, db *gorm.DB, id string) time.Time {
	t.Helper()
	var got time.Time
	if err := db.Raw("SELECT created_at FROM fleet.fleets WHERE id = ?", id).Scan(&got).Error; err != nil {
		t.Fatalf("read created_at: %v", err)
	}
	return got
}

// FR-FIX-2. Processor.Rename is Update(m.WithName(name)) — so before the fix,
// renaming a fleet destroyed its creation date. No read consumes it today, but
// Model.CreatedAt() is exported and the loss is unrecoverable once it happens.
func TestUpdate_preservesCreatedAtAcrossRename(t *testing.T) {
	db := newFleetDB(t)
	created := seedFleet(t, db)
	want := readFleetCreatedAt(t, db, created.ID())

	updated, err := NewAdministrator(db).Update(created.WithName("renamed"))
	if err != nil {
		t.Fatalf("update: %v", err)
	}

	got := readFleetCreatedAt(t, db, created.ID())
	if got.IsZero() {
		t.Fatal("Rename zeroed created_at; a full-column Save must not write this column")
	}
	if !got.Equal(want) {
		t.Fatalf("created_at changed across Rename: got %v, want %v", got, want)
	}
	if updated.CreatedAt().IsZero() {
		t.Fatal("Update returned a Model with a zero CreatedAt(); ToEntity must assign the field")
	}
	if updated.Name() != "renamed" {
		t.Fatalf("Name = %q, want %q — the rename itself must still apply", updated.Name(), "renamed")
	}
	// deleted_at must remain NULL: a live fleet must not acquire a tombstone.
	// Row().Scan (not GORM's own Scan) because GORM's Scan fast-paths *time.Time
	// but not **time.Time, and falls back to a reflection path that errors on a
	// NULL column; Row().Scan goes straight to database/sql, which handles a nil
	// destination pointer correctly.
	var deleted *time.Time
	if err := db.Raw("SELECT deleted_at FROM fleet.fleets WHERE id = ?", created.ID()).
		Row().Scan(&deleted); err != nil {
		t.Fatalf("read deleted_at: %v", err)
	}
	if deleted != nil {
		t.Fatalf("Rename set deleted_at to %v on a live fleet", *deleted)
	}
}

// FR-FIX-3, the design §2 V4 shape: ToEntity dropped DeletedAt, so a
// full-column Save wrote NULL over any soft-delete tombstone. Unreachable today
// (nothing loads a deleted fleet), which is why the test drives GORM's soft
// delete directly rather than through a domain method that does not exist yet.
func TestUpdate_doesNotResurrectASoftDeletedFleet(t *testing.T) {
	db := newFleetDB(t)
	created := seedFleet(t, db)

	if err := db.Delete(&Entity{ID: created.ID()}).Error; err != nil {
		t.Fatalf("soft delete: %v", err)
	}

	var e Entity
	if err := db.Unscoped().First(&e, "id = ?", created.ID()).Error; err != nil {
		t.Fatalf("unscoped read: %v", err)
	}
	m := Make(e)
	if m.DeletedAt() == nil {
		t.Fatal("Make() dropped the soft-delete tombstone; the Model must carry deletedAt")
	}
	if _, err := NewAdministrator(db).Update(m.WithName("renamed")); err != nil {
		t.Fatalf("update: %v", err)
	}

	var visible int64
	if err := db.Model(&Entity{}).Where("id = ?", created.ID()).Count(&visible).Error; err != nil {
		t.Fatalf("count: %v", err)
	}
	if visible != 0 {
		t.Fatalf("a full-column Save resurrected the soft-deleted fleet: it is visible to a "+
			"scoped read again (count=%d)", visible)
	}
}
