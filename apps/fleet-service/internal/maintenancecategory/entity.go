package maintenancecategory

import (
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Entity maps to fleet.maintenance_categories (PRD §6, design §8.2).
type Entity struct {
	ID            string `gorm:"type:uuid;primaryKey"`
	Name          string `gorm:"not null"`
	Description   string
	SystemDefined bool `gorm:"not null;default:false"`
}

func (Entity) TableName() string { return "fleet.maintenance_categories" }

func Migration(db *gorm.DB) error { return db.AutoMigrate(&Entity{}) }

// Make converts an Entity to a Model.
func Make(e Entity) Model {
	return Model{
		id:            e.ID,
		name:          e.Name,
		description:   e.Description,
		systemDefined: e.SystemDefined,
	}
}

// seeds is the canonical list of system-defined maintenance categories
// (FR-MAINT-1). Seeding is keyed by Name so it is idempotent.
var seeds = []Entity{
	{Name: "Oil Change", SystemDefined: true},
	{Name: "Tire Rotation", SystemDefined: true},
	{Name: "Brake Service", SystemDefined: true},
	{Name: "Air Filter", SystemDefined: true},
	{Name: "Transmission Service", SystemDefined: true},
	{Name: "Coolant Flush", SystemDefined: true},
	{Name: "Battery", SystemDefined: true},
	{Name: "Inspection", SystemDefined: true},
}

// Seed inserts the system-defined categories. It is idempotent: FirstOrCreate
// keyed by name means re-running does not create duplicates (plan Task 9.1).
func Seed(db *gorm.DB) error {
	for _, s := range seeds {
		var e Entity
		if err := db.Where("name = ?", s.Name).
			Attrs(Entity{
				ID:            uuid.NewString(),
				Name:          s.Name,
				Description:   s.Description,
				SystemDefined: s.SystemDefined,
			}).
			FirstOrCreate(&e).Error; err != nil {
			return err
		}
	}
	return nil
}
