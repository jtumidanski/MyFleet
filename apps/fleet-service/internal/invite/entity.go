package invite

import (
	"time"

	"gorm.io/gorm"
)

// Entity maps to fleet.fleet_invites (PRD §6).
type Entity struct {
	ID              string     `gorm:"type:uuid;primaryKey"`
	FleetID         string     `gorm:"type:uuid;not null;index"`
	Email           string     `gorm:"not null"`
	Role            string     `gorm:"not null"`
	Token           string     `gorm:"not null;uniqueIndex"`
	ExpiresAt       time.Time  `gorm:"not null"`
	AcceptedAt      *time.Time
	InvitedByUserID string    `gorm:"not null"`
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

func (Entity) TableName() string { return "fleet.fleet_invites" }

func Migration(db *gorm.DB) error { return db.AutoMigrate(&Entity{}) }

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
	}
}
