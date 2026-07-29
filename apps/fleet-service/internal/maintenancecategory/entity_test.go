package maintenancecategory

import (
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func newTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	// TableName is schema-qualified (fleet.maintenance_categories) for Postgres.
	// SQLite has no schemas, so attach an in-memory database aliased "fleet" so
	// the qualified name resolves in the test.
	if err := db.Exec("ATTACH DATABASE ':memory:' AS fleet").Error; err != nil {
		t.Fatalf("attach fleet schema: %v", err)
	}
	if err := Migration(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

// TestSeedIsIdempotent verifies that running Seed twice does not duplicate rows.
func TestSeedIsIdempotent(t *testing.T) {
	db := newTestDB(t)

	if err := Seed(db); err != nil {
		t.Fatalf("first seed: %v", err)
	}
	if err := Seed(db); err != nil {
		t.Fatalf("second seed: %v", err)
	}

	var count int64
	if err := db.Model(&Entity{}).Count(&count).Error; err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != int64(len(seeds)) {
		t.Fatalf("want %d categories after double-seed, got %d", len(seeds), count)
	}
}
