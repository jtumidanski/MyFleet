package mediavariant

import (
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
		created_at      DATETIME
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

	got, found, err := NewProvider(db).GetByMediaObjectAndVariant("m1", VariantThumbnail)
	if err != nil {
		t.Fatalf("lookup: %v", err)
	}
	if !found {
		t.Fatal("found = false, want true for a seeded variant")
	}
	if got.ObjectKey() != "fleet-a/m1/thumbnail.jpg" {
		t.Fatalf("ObjectKey = %q, want fleet-a/m1/thumbnail.jpg", got.ObjectKey())
	}
	if got.ContentType() != "image/jpeg" {
		t.Fatalf("ContentType = %q, want image/jpeg", got.ContentType())
	}
}

// TestGetByMediaObjectAndVariant_missIsNotAnError pins the contract the content
// route leans on: a media object whose processing has not run (or which is not a
// processable image) reports found=false with a nil error, so the caller can
// fall back to the original rather than failing the request.
func TestGetByMediaObjectAndVariant_missIsNotAnError(t *testing.T) {
	db := newVariantTestDB(t)
	seedVariant(t, db, "m1", VariantThumbnail, "fleet-a/m1/thumbnail.jpg", "image/jpeg")

	// Right media object, variant kind that was never generated.
	if _, found, err := NewProvider(db).GetByMediaObjectAndVariant("m1", VariantDisplay); err != nil || found {
		t.Fatalf("GetByMediaObjectAndVariant(m1, display) = (_, %v, %v), want (_, false, nil)", found, err)
	}
	// Right variant kind, different media object — must not leak another row.
	if _, found, err := NewProvider(db).GetByMediaObjectAndVariant("m2", VariantThumbnail); err != nil || found {
		t.Fatalf("GetByMediaObjectAndVariant(m2, thumbnail) = (_, %v, %v), want (_, false, nil)", found, err)
	}
}
