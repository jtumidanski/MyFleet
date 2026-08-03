package purge

import (
	"context"
	"io"
	"testing"

	"github.com/sirupsen/logrus"
	"gorm.io/gorm"

	"github.com/jtumidanski/myfleet/apps/media-service/internal/mediaobject"
)

func newSweeper(t *testing.T, db *gorm.DB, store ObjectRemover, cfg Config) *Sweeper {
	t.Helper()
	log := logrus.New()
	log.SetOutput(io.Discard)
	return NewSweeper(log, db, store, cfg)
}

// T5 / FR-PURGE-5. An admin-stamped object belongs to a cancellable operation
// whose lifecycle the admin reaper owns. The sweep must not hard-delete it and,
// worse, must not remove its MinIO object, which no restore could bring back.
//
// This test is written BEFORE the sweep is rewritten and must stay green
// throughout: the one thing a relocation can silently lose is the
// purge_operation_id IS NULL narrowing.
func TestRunOnce_leavesAdminStampedObjectsAlone(t *testing.T) {
	db := newTestDB(t)
	opID := "op-1"
	seedMediaObject(t, db, "mo-user", hourAgo(), nil)
	seedMediaObject(t, db, "mo-admin", hourAgo(), &opID)

	store := newRemover()
	if err := newSweeper(t, db, store, Config{}).RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}

	if store.wasAsked("k/mo-admin") {
		t.Error("the sweep removed an admin-stamped object's bytes; no restore could bring them back")
	}
	if n := countRows(t, db, `SELECT count(*) FROM media.media_objects WHERE id = 'mo-admin'`); n != 1 {
		t.Errorf("the sweep hard-deleted an admin-stamped media object: %d of 1 left", n)
	}
	if n := countRows(t, db, `SELECT count(*) FROM media.media_objects WHERE id = 'mo-user'`); n != 0 {
		t.Errorf("the sweep did not purge the user-deleted object: %d rows left", n)
	}
	if !store.didRemove("k/mo-user") {
		t.Error("the sweep did not remove the purged object's own bytes")
	}
}

// T1 / FR-PURGE-1/2/3. The whole point: four keys removed — the original and
// all three variants, including the soft-deleted one — and nothing left in any
// of the three tables.
func TestRunOnce_removesEveryByteAndRowForAPurgedObject(t *testing.T) {
	db := newTestDB(t)
	seedMediaObject(t, db, "mo-1", hourAgo(), nil)
	seedVariant(t, db, "mv-t", "mo-1", "thumbnail", "k/mo-1-thumbnail", nil)
	seedVariant(t, db, "mv-c", "mo-1", "card", "k/mo-1-card", nil)
	seedVariant(t, db, "mv-d", "mo-1", "display", "k/mo-1-display", nil)
	// A soft-deleted card left behind by an earlier admin purge. The partial
	// unique index admits it alongside the live one, and its bytes leak unless
	// the sweep removes them too (FR-PURGE-1).
	seedVariant(t, db, "mv-old", "mo-1", "card", "k/mo-1-card-old", hourAgo())
	seedLedgerRow(t, db, "mo-1", "card")

	store := newRemover()
	if err := newSweeper(t, db, store, Config{}).RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}

	for _, key := range []string{
		"k/mo-1", "k/mo-1-thumbnail", "k/mo-1-card", "k/mo-1-display", "k/mo-1-card-old",
	} {
		if !store.didRemove(key) {
			t.Errorf("%s was never removed — its bytes leak forever once the rows are gone", key)
		}
	}
	for _, c := range []struct {
		what  string
		query string
	}{
		{"variant", `SELECT count(*) FROM media.media_variants WHERE media_object_id = 'mo-1'`},
		{"ledger", `SELECT count(*) FROM media.media_variant_failures WHERE media_object_id = 'mo-1'`},
		{"media object", `SELECT count(*) FROM media.media_objects WHERE id = 'mo-1'`},
	} {
		if n := countRows(t, db, c.query); n != 0 {
			t.Errorf("%d %s rows survived the purge", n, c.what)
		}
	}
}

