package maintenancecategory

import (
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Entity maps to fleet.maintenance_categories (PRD §6, design §8.2).
type Entity struct {
	ID            string `gorm:"type:uuid;primaryKey"`
	// uniqueIndex (shared with Kind and FleetID below) is a case-SENSITIVE
	// backstop against a double-submit or client retry inserting the same
	// literal name twice (design §10.1 line 220-222); it does not replace the
	// case-insensitive LOWER() dedupe in FindByName, which is the real
	// user-facing match. PostgreSQL treats each NULL FleetID as distinct, so
	// this index does not constrain system rows — Seed's FirstOrCreate is
	// their guard. The explicit priority values (shared across all three
	// fields below) order the composite index's columns (fleet_id, name,
	// kind) to match design.md's specification — GORM otherwise orders
	// composite index columns by struct field declaration order, which here
	// would produce (name, kind, fleet_id) instead. Equivalent for
	// uniqueness either way, but only the design's order is useful for a
	// prefix scan.
	Name          string `gorm:"not null;uniqueIndex:idx_maintenance_categories_scope,priority:2"`
	Description   string
	SystemDefined bool `gorm:"not null;default:false"`
	// The DEFAULT is what classifies the eight pre-existing rows in the same
	// ALTER TABLE that adds the column — no backfill step (PRD FR-KIND-1).
	// The literal is quoted because GORM copies the tag value verbatim into the
	// DDL; unquoted, PostgreSQL reads `maintenance` as a column reference.
	Kind string `gorm:"type:varchar(20);not null;default:'maintenance';uniqueIndex:idx_maintenance_categories_scope,priority:3"`
	// NULL means a system/global category visible to every fleet. A non-NULL
	// value scopes the row to one fleet, so free-form names entered by one
	// household never appear in another's picker (design §10.1).
	FleetID *string `gorm:"type:uuid;index;uniqueIndex:idx_maintenance_categories_scope,priority:1"`
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

// ToEntity converts a Model to an Entity for persistence. It is the only
// place a Model turns back into a GORM Entity — entities otherwise never
// leave the provider/administrator layer.
func (m Model) ToEntity() Entity {
	return Entity{
		ID:            m.id,
		Name:          m.name,
		Description:   m.description,
		SystemDefined: m.systemDefined,
		Kind:          string(m.kind),
		FleetID:       m.fleetID,
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
//
// The lookup is constrained to fleet_id IS NULL (system rows) as well as
// name: now that fleets can create their own free-form categories (Task 2), a
// fleet-scoped row sharing a system name would otherwise satisfy a
// name-only match and make Seed skip creating the system row on a later
// startup.
func Seed(db *gorm.DB) error {
	for _, s := range seeds {
		var e Entity
		if err := db.Where("name = ? AND fleet_id IS NULL", s.Name).
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
