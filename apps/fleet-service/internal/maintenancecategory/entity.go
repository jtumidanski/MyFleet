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
	// The DEFAULT is what classifies the eight pre-existing rows in the same
	// ALTER TABLE that adds the column — no backfill step (PRD FR-KIND-1).
	// The literal is quoted because GORM copies the tag value verbatim into the
	// DDL; unquoted, PostgreSQL reads `maintenance` as a column reference.
	Kind string `gorm:"type:varchar(20);not null;default:'maintenance'"`
	// NULL means a system/global category visible to every fleet. A non-NULL
	// value scopes the row to one fleet, so free-form names entered by one
	// household never appear in another's picker (design §10.1).
	FleetID *string `gorm:"type:uuid;index"`
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
		kind:          Kind(e.Kind),
		fleetID:       e.FleetID,
	}
}

// seeds is the canonical list of system-defined categories (FR-MAINT-1,
// FR-KIND-2). Seeding is keyed by Name so it is idempotent; no modification
// name collides with a maintenance one.
var seeds = []Entity{
	{Name: "Oil Change", SystemDefined: true, Kind: string(KindMaintenance)},
	{Name: "Tire Rotation", SystemDefined: true, Kind: string(KindMaintenance)},
	{Name: "Brake Service", SystemDefined: true, Kind: string(KindMaintenance)},
	{Name: "Air Filter", SystemDefined: true, Kind: string(KindMaintenance)},
	{Name: "Transmission Service", SystemDefined: true, Kind: string(KindMaintenance)},
	{Name: "Coolant Flush", SystemDefined: true, Kind: string(KindMaintenance)},
	{Name: "Battery", SystemDefined: true, Kind: string(KindMaintenance)},
	{Name: "Inspection", SystemDefined: true, Kind: string(KindMaintenance)},

	{Name: "Performance / Tune", SystemDefined: true, Kind: string(KindModification)},
	{Name: "Suspension", SystemDefined: true, Kind: string(KindModification)},
	{Name: "Wheels & Tires", SystemDefined: true, Kind: string(KindModification)},
	{Name: "Exhaust", SystemDefined: true, Kind: string(KindModification)},
	{Name: "Intake", SystemDefined: true, Kind: string(KindModification)},
	{Name: "Brake Upgrade", SystemDefined: true, Kind: string(KindModification)},
	{Name: "Exterior / Body", SystemDefined: true, Kind: string(KindModification)},
	{Name: "Interior", SystemDefined: true, Kind: string(KindModification)},
	{Name: "Audio & Electronics", SystemDefined: true, Kind: string(KindModification)},
	{Name: "Lighting", SystemDefined: true, Kind: string(KindModification)},
	{Name: "Towing", SystemDefined: true, Kind: string(KindModification)},
	{Name: "Other Modification", SystemDefined: true, Kind: string(KindModification)},
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
				Kind:          s.Kind,
			}).
			FirstOrCreate(&e).Error; err != nil {
			return err
		}
	}
	return nil
}
