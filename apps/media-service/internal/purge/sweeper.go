// Package purge is media-service's time-based hard-delete sweep: it removes
// soft-deleted media objects whose five-day recovery window has elapsed,
// together with every byte and row derived from them.
//
// It is NOT the admin purge protocol. internal/admin serves an operator-driven,
// cancellable stamp/restore/reap lifecycle keyed on purge_operation_id; this
// package is the unattended hourly sweep keyed on purge_after, and
// ListPurgeable's `purge_operation_id IS NULL` is the seam that keeps the two
// off each other's rows.
//
// It imports mediaobject, mediavariant and variantfailures directly rather than
// receiving composition-root adapters (design D-1). The invariant that matters —
// those three packages stay independent of ONE ANOTHER — is unchanged and, for
// the first time, enforced by a test (arch_test.go). Ports whose only
// implementations lived in package main would leave this package's tests unable
// to exercise the production SQL, and assertions about stored rows are the whole
// point of the suite. ObjectRemover stays a port for the ordinary reason: MinIO
// cannot be in a unit test.
package purge

import (
	"context"
	"fmt"

	"github.com/sirupsen/logrus"
	"gorm.io/gorm"

	"github.com/jtumidanski/myfleet/apps/media-service/internal/mediaobject"
	"github.com/jtumidanski/myfleet/apps/media-service/internal/mediavariant"
	"github.com/jtumidanski/myfleet/apps/media-service/internal/variantfailures"
)

// ObjectRemover is the slice of storage.Client the sweep needs. Declaring the
// port here rather than importing the concrete client keeps the dependency
// one-way and makes the sweep testable without MinIO (FR-TEST-3).
type ObjectRemover interface {
	RemoveObject(ctx context.Context, key string) error
}

// Config is the sweep's tunable surface.
type Config struct {
	// ReconcileLimit bounds orphan rows processed per tick (FR-RECON-5).
	// 0 or negative disables the reconciliation pass entirely.
	ReconcileLimit int
}

// Sweeper runs one purge tick.
type Sweeper struct {
	log   logrus.FieldLogger
	db    *gorm.DB
	store ObjectRemover
	cfg   Config
}

// NewSweeper builds a Sweeper. Nothing is started; the caller owns the ticker.
func NewSweeper(log logrus.FieldLogger, db *gorm.DB, store ObjectRemover, cfg Config) *Sweeper {
	return &Sweeper{log: log, db: db, store: store, cfg: cfg}
}

// summary accumulates what one tick actually completed, so a quiet tick can log
// nothing. This job runs hourly forever and finds nothing on almost every run.
type summary struct {
	mediaObjectsPurged int
	objectsRemoved     int
	orphanVariants     int
	orphanFailures     int
	// reconcileCapped is a heuristic, not a fact: exactly `limit` orphans could
	// be exactly all of them. It is reported rather than swallowed because a
	// silently truncated cleanup reads as "finished" when it is not
	// (FR-RECON-6).
	reconcileCapped bool
}

func (sum summary) empty() bool {
	return sum.mediaObjectsPurged == 0 && sum.objectsRemoved == 0 &&
		sum.orphanVariants == 0 && sum.orphanFailures == 0
}

func (sum summary) log(log logrus.FieldLogger) {
	if sum.empty() {
		return
	}
	log.WithFields(logrus.Fields{
		"media_objects_purged":       sum.mediaObjectsPurged,
		"objects_removed":            sum.objectsRemoved,
		"orphan_variants_reconciled": sum.orphanVariants,
		"orphan_failures_deleted":    sum.orphanFailures,
		"reconcile_capped":           sum.reconcileCapped,
	}).Info("media purge sweep completed")
}

// RunOnce executes one tick: the per-object pass, then reconciliation.
//
// Call it under database.WithLeaderLock(db, "media-purge", …) so only one
// replica sweeps per tick (FR-RECON-8).
//
// It returns an error only for a failure that aborts a whole pass — the
// ListPurgeable query, or the orphan query. Per-object and per-orphan failures
// are logged and stepped over, never returned: returning one would abandon
// every item behind it, which is exactly what FR-PURGE-7 forbids.
//
// Both passes run even if the first one fails. They are independent — the
// reconciliation pass is defined by the ABSENCE of a media object, so it shares
// no state with the per-object pass — and the self-healing safety net should not
// be silenced by an unrelated query failure.
func (s *Sweeper) RunOnce(ctx context.Context) error {
	var sum summary
	perObjectErr := s.purgeExpired(ctx, &sum)
	reconcileErr := s.reconcile(ctx, &sum)
	sum.log(s.log)
	if perObjectErr != nil {
		return perObjectErr
	}
	return reconcileErr
}

