package maintenancecategory

import (
	"errors"
	"sort"
	"sync"
	"testing"

	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"

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
	//
	// PIN: this DDL's column set MUST mirror the Entity struct exactly. A new
	// Entity field with no matching column here does not fail to compile —
	// it silently vanishes from every query gorm builds against this test DB
	// (Find/Create pick columns from the struct via reflection, so a column
	// gorm expects but the table lacks fails per-column, not per-test). That
	// is exactly how the PostgreSQL-only uuid-bind bug in visibleTo went
	// undetected here: FleetID was TEXT in this DDL but uuid in production.
	// TestDDLColumnsMatchEntity below fails loudly the moment this list and
	// the Entity struct's columns disagree; keep them in lockstep by hand.
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

// TestDDLColumnsMatchEntity guards the pin above: newTestDB's hand-written
// DDL must declare exactly the columns gorm derives from the Entity struct,
// no more and no fewer. AutoMigrate itself can't be used as the source of
// truth here — that's the whole reason this DDL exists (see the comment in
// newTestDB): gorm's sqlite driver breaks on the schema-qualified table's
// indexed column. So this compares two independent sources instead: gorm's
// own schema parser (what Entity SHOULD produce, via reflection — no table
// involved) against sqlite's PRAGMA table_info for the table newTestDB
// actually created (what the DDL DID produce). A field added to Entity with
// no corresponding line in the DDL — or a DDL column with no matching
// Entity field — fails this test immediately instead of drifting silently.
func TestDDLColumnsMatchEntity(t *testing.T) {
	s, err := schema.Parse(&Entity{}, &sync.Map{}, schema.NamingStrategy{})
	if err != nil {
		t.Fatalf("parse Entity schema: %v", err)
	}
	wantCols := append([]string(nil), s.DBNames...)
	sort.Strings(wantCols)

	db := newTestDB(t)
	rows, err := db.Raw("PRAGMA fleet.table_info(maintenance_categories)").Rows()
	if err != nil {
		t.Fatalf("table_info: %v", err)
	}
	defer rows.Close()

	var gotCols []string
	for rows.Next() {
		var cid int
		var name, colType string
		var notNull int
		var dflt any
		var pk int
		if err := rows.Scan(&cid, &name, &colType, &notNull, &dflt, &pk); err != nil {
			t.Fatalf("scan table_info row: %v", err)
		}
		gotCols = append(gotCols, name)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate table_info rows: %v", err)
	}
	sort.Strings(gotCols)

	if len(wantCols) != len(gotCols) {
		t.Fatalf("column count mismatch: Entity has %v, newTestDB's DDL has %v — "+
			"update the hand-written DDL in newTestDB to match", wantCols, gotCols)
	}
	for i := range wantCols {
		if wantCols[i] != gotCols[i] {
			t.Fatalf("column mismatch: Entity has %v, newTestDB's DDL has %v — "+
				"update the hand-written DDL in newTestDB to match", wantCols, gotCols)
		}
	}
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

// TestSeed_ignoresFleetScopedNameCollision proves Seed's name lookup is
// constrained to system rows (fleet_id IS NULL): a fleet-scoped row sharing a
// system name must NOT satisfy the FirstOrCreate lookup, or the system row
// would never get created on a later startup that runs Seed after a fleet
// already created a same-named category.
func TestSeed_ignoresFleetScopedNameCollision(t *testing.T) {
	db := newTestDB(t)

	fleetA := "11111111-1111-1111-1111-111111111111"
	if err := db.Create(&Entity{
		ID:      uuid.NewString(),
		Name:    "Oil Change",
		Kind:    string(KindMaintenance),
		FleetID: &fleetA,
	}).Error; err != nil {
		t.Fatalf("create fleet-scoped row: %v", err)
	}

	if err := Seed(db); err != nil {
		t.Fatalf("seed: %v", err)
	}

	var systemCount int64
	if err := db.Model(&Entity{}).
		Where("name = ? AND fleet_id IS NULL", "Oil Change").
		Count(&systemCount).Error; err != nil {
		t.Fatalf("count system rows: %v", err)
	}
	if systemCount != 1 {
		t.Fatalf("want exactly 1 system-defined 'Oil Change' row after seed, got %d", systemCount)
	}

	var total int64
	if err := db.Model(&Entity{}).Where("name = ?", "Oil Change").Count(&total).Error; err != nil {
		t.Fatalf("count all rows: %v", err)
	}
	if total != 2 {
		t.Fatalf("want the fleet-scoped row AND the system row to coexist (2 total), got %d", total)
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
