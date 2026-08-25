package maintenancerecord

import (
	"errors"
	"testing"

	"github.com/jtumidanski/myfleet/packages/shared-go/server"
)

// Update's column allow-list is the layer that silently dropped an edited
// category: the map named six columns and category_id was not among them, so
// the write was a no-op. It looked like it worked because Update returns a
// Model built from the in-memory entity rather than a re-read — the response
// carried the new category while the row kept the old one.
//
// So this asserts against a RE-READ through the Provider, not against Update's
// return value. Asserting on the return would have passed even before the fix.
func TestUpdate_persistsAReassignedCategory(t *testing.T) {
	db := newTestDB(t)
	admin := NewAdministrator(db)
	provider := NewProvider(db)

	id := insertRecord(t, admin, "v1", "cat-original", 5, 0)

	before, err := provider.GetByID(id)
	if err != nil {
		t.Fatalf("get before: %v", err)
	}
	if _, err := admin.Update(before.WithCategoryID("cat-reassigned")); err != nil {
		t.Fatalf("update: %v", err)
	}

	after, err := provider.GetByID(id)
	if err != nil {
		t.Fatalf("get after: %v", err)
	}
	if after.CategoryID() != "cat-reassigned" {
		t.Errorf("persisted CategoryID() = %q, want %q", after.CategoryID(), "cat-reassigned")
	}
}

// Adding a column to the allow-list is easy to do destructively (a typo'd key
// writes a zero value over a neighbouring column), so pin the fields that were
// already there.
func TestUpdate_leavesTheOtherEditableFieldsIntact(t *testing.T) {
	db := newTestDB(t)
	admin := NewAdministrator(db)
	provider := NewProvider(db)

	id := insertRecord(t, admin, "v1", "cat-original", 5, 0)
	seed, err := provider.GetByID(id)
	if err != nil {
		t.Fatalf("get seed: %v", err)
	}
	if _, err := admin.Update(seed.
		WithVendor("Corner Garage").
		WithNotes("torque to spec").
		WithMileage(84999).
		WithCost(129.5).
		WithDescription("  front pads  ")); err != nil {
		t.Fatalf("update: %v", err)
	}

	got, err := provider.GetByID(id)
	if err != nil {
		t.Fatalf("get after: %v", err)
	}
	if got.Vendor() != "Corner Garage" {
		t.Errorf("Vendor() = %q", got.Vendor())
	}
	if got.Notes() != "torque to spec" {
		t.Errorf("Notes() = %q", got.Notes())
	}
	if got.Mileage() != 84999 {
		t.Errorf("Mileage() = %d", got.Mileage())
	}
	if got.Cost() != 129.5 {
		t.Errorf("Cost() = %v", got.Cost())
	}
	// WithDescription trims, so storage and measurement agree.
	if got.Description() != "front pads" {
		t.Errorf("Description() = %q, want %q", got.Description(), "front pads")
	}
	// Untouched by this update.
	if got.CategoryID() != "cat-original" {
		t.Errorf("CategoryID() = %q, want it unchanged", got.CategoryID())
	}
}

// A fresh attach adds exactly one live row and the returned model reflects it.
func TestAttachDocument_addsOneLiveRowAndReturnsTheStoredSet(t *testing.T) {
	db := newTestDB(t)
	a := NewAdministrator(db)
	id := insertRecord(t, a, "v1", "cat-1", 5, 0)

	m, err := a.AttachDocument(id, "media-1")
	if err != nil {
		t.Fatalf("attach: %v", err)
	}
	if got := m.DocumentMediaIDs(); len(got) != 1 || got[0] != "media-1" {
		t.Errorf("DocumentMediaIDs() = %v, want [media-1]", got)
	}
	if got := countLiveDocs(t, db, id); got != 1 {
		t.Errorf("live rows = %d, want 1", got)
	}
	// The returned model must be a re-read, not the in-memory entity — a
	// zero createdAt is how the PATCH path used to lie about stored state.
	if m.CreatedAt().IsZero() {
		t.Error("returned model carries a zero CreatedAt; it was not re-read")
	}
}

// FR-ATT-8: the drawer retries its sequential loop after a partial failure, so
// re-attaching an id the record already holds must succeed and must not create
// a second row.
func TestAttachDocument_isIdempotentForAnAlreadyAttachedMedia(t *testing.T) {
	db := newTestDB(t)
	a := NewAdministrator(db)
	id := insertRecord(t, a, "v1", "cat-1", 5, 0)

	if _, err := a.AttachDocument(id, "media-1"); err != nil {
		t.Fatalf("first attach: %v", err)
	}
	m, err := a.AttachDocument(id, "media-1")
	if err != nil {
		t.Fatalf("second attach: %v", err)
	}
	if got := m.DocumentMediaIDs(); len(got) != 1 {
		t.Errorf("DocumentMediaIDs() = %v, want the id exactly once", got)
	}
	if got := countLiveDocs(t, db, id); got != 1 {
		t.Errorf("live rows = %d, want 1", got)
	}
}

