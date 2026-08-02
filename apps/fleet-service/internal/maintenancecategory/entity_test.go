package maintenancecategory

import (
	"errors"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/jtumidanski/myfleet/packages/shared-go/server"
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
	// Not Migration(db)/AutoMigrate: gorm's sqlite driver builds CREATE INDEX
	// without the schema qualifier for a schema-qualified table (it uses the
	// bare table name instead of routing through CurrentTable like the core
	// migrator does), so an indexed column on a "fleet."-prefixed TableName
	// fails with "no such table: main.maintenance_categories" under
	// AutoMigrate on SQLite even though the table lives in the attached
	// "fleet" schema. Postgres's migrator does not have this bug. The
	// activity package hits the identical issue with its own indexed
	// FleetID and works around it the same way (see
	// activity/processor_test.go's newActivityDB): create the table with
	// explicit DDL mirroring the GORM entity instead of AutoMigrate.
	if err := db.Exec(`CREATE TABLE fleet.maintenance_categories (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		description TEXT,
		system_defined BOOLEAN NOT NULL DEFAULT 0,
		kind TEXT NOT NULL DEFAULT 'maintenance',
		fleet_id TEXT
	)`).Error; err != nil {
		t.Fatalf("ddl: %v", err)
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

// ParseKind is the single place the permitted ?kind= values are defined, so the
// category and record endpoints cannot drift on what they accept.
func TestParseKind(t *testing.T) {
	if k, err := ParseKind(""); err != nil || k != "" {
		t.Fatalf(`ParseKind("")=(%q,%v) want ("",nil) — empty means "no filter"`, k, err)
	}
	if k, err := ParseKind("maintenance"); err != nil || k != KindMaintenance {
		t.Fatalf("ParseKind(maintenance)=(%q,%v)", k, err)
	}
	if k, err := ParseKind("modification"); err != nil || k != KindModification {
		t.Fatalf("ParseKind(modification)=(%q,%v)", k, err)
	}
	// An unrecognised value is a validation error, never a silent empty result.
	for _, in := range []string{"bogus", "MAINTENANCE", "mod", " maintenance"} {
		if _, err := ParseKind(in); !errors.Is(err, server.ErrValidation) {
			t.Fatalf("ParseKind(%q) err = %v, want ErrValidation", in, err)
		}
	}
}

// The eight pre-existing rows must read as maintenance after migration with no
// manual backfill; the twelve modification categories must seed alongside them.
func TestSeed_classifiesEveryCategory(t *testing.T) {
	db := newTestDB(t)
	if err := Seed(db); err != nil {
		t.Fatalf("seed: %v", err)
	}

	var maintenance, modification int64
	if err := db.Model(&Entity{}).Where("kind = ?", string(KindMaintenance)).
		Count(&maintenance).Error; err != nil {
		t.Fatalf("count maintenance: %v", err)
	}
	if err := db.Model(&Entity{}).Where("kind = ?", string(KindModification)).
		Count(&modification).Error; err != nil {
		t.Fatalf("count modification: %v", err)
	}

	if maintenance != 8 {
		t.Fatalf("maintenance categories = %d, want 8", maintenance)
	}
	if modification != 12 {
		t.Fatalf("modification categories = %d, want 12", modification)
	}
	if int(maintenance+modification) != len(seeds) {
		t.Fatalf("kinds sum to %d but seeds has %d rows", maintenance+modification, len(seeds))
	}
}

// No modification name may collide with an existing maintenance name — Seed is
// keyed by Name, so a collision would silently reclassify nothing and leave a
// category missing.
func TestSeed_namesAreUnique(t *testing.T) {
	seen := map[string]bool{}
	for _, s := range seeds {
		if seen[s.Name] {
			t.Fatalf("duplicate seed name %q", s.Name)
		}
		seen[s.Name] = true
	}
}
