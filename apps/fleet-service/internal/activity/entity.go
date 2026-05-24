package activity

import (
	"time"

	"gorm.io/gorm"
)

// Entity maps to fleet.activity_events (PRD §6). Append-only: rows are inserted
// once and never updated or deleted. payload is JSONB on Postgres.
type Entity struct {
	ID          string  `gorm:"type:uuid;primaryKey"`
	FleetID     string  `gorm:"type:uuid;not null;index"`
	VehicleID   *string `gorm:"type:uuid;index"`
	ActorUserID string  `gorm:"type:uuid;not null"`
	Type        string  `gorm:"not null"`
	Payload     []byte  `gorm:"type:jsonb"`
	CreatedAt   time.Time
}

func (Entity) TableName() string { return "fleet.activity_events" }

func Migration(db *gorm.DB) error { return db.AutoMigrate(&Entity{}) }

// Make converts an Entity to a Model.
func Make(e Entity) Model {
	return Model{
		id:          e.ID,
		fleetID:     e.FleetID,
		vehicleID:   e.VehicleID,
		actorUserID: e.ActorUserID,
		eventType:   e.Type,
		payload:     e.Payload,
		createdAt:   e.CreatedAt,
	}
}

// ToEntity converts a Model to an Entity for persistence.
func (m Model) ToEntity() Entity {
	return Entity{
		ID:          m.id,
		FleetID:     m.fleetID,
		VehicleID:   m.vehicleID,
		ActorUserID: m.actorUserID,
		Type:        m.eventType,
		Payload:     m.payload,
		CreatedAt:   m.createdAt,
	}
}
