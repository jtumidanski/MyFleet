package mediavariant

import (
	"context"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func newVariantVisibilityDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.Exec("ATTACH DATABASE ':memory:' AS media").Error; err != nil {
		t.Fatalf("attach media schema: %v", err)
	}
	ddl := `CREATE TABLE media.media_variants (
		id TEXT PRIMARY KEY, media_object_id TEXT, variant TEXT, object_key TEXT,
		width INTEGER, height INTEGER, content_type TEXT, created_at DATETIME,
		deleted_at DATETIME, purge_operation_id TEXT)`
	if err := db.Exec(ddl).Error; err != nil {
		t.Fatalf("ddl: %v", err)
	}
	if err := db.Exec(`INSERT INTO media.media_variants
		(id, media_object_id, variant, object_key, content_type)
		VALUES ('v1', 'mo-1', 'thumb', 'k/thumb', 'image/webp')`).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}
	return db
}

func TestMediaVariantReads_hideSoftDeleted(t *testing.T) {
	db := newVariantVisibilityDB(t)
	prov := NewProvider(db)
	ctx := context.Background()

	if vs, err := prov.ListByMediaObject("mo-1"); err != nil || len(vs) != 1 {
		t.Fatalf("fixture expected one variant, got %d err %v", len(vs), err)
	}

	if err := db.Exec(`UPDATE media.media_variants SET deleted_at = CURRENT_TIMESTAMP,
	                   purge_operation_id = 'op-1' WHERE id = 'v1'`).Error; err != nil {
		t.Fatalf("soft delete: %v", err)
	}

	if vs, err := prov.ListByMediaObject("mo-1"); err != nil || len(vs) != 0 {
		t.Errorf("ListByMediaObject must ignore soft-deleted rows, got %d err %v", len(vs), err)
	}
	if _, err := prov.GetByMediaObjectAndVariant(ctx, "mo-1", Variant("thumb")); err != ErrNotFound {
		t.Errorf("GetByMediaObjectAndVariant must ignore soft-deleted rows, got %v", err)
	}
}
