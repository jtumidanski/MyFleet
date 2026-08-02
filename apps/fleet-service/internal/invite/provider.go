package invite

import (
	"errors"
	"time"

	"gorm.io/gorm"
)

var ErrNotFound = errors.New("invite not found")

// Provider is the read-only interface for invite data access.
type Provider interface {
	GetByID(id string) (Model, error)
	GetByToken(token string) (Model, error)
	ListByFleetID(fleetID string) ([]Model, error)
	// CountByFleetSince backs the per-fleet creation rate limit. It is a query,
	// not an in-process counter, because fleet-service runs multiple replicas
	// and a per-pod limiter enforces nothing (FR-RATE-1).
	CountByFleetSince(fleetID string, since time.Time) (int64, error)
}

type dbProvider struct{ db *gorm.DB }

// NewProvider returns a read-only Provider backed by the given database.
func NewProvider(db *gorm.DB) Provider { return &dbProvider{db: db} }

func (p *dbProvider) GetByID(id string) (Model, error) {
	var e Entity
	if err := p.db.First(&e, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return Model{}, ErrNotFound
		}
		return Model{}, err
	}
	return Make(e), nil
}

func (p *dbProvider) GetByToken(token string) (Model, error) {
	var e Entity
	if err := p.db.First(&e, "token = ?", token).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return Model{}, ErrNotFound
		}
		return Model{}, err
	}
	return Make(e), nil
}

func (p *dbProvider) ListByFleetID(fleetID string) ([]Model, error) {
	var es []Entity
	if err := p.db.Where("fleet_id = ?", fleetID).Find(&es).Error; err != nil {
		return nil, err
	}
	out := make([]Model, 0, len(es))
	for _, e := range es {
		out = append(out, Make(e))
	}
	return out, nil
}

func (p *dbProvider) CountByFleetSince(fleetID string, since time.Time) (int64, error) {
	var n int64
	err := p.db.Model(&Entity{}).
		Where("fleet_id = ? AND created_at > ?", fleetID, since).
		Count(&n).Error
	return n, err
}
