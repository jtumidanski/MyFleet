package purge

import (
	"context"
	"testing"
)

// T6 / FR-RECON-1/2/3. The orphan's bytes and row go; a variant whose media
// object is soft-deleted but still PRESENT is untouched. The negative half is
// the important one: that object is still inside its five-day recovery window,
// and reconciling its variants would silently break restore.
func TestReconcile_takesOrphansAndSparesRecoverableVariants(t *testing.T) {
	db := newTestDB(t)
	// Live parent.
	seedMediaObject(t, db, "mo-live", nil, nil)
	seedVariant(t, db, "mv-live", "mo-live", "card", "k/live", nil)
	// Soft-deleted but present parent — inside the recovery window.
	seedMediaObject(t, db, "mo-soft", hourAgo(), nil)
	seedVariant(t, db, "mv-soft", "mo-soft", "card", "k/soft", nil)
	// True orphan: no parent row at all.
	seedVariant(t, db, "mv-orphan", "mo-gone", "card", "k/orphan", nil)

	store := newRemover()
	// mo-soft's purge_after is in the past, so exclude it from the per-object
	// pass by giving the sweeper only the reconciliation job to do: seed it as
	// soft-deleted with NO purge_after instead.
	if err := db.Exec(`UPDATE media.media_objects SET purge_after = NULL WHERE id = 'mo-soft'`).Error; err != nil {
		t.Fatalf("clear purge_after: %v", err)
	}

	if err := newSweeper(t, db, store, Config{ReconcileLimit: 500}).RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}

	if !store.didRemove("k/orphan") {
		t.Error("the orphaned variant's bytes were never removed")
	}
	if n := countRows(t, db, `SELECT count(*) FROM media.media_variants WHERE id = 'mv-orphan'`); n != 0 {
		t.Error("the orphaned variant row survived")
	}
	if store.wasAsked("k/soft") {
		t.Error("reconciliation removed the bytes of a variant whose media object is still " +
			"recoverable; restore is now broken and no cancel can undo it")
	}
	if n := countRows(t, db, `SELECT count(*) FROM media.media_variants WHERE id = 'mv-soft'`); n != 1 {
		t.Error("reconciliation deleted a variant of a soft-deleted-but-present media object")
	}
	if n := countRows(t, db, `SELECT count(*) FROM media.media_variants WHERE id = 'mv-live'`); n != 1 {
		t.Error("reconciliation deleted a variant of a live media object")
	}
}

// T7 / FR-RECON-5. With more orphans than the cap, exactly the cap is processed
// and the remainder survives to the next tick. One tick must not turn into an
// unbounded bucket-deletion loop on first deployment.
func TestReconcile_processesAtMostTheCapPerTick(t *testing.T) {
	db := newTestDB(t)
	for _, id := range []string{"mv-1", "mv-2", "mv-3", "mv-4", "mv-5"} {
		seedVariant(t, db, id, "mo-gone", "card-"+id, "k/"+id, nil)
	}

	store := newRemover()
	if err := newSweeper(t, db, store, Config{ReconcileLimit: 2}).RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}

	if len(store.removed) != 2 {
		t.Errorf("removed %d objects (%v), want exactly the cap of 2", len(store.removed), store.removed)
	}
	if n := countRows(t, db, `SELECT count(*) FROM media.media_variants`); n != 3 {
		t.Errorf("%d orphan rows left, want the 3 over the cap", n)
	}

	// The remainder must drain on subsequent ticks, not be lost.
	next := newRemover()
	if err := newSweeper(t, db, next, Config{ReconcileLimit: 2}).RunOnce(context.Background()); err != nil {
		t.Fatalf("second RunOnce: %v", err)
	}
	if n := countRows(t, db, `SELECT count(*) FROM media.media_variants`); n != 1 {
		t.Errorf("%d orphan rows left after the second tick, want 1", n)
	}
}

// FR-RECON-3. A failed byte removal spares exactly its own row, so the bytes
// stay reachable — the row's object_key is the only record of them.
func TestReconcile_aFailedRemovalKeepsThatOrphanRow(t *testing.T) {
	db := newTestDB(t)
	seedVariant(t, db, "mv-bad", "mo-gone", "card", "k/bad", nil)
	seedVariant(t, db, "mv-ok", "mo-gone", "display", "k/ok", nil)

	store := newRemover("k/bad")
	if err := newSweeper(t, db, store, Config{ReconcileLimit: 500}).RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}

	if n := countRows(t, db, `SELECT count(*) FROM media.media_variants WHERE id = 'mv-bad'`); n != 1 {
		t.Error("the row whose bytes could not be removed was deleted; those bytes are now unreachable")
	}
	if n := countRows(t, db, `SELECT count(*) FROM media.media_variants WHERE id = 'mv-ok'`); n != 0 {
		t.Error("one orphan's failure spared an unrelated orphan")
	}
}

// T8 / FR-RECON-1/4. Orphaned ledger rows are deleted directly — they carry no
// object_key, so there are no bytes to reclaim first — and a ledger row whose
// media object is still present is untouched.
func TestReconcile_takesOrphanedLedgerRowsOnly(t *testing.T) {
	db := newTestDB(t)
	seedMediaObject(t, db, "mo-live", nil, nil)
	seedLedgerRow(t, db, "mo-live", "card")
	seedLedgerRow(t, db, "mo-gone", "card")

	if err := newSweeper(t, db, newRemover(), Config{ReconcileLimit: 500}).RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}

	if n := countRows(t, db, `SELECT count(*) FROM media.media_variant_failures WHERE media_object_id = 'mo-gone'`); n != 0 {
		t.Error("the orphaned ledger row survived")
	}
	if n := countRows(t, db, `SELECT count(*) FROM media.media_variant_failures WHERE media_object_id = 'mo-live'`); n != 1 {
		t.Error("reconciliation deleted a ledger row whose media object still exists")
	}
}

// A-3. The operator off switch. A background pass that touches object storage
// needs one that is not a rollback.
func TestReconcile_isDisabledByANonPositiveLimit(t *testing.T) {
	db := newTestDB(t)
	seedVariant(t, db, "mv-orphan", "mo-gone", "card", "k/orphan", nil)
	seedLedgerRow(t, db, "mo-gone", "card")

	store := newRemover()
	if err := newSweeper(t, db, store, Config{ReconcileLimit: 0}).RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}

	if store.wasAsked("k/orphan") {
		t.Error("reconciliation ran with the limit at 0; the off switch does not work")
	}
	if n := countRows(t, db, `SELECT count(*) FROM media.media_variants`); n != 1 {
		t.Error("reconciliation deleted a row with the limit at 0")
	}
	if n := countRows(t, db, `SELECT count(*) FROM media.media_variant_failures`); n != 1 {
		t.Error("ledger reconciliation ran with the limit at 0")
	}
}
