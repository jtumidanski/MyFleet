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

	"github.com/sirupsen/logrus"
	"gorm.io/gorm"

	"github.com/jtumidanski/myfleet/apps/media-service/internal/mediaobject"
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

// RunOnce executes one tick.
//
// Call it under database.WithLeaderLock(db, "media-purge", …) so only one
// replica sweeps per tick (FR-RECON-8).
func (s *Sweeper) RunOnce(ctx context.Context) error {
	objs, err := mediaobject.ListPurgeable(s.db)
	if err != nil {
		return err
	}
	for _, o := range objs {
		if err := s.store.RemoveObject(ctx, o.ObjectKey()); err != nil {
			s.log.WithError(err).WithField("media_id", o.ID()).
				Warn("remove minio object during purge failed")
			continue // leave the row so a later sweep retries
		}
		if err := mediaobject.DeleteRow(s.db, o.ID()); err != nil {
			s.log.WithError(err).WithField("media_id", o.ID()).
				Warn("delete media row during purge failed")
		}
	}
	return nil
}
