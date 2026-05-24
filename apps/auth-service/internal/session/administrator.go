package session

import (
	"time"

	"gorm.io/gorm"
)

// Administrator is the write interface for refresh-token data access.
type Administrator interface {
	Insert(Model) (Model, error)
	// Consume marks a single token consumed (single-use rotation).
	Consume(id string, at time.Time) error
	// RevokeFamily revokes every unrevoked token in a family (reuse defense).
	RevokeFamily(familyID string, at time.Time) error
}

type dbAdministrator struct{ db *gorm.DB }

// NewAdministrator returns an Administrator backed by the given database.
func NewAdministrator(db *gorm.DB) Administrator { return &dbAdministrator{db: db} }

func (s *dbAdministrator) Insert(m Model) (Model, error) {
	e := m.ToEntity()
	if err := s.db.Create(&e).Error; err != nil {
		return Model{}, err
	}
	return Make(e), nil
}

func (s *dbAdministrator) Consume(id string, at time.Time) error {
	return s.db.Model(&Entity{}).
		Where("id = ?", id).
		Update("consumed_at", at).Error
}

func (s *dbAdministrator) RevokeFamily(familyID string, at time.Time) error {
	return s.db.Model(&Entity{}).
		Where("family_id = ? AND revoked_at IS NULL", familyID).
		Update("revoked_at", at).Error
}
