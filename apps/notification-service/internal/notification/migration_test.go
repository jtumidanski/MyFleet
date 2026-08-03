package notification

import (
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func newNotificationMigrationDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.Exec("ATTACH DATABASE ':memory:' AS notification").Error; err != nil {
		t.Fatalf("attach notification schema: %v", err)
	}
	ddl := `CREATE TABLE notification.notifications (
		id TEXT PRIMARY KEY, user_id TEXT, type TEXT, title TEXT, body TEXT,
		dedupe_key TEXT, vehicle_id TEXT, fleet_id TEXT, read_at DATETIME,
		created_at DATETIME, deleted_at DATETIME, purge_operation_id TEXT)`
	if err := db.Exec(ddl).Error; err != nil {
		t.Fatalf("ddl: %v", err)
	}
	return db
}

// TestPartialDedupeIndex_allowsRegenerationAfterSoftDelete is design F1 in test
// form. A purged notification whose dedupe_key still occupies a total unique
// index can never be regenerated — the reminder safety-net and event
// redelivery would both be permanently suppressed for it.
func TestPartialDedupeIndex_allowsRegenerationAfterSoftDelete(t *testing.T) {
	db := newNotificationMigrationDB(t)
	if err := ApplyPartialIndexes(db); err != nil {
		t.Fatalf("apply partial indexes: %v", err)
	}

	insert := `INSERT INTO notification.notifications (id, user_id, type, title, dedupe_key)
	           VALUES (?, 'user-1', 'schedule.overdue', 'Oil change due', 'dk-1')`
	if err := db.Exec(insert, "n1").Error; err != nil {
		t.Fatalf("first insert: %v", err)
	}
	if err := db.Exec(insert, "n2").Error; err == nil {
		t.Fatal("a second LIVE notification with the same dedupe_key must be rejected")
	}

	if err := db.Exec(`UPDATE notification.notifications SET deleted_at = CURRENT_TIMESTAMP,
	                   purge_operation_id = 'op-1' WHERE id = 'n1'`).Error; err != nil {
		t.Fatalf("soft delete: %v", err)
	}
	if err := db.Exec(insert, "n3").Error; err != nil {
		t.Fatalf("regeneration after soft delete must succeed, got %v", err)
	}
}

func TestNotificationApplyPartialIndexes_isIdempotent(t *testing.T) {
	db := newNotificationMigrationDB(t)
	for i := 0; i < 3; i++ {
		if err := ApplyPartialIndexes(db); err != nil {
			t.Fatalf("apply %d: %v", i+1, err)
		}
	}
}
