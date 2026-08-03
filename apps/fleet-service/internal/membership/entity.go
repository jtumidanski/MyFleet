package membership

import (
	"time"

	"gorm.io/gorm"
)

// Entity maps to fleet.fleet_memberships (PRD §6).
//
// The (fleet_id, user_id) uniqueness constraint is NOT expressed here. GORM can
// only emit a total unique index, and a total index over a soft-deletable table
// turns a purge into a permanent lockout (risks.md R2): the purged row keeps
// occupying the index and the user can never rejoin. The real constraint is the
// partial index in ApplyPartialIndexes below, predicated on deleted_at IS NULL.
// The tag here is a plain composite index under a DIFFERENT name so AutoMigrate
// and the hand-written DDL never fight over the same object.
type Entity struct {
	ID               string `gorm:"type:uuid;primaryKey"`
	FleetID          string `gorm:"type:uuid;not null;index:idx_membership_fleet_user"`
	UserID           string `gorm:"not null;index:idx_membership_fleet_user"`
	Role             string `gorm:"not null"` // owner | member | viewer
	Status           string `gorm:"not null"` // active
	CreatedAt        time.Time
	UpdatedAt        time.Time
	DeletedAt        *time.Time `gorm:"index"`
	PurgeOperationID *string    `gorm:"type:uuid;index"`
}

func (Entity) TableName() string { return "fleet.fleet_memberships" }

func Migration(db *gorm.DB) error {
	if err := db.AutoMigrate(&Entity{}); err != nil {
		return err
	}
	return ApplyPartialIndexes(db)
}

// ApplyPartialIndexes replaces the legacy TOTAL unique index on
// (fleet_id, user_id) with one predicated on deleted_at IS NULL.
//
// It is split out of Migration so it can be tested without AutoMigrate, which
// cannot create schema-qualified indexes on SQLite.
//
// The DROP names the LEGACY index only. AutoMigrate owns
// idx_membership_fleet_user and would recreate anything dropped from under it on
// the next boot; dropping only the name AutoMigrate no longer emits keeps both
// halves idempotent.
//
// CREATE INDEX is schema-qualified differently on the two dialects this runs
// against: Postgres puts the schema on the TABLE and never accepts a
// schema-qualified index name (it always creates the index in its table's
// schema); SQLite, addressing the "fleet" alias attached by the test harness,
// requires the schema on the INDEX name and resolves the table only via
// "main" if left unqualified. The DROP form (schema on the index) is valid on
// both, so only the CREATE branches.
func ApplyPartialIndexes(db *gorm.DB) error {
	create := `CREATE UNIQUE INDEX IF NOT EXISTS ux_membership_fleet_user
	 ON fleet.fleet_memberships (fleet_id, user_id) WHERE deleted_at IS NULL`
	if db.Name() == "sqlite" {
		create = `CREATE UNIQUE INDEX IF NOT EXISTS fleet.ux_membership_fleet_user
		 ON fleet_memberships (fleet_id, user_id) WHERE deleted_at IS NULL`
	}
	stmts := []string{
		`DROP INDEX IF EXISTS fleet.idx_fleet_user`,
		create,
	}
	for _, s := range stmts {
		if err := db.Exec(s).Error; err != nil {
			return err
		}
	}
	return nil
}

// Make converts an Entity to a Model.
func Make(e Entity) Model {
	return Model{
		id:      e.ID,
		fleetID: e.FleetID,
		userID:  e.UserID,
		role:    e.Role,
		status:  e.Status,
	}
}

// ToEntity converts a Model to an Entity for persistence.
func (m Model) ToEntity() Entity {
	return Entity{
		ID:      m.id,
		FleetID: m.fleetID,
		UserID:  m.userID,
		Role:    m.role,
		Status:  m.status,
	}
}