// T2 / FR-PURGE-7. A failing VARIANT removal must leave every row in place —
// including the media object's, which today's code deletes regardless. The
// object must still be purgeable on the next tick.
func TestRunOnce_aFailedVariantRemovalKeepsEveryRow(t *testing.T) {
	db := newTestDB(t)
	seedMediaObject(t, db, "mo-1", hourAgo(), nil)
	seedVariant(t, db, "mv-c", "mo-1", "card", "k/mo-1-card", nil)
	seedLedgerRow(t, db, "mo-1", "card")
	// A second object that must still be processed: one object's failure must
	// not abort the sweep.
	seedMediaObject(t, db, "mo-2", hourAgo(), nil)

	store := newRemover("k/mo-1-card")
	if err := newSweeper(t, db, store, Config{}).RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce must not return a per-object failure: %v", err)
	}

	if n := countRows(t, db, `SELECT count(*) FROM media.media_objects WHERE id = 'mo-1'`); n != 1 {
		t.Error("the media row was deleted even though a variant's bytes survive — " +
			"the variant row is now unreachable and its bytes are stranded")
	}
	if n := countRows(t, db, `SELECT count(*) FROM media.media_variants WHERE media_object_id = 'mo-1'`); n != 1 {
		t.Errorf("%d of 1 variant rows left; a partial failure must delete nothing", n)
	}
	if n := countRows(t, db, `SELECT count(*) FROM media.media_variant_failures WHERE media_object_id = 'mo-1'`); n != 1 {
		t.Errorf("%d of 1 ledger rows left; a partial failure must delete nothing", n)
	}
	if n := countRows(t, db, `SELECT count(*) FROM media.media_objects WHERE id = 'mo-2'`); n != 0 {
		t.Error("one object's failure aborted the sweep; the remaining objects must still be processed")
	}

	// And the next sweep must still see it.
	objs, err := mediaobject.ListPurgeable(db)
	if err != nil {
		t.Fatalf("ListPurgeable: %v", err)
	}
	if len(objs) != 1 || objs[0].ID() != "mo-1" {
		t.Errorf("ListPurgeable = %+v, want mo-1 still awaiting retry", objs)
	}
}

// T3 / FR-PURGE-7. A failing ORIGINAL removal behaves identically. "The variant
// rows survive" would pass vacuously in code that never touches variants, so
// this asserts the stronger claim: the variant keys were never OFFERED to the
// remover, because the object was abandoned before any of them was reached.
func TestRunOnce_aFailedOriginalRemovalOffersNoVariantKeys(t *testing.T) {
	db := newTestDB(t)
	seedMediaObject(t, db, "mo-1", hourAgo(), nil)
	seedVariant(t, db, "mv-c", "mo-1", "card", "k/mo-1-card", nil)
	seedLedgerRow(t, db, "mo-1", "card")

	store := newRemover("k/mo-1")
	if err := newSweeper(t, db, store, Config{}).RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}

	if store.wasAsked("k/mo-1-card") {
		t.Error("the sweep kept removing bytes after the original failed; " +
			"the object is abandoned as a unit or not at all")
	}
	if n := countRows(t, db, `SELECT count(*) FROM media.media_variants WHERE media_object_id = 'mo-1'`); n != 1 {
		t.Errorf("%d of 1 variant rows left", n)
	}
	if n := countRows(t, db, `SELECT count(*) FROM media.media_objects WHERE id = 'mo-1'`); n != 1 {
		t.Errorf("%d of 1 media rows left", n)
	}
}

