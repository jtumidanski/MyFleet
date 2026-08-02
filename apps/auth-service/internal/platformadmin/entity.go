// Package platformadmin owns auth.platform_admins — the runtime source of truth
// for who may use the platform-admin console.
//
// The bootstrap email list SEEDS this table and is never consulted per request
// (FR-ADMIN-AUTH-3). That distinction is the whole point: an admin can be
// revoked — durably, across restarts and re-logins — by setting revoked_at on
// their row.
//
// Revocation is deliberately NOT a DELETE. A deleted row leaves nothing behind
// for the next startup seed or provisioning login to see, so both would
// re-read the bootstrap list, find no row keyed on that user, and grant the
// privilege right back. revoked_at is a tombstone, not a soft delete: the row
// stays present and visible to every query so both seeding hooks can check it
// and refuse to recreate the grant.
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
	// RevokedAt is the durable-revocation tombstone (see package doc). It is
	// deliberately a plain nullable timestamp, NOT gorm.DeletedAt: this must
	// never make GORM auto-filter queries by it. Every read that needs to tell
	// an active admin apart from a revoked one does so explicitly — IsAdmin
	// and IsRevoked below — so a bare Model/Find call never silently hides a
	// revoked row from code that isn't expecting that.
	RevokedAt *time.Time
}

func (Entity) TableName() string { return "auth.platform_admins" }

func Migration(db *gorm.DB) error { return db.AutoMigrate(&Entity{}) }
