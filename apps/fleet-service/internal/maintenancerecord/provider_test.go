package maintenancerecord

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/jtumidanski/myfleet/packages/shared-go/server"
)

// newTestDBWithoutIndexes builds the schema-qualified sqlite fixture WITHOUT
// the partial unique indexes, so a test can seed rows that the index would
// reject and then prove ApplyPartialIndexes cleans them up.
func newTestDBWithoutIndexes(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	// TableName is schema-qualified (fleet.maintenance_records) for Postgres.
	// SQLite has no schemas, so attach an in-memory database aliased "fleet".
	if err := db.Exec("ATTACH DATABASE ':memory:' AS fleet").Error; err != nil {
		t.Fatalf("attach fleet schema: %v", err)
	}
	// GORM's AutoMigrate emits CREATE INDEX with the schema prefix stripped (a
	// SQLite quirk), so Migration(db) fails here: Entity.DeletedAt and
	// DocumentEntity.MaintenanceRecordID both carry gorm:"index" tags. The
	// schema-qualified tables are created with explicit DDL instead, mirroring
	// the same workaround used in maintenanceschedule/completion_db_test.go,
	// dashboard/aggregate_test.go, and activity/processor_test.go.
	ddl := []string{
		`CREATE TABLE fleet.maintenance_records (
			id TEXT PRIMARY KEY, vehicle_id TEXT, category_id TEXT, description TEXT,
			performed_at DATETIME, mileage INTEGER, cost REAL, vendor TEXT, notes TEXT,
			created_by_user_id TEXT, created_at DATETIME, updated_at DATETIME, deleted_at DATETIME,
			purge_operation_id TEXT)`,
		`CREATE TABLE fleet.maintenance_record_documents (
			id TEXT PRIMARY KEY, maintenance_record_id TEXT, media_id TEXT,
			deleted_at DATETIME, purge_operation_id TEXT)`,
	}
	for _, stmt := range ddl {
		if err := db.Exec(stmt).Error; err != nil {
			t.Fatalf("ddl: %v", err)
		}
	}
	return db
}

// newTestDB is the fixture every other test uses: the same schema production
// gets, indexes included. Applying the real ApplyPartialIndexes here rather
// than hand-writing the DDL is deliberate — a test database without the
// index would let a duplicate-row bug pass.
func newTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db := newTestDBWithoutIndexes(t)
	if err := ApplyPartialIndexes(db); err != nil {
		t.Fatalf("apply partial indexes: %v", err)
	}
	return db
}

// insertRecord writes one record plus docCount document rows and returns its ID.
func insertRecord(t *testing.T, a Administrator, vehicleID, categoryID string, day int, docCount int) string {
	t.Helper()
	ids := make([]string, 0, docCount)
	for i := 0; i < docCount; i++ {
		ids = append(ids, uuid.NewString())
	}
	m, err := NewBuilder().
		SetVehicleID(vehicleID).
		SetCategoryID(categoryID).
		SetPerformedAt(time.Date(2026, 1, day, 0, 0, 0, 0, time.UTC)).
		SetDocumentMediaIDs(ids).
		Build()
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	created, err := a.Insert(m)
	if err != nil {
		t.Fatalf("insert: %v", err)
	}
	return created.ID()
}

// nil means "no filter".
func TestListByVehicle_nilCategoryIDsReturnsEverything(t *testing.T) {
	db := newTestDB(t)
	a := NewAdministrator(db)
	insertRecord(t, a, "v1", "maint-1", 1, 0)
	insertRecord(t, a, "v1", "mod-1", 2, 0)
	insertRecord(t, a, "v2", "maint-1", 3, 0)

	ms, total, err := NewProvider(db).ListByVehicle("v1", nil, server.Page{Number: 1, Size: 25})
	if err != nil {
		t.Fatalf("ListByVehicle: %v", err)
	}
	if total != 2 || len(ms) != 2 {
		t.Fatalf("len=%d total=%d, want 2/2", len(ms), total)
	}
}

// Empty-but-non-nil means "match nothing" — NOT "match everything". This is the
// difference between a fleet with no modifications seeing an empty tab and
// seeing every maintenance record labelled as a modification (design D3).
func TestListByVehicle_emptyCategoryIDsMatchesNothing(t *testing.T) {
	db := newTestDB(t)
	a := NewAdministrator(db)
	insertRecord(t, a, "v1", "maint-1", 1, 0)
	insertRecord(t, a, "v1", "maint-2", 2, 0)

	ms, total, err := NewProvider(db).ListByVehicle("v1", []string{}, server.Page{Number: 1, Size: 25})
	if err != nil {
		t.Fatalf("ListByVehicle: %v", err)
	}
	if total != 0 || len(ms) != 0 {
		t.Fatalf("len=%d total=%d, want 0/0", len(ms), total)
	}
}

func TestListByVehicle_filtersByCategoryIDs(t *testing.T) {
	db := newTestDB(t)
	a := NewAdministrator(db)
	insertRecord(t, a, "v1", "maint-1", 1, 0)
	insertRecord(t, a, "v1", "mod-1", 2, 0)
	insertRecord(t, a, "v1", "mod-2", 3, 0)

	ms, total, err := NewProvider(db).ListByVehicle("v1", []string{"mod-1", "mod-2"}, server.Page{Number: 1, Size: 25})
	if err != nil {
		t.Fatalf("ListByVehicle: %v", err)
	}
	if total != 2 || len(ms) != 2 {
		t.Fatalf("len=%d total=%d, want 2/2", len(ms), total)
	}
	for _, m := range ms {
		if m.CategoryID() == "maint-1" {
			t.Fatal("a maintenance record leaked through the modification filter")
		}
	}
}

// meta.total must be the count AFTER filtering, verified with more records than
// fit on one page (PRD FR-LIST-2).
func TestListByVehicle_filteredTotalSurvivesPaging(t *testing.T) {
	db := newTestDB(t)
	a := NewAdministrator(db)
	for i := 1; i <= 7; i++ {
		insertRecord(t, a, "v1", "mod-1", i, 0)
	}
	for i := 8; i <= 12; i++ {
		insertRecord(t, a, "v1", "maint-1", i, 0)
	}

	ms, total, err := NewProvider(db).ListByVehicle("v1", []string{"mod-1"}, server.Page{Number: 1, Size: 5})
	if err != nil {
		t.Fatalf("ListByVehicle: %v", err)
	}
	if total != 7 {
		t.Fatalf("total = %d, want the filtered count 7 (not the unfiltered 12)", total)
	}
	if len(ms) != 5 {
		t.Fatalf("page 1 len = %d, want 5", len(ms))
	}
}

// The page's documents are fetched in one query and grouped in memory (D21).
// The observable contract is that every record still carries exactly its own.
func TestListByVehicle_attachesEachRecordsOwnDocuments(t *testing.T) {
	db := newTestDB(t)
	a := NewAdministrator(db)
	insertRecord(t, a, "v1", "c1", 1, 2)
	insertRecord(t, a, "v1", "c1", 2, 0)
	insertRecord(t, a, "v1", "c1", 3, 3)

	ms, _, err := NewProvider(db).ListByVehicle("v1", nil, server.Page{Number: 1, Size: 25})
	if err != nil {
		t.Fatalf("ListByVehicle: %v", err)
	}
	// Newest first: day 3 (3 docs), day 2 (0), day 1 (2).
	want := []int{3, 0, 2}
	for i, m := range ms {
		if len(m.DocumentMediaIDs()) != want[i] {
			t.Fatalf("record %d has %d documents, want %d", i, len(m.DocumentMediaIDs()), want[i])
		}
	}
}
