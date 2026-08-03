package mediavariant

import (
	"testing"
	"time"

	"gorm.io/gorm"
)

// newPurgeVariantDB extends newVariantTestDB (provider_test.go) with the
// media_objects table the orphan anti-join needs. The DDL is hand-written for
// the same reason that helper's is: AutoMigrate emits its CREATE INDEX without
// the schema qualifier and fails on SQLite.
func newPurgeVariantDB(t *testing.T) *gorm.DB {
	t.Helper()
	db := newVariantTestDB(t)
	if err := db.Exec(`CREATE TABLE media.media_objects (
		id TEXT PRIMARY KEY, fleet_id TEXT, uploaded_by_user_id TEXT, bucket TEXT,
		object_key TEXT, content_type TEXT, size INTEGER, original_filename TEXT,
		status TEXT, created_at DATETIME, deleted_at DATETIME, purge_after DATETIME,
		purge_operation_id TEXT)`).Error; err != nil {
		t.Fatalf("create media_objects: %v", err)
	}
	return db
}

func insertVariantRow(t *testing.T, db *gorm.DB, id, mediaObjectID, variant, key string, deletedAt *time.Time) {
	t.Helper()
	if err := db.Exec(`INSERT INTO media.media_variants
		(id, media_object_id, variant, object_key, deleted_at)
		VALUES (?, ?, ?, ?, ?)`, id, mediaObjectID, variant, key, deletedAt).Error; err != nil {
		t.Fatalf("insert variant %s: %v", id, err)
	}
}

func insertMediaObject(t *testing.T, db *gorm.DB, id string, deletedAt *time.Time) {
	t.Helper()
	if err := db.Exec(`INSERT INTO media.media_objects (id, fleet_id, bucket, object_key, status, deleted_at)
		VALUES (?, 'fleet-1', 'media', ?, 'ready', ?)`, id, "k/"+id, deletedAt).Error; err != nil {
		t.Fatalf("insert media object %s: %v", id, err)
	}
}

// FR-PURGE-1/2. Every stored key, INCLUDING rows a purge has soft-deleted, and
// read from the object_key column rather than recomputed — the column is the
// record of what was actually written.
func TestObjectKeysForMediaObject_includesSoftDeletedRows(t *testing.T) {
	db := newPurgeVariantDB(t)
	past := time.Now().UTC().Add(-time.Hour)
	insertVariantRow(t, db, "mv-t", "mo-1", "thumbnail", "k/mo-1-thumbnail", nil)
	insertVariantRow(t, db, "mv-c", "mo-1", "card", "k/mo-1-card", nil)
	insertVariantRow(t, db, "mv-old", "mo-1", "card", "k/mo-1-card-old", &past)
	insertVariantRow(t, db, "mv-other", "mo-2", "card", "k/mo-2-card", nil)

	got, err := ObjectKeysForMediaObject(db, "mo-1")
	if err != nil {
		t.Fatalf("ObjectKeysForMediaObject: %v", err)
	}
	want := map[string]bool{"k/mo-1-thumbnail": true, "k/mo-1-card": true, "k/mo-1-card-old": true}
	if len(got) != len(want) {
		t.Fatalf("got %d keys %v, want %d", len(got), got, len(want))
	}
	for _, k := range got {
		if !want[k] {
			t.Errorf("unexpected key %q", k)
		}
		delete(want, k)
	}
	if len(want) != 0 {
		t.Errorf("these keys were never returned, so their bytes would leak: %v", want)
	}
}

func TestDeleteForMediaObject_removesEveryRowForThatObjectOnly(t *testing.T) {
	db := newPurgeVariantDB(t)
	past := time.Now().UTC().Add(-time.Hour)
	insertVariantRow(t, db, "mv-c", "mo-1", "card", "k/mo-1-card", nil)
	insertVariantRow(t, db, "mv-old", "mo-1", "display", "k/mo-1-old", &past)
	insertVariantRow(t, db, "mv-other", "mo-2", "card", "k/mo-2-card", nil)

	if err := DeleteForMediaObject(db, "mo-1"); err != nil {
		t.Fatalf("DeleteForMediaObject: %v", err)
	}
	var mine, others int64
	db.Raw(`SELECT count(*) FROM media.media_variants WHERE media_object_id = 'mo-1'`).Scan(&mine)
	db.Raw(`SELECT count(*) FROM media.media_variants WHERE media_object_id = 'mo-2'`).Scan(&others)
	if mine != 0 {
		t.Errorf("left %d rows for the purged media object", mine)
	}
	if others != 1 {
		t.Errorf("deleted another media object's variants: %d of 1 left", others)
	}
}

// FR-RECON-2, the single most important behaviour in the reconciliation pass.
// The test is the ABSENCE of the parent row, never deleted_at: a variant of a
// soft-deleted-but-recoverable media object is not an orphan, and deleting it
// would silently break both the five-day restore and the admin console's cancel.
func TestListOrphaned_testsForTheParentRowNotDeletedAt(t *testing.T) {
	db := newPurgeVariantDB(t)
	past := time.Now().UTC().Add(-time.Hour)
	insertMediaObject(t, db, "mo-live", nil)
	insertMediaObject(t, db, "mo-soft", &past) // soft-deleted, still recoverable
	insertVariantRow(t, db, "mv-live", "mo-live", "card", "k/live", nil)
	insertVariantRow(t, db, "mv-soft", "mo-soft", "card", "k/soft", nil)
	insertVariantRow(t, db, "mv-orphan", "mo-gone", "card", "k/orphan", nil)

	got, err := ListOrphaned(db, 100)
	if err != nil {
		t.Fatalf("ListOrphaned: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("ListOrphaned = %+v, want exactly the one row whose media object is gone", got)
	}
	if got[0].ID != "mv-orphan" || got[0].ObjectKey != "k/orphan" {
		t.Errorf("ListOrphaned = %+v, want {mv-orphan k/orphan}", got[0])
	}
}

// FR-RECON-5. One tick must not turn into an unbounded bucket-deletion loop on
// first deployment against a database with a large accumulated backlog.
func TestListOrphaned_honoursTheLimit(t *testing.T) {
	db := newPurgeVariantDB(t)
	// Distinct variant values: the partial unique index on
	// (media_object_id, variant) WHERE deleted_at IS NULL would reject three
	// live "card" rows for the same media object.
	for _, id := range []string{"mv-1", "mv-2", "mv-3"} {
		insertVariantRow(t, db, id, "mo-gone", "card-"+id, "k/"+id, nil)
	}
	got, err := ListOrphaned(db, 2)
	if err != nil {
		t.Fatalf("ListOrphaned: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("ListOrphaned(limit 2) returned %d rows, want 2", len(got))
	}
}

func TestDeleteByID_removesExactlyOneRow(t *testing.T) {
	db := newPurgeVariantDB(t)
	insertVariantRow(t, db, "mv-1", "mo-gone", "card", "k/1", nil)
	insertVariantRow(t, db, "mv-2", "mo-gone", "display", "k/2", nil)

	if err := DeleteByID(db, "mv-1"); err != nil {
		t.Fatalf("DeleteByID: %v", err)
	}
	var left int64
	db.Raw(`SELECT count(*) FROM media.media_variants`).Scan(&left)
	if left != 1 {
		t.Errorf("DeleteByID left %d rows, want exactly the untouched one", left)
	}
}
