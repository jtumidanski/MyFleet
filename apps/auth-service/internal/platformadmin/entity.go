// Package platformadmin owns auth.platform_admins — the runtime source of truth
// for who may use the platform-admin console.
//
// The bootstrap email list SEEDS this table and is never consulted per request
// (FR-ADMIN-AUTH-3). That distinction is the whole point: an admin can be
// revoked by deleting a row, with no redeploy and no config change.
package platformadmin

import (
	"time"

	"gorm.io/gorm"
)

// BootstrapGrantedBy is the granted_by value for rows the seed creates. A real
// grant records the granting user's id instead.
const BootstrapGrantedBy = "bootstrap"

// Entity maps to auth.platform_admins (PRD §6.1). user_id references
// auth.users.id logically; there are no foreign keys in this schema.
type Entity struct {
	UserID    string    `gorm:"type:uuid;primaryKey"`
	GrantedBy string    `gorm:"not null"`
	GrantedAt time.Time `gorm:"not null"`
}

func (Entity) TableName() string { return "auth.platform_admins" }

func Migration(db *gorm.DB) error { return db.AutoMigrate(&Entity{}) }
