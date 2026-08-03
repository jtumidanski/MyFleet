package mediavariant

import (
	"context"
	"errors"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// newVariantTestDB opens an in-memory SQLite database with a "media" schema
// attached and creates media_variants with raw SQL.
//
// GORM AutoMigrate mishandles schema-qualified table names (media.media_variants)
// on SQLite when the entity carries index tags, so the table is created directly
// — the same approach mediaobject's tests take.
//
// The uniqueness on (media_object_id, variant) is created here as the same
// PARTIAL index Migration applies, not as an inline UNIQUE constraint. It is
// restated because AutoMigrate does not run in these tests, and without it
// SQLite rejects Upsert's ON CONFLICT clause outright — so a suite that omitted
// it would pass against a schema production does not have.
//
// Partial specifically: a plain constraint would let a variant soft-deleted by
// an admin purge keep occupying the slot, and these tests would then prove the
// wrong behaviour.
func newVariantTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	// The ":memory:" DSN carries no cache=shared, so every physical connection
	// is an independent empty database. Nothing in this package's tests runs
	// concurrently today, but capping the pool to one connection keeps the
	// ATTACH and CREATE TABLE below from silently applying to a different,
	// schema-less connection than a later query lands on — the same fix
	// processing's newCardTestDB needed once it drove concurrent goroutines
	// against one gorm.DB.
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("get sql.DB: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)
	if err := db.Exec("ATTACH DATABASE ':memory:' AS media").Error; err != nil {
		t.Fatalf("attach media schema: %v", err)
	}
	if err := db.Exec(`CREATE TABLE media.media_variants (
		id              TEXT PRIMARY KEY,
		media_object_id TEXT NOT NULL,
		variant         TEXT NOT NULL,
		object_key      TEXT NOT NULL,
		width           INTEGER,
		height          INTEGER,
		content_type    TEXT,
		created_at      DATETIME,
		deleted_at      DATETIME,
		purge_operation_id TEXT
	)`).Error; err != nil {
		t.Fatalf("create media_variants: %v", err)
	}
	if err := ApplyPartialIndexes(db); err != nil {
		t.Fatalf("partial indexes: %v", err)
	}
	return db
}

func seedVariant(t *testing.T, db *gorm.DB, mediaObjectID string, v Variant, key, contentType string) {
	t.Helper()
	m, err := NewBuilder().
		SetMediaObjectID(mediaObjectID).
		SetVariant(v).
		SetObjectKey(key).
		SetWidth(320).
		SetHeight(240).
		SetContentType(contentType).
		Build()
	if err != nil {
		t.Fatalf("build variant: %v", err)
	}
	if err := NewAdministrator(db).ReplaceForMediaObject(mediaObjectID, []Model{m}); err != nil {
		t.Fatalf("seed variant: %v", err)
	}
}

// TestGetByMediaObjectAndVariant_returnsTheNamedRow is the read the content
// route depends on: one row, selected by both media object AND variant kind.
func TestGetByMediaObjectAndVariant_returnsTheNamedRow(t *testing.T) {
	db := newVariantTestDB(t)
	seedVariant(t, db, "m1", VariantThumbnail, "fleet-a/m1/thumbnail.jpg", "image/jpeg")

	got, err := NewProvider(db).GetByMediaObjectAndVariant(context.Background(), "m1", VariantThumbnail)
	if err != nil {
		t.Fatalf("lookup: %v", err)
	}
	if got.ObjectKey() != "fleet-a/m1/thumbnail.jpg" {
		t.Fatalf("ObjectKey = %q, want fleet-a/m1/thumbnail.jpg", got.ObjectKey())
	}
	if got.ContentType() != "image/jpeg" {
		t.Fatalf("ContentType = %q, want image/jpeg", got.ContentType())
	}
}

// TestGetByMediaObjectAndVariant_missIsErrNotFound pins the contract the content
// route leans on: a media object whose processing has not run (or which is not a
// processable image) reports the package's ErrNotFound sentinel — distinguishable
// from a genuine database failure, which the route answers with a 500.
func TestGetByMediaObjectAndVariant_missIsErrNotFound(t *testing.T) {
	db := newVariantTestDB(t)
	seedVariant(t, db, "m1", VariantThumbnail, "fleet-a/m1/thumbnail.jpg", "image/jpeg")

	// Right media object, variant kind that was never generated.
	if _, err := NewProvider(db).GetByMediaObjectAndVariant(context.Background(), "m1", VariantDisplay); !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetByMediaObjectAndVariant(m1, display) = (_, %v), want ErrNotFound", err)
	}
	// Right variant kind, different media object — must not leak another row.
	if _, err := NewProvider(db).GetByMediaObjectAndVariant(context.Background(), "m2", VariantThumbnail); !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetByMediaObjectAndVariant(m2, thumbnail) = (_, %v), want ErrNotFound", err)
	}
}

// TestGetByMediaObjectAndVariant_honoursContextCancellation proves the ctx
// parameter actually reaches the driver: a client that disconnects mid-request
// must cancel the query rather than leave it running on a bare connection.
func TestGetByMediaObjectAndVariant_honoursContextCancellation(t *testing.T) {
	db := newVariantTestDB(t)
	seedVariant(t, db, "m1", VariantThumbnail, "fleet-a/m1/thumbnail.jpg", "image/jpeg")

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := NewProvider(db).GetByMediaObjectAndVariant(ctx, "m1", VariantThumbnail)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("GetByMediaObjectAndVariant with a cancelled ctx = %v, want context.Canceled — "+
			"the context is not reaching the query", err)
	}
}

// newPurgeVariantDB extends newVariantTestDB with the media_objects table the
// orphan anti-join needs. The DDL is hand-written for the same reason that
// helper's is: AutoMigrate emits its CREATE INDEX without the schema qualifier
// and fails on SQLite.
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
