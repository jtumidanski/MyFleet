package notification

import (
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/jtumidanski/myfleet/packages/shared-go/server"
)

func newVisibilityDB(t *testing.T) *gorm.DB {
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
	if err := db.Exec(`INSERT INTO notification.notifications
		(id, user_id, type, title, dedupe_key, fleet_id, created_at)
		VALUES ('n1', 'user-1', 'schedule.overdue', 'Oil change due', 'dk-1', 'fleet-1', CURRENT_TIMESTAMP)`).
		Error; err != nil {
		t.Fatalf("seed: %v", err)
	}
	return db
}

func TestNotificationReads_hideSoftDeleted(t *testing.T) {
	db := newVisibilityDB(t)
	prov := NewProvider(db)
	adm := NewAdministrator(db)
	page := server.Page{Number: 1, Size: 25}

	if rows, total, err := prov.ListByUser("user-1", ListFilter{}, page); err != nil || len(rows) != 1 || total != 1 {
		t.Fatalf("fixture expected one notification, got %d/%d err %v", len(rows), total, err)
	}
	if exists, err := adm.ExistsByDedupeKey("dk-1"); err != nil || !exists {
		t.Fatalf("fixture dedupe key should exist, got %v err %v", exists, err)
	}

	if err := db.Exec(`UPDATE notification.notifications SET deleted_at = CURRENT_TIMESTAMP,
	                   purge_operation_id = 'op-1' WHERE id = 'n1'`).Error; err != nil {
		t.Fatalf("soft delete: %v", err)
	}

	if rows, total, err := prov.ListByUser("user-1", ListFilter{}, page); err != nil || len(rows) != 0 || total != 0 {
		t.Errorf("ListByUser must ignore soft-deleted rows, got %d/%d err %v", len(rows), total, err)
	}
	// design F1: the partial index frees the key, but ExistsByDedupeKey would
	// still veto regeneration unless it agrees with the index.
	if exists, err := adm.ExistsByDedupeKey("dk-1"); err != nil || exists {
		t.Errorf("ExistsByDedupeKey must ignore soft-deleted rows, got %v err %v", exists, err)
	}
	if err := adm.MarkRead("user-1", "n1"); err != ErrNotFound {
		t.Errorf("MarkRead on a soft-deleted notification must be ErrNotFound, got %v", err)
	}
}
