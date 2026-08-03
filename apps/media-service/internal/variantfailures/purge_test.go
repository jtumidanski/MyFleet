package variantfailures

import (
	"testing"
	"time"

	"gorm.io/gorm"
)

// newLedgerPurgeDB extends newTestDB (variantfailures_test.go) with the
// media_objects table the orphan anti-join needs. media_objects is hand-written
// because its entity's index tags make AutoMigrate fail on SQLite; the ledger
// table itself comes from the real Migration, which is the entity this task
// changes and therefore the one that must not drift.
func newLedgerPurgeDB(t *testing.T) *gorm.DB {
	t.Helper()
	db := newTestDB(t)
	if err := db.Exec(`CREATE TABLE media.media_objects (
		id TEXT PRIMARY KEY, fleet_id TEXT, uploaded_by_user_id TEXT, bucket TEXT,
		object_key TEXT, content_type TEXT, size INTEGER, original_filename TEXT,
		status TEXT, created_at DATETIME, deleted_at DATETIME, purge_after DATETIME,
		purge_operation_id TEXT)`).Error; err != nil {
		t.Fatalf("create media_objects: %v", err)
	}
	return db
}

func seedLedgerRow(t *testing.T, db *gorm.DB, mediaObjectID, variant string) {
	t.Helper()
	if err := db.Exec(`INSERT INTO media.media_variant_failures (media_object_id, variant, reason, failed_at)
		VALUES (?, ?, ?, ?)`, mediaObjectID, variant, ReasonUndecodable, time.Now().UTC()).Error; err != nil {
		t.Fatalf("seed ledger row (%s,%s): %v", mediaObjectID, variant, err)
	}
}

func seedParent(t *testing.T, db *gorm.DB, id string, deletedAt *time.Time) {
	t.Helper()
	if err := db.Exec(`INSERT INTO media.media_objects (id, fleet_id, bucket, object_key, status, deleted_at)
		VALUES (?, 'fleet-1', 'media', ?, 'ready', ?)`, id, "k/"+id, deletedAt).Error; err != nil {
		t.Fatalf("seed media object %s: %v", id, err)
	}
}

func TestDeleteForMediaObject_removesTheLedgerForThatObjectOnly(t *testing.T) {
	db := newLedgerPurgeDB(t)
	seedLedgerRow(t, db, "mo-1", "card")
	seedLedgerRow(t, db, "mo-1", "display")
	seedLedgerRow(t, db, "mo-2", "card")

	if err := DeleteForMediaObject(db, "mo-1"); err != nil {
		t.Fatalf("DeleteForMediaObject: %v", err)
	}
	var mine, others int64
	db.Raw(`SELECT count(*) FROM media.media_variant_failures WHERE media_object_id = 'mo-1'`).Scan(&mine)
	db.Raw(`SELECT count(*) FROM media.media_variant_failures WHERE media_object_id = 'mo-2'`).Scan(&others)
	if mine != 0 {
		t.Errorf("left %d ledger rows for the purged media object", mine)
	}
	if others != 1 {
		t.Errorf("deleted another media object's ledger rows: %d of 1 left", others)
	}
}

// FR-RECON-1/2/4. Absence of the parent row is the test — a soft-deleted but
// still-present media object keeps its ledger.
func TestDeleteOrphaned_keepsRowsWhoseMediaObjectStillExists(t *testing.T) {
	db := newLedgerPurgeDB(t)
	past := time.Now().UTC().Add(-time.Hour)
	seedParent(t, db, "mo-live", nil)
	seedParent(t, db, "mo-soft", &past)
	seedLedgerRow(t, db, "mo-live", "card")
	seedLedgerRow(t, db, "mo-soft", "card")
	seedLedgerRow(t, db, "mo-gone", "card")

	n, err := DeleteOrphaned(db, 100)
	if err != nil {
		t.Fatalf("DeleteOrphaned: %v", err)
	}
	if n != 1 {
		t.Errorf("DeleteOrphaned removed %d rows, want exactly the one orphan", n)
	}
	var live, soft, gone int64
	db.Raw(`SELECT count(*) FROM media.media_variant_failures WHERE media_object_id = 'mo-live'`).Scan(&live)
	db.Raw(`SELECT count(*) FROM media.media_variant_failures WHERE media_object_id = 'mo-soft'`).Scan(&soft)
	db.Raw(`SELECT count(*) FROM media.media_variant_failures WHERE media_object_id = 'mo-gone'`).Scan(&gone)
	if live != 1 || soft != 1 {
		t.Errorf("reconciliation reached rows with a surviving parent: live=%d soft=%d, want 1 and 1", live, soft)
	}
	if gone != 0 {
		t.Errorf("the orphaned row survived: %d left", gone)
	}
}

// A-2. The cap counts MEDIA OBJECTS, not rows, so an object's ledger is never
// left half-reconciled — and the first deployment against a large backlog does
// not open one enormous transaction.
func TestDeleteOrphaned_capsByMediaObjectAndLeavesTheRemainder(t *testing.T) {
	db := newLedgerPurgeDB(t)
	for _, id := range []string{"mo-a", "mo-b", "mo-c"} {
		seedLedgerRow(t, db, id, "card")
		seedLedgerRow(t, db, id, "display")
	}

	n, err := DeleteOrphaned(db, 2)
	if err != nil {
		t.Fatalf("DeleteOrphaned: %v", err)
	}
	if n != 4 {
		t.Errorf("DeleteOrphaned(limit 2) removed %d rows, want 4 — both rows of each of two media objects", n)
	}
	var left int64
	db.Raw(`SELECT count(*) FROM media.media_variant_failures`).Scan(&left)
	if left != 2 {
		t.Errorf("%d rows survived, want the 2 belonging to the one media object over the cap", left)
	}
}
