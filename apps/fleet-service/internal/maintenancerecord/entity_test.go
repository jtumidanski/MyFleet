package maintenancerecord

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// countLiveDocs returns the number of non-deleted document rows for a record.
func countLiveDocs(t *testing.T, db *gorm.DB, recordID string) int64 {
	t.Helper()
	var n int64
	if err := db.Model(&DocumentEntity{}).
		Where("maintenance_record_id = ? AND deleted_at IS NULL", recordID).
		Count(&n).Error; err != nil {
		t.Fatalf("count docs: %v", err)
	}
	return n
}

// The index is what keeps a second live row for the same (record, media) pair
// out of the table even if an application-level check is ever bypassed.
func TestApplyPartialIndexes_rejectsASecondLiveRowForTheSamePair(t *testing.T) {
	db := newTestDB(t)

	first := DocumentEntity{ID: uuid.NewString(), MaintenanceRecordID: "r1", MediaID: "m1"}
	if err := db.Create(&first).Error; err != nil {
		t.Fatalf("first insert: %v", err)
	}

	second := DocumentEntity{ID: uuid.NewString(), MaintenanceRecordID: "r1", MediaID: "m1"}
	if err := db.Create(&second).Error; err == nil {
		t.Fatal("second live row for the same (record, media) pair was accepted; the unique index is missing")
	}
	if got := countLiveDocs(t, db, "r1"); got != 1 {
		t.Errorf("live rows = %d, want 1", got)
	}
}

// The index MUST be partial. A plain unique index would let a soft-deleted row
// occupy the slot forever, and detach-then-reattach is the flow this task
// exists to enable.
func TestApplyPartialIndexes_allowsReattachAfterSoftDelete(t *testing.T) {
	db := newTestDB(t)

	first := DocumentEntity{ID: uuid.NewString(), MaintenanceRecordID: "r1", MediaID: "m1"}
	if err := db.Create(&first).Error; err != nil {
		t.Fatalf("first insert: %v", err)
	}
	if err := db.Model(&DocumentEntity{}).Where("id = ?", first.ID).
		Update("deleted_at", time.Now().UTC()).Error; err != nil {
		t.Fatalf("soft delete: %v", err)
	}

	again := DocumentEntity{ID: uuid.NewString(), MaintenanceRecordID: "r1", MediaID: "m1"}
	if err := db.Create(&again).Error; err != nil {
		t.Fatalf("reattach after soft delete: %v — the index is not partial", err)
	}
	if got := countLiveDocs(t, db, "r1"); got != 1 {
		t.Errorf("live rows = %d, want 1", got)
	}
}

// Existing data can already violate the index (the create path could be handed
// the same id twice). Without de-duplication first, index creation fails and
// takes the service down at startup.
func TestApplyPartialIndexes_dedupesPreexistingLiveDuplicates(t *testing.T) {
	db := newTestDBWithoutIndexes(t)

	keep := DocumentEntity{ID: "00000000-aaaa", MaintenanceRecordID: "r1", MediaID: "m1"}
	dupe := DocumentEntity{ID: "ffffffff-zzzz", MaintenanceRecordID: "r1", MediaID: "m1"}
	other := DocumentEntity{ID: uuid.NewString(), MaintenanceRecordID: "r1", MediaID: "m2"}
	for _, d := range []DocumentEntity{keep, dupe, other} {
		if err := db.Create(&d).Error; err != nil {
			t.Fatalf("seed %s: %v", d.ID, err)
		}
	}

	if err := ApplyPartialIndexes(db); err != nil {
		t.Fatalf("ApplyPartialIndexes on dirty data: %v", err)
	}

	if got := countLiveDocs(t, db, "r1"); got != 2 {
		t.Errorf("live rows = %d, want 2 (one per distinct media id)", got)
	}
	// The lowest id survives, and the loser is soft-deleted rather than removed.
	var survivor DocumentEntity
	if err := db.Where("maintenance_record_id = ? AND media_id = ? AND deleted_at IS NULL", "r1", "m1").
		First(&survivor).Error; err != nil {
		t.Fatalf("find survivor: %v", err)
	}
	if survivor.ID != keep.ID {
		t.Errorf("survivor ID = %q, want the lowest id %q", survivor.ID, keep.ID)
	}
	var loser DocumentEntity
	if err := db.Where("id = ?", dupe.ID).First(&loser).Error; err != nil {
		t.Fatalf("find loser: %v", err)
	}
	if loser.DeletedAt == nil {
		t.Error("duplicate row was not soft-deleted")
	}
}

// Re-running the migration must be a no-op, not an error.
func TestApplyPartialIndexes_isIdempotent(t *testing.T) {
	db := newTestDB(t)
	if err := ApplyPartialIndexes(db); err != nil {
		t.Fatalf("second ApplyPartialIndexes: %v", err)
	}
}
