package maintenancerecord

import (
	"os"
	"testing"

	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// newPostgresTestDB opens the Postgres instance named by FLEET_TEST_POSTGRES_DSN
// and resets the fleet schema. The suite skips when the variable is unset, so
// `make test` stays hermetic; point it at a throwaway container to run it:
//
//	docker run -d --name pg -e POSTGRES_PASSWORD=test -e POSTGRES_DB=fleettest -p 55432:5432 postgres:17-alpine
//	FLEET_TEST_POSTGRES_DSN='postgres://postgres:test@127.0.0.1:55432/fleettest?sslmode=disable' go test ./internal/maintenancerecord/
//
// This exists because the SQLite fixture cannot catch dialect faults in the
// startup migration. SQLite stores every column as TEXT, so aggregates and
// comparisons over `id` always succeed there; Postgres types that column as
// `uuid`, which has no min()/max() aggregate. A dedupe statement that passed
// every SQLite test crash-looped fleet-service in the cluster with
// `function min(uuid) does not exist` — the migration is fatal on failure, so
// the pod never served a request.
func newPostgresTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("FLEET_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("FLEET_TEST_POSTGRES_DSN not set; skipping Postgres migration test")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open postgres: %v", err)
	}
	if err := db.Exec(`DROP SCHEMA IF EXISTS fleet CASCADE`).Error; err != nil {
		t.Fatalf("drop schema: %v", err)
	}
	if err := db.Exec(`CREATE SCHEMA fleet`).Error; err != nil {
		t.Fatalf("create schema: %v", err)
	}
	return db
}

// Migration must survive being run against real uuid columns.
func TestMigration_runsOnPostgres(t *testing.T) {
	db := newPostgresTestDB(t)
	if err := Migration(db); err != nil {
		t.Fatalf("Migration on Postgres: %v", err)
	}
	// Idempotent, as at every service start.
	if err := Migration(db); err != nil {
		t.Fatalf("second Migration on Postgres: %v", err)
	}
}

// The de-duplication pass is the part that touches id values directly, so it
// gets its own Postgres run against dirty data.
func TestApplyPartialIndexes_dedupesPreexistingLiveDuplicatesOnPostgres(t *testing.T) {
	db := newPostgresTestDB(t)
	if err := Migration(db); err != nil {
		t.Fatalf("Migration on Postgres: %v", err)
	}
	// Drop the index so pre-migration dirty data can be reproduced.
	if err := db.Exec(`DROP INDEX fleet.ux_maintenance_record_documents_record_media`).Error; err != nil {
		t.Fatalf("drop unique index: %v", err)
	}

	recordID := uuid.NewString()
	mediaID := uuid.NewString()
	keep := DocumentEntity{ID: "00000000-0000-4000-8000-000000000001", MaintenanceRecordID: recordID, MediaID: mediaID}
	dupe := DocumentEntity{ID: "ffffffff-0000-4000-8000-000000000002", MaintenanceRecordID: recordID, MediaID: mediaID}
	other := DocumentEntity{ID: uuid.NewString(), MaintenanceRecordID: recordID, MediaID: uuid.NewString()}
	for _, d := range []DocumentEntity{keep, dupe, other} {
		if err := db.Create(&d).Error; err != nil {
			t.Fatalf("seed %s: %v", d.ID, err)
		}
	}

	if err := ApplyPartialIndexes(db); err != nil {
		t.Fatalf("ApplyPartialIndexes on dirty Postgres data: %v", err)
	}

	if got := countLiveDocs(t, db, recordID); got != 2 {
		t.Errorf("live rows = %d, want 2 (one per distinct media id)", got)
	}
	var survivor DocumentEntity
	if err := db.Where("maintenance_record_id = ? AND media_id = ? AND deleted_at IS NULL", recordID, mediaID).
		First(&survivor).Error; err != nil {
		t.Fatalf("find survivor: %v", err)
	}
	if survivor.ID != keep.ID {
		t.Errorf("survivor ID = %q, want the lowest id %q", survivor.ID, keep.ID)
	}
}