// reconcile removes derived state whose media object no longer exists: variant
// rows (and the bytes they are the last record of) and variant-failure ledger
// rows.
//
// It is permanent, not a one-shot migration (FR-RECON-7). Once the backlog left
// by earlier versions drains it matches zero rows per tick and costs one
// anti-join per table — an honest recurring cost, which is why ReconcileLimit
// <= 0 is an off switch rather than something an operator has to roll back to
// escape. It stays because lazy card generation writes variant rows from REQUEST
// context, outside the media-purge lock, so a variant row can in principle
// appear for a media object the sweep is deleting. A per-tick pass makes that
// race self-healing within an hour.
func (s *Sweeper) reconcile(ctx context.Context, sum *summary) error {
	if s.cfg.ReconcileLimit <= 0 {
		return nil
	}

	orphans, err := mediavariant.ListOrphaned(s.db, s.cfg.ReconcileLimit)
	if err != nil {
		return fmt.Errorf("list orphaned variants: %w", err)
	}
	// Each row is handled individually so one failed byte removal spares exactly
	// its own row. Volume is bounded by the cap, so the per-row statement cost is
	// acceptable; a batched delete would lose the sparing.
	for _, v := range orphans {
		if rerr := s.store.RemoveObject(ctx, v.ObjectKey); rerr != nil {
			// The row survives, so its object_key — the only remaining record of
			// these bytes — stays reachable for the next tick (FR-RECON-3).
			s.log.WithError(rerr).WithFields(logrus.Fields{
				"variant_id": v.ID, "object_key": v.ObjectKey,
			}).Warn("remove orphaned variant object failed")
			continue
		}
		if derr := mediavariant.DeleteByID(s.db, v.ID); derr != nil {
			s.log.WithError(derr).WithField("variant_id", v.ID).
				Warn("delete orphaned variant row failed")
			continue
		}
		sum.orphanVariants++
	}
	sum.reconcileCapped = len(orphans) == s.cfg.ReconcileLimit

	// The ledger carries no object_key, so orphans there are deleted directly
	// (FR-RECON-4).
	n, err := variantfailures.DeleteOrphaned(s.db, s.cfg.ReconcileLimit)
	if err != nil {
		return fmt.Errorf("delete orphaned variant failures: %w", err)
	}
	sum.orphanFailures += n
	return nil
}

// purgeExpired is the per-object pass: for each media object whose recovery
// window has elapsed, remove every byte it owns and then every row that
// describes it.
func (s *Sweeper) purgeExpired(ctx context.Context, sum *summary) error {
	objs, err := mediaobject.ListPurgeable(s.db)
	if err != nil {
		return fmt.Errorf("list purgeable media objects: %w", err)
	}
	for _, o := range objs {
		variantKeys, err := mediavariant.ObjectKeysForMediaObject(s.db, o.ID())
		if err != nil {
			s.log.WithError(err).WithField("media_id", o.ID()).
				Warn("list variant object keys during purge failed")
			continue
		}

		// Bytes before rows, always (FR-PURGE-6). The rows are the only record
		// of which objects exist, so deleting them first would strand the bytes
		// in the bucket with nothing left pointing at them.
		//
		// The original goes first so that a bucket-wide outage abandons the
		// object before any of its derived bytes are touched.
		removed := 0
		failed := false
		for _, key := range append([]string{o.ObjectKey()}, variantKeys...) {
			if rerr := s.store.RemoveObject(ctx, key); rerr != nil {
				s.log.WithError(rerr).WithFields(logrus.Fields{
					"media_id": o.ID(), "object_key": key,
				}).Warn("remove minio object during purge failed")
				failed = true
				break
			}
			removed++
		}
		if failed {
			// EVERY row for this object stays put, so the handle on whatever
			// bytes remain survives and a later sweep retries the object whole
			// (FR-PURGE-7). Deleting the subset whose bytes did go would strand
			// the rest — the defect this task exists to eliminate.
			continue
		}

		// One transaction per media object, not one per tick: FR-PURGE-7 and
		// FR-PURGE-9 both require the sweep to log one object's failure and
		// CONTINUE, and a tick-wide transaction would roll the successful
		// objects back with the failed one. Everything inside uses tx.
		if terr := s.db.Transaction(func(tx *gorm.DB) error {
			if err := mediavariant.DeleteForMediaObject(tx, o.ID()); err != nil {
				return err
			}
			if err := variantfailures.DeleteForMediaObject(tx, o.ID()); err != nil {
				return err
			}
			return mediaobject.DeleteRow(tx, o.ID())
		}); terr != nil {
			// The bytes are already gone. The surviving rows are retried on the
			// next sweep, and removal of an absent key is a no-op, so that retry
			// completes rather than wedging (FR-PURGE-8/9).
			s.log.WithError(terr).WithField("media_id", o.ID()).
				Warn("delete rows during purge failed")
			continue
		}

		// Counted only on a completed object, so the summary reads as "what this
		// tick finished". A partial failure has already logged at WARN.
		sum.mediaObjectsPurged++
		sum.objectsRemoved += removed
	}
	return nil
}
