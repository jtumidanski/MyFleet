package vehicle

import (
	"errors"

	"gorm.io/gorm"

	"github.com/jtumidanski/myfleet/packages/shared-go/server"
)

var ErrNotFound = errors.New("vehicle not found")

// Provider is the read-only interface for vehicle data access.
type Provider interface {
	GetByID(id string) (Model, error)
	ListByFleet(fleetID string, page server.Page) ([]Model, int, error)
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

func (p *dbProvider) ListByFleet(fleetID string, page server.Page) ([]Model, int, error) {
	var total int64
	if err := p.db.Model(&Entity{}).Where("fleet_id = ? AND deleted_at IS NULL", fleetID).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var es []Entity
	if err := p.db.Where("fleet_id = ? AND deleted_at IS NULL", fleetID).
		Offset(page.Offset()).Limit(page.Size).Find(&es).Error; err != nil {
		return nil, 0, err
	}
	out := make([]Model, 0, len(es))
	for _, e := range es {
		out = append(out, Make(e))
	}
	return out, int(total), nil
}
