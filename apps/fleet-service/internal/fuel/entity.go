package fuel

import (
	"time"

	"gorm.io/gorm"
)

// Entity maps to fleet.fuel_logs (PRD §6).
type Entity struct {
	ID               string    `gorm:"type:uuid;primaryKey"`
	VehicleID        string    `gorm:"type:uuid;not null;index"`
	Date             time.Time `gorm:"not null"`
	Mileage          int       `gorm:"not null"`
	Gallons          float64   `gorm:"not null"`
	TotalCost        float64   `gorm:"not null"`
	PricePerGallon   float64   `gorm:"not null"`
	CreatedByUserID  string
	CreatedAt        time.Time
	UpdatedAt        time.Time
	DeletedAt        *time.Time `gorm:"index"`
	PurgeOperationID *string    `gorm:"type:uuid;index"`
}

func (Entity) TableName() string { return "fleet.fuel_logs" }

func Migration(db *gorm.DB) error { return db.AutoMigrate(&Entity{}) }

// Make converts an Entity to a Model.
func Make(e Entity) Model {
	return Model{
		id:              e.ID,
		vehicleID:       e.VehicleID,
		date:            e.Date,
		mileage:         e.Mileage,
		gallons:         e.Gallons,
		totalCost:       e.TotalCost,
		pricePerGallon:  e.PricePerGallon,
		createdByUserID: e.CreatedByUserID,
		createdAt:       e.CreatedAt,
		updatedAt:       e.UpdatedAt,
		deletedAt:       e.DeletedAt,
	}
}

// ToEntity converts a Model to an Entity for persistence.
func (m Model) ToEntity() Entity {
	return Entity{
		ID:              m.id,
		VehicleID:       m.vehicleID,
		Date:            m.date,
		Mileage:         m.mileage,
		Gallons:         m.gallons,
		TotalCost:       m.totalCost,
		PricePerGallon:  m.pricePerGallon,
		CreatedByUserID: m.createdByUserID,
		DeletedAt:       m.deletedAt,
	}
}
