package user

import (
	"errors"

	"gorm.io/gorm"

	"github.com/jtumidanski/myfleet/packages/shared-go/database"
)

// Provider is the read-only interface for user data access.
//
// The two lookups take different identifiers and are not interchangeable:
//   - GetByID takes our internal user id (the users primary key), which is what
//     the JWT `sub` claim carries (see session.Processor).
//   - GetBySub takes Google's subject identifier, which only the OAuth callback
//     has.
//
// "sub" is overloaded — OIDC's subject vs. the JWT subject claim — and passing
// one where the other is expected silently returns ErrNotFound rather than
// failing loudly. That mistake logged every user straight back out: /auth/me
// looked its user id up in google_sub, always missed, and 404'd.
type Provider interface {
	GetByID(id string) (Model, error)
	GetBySub(sub string) (Model, error)
}

type dbProvider struct{ db *gorm.DB }

// NewProvider returns a read-only Provider backed by the given database.
func NewProvider(db *gorm.DB) Provider { return &dbProvider{db: db} }

// GetByID looks a user up by primary key — NOT by google_sub.
func (s *dbProvider) GetByID(id string) (Model, error) {
	return database.Query(func() (Model, error) {
		var e Entity
		if err := s.db.Where("id = ?", id).First(&e).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return Model{}, ErrNotFound
			}
			return Model{}, err
		}
		return Make(e), nil
	})()
}

// GetBySub looks a user up by Google's subject identifier — NOT by our user id.
func (s *dbProvider) GetBySub(sub string) (Model, error) {
	return database.Query(func() (Model, error) {
		var e Entity
		if err := s.db.Where("google_sub = ?", sub).First(&e).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return Model{}, ErrNotFound
			}
			return Model{}, err
		}
		return Make(e), nil
	})()
}
