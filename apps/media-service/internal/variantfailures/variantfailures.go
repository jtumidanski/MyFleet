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
type Entity struct {
	MediaObjectID string `gorm:"type:uuid;primaryKey"`
	Variant       string `gorm:"primaryKey"`
	Reason        string
	FailedAt      time.Time
}

func (Entity) TableName() string { return "media.media_variant_failures" }

// Migration auto-migrates the variant-failure ledger table.
func Migration(db *gorm.DB) error { return db.AutoMigrate(&Entity{}) }

// Store records and queries permanently-failed variant generations.
type Store struct {
	log logrus.FieldLogger
	db  *gorm.DB
}

// New returns a Store backed by the given database.
func New(log logrus.FieldLogger, db *gorm.DB) *Store { return &Store{log: log, db: db} }

// Recorded reports whether generation of this variant for this media object is
// known to be impossible.
func (s *Store) Recorded(mediaObjectID, variant string) (bool, error) {
	var count int64
	err := s.db.Model(&Entity{}).
		Where("media_object_id = ? AND variant = ?", mediaObjectID, variant).
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
