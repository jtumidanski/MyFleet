// Package variantfailures is a ledger of derived-variant generations that can
// never succeed. Lazy card generation consults it before doing any work, so an
// original that does not decode is attempted once rather than on every request
// for the rest of the object's life (task-013 PRD §4.6).
//
// It is a ledger, not a domain aggregate — the same kind of thing as
// processedevents — so it gets an Entity and a Store rather than the
// immutable-Model-plus-builder treatment the domain packages use.
//
// In normal operation this table is empty: for lazy generation to fail
// permanently, a ready image must have an original that is now missing or
// undecodable, yet the processing worker decoded that same original
// successfully to produce its thumbnail and display. It is designed to be cheap
// at rest, not to carry throughput.
package variantfailures

import (
	"time"

	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Recorded reasons. Short constants rather than raw error text: this is a
// diagnostic aid, and error strings can carry object keys and filenames.
const (
	ReasonUndecodable     = "undecodable"
	ReasonOriginalMissing = "original-missing"
)

// Entity maps to media.media_variant_failures. The composite primary key is the
// uniqueness guarantee — no surrogate ID and no extra index needed.
//
// DeletedAt and PurgeOperationID exist because this table is in admin.Manifest:
// admin.Stamp writes both, admin.Restore and admin.Reap key on
// purge_operation_id, and admin.Count filters deleted_at IS NULL. Without them
// the manifest entry fails at RUNTIME, not at compile time.
//
// Neither carries a `gorm:"index"` tag, deliberately. AutoMigrate on a
// schema-qualified table emits its CREATE INDEX WITHOUT the schema qualifier
// (`ON media_variant_failures`, not `ON media.media_variant_failures`), which
// fails outright on the SQLite test harness. The indexes are therefore created
// explicitly in ApplyIndexes below — the same reason and the same shape as
// mediavariant.ApplyPartialIndexes.
//
// No unique index, unlike media.media_variants: the composite primary key is the
// uniqueness guarantee, and Record's ON CONFLICT DO NOTHING names no columns, so
// Postgres needs no inferred arbiter and a soft-deleted row cannot be silently
// updated in place.
type Entity struct {
	MediaObjectID    string `gorm:"type:uuid;primaryKey"`
	Variant          string `gorm:"primaryKey"`
	Reason           string
	FailedAt         time.Time
	DeletedAt        *time.Time
	PurgeOperationID *string `gorm:"type:uuid"`
}

func (Entity) TableName() string { return "media.media_variant_failures" }

// Migration auto-migrates the variant-failure ledger table and its indexes.
//
// Both columns are nullable with no default, so AutoMigrate adds them to a
// populated table without a rewrite, and NULL is the correct value for every
// existing row — live, not soft-deleted, attached to no purge operation — so
// there is no backfill.
func Migration(db *gorm.DB) error {
	if err := db.AutoMigrate(&Entity{}); err != nil {
		return err
	}
	return ApplyIndexes(db)
}

// ApplyIndexes creates the deleted_at and purge_operation_id indexes the admin
// purge protocol's Restore and Reap scans depend on.
//
// It is separate from Migration, and exported, for the same reason
// mediavariant.ApplyPartialIndexes is: a test harness that hand-writes the table
// still needs the real indexes, and struct tags cannot express the
// schema-qualified form SQLite requires.
func ApplyIndexes(db *gorm.DB) error {
	stmts := []string{
		`CREATE INDEX IF NOT EXISTS idx_media_variant_failures_deleted_at
		 ON media.media_variant_failures (deleted_at)`,
		`CREATE INDEX IF NOT EXISTS idx_media_variant_failures_purge_operation_id
		 ON media.media_variant_failures (purge_operation_id)`,
	}
	if db.Name() == "sqlite" {
		stmts = []string{
			`CREATE INDEX IF NOT EXISTS media.idx_media_variant_failures_deleted_at
			 ON media_variant_failures (deleted_at)`,
			`CREATE INDEX IF NOT EXISTS media.idx_media_variant_failures_purge_operation_id
			 ON media_variant_failures (purge_operation_id)`,
		}
	}
	for _, q := range stmts {
		if err := db.Exec(q).Error; err != nil {
			return err
		}
	}
	return nil
}

// Store records and queries permanently-failed variant generations.
type Store struct {
	log logrus.FieldLogger
	db  *gorm.DB
}

// New returns a Store backed by the given database.
func New(log logrus.FieldLogger, db *gorm.DB) *Store { return &Store{log: log, db: db} }

// Recorded reports whether generation of this variant for this media object is
// known to be impossible.
//
// Soft-deleted rows do not count (FR-ADMIN-3). A ledger row an admin purge has
// stamped belongs to an operation that may still be cancelled, and a purge that
// gets undone must not leave the object's card permanently disabled.
//
// One consequence, documented rather than engineered around: while a row is
// soft-deleted its primary-key slot is still occupied, so a fresh Record for the
// same (media_object_id, variant) is a silent no-op and Recorded keeps returning
// false. In principle that is an unbounded retry loop; in practice it is
// unreachable, because Recorded is consulted only by processing.CardGenerator,
// which is reached only through Content, which resolves a LIVE media object
// first — and a media object whose ledger rows are stamped is itself stamped,
// and therefore soft-deleted, in the same transaction. Restore clears both
// columns and the ledger is live again.
func (s *Store) Recorded(mediaObjectID, variant string) (bool, error) {
	var count int64
	err := s.db.Model(&Entity{}).
		Where("media_object_id = ? AND variant = ? AND deleted_at IS NULL", mediaObjectID, variant).
		Count(&count).Error
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// Record notes a permanent failure. First failure wins: re-recording is a no-op
// (ON CONFLICT DO NOTHING), so the original reason is never overwritten by a
// later, less informative one.
//
// Only permanent failures belong here. A transport error from the object store
// or a database error is transient and must NOT be recorded — the next request
// is allowed to try again.
func (s *Store) Record(mediaObjectID, variant, reason string) error {
	return s.db.Clauses(clause.OnConflict{DoNothing: true}).
		Create(&Entity{
			MediaObjectID: mediaObjectID,
			Variant:       variant,
			Reason:        reason,
			FailedAt:      time.Now().UTC(),
		}).Error
}
