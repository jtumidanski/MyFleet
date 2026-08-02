package invite

import (
	"errors"

	"gorm.io/gorm"
)

var ErrNotFound = errors.New("invite not found")

// Provider is the read-only interface for invite data access.
type Provider interface {
	GetByID(id string) (Model, error)
	GetByToken(token string) (Model, error)
	ListByFleetID(fleetID string) ([]Model, error)
}

type dbProvider struct{ db *gorm.DB }

// NewProvider returns a read-only Provider backed by the given database.
func NewProvider(db *gorm.DB) Provider { return &dbProvider{db: db} }

func (p *dbProvider) GetByID(id string) (Model, error) {
	var e Entity
	if err := p.db.First(&e, "id = ? AND deleted_at IS NULL", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return Model{}, ErrNotFound
		}
		return Model{}, err
	}
	return Make(e), nil
}

func (p *dbProvider) GetByToken(token string) (Model, error) {
	var e Entity
	if err := p.db.First(&e, "token = ? AND deleted_at IS NULL", token).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return Model{}, ErrNotFound
		}
		return Model{}, err
	}
	return Make(e), nil
}

func (p *dbProvider) ListByFleetID(fleetID string) ([]Model, error) {
	var es []Entity
	if err := p.db.Where("fleet_id = ? AND deleted_at IS NULL", fleetID).Find(&es).Error; err != nil {
		return nil, err
	}
	out := make([]Model, 0, len(es))
	for _, e := range es {
		out = append(out, Make(e))
	}
	return out, nil
}
