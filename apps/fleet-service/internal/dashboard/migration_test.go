package dashboard

import (
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// newDashboardMigrationDB gives Migration a schema-qualified "fleet" database
// to run against. SQLite has no schemas, so one is attached under that alias.
func newDashboardMigrationDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.Exec("ATTACH DATABASE ':memory:' AS fleet").Error; err != nil {
		t.Fatalf("attach fleet schema: %v", err)
	}
	// AutoMigrate cannot create a schema-qualified index on SQLite (GORM's
	// driver strips the prefix), so the table is created by hand and only the
	// partial-index half of Migration is exercised here. That half is the one
	// with the lockout consequence.
	ddl := `CREATE TABLE fleet.dashboards (
		id TEXT PRIMARY KEY, fleet_id TEXT, user_id TEXT,
		created_at DATETIME, updated_at DATETIME,
		deleted_at DATETIME, purge_operation_id TEXT)`
	if err := db.Exec(ddl).Error; err != nil {
		t.Fatalf("ddl: %v", err)
	}
	return db
}

// TestPartialUniqueIndex_allowsNewDashboardAfterSoftDelete proves the
// (fleet_id, user_id) uniqueness invariant — a comment-only rule before this
// task — does not turn a purge into a permanent lockout: a soft-deleted
// dashboard must not occupy the partial index, or the user could never save a
// new layout after their old one is purged.
func TestPartialUniqueIndex_allowsNewDashboardAfterSoftDelete(t *testing.T) {
	db := newDashboardMigrationDB(t)
	if err := ApplyPartialIndexes(db); err != nil {
		t.Fatalf("apply partial indexes: %v", err)
	}

	insert := `INSERT INTO fleet.dashboards (id, fleet_id, user_id)
	           VALUES (?, 'fleet-1', 'user-1')`
	if err := db.Exec(insert, "d1").Error; err != nil {
		t.Fatalf("first insert: %v", err)
	}

	// A live duplicate is still rejected.
	if err := db.Exec(insert, "d2").Error; err == nil {
		t.Fatal("a second LIVE dashboard for the same (fleet, user) must violate the unique index")
	}

	// Soft-delete the first, then save a new one.
	if err := db.Exec(`UPDATE fleet.dashboards SET deleted_at = CURRENT_TIMESTAMP,
	                   purge_operation_id = 'op-1' WHERE id = 'd1'`).Error; err != nil {
		t.Fatalf("soft delete: %v", err)
	}
	if err := db.Exec(insert, "d3").Error; err != nil {
		t.Fatalf("new dashboard after soft delete must succeed, got %v", err)
	}
}

// TestApplyPartialIndexes_isIdempotent guards the boot path: Migration runs on
// every startup, and a non-idempotent DDL step would fail the second boot.
func TestApplyPartialIndexes_isIdempotent(t *testing.T) {
	db := newDashboardMigrationDB(t)
	for i := 0; i < 3; i++ {
		if err := ApplyPartialIndexes(db); err != nil {
			t.Fatalf("apply %d: %v", i+1, err)
		}
	}
}
