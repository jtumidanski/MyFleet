package platformadmin

import "gorm.io/gorm"

// Provider is the read-only interface for platform-admin lookups.
type Provider interface {
	// IsAdmin reports whether the user currently holds the privilege. It reads
	// the table on every call by design: this is the check that bounds the
	// stale-claim window on irreversible operations (FR-ADMIN-AUTH-7), so a
	// cache would defeat its only purpose.
	IsAdmin(userID string) (bool, error)
}

type dbProvider struct{ db *gorm.DB }

// NewProvider returns a read-only Provider backed by the given database.
func NewProvider(db *gorm.DB) Provider { return &dbProvider{db: db} }

func (p *dbProvider) IsAdmin(userID string) (bool, error) {
	if userID == "" {
		return false, nil
	}
	var count int64
	if err := p.db.Model(&Entity{}).Where("user_id = ?", userID).Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}
