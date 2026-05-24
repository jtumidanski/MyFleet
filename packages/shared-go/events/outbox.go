package events

import (
	"context"
	"encoding/json"
	"time"

	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

// OutboxRow is the transactional-outbox table (design A8, §13).
type OutboxRow struct {
	EventID    string `gorm:"primaryKey"`
	Type       string
	Payload    []byte
	OccurredAt time.Time
	SentAt     *time.Time
}

func (OutboxRow) TableName() string { return "outbox" }

func MigrateOutbox(db *gorm.DB) error { return db.AutoMigrate(&OutboxRow{}) }

// Enqueue writes an event into the outbox in the caller's transaction.
func Enqueue(tx *gorm.DB, e Envelope) error {
	payload, err := json.Marshal(e)
	if err != nil {
		return err
	}
	return tx.Create(&OutboxRow{EventID: e.EventID, Type: e.Type, Payload: payload, OccurredAt: e.OccurredAt}).Error
}

// RelayOnce publishes all unsent outbox rows and marks them sent. Run under the
// advisory lock (design A8/A9).
func RelayOnce(ctx context.Context, log logrus.FieldLogger, db *gorm.DB, p Producer) error {
	var rows []OutboxRow
	if err := db.Where("sent_at IS NULL").Order("occurred_at").Limit(100).Find(&rows).Error; err != nil {
		return err
	}
	for _, row := range rows {
		var e Envelope
		if err := json.Unmarshal(row.Payload, &e); err != nil {
			log.WithError(err).Error("corrupt outbox payload")
			continue
		}
		if err := p.Publish(ctx, e); err != nil {
			return err
		}
		now := time.Now()
		if err := db.Model(&OutboxRow{}).Where("event_id = ?", row.EventID).Update("sent_at", &now).Error; err != nil {
			return err
		}
	}
	return nil
}
