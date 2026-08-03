package vehicle

import (
	"time"

	"gorm.io/gorm"
)

// Entity maps to fleet.vehicles (PRD §6).
type Entity struct {
	ID                  string `gorm:"type:uuid;primaryKey"`
	FleetID             string `gorm:"type:uuid;not null;index"`
	Nickname            string
	Make                string `gorm:"not null"`
	Model               string `gorm:"not null"`
	Trim                string
	Year                int `gorm:"not null"`
	VIN                 string
	CurrentMileage      int
	PrimaryImageMediaID string
	Notes               string
	// Both layers are deliberate (task-006 design §4). `<-:create` protects the
	// COLUMN: db.Save in Administrator.Update UPDATEs every column, and GORM
	// gives CreatedAt none of the auto-management it gives UpdatedAt, so an
	// untagged column is written as 0001-01-01 on every PATCH. Assigning the
	// field in ToEntity() protects the MODEL that Make(e) returns after the
	// write — DeriveStatus falls back to CreatedAt(), so a zero there reports a
	// healthy vehicle as "Inactive" even when the row is fine.
	CreatedAt        time.Time `gorm:"<-:create"`
	UpdatedAt        time.Time
	DeletedAt        *time.Time `gorm:"index"`
	PurgeAfter       *time.Time
	PurgeOperationID *string `gorm:"type:uuid;index"`
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
		purgeOperationID:    e.PurgeOperationID,
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
		CreatedAt:           m.createdAt,
		DeletedAt:           m.deletedAt,
		PurgeAfter:          m.purgeAfter,
		PurgeOperationID:    m.purgeOperationID,
	}
}
