package notification

import (
	"time"

	"gorm.io/gorm"
)

// Entity maps to notification.notifications (PRD §6). dedupe_key carries a UNIQUE
// constraint so duplicate generation (event redelivery or the reminder
// safety-net) is rejected at the database level as well as in the processor.
type Entity struct {
	ID        string `gorm:"type:uuid;primaryKey"`
	UserID    string `gorm:"type:uuid;not null;index"`
	Type      string `gorm:"not null"`
	Title     string `gorm:"not null"`
	Body      string
	DedupeKey string `gorm:"not null;uniqueIndex"`
	VehicleID string `gorm:"type:uuid"`
	FleetID   string `gorm:"type:uuid"`
	ReadAt    *time.Time
	CreatedAt time.Time
}

func (Entity) TableName() string { return "notification.notifications" }

// Migration auto-migrates the notifications table.
func Migration(db *gorm.DB) error { return db.AutoMigrate(&Entity{}) }

// Make converts an Entity to a Model.
func Make(e Entity) Model {
	return Model{
		id:        e.ID,
		userID:    e.UserID,
		typ:       e.Type,
		title:     e.Title,
		body:      e.Body,
		dedupeKey: e.DedupeKey,
		vehicleID: e.VehicleID,
		fleetID:   e.FleetID,
		readAt:    e.ReadAt,
		createdAt: e.CreatedAt,
	}
}

// ToEntity converts a Model to an Entity for persistence.
func (m Model) ToEntity() Entity {
	return Entity{
		ID:        m.id,
		UserID:    m.userID,
		Type:      m.typ,
		Title:     m.title,
		Body:      m.body,
		DedupeKey: m.dedupeKey,
		VehicleID: m.vehicleID,
		FleetID:   m.fleetID,
		ReadAt:    m.readAt,
		CreatedAt: m.createdAt,
	}
}
