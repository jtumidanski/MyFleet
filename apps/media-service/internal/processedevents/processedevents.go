// Package processedevents provides an idempotency ledger for the media variant
// worker. Each consumed event is recorded by its event_id; re-delivery of an
// already-recorded event is a no-op (design §7 — at-least-once consumers must be
// idempotent).
package processedevents

import (
	"errors"
	"time"

	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Entity maps to media.processed_events.
type Entity struct {
	EventID     string `gorm:"primaryKey"`
	ProcessedAt time.Time
}

func (Entity) TableName() string { return "media.processed_events" }

// Migration auto-migrates the processed-events ledger table.
func Migration(db *gorm.DB) error { return db.AutoMigrate(&Entity{}) }

// Store records and queries processed event IDs.
type Store struct {
	log logrus.FieldLogger
	db  *gorm.DB
}

// New returns a Store backed by the given database.
func New(log logrus.FieldLogger, db *gorm.DB) *Store { return &Store{log: log, db: db} }

// MarkProcessed records the event_id and reports whether it had already been
// processed. It inserts with ON CONFLICT DO NOTHING; a zero-rows-affected result
// means the event was seen before (alreadyProcessed=true). This is the dedupe
// gate the worker checks before doing any expensive work.
func (s *Store) MarkProcessed(eventID string) (alreadyProcessed bool, err error) {
	res := s.db.Clauses(clause.OnConflict{DoNothing: true}).
		Create(&Entity{EventID: eventID, ProcessedAt: time.Now().UTC()})
	if res.Error != nil {
		// Belt-and-suspenders: treat a duplicate-key error as already-processed.
		if errors.Is(res.Error, gorm.ErrDuplicatedKey) {
			return true, nil
		}
		return false, res.Error
	}
	return res.RowsAffected == 0, nil
}