// FR-PURGE-9, second clause. The bytes can go cleanly and the ROW deletion
// still fail — a lock timeout, a migration mid-flight, a table that is not
// there. That failure must log at WARN and the sweep must continue, and the
// object it happened to must stay whole: the transaction rolls back, so every
// row survives together and the next tick retries the object rather than
// finding a half-deleted one it can no longer reason about.
//
// Dropping the ledger table is the cheapest way to make the transaction fail
// AFTER its first statement has already deleted the variant rows, so this also
// asserts the rollback and not merely that nothing was attempted.
func TestRunOnce_aFailedRowDeletionKeepsEveryRowAndContinues(t *testing.T) {
	db := newTestDB(t)
	seedMediaObject(t, db, "mo-1", hourAgo(), nil)
	seedVariant(t, db, "mv-c", "mo-1", "card", "k/mo-1-card", nil)
	// A second object that must still be reached: one object's row-deletion
	// failure must not abort the sweep.
	seedMediaObject(t, db, "mo-2", hourAgo(), nil)

	if err := db.Exec("DROP TABLE media.media_variant_failures").Error; err != nil {
		t.Fatalf("drop ledger table: %v", err)
	}

	store := newRemover()
	if err := newSweeper(t, db, store, Config{}).RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce must not return a row-deletion failure: %v", err)
	}

	if n := countRows(t, db, `SELECT count(*) FROM media.media_objects WHERE id = 'mo-1'`); n != 1 {
		t.Errorf("%d of 1 media rows left; a failed transaction must delete nothing", n)
	}
	if n := countRows(t, db, `SELECT count(*) FROM media.media_variants WHERE media_object_id = 'mo-1'`); n != 1 {
		t.Error("the variant rows were deleted even though the transaction failed — " +
			"the rollback is the only thing keeping the object retryable as a unit")
	}
	if !store.didRemove("k/mo-2") {
		t.Error("the sweep stopped at the failing object; the remaining objects must still be processed")
	}

	// Both objects are still retryable: nothing was consumed by the failure.
	objs, err := mediaobject.ListPurgeable(db)
	if err != nil {
		t.Fatalf("ListPurgeable: %v", err)
	}
	if len(objs) != 2 {
		t.Errorf("ListPurgeable returned %d objects, want both still awaiting retry: %+v", len(objs), objs)
	}
}

// T4 / FR-PURGE-8. Retry safety, asserted rather than assumed. A crash between
// the byte removals and the transaction leaves rows pointing at absent objects;
// the next sweep re-issues removal for keys that are already gone. S3 DELETE is
// idempotent and storage.Client.RemoveObject returns nil for a missing key, so
// the retry must proceed all the way to the row deletion.
func TestRunOnce_retryAfterBytesAreAlreadyGoneCompletesThePurge(t *testing.T) {
	db := newTestDB(t)
	seedMediaObject(t, db, "mo-1", hourAgo(), nil)
	seedVariant(t, db, "mv-c", "mo-1", "card", "k/mo-1-card", nil)
	seedLedgerRow(t, db, "mo-1", "card")

	// First tick: the bytes go, then the process "crashes" before the rows do.
	failing := newRemover("k/mo-1-card")
	if err := newSweeper(t, db, failing, Config{}).RunOnce(context.Background()); err != nil {
		t.Fatalf("first RunOnce: %v", err)
	}
	if n := countRows(t, db, `SELECT count(*) FROM media.media_objects WHERE id = 'mo-1'`); n != 1 {
		t.Fatalf("setup: the object should still be awaiting retry, got %d rows", n)
	}

	// Second tick: the bucket is healthy and the already-absent keys are no-ops.
	store := newRemover()
	if err := newSweeper(t, db, store, Config{}).RunOnce(context.Background()); err != nil {
		t.Fatalf("retry RunOnce: %v", err)
	}
	for _, c := range []struct {
		what  string
		query string
	}{
		{"variant", `SELECT count(*) FROM media.media_variants WHERE media_object_id = 'mo-1'`},
		{"ledger", `SELECT count(*) FROM media.media_variant_failures WHERE media_object_id = 'mo-1'`},
		{"media object", `SELECT count(*) FROM media.media_objects WHERE id = 'mo-1'`},
	} {
		if n := countRows(t, db, c.query); n != 0 {
			t.Errorf("the retry left %d %s rows behind", n, c.what)
		}
	}
}