// FR-ATT-7: the eleventh attachment is rejected and writes nothing.
func TestAttachDocument_rejectsTheEleventhAndWritesNothing(t *testing.T) {
	db := newTestDB(t)
	a := NewAdministrator(db)
	id := insertRecord(t, a, "v1", "cat-1", 5, MaxDocuments)

	_, err := a.AttachDocument(id, "one-too-many")
	if !errors.Is(err, server.ErrValidation) {
		t.Fatalf("err = %v, want server.ErrValidation", err)
	}
	if got := countLiveDocs(t, db, id); got != int64(MaxDocuments) {
		t.Errorf("live rows = %d, want %d", got, MaxDocuments)
	}
}

// Re-attaching an id a FULL record already holds must succeed: the idempotency
// check runs before the cap check, so a retry of an attach that actually landed
// is not punished with a 422.
func TestAttachDocument_reattachOnAFullRecordSucceeds(t *testing.T) {
	db := newTestDB(t)
	a := NewAdministrator(db)
	id := insertRecord(t, a, "v1", "cat-1", 5, MaxDocuments-1)
	if _, err := a.AttachDocument(id, "media-last"); err != nil {
		t.Fatalf("attach to fill: %v", err)
	}

	if _, err := a.AttachDocument(id, "media-last"); err != nil {
		t.Fatalf("reattach on a full record: %v, want success", err)
	}
	if got := countLiveDocs(t, db, id); got != int64(MaxDocuments) {
		t.Errorf("live rows = %d, want %d", got, MaxDocuments)
	}
}

func TestAttachDocument_missingRecordIsNotFound(t *testing.T) {
	db := newTestDB(t)
	if _, err := NewAdministrator(db).AttachDocument("no-such-record", "media-1"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestAttachDocument_softDeletedRecordIsNotFound(t *testing.T) {
	db := newTestDB(t)
	a := NewAdministrator(db)
	id := insertRecord(t, a, "v1", "cat-1", 5, 0)
	if err := a.SoftDelete(id); err != nil {
		t.Fatalf("soft delete: %v", err)
	}

	if _, err := a.AttachDocument(id, "media-1"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

// FR-DET-2: the row is soft-deleted, not removed. admin/visibility_document_test
// already depends on a stamped row being invisible to readers.
func TestDetachDocument_soft_deletesTheRowAndLeavesItPresent(t *testing.T) {
	db := newTestDB(t)
	a := NewAdministrator(db)
	id := insertRecord(t, a, "v1", "cat-1", 5, 0)
	if _, err := a.AttachDocument(id, "media-1"); err != nil {
		t.Fatalf("attach: %v", err)
	}

	if err := a.DetachDocument(id, "media-1"); err != nil {
		t.Fatalf("detach: %v", err)
	}
	if got := countLiveDocs(t, db, id); got != 0 {
		t.Errorf("live rows = %d, want 0", got)
	}
	var row DocumentEntity
	if err := db.Where("maintenance_record_id = ? AND media_id = ?", id, "media-1").
		First(&row).Error; err != nil {
		t.Fatalf("the row must still exist: %v", err)
	}
	if row.DeletedAt == nil {
		t.Error("deleted_at was not stamped")
	}
	// And it must be re-attachable, which is the partial index's whole point.
	if _, err := a.AttachDocument(id, "media-1"); err != nil {
		t.Fatalf("reattach after detach: %v", err)
	}
}

// FR-DET-4: never attached and already detached collapse to the same answer.
func TestDetachDocument_unattachedMediaIsNotFound(t *testing.T) {
	db := newTestDB(t)
	a := NewAdministrator(db)
	id := insertRecord(t, a, "v1", "cat-1", 5, 0)

	if err := a.DetachDocument(id, "never-attached"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}

	if _, err := a.AttachDocument(id, "media-1"); err != nil {
		t.Fatalf("attach: %v", err)
	}
	if err := a.DetachDocument(id, "media-1"); err != nil {
		t.Fatalf("first detach: %v", err)
	}
	if err := a.DetachDocument(id, "media-1"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("second detach err = %v, want ErrNotFound", err)
	}
}

// Detach must not reach across records.
func TestDetachDocument_doesNotDetachFromAnotherRecord(t *testing.T) {
	db := newTestDB(t)
	a := NewAdministrator(db)
	mine := insertRecord(t, a, "v1", "cat-1", 5, 0)
	theirs := insertRecord(t, a, "v1", "cat-1", 6, 0)
	if _, err := a.AttachDocument(theirs, "media-1"); err != nil {
		t.Fatalf("attach: %v", err)
	}

	if err := a.DetachDocument(mine, "media-1"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
	if got := countLiveDocs(t, db, theirs); got != 1 {
		t.Errorf("other record's live rows = %d, want 1 (untouched)", got)
	}
}
