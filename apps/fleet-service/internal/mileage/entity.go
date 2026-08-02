package mileage

import (
	"time"

	"gorm.io/gorm"
)

// Entity maps to fleet.mileage_records (PRD §6, plan Task 8.1).
type Entity struct {
	ID               string    `gorm:"type:uuid;primaryKey"`
	VehicleID        string    `gorm:"type:uuid;not null;index"`
	Mileage          int       `gorm:"not null"`
	RecordedAt       time.Time `gorm:"not null;index"`
	Source           string    `gorm:"not null"` // fuel | maintenance | manual
	SourceRefID      string
	CreatedByUserID  string
	CreatedAt        time.Time
	DeletedAt        *time.Time `gorm:"index"`
	PurgeOperationID *string    `gorm:"type:uuid;index"`
}

func (Entity) TableName() string { return "fleet.mileage_records" }

func Migration(db *gorm.DB) error { return db.AutoMigrate(&Entity{}) }

// Make converts an Entity to a Model.
func Make(e Entity) Model {
	return Model{
		id:              e.ID,
		vehicleID:       e.VehicleID,
		mileage:         e.Mileage,
		recordedAt:      e.RecordedAt,
		source:          e.Source,
		sourceRefID:     e.SourceRefID,
		createdByUserID: e.CreatedByUserID,
		createdAt:       e.CreatedAt,
	}
}

// ToEntity converts a Model to an Entity for persistence.
func (m Model) ToEntity() Entity {
	return Entity{
		ID:              m.id,
		VehicleID:       m.vehicleID,
		Mileage:         m.mileage,
		RecordedAt:      m.recordedAt,
		Source:          m.source,
		SourceRefID:     m.sourceRefID,
		CreatedByUserID: m.createdByUserID,
		CreatedAt:       m.createdAt,
	}
}
