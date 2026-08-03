package preferences

import (
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// newPreferencesMigrationDB gives ApplyPartialIndexes a schema-qualified
// "notification" database to run against, mirroring
// notification/migration_test.go. SQLite has no schemas, so one is attached
// under that alias, and the table is created by hand since AutoMigrate cannot
// create a schema-qualified index on SQLite.
func newPreferencesMigrationDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.Exec("ATTACH DATABASE ':memory:' AS notification").Error; err != nil {
		t.Fatalf("attach notification schema: %v", err)
	}
	ddl := `CREATE TABLE notification.notification_preferences (
		id TEXT PRIMARY KEY, user_id TEXT, type TEXT, in_app_enabled BOOLEAN,
		deleted_at DATETIME, purge_operation_id TEXT)`
	if err := db.Exec(ddl).Error; err != nil {
		t.Fatalf("ddl: %v", err)
	}
	return db
}

// TestPartialUniqueIndex_allowsReCreationAfterSoftDelete is design F1 in test
// form: after a system purge, a user's preference row for a given type must be
// re-creatable — a total unique index would leave the purged row permanently
// occupying the key.
func TestPartialUniqueIndex_allowsReCreationAfterSoftDelete(t *testing.T) {
	db := newPreferencesMigrationDB(t)
	if err := ApplyPartialIndexes(db); err != nil {
		t.Fatalf("apply partial indexes: %v", err)
	}

	insert := `INSERT INTO notification.notification_preferences (id, user_id, type, in_app_enabled)
	           VALUES (?, 'user-1', 'schedule.overdue', 1)`
	if err := db.Exec(insert, "p1").Error; err != nil {
		t.Fatalf("first insert: %v", err)
	}
	if err := db.Exec(insert, "p2").Error; err == nil {
		t.Fatal("a second LIVE preference row for the same (user_id, type) must be rejected")
	}

	if err := db.Exec(`UPDATE notification.notification_preferences SET deleted_at = CURRENT_TIMESTAMP,
	                   purge_operation_id = 'op-1' WHERE id = 'p1'`).Error; err != nil {
		t.Fatalf("soft delete: %v", err)
	}
	if err := db.Exec(insert, "p3").Error; err != nil {
		t.Fatalf("re-creation after soft delete must succeed, got %v", err)
	}
}

func TestPreferencesApplyPartialIndexes_isIdempotent(t *testing.T) {
	db := newPreferencesMigrationDB(t)
	for i := 0; i < 3; i++ {
		if err := ApplyPartialIndexes(db); err != nil {
			t.Fatalf("apply %d: %v", i+1, err)
		}
	}
}

// TestUpsert_returnsLiveRowAfterSoftDelete is a regression test for the
// post-write re-read in Upsert: once (user_id, type) uniqueness is partial, a
// soft-deleted historical row can coexist with a new live one, and the re-read
// must not resolve to the stale/purged row.
//
// GORM's First() with no explicit Order ties-break on primary key (id)
// ascending, which is a random UUID here and unrelated to which row is live.
// The purged row's id is deliberately set to the lexicographically-smallest
// possible UUID ("000...0") so that, without a deleted_at filter, First()
// would deterministically pick it over the new live row's randomly-generated
// id — making this a reliable regression test rather than one that only fails
// by chance on unlucky UUID ordering.
func TestUpsert_returnsLiveRowAfterSoftDelete(t *testing.T) {
	db := newPreferencesMigrationDB(t)
	if err := ApplyPartialIndexes(db); err != nil {
		t.Fatalf("apply partial indexes: %v", err)
	}

	const purgedID = "00000000-0000-0000-0000-000000000000"
	insert := `INSERT INTO notification.notification_preferences
	           (id, user_id, type, in_app_enabled, deleted_at, purge_operation_id)
	           VALUES (?, 'user-1', 'schedule.overdue', 1, CURRENT_TIMESTAMP, 'op-1')`
	if err := db.Exec(insert, purgedID).Error; err != nil {
		t.Fatalf("seed purged row: %v", err)
	}

	adm := NewAdministrator(db)
	got, err := adm.Upsert("user-1", "schedule.overdue", false)
	if err != nil {
		t.Fatalf("upsert after pre-existing purge: %v", err)
	}

	if got.ID() == purgedID {
		t.Fatalf("upsert must return the NEW live row, not resolve back to the purged one (id %s)", purgedID)
	}
	if got.InAppEnabled() != false {
		t.Fatalf("upsert must reflect the newly written value, got InAppEnabled=%v", got.InAppEnabled())
	}
}
