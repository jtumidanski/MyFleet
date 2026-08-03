package platformadmin

import "gorm.io/gorm"

// Provider is the read-only interface for platform-admin lookups.
type Provider interface {
	// IsAdmin reports whether the user currently holds the privilege — a row
	// exists AND it has not been revoked. It reads the table on every call by
	// design: this is the check that bounds the stale-claim window on
	// irreversible operations (FR-ADMIN-AUTH-7), so a cache would defeat its
	// only purpose.
	IsAdmin(userID string) (bool, error)

	// IsRevoked reports whether the user carries an explicit, durable
	// revocation tombstone (revoked_at set). Both seeding hooks consult this
	// before granting, so a revoked admin stays revoked across restarts and
	// re-logins instead of being silently re-granted.
	IsRevoked(userID string) (bool, error)
}

type dbProvider struct{ db *gorm.DB }

// NewProvider returns a read-only Provider backed by the given database.
func NewProvider(db *gorm.DB) Provider { return &dbProvider{db: db} }

func (p *dbProvider) IsAdmin(userID string) (bool, error) {
	if userID == "" {
		return false, nil
	}
	var count int64
	if err := p.db.Model(&Entity{}).Where("user_id = ? AND revoked_at IS NULL", userID).Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

func (p *dbProvider) IsRevoked(userID string) (bool, error) {
	if userID == "" {
		return false, nil
	}
	var count int64
	if err := p.db.Model(&Entity{}).Where("user_id = ? AND revoked_at IS NOT NULL", userID).Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}
