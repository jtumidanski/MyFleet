package membership

import (
	"errors"

	"gorm.io/gorm"
)

var ErrNotFound = errors.New("membership not found")

// Provider is the read-only interface for membership data access.
type Provider interface {
	// GetActiveByUserID returns the single active membership for a user, or ErrNotFound.
	GetActiveByUserID(userID string) (Model, error)
	// ListByFleetID returns all memberships in a fleet.
	ListByFleetID(fleetID string) ([]Model, error)
	// GetByFleetAndUser returns the membership for a specific user in a fleet.
	GetByFleetAndUser(fleetID, userID string) (Model, error)
	// CountOwners returns the number of owners in a fleet.
	CountOwners(fleetID string) (int, error)
}

type dbProvider struct{ db *gorm.DB }

// NewProvider returns a read-only Provider backed by the given database.
func NewProvider(db *gorm.DB) Provider { return &dbProvider{db: db} }

func (p *dbProvider) GetActiveByUserID(userID string) (Model, error) {
	var e Entity
	if err := p.db.Where("user_id = ? AND status = ?", userID, "active").First(&e).Error; err != nil {
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

func (p *dbProvider) GetByFleetAndUser(fleetID, userID string) (Model, error) {
	var e Entity
	if err := p.db.Where("fleet_id = ? AND user_id = ?", fleetID, userID).First(&e).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return Model{}, ErrNotFound
		}
		return Model{}, err
	}
	return Make(e), nil
}

func (p *dbProvider) CountOwners(fleetID string) (int, error) {
	var count int64
	if err := p.db.Model(&Entity{}).Where("fleet_id = ? AND role = ?", fleetID, "owner").Count(&count).Error; err != nil {
		return 0, err
	}
	return int(count), nil
}
