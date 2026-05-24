package vehiclemedia

import (
	"time"

	"gorm.io/gorm"
)

// Entity maps to fleet.vehicle_media (PRD §6, design §8.3).
type Entity struct {
	ID        string `gorm:"type:uuid;primaryKey"`
	VehicleID string `gorm:"type:uuid;not null;index"`
	MediaID   string `gorm:"not null"`
	IsPrimary bool   `gorm:"not null;default:false"`
	SortOrder int    `gorm:"not null;default:0"`
	CreatedAt time.Time
	DeletedAt *time.Time `gorm:"index"`
}

func (Entity) TableName() string { return "fleet.vehicle_media" }

func Migration(db *gorm.DB) error { return db.AutoMigrate(&Entity{}) }

// Make converts an Entity to a Model.
func Make(e Entity) Model {
	return Model{
		id:        e.ID,
		vehicleID: e.VehicleID,
		mediaID:   e.MediaID,
		isPrimary: e.IsPrimary,
		sortOrder: e.SortOrder,
		createdAt: e.CreatedAt,
		deletedAt: e.DeletedAt,
	}
}

// ToEntity converts a Model to an Entity for persistence.
func (m Model) ToEntity() Entity {
	return Entity{
		ID:        m.id,
		VehicleID: m.vehicleID,
		MediaID:   m.mediaID,
		IsPrimary: m.isPrimary,
		SortOrder: m.sortOrder,
		DeletedAt: m.deletedAt,
	}
}
