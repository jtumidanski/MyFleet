package purge

import (
	"context"
	"fmt"
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

// T7 / FR-RECON-5/6. With more orphans than the cap, exactly the cap is
// processed and the remainder survives to the next tick. One tick must not
// turn into an unbounded bucket-deletion loop on first deployment.
//
// reconcileCapped is asserted directly, by calling reconcile instead of
// RunOnce so the summary is visible to the test. Without this,
// sum.reconcileCapped at sweeper.go could be hardcoded false, or use >= instead
// of ==, and every test in the package would still pass (FR-TEST-5) —
// reconcileCapped had no coverage at all before this change.
func TestReconcile_processesAtMostTheCapPerTick(t *testing.T) {
	db := newTestDB(t)
	for _, id := range []string{"mv-1", "mv-2", "mv-3", "mv-4", "mv-5"} {
		seedVariant(t, db, id, "mo-gone", "card-"+id, "k/"+id, nil)
	}

	store := newRemover()
	var sum summary
	if err := newSweeper(t, db, store, Config{ReconcileLimit: 2}).reconcile(context.Background(), &sum); err != nil {
		t.Fatalf("reconcile: %v", err)
	}

	if len(store.removed) != 2 {
		t.Errorf("removed %d objects (%v), want exactly the cap of 2", len(store.removed), store.removed)
	}
	if n := countRows(t, db, `SELECT count(*) FROM media.media_variants`); n != 3 {
		t.Errorf("%d orphan rows left, want the 3 over the cap", n)
	}
	if !sum.reconcileCapped {
		t.Error("reconcileCapped = false on a tick that hit the cap (2 orphans processed of 5), want true")
	}

	// The remainder must drain on subsequent ticks, not be lost. This second
	// tick still has more orphans (3) than the cap (2), so it is capped too.
	next := newRemover()
	var sum2 summary
	if err := newSweeper(t, db, next, Config{ReconcileLimit: 2}).reconcile(context.Background(), &sum2); err != nil {
		t.Fatalf("second reconcile: %v", err)
	}
	if n := countRows(t, db, `SELECT count(*) FROM media.media_variants`); n != 1 {
		t.Errorf("%d orphan rows left after the second tick, want 1", n)
	}
	if !sum2.reconcileCapped {
		t.Error("reconcileCapped = false on a second tick that also hit the cap (3 orphans over a limit of 2), want true")
	}

	// A third tick, with fewer orphans (1) than the cap (2), is the negative
	// case: reconcileCapped must read false here, or a silently truncated
	// cleanup would report as "finished" when it is not (FR-RECON-6).
	last := newRemover()
	var sum3 summary
	if err := newSweeper(t, db, last, Config{ReconcileLimit: 2}).reconcile(context.Background(), &sum3); err != nil {
		t.Fatalf("third reconcile: %v", err)
	}
	if n := countRows(t, db, `SELECT count(*) FROM media.media_variants`); n != 0 {
		t.Errorf("%d orphan rows left after the third tick, want 0", n)
	}
	if sum3.reconcileCapped {
		t.Error("reconcileCapped = true on a tick with fewer orphans (1) than the cap (2), want false")
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
//
// Both 0 and -1 are exercised, and -1 is the case that actually carries the
// evidence. 0 is the documented contract, but `LIMIT 0` returns zero rows on
// its own, in both SQLite and Postgres — a test run only at 0 would pass
// identically with the `ReconcileLimit <= 0` guard deleted outright, so it
// proves nothing about the guard (FR-TEST-5: a test that cannot be made to go
// red is not evidence). -1 is reachable in production — config.GetInt does a
// bare strconv.Atoi with no clamping, so MEDIA_PURGE_RECONCILE_LIMIT="-1"
// flows straight through — and it is where the guard is load-bearing: without
// it, SQLite's `LIMIT -1` is unbounded (reconciling everything every tick,
// exactly the unbounded bucket-deletion loop FR-RECON-5 forbids) and Postgres
// rejects a negative LIMIT outright, killing reconciliation for good.
func TestReconcile_isDisabledByANonPositiveLimit(t *testing.T) {
	for _, limit := range []int{0, -1} {
		t.Run(fmt.Sprintf("limit=%d", limit), func(t *testing.T) {
			db := newTestDB(t)
			seedVariant(t, db, "mv-orphan", "mo-gone", "card", "k/orphan", nil)
			seedLedgerRow(t, db, "mo-gone", "card")

			store := newRemover()
			if err := newSweeper(t, db, store, Config{ReconcileLimit: limit}).RunOnce(context.Background()); err != nil {
				t.Fatalf("RunOnce: %v", err)
			}

			if store.wasAsked("k/orphan") {
				t.Errorf("reconciliation ran with the limit at %d; the off switch does not work", limit)
			}
			if n := countRows(t, db, `SELECT count(*) FROM media.media_variants`); n != 1 {
				t.Errorf("reconciliation deleted a row with the limit at %d", limit)
			}
			if n := countRows(t, db, `SELECT count(*) FROM media.media_variant_failures`); n != 1 {
				t.Errorf("ledger reconciliation ran with the limit at %d", limit)
			}
		})
	}
}
