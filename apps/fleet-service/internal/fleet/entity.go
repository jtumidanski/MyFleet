package fleet

import (
	"time"

	"gorm.io/gorm"
)

// Entity maps to fleet.fleets (PRD §6).
type Entity struct {
	ID              string `gorm:"type:uuid;primaryKey"`
	Name            string `gorm:"not null"`
	CreatedByUserID string `gorm:"not null"`
	CreatedAt       time.Time
	UpdatedAt       time.Time
	DeletedAt       gorm.DeletedAt `gorm:"index"`
}

func (Entity) TableName() string { return "fleet.fleets" }

func Migration(db *gorm.DB) error { return db.AutoMigrate(&Entity{}) }

// Make converts an Entity to a Model.
func Make(e Entity) Model {
	return Model{
		id:              e.ID,
		name:            e.Name,
		createdByUserID: e.CreatedByUserID,
		createdAt:       e.CreatedAt,
		updatedAt:       e.UpdatedAt,
	}
}

// ToEntity converts a Model to an Entity for persistence.
func (m Model) ToEntity() Entity {
	return Entity{
		ID:              m.id,
		Name:            m.name,
		CreatedByUserID: m.createdByUserID,
	}
}
