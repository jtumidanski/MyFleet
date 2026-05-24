package preferences

import (
	"errors"

	"gorm.io/gorm"
)

// ErrNotFound is returned when no preference row exists for a (user, type) pair.
var ErrNotFound = errors.New("preference not found")

// Provider is the read-only interface for preference data access.
type Provider interface {
	// GetByUserAndType returns the preference for a (user, type) pair or ErrNotFound.
	GetByUserAndType(userID, typ string) (Model, error)
	// ListByUser returns all of a user's preference rows.
	ListByUser(userID string) ([]Model, error)
}

type dbProvider struct{ db *gorm.DB }

// NewProvider returns a read-only Provider backed by the given database.
func NewProvider(db *gorm.DB) Provider { return &dbProvider{db: db} }

func (p *dbProvider) GetByUserAndType(userID, typ string) (Model, error) {
	var e Entity
	if err := p.db.Where("user_id = ? AND type = ?", userID, typ).First(&e).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return Model{}, ErrNotFound
		}
		return Model{}, err
	}
	return Make(e), nil
}

func (p *dbProvider) ListByUser(userID string) ([]Model, error) {
	var es []Entity
	if err := p.db.Where("user_id = ?", userID).Order("type asc").Find(&es).Error; err != nil {
		return nil, err
	}
	out := make([]Model, 0, len(es))
	for _, e := range es {
		out = append(out, Make(e))
	}
	return out, nil
}
