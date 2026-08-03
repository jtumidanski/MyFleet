package mediaobject

import (
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func newPurgeDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.Exec("ATTACH DATABASE ':memory:' AS media").Error; err != nil {
		t.Fatalf("attach media schema: %v", err)
	}
	ddl := `CREATE TABLE media.media_objects (
		id TEXT PRIMARY KEY, fleet_id TEXT, uploaded_by_user_id TEXT, bucket TEXT,
		object_key TEXT, content_type TEXT, size INTEGER, original_filename TEXT,
		status TEXT, created_at DATETIME, deleted_at DATETIME, purge_after DATETIME,
		purge_operation_id TEXT)`
	if err := db.Exec(ddl).Error; err != nil {
		t.Fatalf("ddl: %v", err)
	}
	return db
}

// FR-ADMIN-RESTORE-7 / design F3. An admin-stamped object belongs to a
// cancellable operation whose lifecycle the admin reaper owns; the legacy sweep
// must not hard-delete it — and, worse, must not remove its MinIO object, which
// no restore could bring back.
func TestListPurgeable_skipsAdminStampedObjects(t *testing.T) {
	db := newPurgeDB(t)
	past := time.Now().UTC().Add(-time.Hour)
	seed := `INSERT INTO media.media_objects (id, fleet_id, bucket, object_key, status,
	         deleted_at, purge_after, purge_operation_id) VALUES (?, 'f1', 'media', ?, 'ready', ?, ?, ?)`
	if err := db.Exec(seed, "mo-user", "k/user", past, past, nil).Error; err != nil {
		t.Fatalf("seed user-deleted object: %v", err)
	}
	// purge_after is set here deliberately: an admin stamp never writes it, so
	// this row could only exist if someone later "helpfully" did. The explicit
	// purge_operation_id IS NULL narrowing is what still saves it.
	if err := db.Exec(seed, "mo-admin", "k/admin", past, past, "op-1").Error; err != nil {
		t.Fatalf("seed admin-stamped object: %v", err)
	}

	got, err := ListPurgeable(db)
	if err != nil {
		t.Fatalf("list purgeable: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("want exactly one purgeable object, got %d", len(got))
	}
	if got[0].ID() != "mo-user" {
		t.Errorf("the legacy sweep picked up an admin-stamped object: %q", got[0].ID())
	}
}
