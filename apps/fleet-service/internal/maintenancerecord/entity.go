package maintenancerecord

import (
	"time"

	"gorm.io/gorm"
)

// Entity maps to fleet.maintenance_records (PRD §6, design §8.2).
type Entity struct {
	ID               string    `gorm:"type:uuid;primaryKey"`
	VehicleID        string    `gorm:"type:uuid;not null;index"`
	CategoryID       string    `gorm:"type:uuid;not null"`
	Description      string    `gorm:"type:varchar(200)"`
	PerformedAt      time.Time `gorm:"not null"`
	Mileage          int
	Cost             float64
	Vendor           string
	Notes            string
	CreatedByUserID  string
	CreatedAt        time.Time
	UpdatedAt        time.Time
	DeletedAt        *time.Time `gorm:"index"`
	PurgeOperationID *string    `gorm:"type:uuid;index"`
}

func (Entity) TableName() string { return "fleet.maintenance_records" }

// DocumentEntity maps to fleet.maintenance_record_documents — attached media
// references for a maintenance record (receipts, photos, etc.).
type DocumentEntity struct {
	ID                  string     `gorm:"type:uuid;primaryKey"`
	MaintenanceRecordID string     `gorm:"type:uuid;not null;index"`
	MediaID             string     `gorm:"type:uuid;not null"`
	DeletedAt           *time.Time `gorm:"index"`
	PurgeOperationID    *string    `gorm:"type:uuid;index"`
}

func (DocumentEntity) TableName() string { return "fleet.maintenance_record_documents" }

func Migration(db *gorm.DB) error { return db.AutoMigrate(&Entity{}, &DocumentEntity{}) }

// Make converts an Entity (and its document rows) to a Model.
func Make(e Entity, docs []DocumentEntity) Model {
	ids := make([]string, 0, len(docs))
	for _, d := range docs {
		ids = append(ids, d.MediaID)
	}
	return Model{
		id:               e.ID,
		vehicleID:        e.VehicleID,
		categoryID:       e.CategoryID,
		description:      e.Description,
		performedAt:      e.PerformedAt,
		mileage:          e.Mileage,
		cost:             e.Cost,
		vendor:           e.Vendor,
		notes:            e.Notes,
		createdByUserID:  e.CreatedByUserID,
		createdAt:        e.CreatedAt,
		updatedAt:        e.UpdatedAt,
		deletedAt:        e.DeletedAt,
		documentMediaIDs: ids,
	}
}

// ToEntity converts a Model to an Entity for persistence (documents handled separately).
func (m Model) ToEntity() Entity {
	return Entity{
		ID:              m.id,
		VehicleID:       m.vehicleID,
		CategoryID:      m.categoryID,
		Description:     m.description,
		PerformedAt:     m.performedAt,
		Mileage:         m.mileage,
		Cost:            m.cost,
		Vendor:          m.vendor,
		Notes:           m.notes,
		CreatedByUserID: m.createdByUserID,
		DeletedAt:       m.deletedAt,
	}
}
