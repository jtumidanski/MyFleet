package mediavariant

import (
	"context"
	"errors"
	"testing"

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
// The UNIQUE (media_object_id, variant) constraint mirrors the composite
// uniqueIndex tag on Entity. It is restated here because AutoMigrate does not
// run in these tests, and without it SQLite rejects Upsert's ON CONFLICT clause
// outright — so a suite that omitted it would pass against a schema production
// does not have.
func newVariantTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
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
		UNIQUE (media_object_id, variant)
	)`).Error; err != nil {
		t.Fatalf("create media_variants: %v", err)
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
