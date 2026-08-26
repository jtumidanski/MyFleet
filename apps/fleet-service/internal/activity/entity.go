package activity

import (
	"time"

	"gorm.io/gorm"
)

// Entity maps to fleet.activity_events (PRD §6). payload is JSONB on Postgres.
//
// Append-only, with ONE exception: rows are inserted once and never updated or
// deleted by any ordinary code path, and this package exposes no way to do so.
// An admin vehicle transfer re-points fleet_id — and ONLY fleet_id — on the
// moved vehicle's rows, in raw SQL inside internal/admin (see that package's
// transfer.go). fleet_id is not part of "what happened"; it is a denormalised
// routing column answering "whose feed does this appear in", and correcting it
// when the vehicle changes owners preserves the feed's meaning rather than
// revising it. Leaving it stale would leak one household's activity into
// another's vehicle detail view, because Provider.ListByVehicle selects on
// vehicle_id alone.
type Entity struct {
	ID               string  `gorm:"type:uuid;primaryKey"`
	FleetID          string  `gorm:"type:uuid;not null;index"`
	VehicleID        *string `gorm:"type:uuid;index"`
	ActorUserID      string  `gorm:"type:uuid;not null"`
	Type             string  `gorm:"not null"`
	Payload          []byte  `gorm:"type:jsonb"`
	CreatedAt        time.Time
	DeletedAt        *time.Time `gorm:"index"`
	PurgeOperationID *string    `gorm:"type:uuid;index"`
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
