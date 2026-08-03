package invite

import (
	"time"

	"gorm.io/gorm"
)

// Entity maps to fleet.fleet_invites (PRD §6).
type Entity struct {
	ID      string `gorm:"type:uuid;primaryKey"`
	FleetID string `gorm:"type:uuid;not null;index"`
	Email   string `gorm:"not null"`
	Role    string `gorm:"not null"`
	// Token's uniqueness moves to a partial index for the same reason as
	// fleet_memberships: a soft-deleted invite must not reserve its token
	// forever. The tag is a plain index under a name AutoMigrate owns.
	Token            string    `gorm:"not null;index:idx_invite_token"`
	ExpiresAt        time.Time `gorm:"not null"`
	AcceptedAt       *time.Time
	InvitedByUserID  string `gorm:"not null"`
	CreatedAt        time.Time
	UpdatedAt        time.Time
	DeletedAt        *time.Time `gorm:"index"`
	PurgeOperationID *string    `gorm:"type:uuid;index"`
}

func (Entity) TableName() string { return "fleet.fleet_invites" }

func Migration(db *gorm.DB) error {
	if err := db.AutoMigrate(&Entity{}); err != nil {
		return err
	}
	return ApplyPartialIndexes(db)
}

// ApplyPartialIndexes replaces the legacy total unique index on token with one
// predicated on deleted_at IS NULL. See membership.ApplyPartialIndexes for why
// the dropped name and the AutoMigrate-managed name must differ, and why the
// CREATE branches by dialect.
func ApplyPartialIndexes(db *gorm.DB) error {
	create := `CREATE UNIQUE INDEX IF NOT EXISTS ux_invite_token
	 ON fleet.fleet_invites (token) WHERE deleted_at IS NULL`
	if db.Name() == "sqlite" {
		create = `CREATE UNIQUE INDEX IF NOT EXISTS fleet.ux_invite_token
		 ON fleet_invites (token) WHERE deleted_at IS NULL`
	}
	stmts := []string{
		`DROP INDEX IF EXISTS fleet.idx_fleet_fleet_invites_token`,
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
		id:              e.ID,
		fleetID:         e.FleetID,
		email:           e.Email,
		role:            e.Role,
		token:           e.Token,
		expiresAt:       e.ExpiresAt,
		acceptedAt:      e.AcceptedAt,
		invitedByUserID: e.InvitedByUserID,
		updatedAt:       e.UpdatedAt,
	}
}

// ToEntity converts a Model to an Entity for persistence.
func (m Model) ToEntity() Entity {
	return Entity{
		ID:              m.id,
		FleetID:         m.fleetID,
		Email:           m.email,
		Role:            m.role,
		Token:           m.token,
		ExpiresAt:       m.expiresAt,
		AcceptedAt:      m.acceptedAt,
		InvitedByUserID: m.invitedByUserID,
		UpdatedAt:       m.updatedAt,
	}
}
