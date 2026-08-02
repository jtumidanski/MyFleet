package membership

import (
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// newMigrationDB gives Migration a schema-qualified "fleet" database to run
// against. SQLite has no schemas, so one is attached under that alias.
func newMigrationDB(t *testing.T) *gorm.DB {
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
	ddl := `CREATE TABLE fleet.fleet_memberships (
		id TEXT PRIMARY KEY, fleet_id TEXT, user_id TEXT, role TEXT, status TEXT,
		created_at DATETIME, updated_at DATETIME,
		deleted_at DATETIME, purge_operation_id TEXT)`
	if err := db.Exec(ddl).Error; err != nil {
		t.Fatalf("ddl: %v", err)
	}
	return db
}

// TestPartialUniqueIndex_allowsRejoinAfterSoftDelete is R2 in test form: a
// soft-deleted membership must not occupy the unique index, or purging a member
// locks them out of the fleet permanently.
func TestPartialUniqueIndex_allowsRejoinAfterSoftDelete(t *testing.T) {
	db := newMigrationDB(t)
	if err := ApplyPartialIndexes(db); err != nil {
		t.Fatalf("apply partial indexes: %v", err)
	}

	insert := `INSERT INTO fleet.fleet_memberships (id, fleet_id, user_id, role, status)
	           VALUES (?, 'fleet-1', 'user-1', 'member', 'active')`
	if err := db.Exec(insert, "m1").Error; err != nil {
		t.Fatalf("first insert: %v", err)
	}

	// A live duplicate is still rejected.
	if err := db.Exec(insert, "m2").Error; err == nil {
		t.Fatal("a second LIVE membership for the same (fleet, user) must violate the unique index")
	}

	// Soft-delete the first, then rejoin.
	if err := db.Exec(`UPDATE fleet.fleet_memberships SET deleted_at = CURRENT_TIMESTAMP,
	                   purge_operation_id = 'op-1' WHERE id = 'm1'`).Error; err != nil {
		t.Fatalf("soft delete: %v", err)
	}
	if err := db.Exec(insert, "m3").Error; err != nil {
		t.Fatalf("rejoin after soft delete must succeed, got %v", err)
	}
}

// TestApplyPartialIndexes_isIdempotent guards the boot path: Migration runs on
// every startup, and a non-idempotent DDL step would fail the second boot.
func TestApplyPartialIndexes_isIdempotent(t *testing.T) {
	db := newMigrationDB(t)
	for i := 0; i < 3; i++ {
		if err := ApplyPartialIndexes(db); err != nil {
			t.Fatalf("apply %d: %v", i+1, err)
		}
	}
}
