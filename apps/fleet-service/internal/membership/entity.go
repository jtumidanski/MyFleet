package membership

import (
	"time"

	"gorm.io/gorm"
)

// Entity maps to fleet.fleet_memberships (PRD §6).
type Entity struct {
	ID        string    `gorm:"type:uuid;primaryKey"`
	FleetID   string    `gorm:"type:uuid;not null;uniqueIndex:idx_fleet_user"`
	UserID    string    `gorm:"not null;uniqueIndex:idx_fleet_user"`
	Role      string    `gorm:"not null"` // owner | member | viewer
	Status    string    `gorm:"not null"` // active
	CreatedAt time.Time
	UpdatedAt time.Time
}

func (Entity) TableName() string { return "fleet.fleet_memberships" }

func Migration(db *gorm.DB) error { return db.AutoMigrate(&Entity{}) }

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
