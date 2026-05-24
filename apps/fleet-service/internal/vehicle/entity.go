package vehicle

import (
	"time"

	"gorm.io/gorm"
)

// Entity maps to fleet.vehicles (PRD §6).
type Entity struct {
	ID                  string         `gorm:"type:uuid;primaryKey"`
	FleetID             string         `gorm:"type:uuid;not null;index"`
	Nickname            string
	Make                string         `gorm:"not null"`
	Model               string         `gorm:"not null"`
	Trim                string
	Year                int            `gorm:"not null"`
	VIN                 string
	CurrentMileage      int
	PrimaryImageMediaID string
	Notes               string
	CreatedAt           time.Time
	UpdatedAt           time.Time
	DeletedAt           *time.Time     `gorm:"index"`
	PurgeAfter          *time.Time
}

func (Entity) TableName() string { return "fleet.vehicles" }

func Migration(db *gorm.DB) error { return db.AutoMigrate(&Entity{}) }

// Make converts an Entity to a Model.
func Make(e Entity) Model {
	return Model{
		id:                  e.ID,
		fleetID:             e.FleetID,
		nickname:            e.Nickname,
		make:                e.Make,
		model:               e.Model,
		trim:                e.Trim,
		year:                e.Year,
		vin:                 e.VIN,
		currentMileage:      e.CurrentMileage,
		primaryImageMediaID: e.PrimaryImageMediaID,
		notes:               e.Notes,
		createdAt:           e.CreatedAt,
		updatedAt:           e.UpdatedAt,
		deletedAt:           e.DeletedAt,
		purgeAfter:          e.PurgeAfter,
	}
}

// ToEntity converts a Model to an Entity for persistence.
func (m Model) ToEntity() Entity {
	return Entity{
		ID:                  m.id,
		FleetID:             m.fleetID,
		Nickname:            m.nickname,
		Make:                m.make,
		Model:               m.model,
		Trim:                m.trim,
		Year:                m.year,
		VIN:                 m.vin,
		CurrentMileage:      m.currentMileage,
		PrimaryImageMediaID: m.primaryImageMediaID,
		Notes:               m.notes,
		DeletedAt:           m.deletedAt,
		PurgeAfter:          m.purgeAfter,
	}
}
