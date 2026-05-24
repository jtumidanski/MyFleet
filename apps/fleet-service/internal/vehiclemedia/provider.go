package vehiclemedia

import (
	"errors"

	"gorm.io/gorm"
)

var ErrNotFound = errors.New("vehicle media not found")

// Provider is the read-only interface for vehicle media data access.
type Provider interface {
	ListByVehicle(vehicleID string) ([]Model, error)
	GetByVehicleAndMedia(vehicleID, mediaID string) (Model, error)
}

type dbProvider struct{ db *gorm.DB }

// NewProvider returns a read-only Provider backed by the given database.
func NewProvider(db *gorm.DB) Provider { return &dbProvider{db: db} }

func (p *dbProvider) ListByVehicle(vehicleID string) ([]Model, error) {
	var es []Entity
	if err := p.db.Where("vehicle_id = ? AND deleted_at IS NULL", vehicleID).
		Order("sort_order ASC").Find(&es).Error; err != nil {
		return nil, err
	}
	out := make([]Model, 0, len(es))
	for _, e := range es {
		out = append(out, Make(e))
	}
	return out, nil
}

func (p *dbProvider) GetByVehicleAndMedia(vehicleID, mediaID string) (Model, error) {
	var e Entity
	if err := p.db.Where("vehicle_id = ? AND media_id = ? AND deleted_at IS NULL", vehicleID, mediaID).
		First(&e).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return Model{}, ErrNotFound
		}
		return Model{}, err
	}
	return Make(e), nil
}
