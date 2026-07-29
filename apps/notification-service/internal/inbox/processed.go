// Package inbox provides the idempotency ledger for notification-service's event
// consumers. Each consumed event is recorded by (event_id, consumer); re-delivery
// of an already-recorded event is a no-op (design §7 — at-least-once consumers
// must be idempotent). Mirrors media-service's processedevents helper, but keys
// on (event_id, consumer) so multiple logical consumers can share the table.
package inbox

import (
	"errors"
	"time"

	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Entity maps to notification.processed_events. The composite (event_id,
// consumer) is the primary key.
type Entity struct {
	EventID     string `gorm:"primaryKey"`
	Consumer    string `gorm:"primaryKey"`
	ProcessedAt time.Time
}

func (Entity) TableName() string { return "notification.processed_events" }

// Migration auto-migrates the processed-events ledger table.
func Migration(db *gorm.DB) error { return db.AutoMigrate(&Entity{}) }

// Store records and queries processed event IDs per consumer.
type Store struct {
	log logrus.FieldLogger
	db  *gorm.DB
}

// New returns a Store backed by the given database.
func New(log logrus.FieldLogger, db *gorm.DB) *Store { return &Store{log: log, db: db} }

// Exists reports whether (eventID, consumer) has already been recorded. Use this
// as a read-only dedupe check at the START of an event handler, before doing any
// work, so the handler can skip immediately without writing.
func (s *Store) Exists(eventID, consumer string) (bool, error) {
	var count int64
	err := s.db.Model(&Entity{}).
		Where("event_id = ? AND consumer = ?", eventID, consumer).
		Count(&count).Error
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// Mark records (eventID, consumer) with INSERT ... ON CONFLICT DO NOTHING. Call
// this AFTER all work succeeds so that a mid-processing failure leaves the event
// unrecorded and eligible for redelivery.
func (s *Store) Mark(eventID, consumer string) error {
	err := s.db.Clauses(clause.OnConflict{DoNothing: true}).
		Create(&Entity{EventID: eventID, Consumer: consumer, ProcessedAt: time.Now().UTC()})
	if err.Error != nil {
		// A duplicate-key error means the event was concurrently recorded; treat
		// as success (idempotent).
		if errors.Is(err.Error, gorm.ErrDuplicatedKey) {
			return nil
		}
		return err.Error
	}
	return nil
}
