package mileage

import (
	"time"

	"gorm.io/gorm"

	"github.com/jtumidanski/myfleet/packages/shared-go/server"
)

// Provider is the read-only interface for mileage data access.
type Provider interface {
	// ListByVehicle returns a page of a vehicle's mileage records, newest first
	// (recorded_at desc), matching the fuel and maintenance-record siblings.
	ListByVehicle(vehicleID string, from, to *time.Time, page server.Page) ([]Model, int, error)
}

type dbProvider struct{ db *gorm.DB }

// NewProvider returns a read-only Provider backed by the given database.
func NewProvider(db *gorm.DB) Provider { return &dbProvider{db: db} }

func (p *dbProvider) ListByVehicle(vehicleID string, from, to *time.Time, page server.Page) ([]Model, int, error) {
	q := p.db.Model(&Entity{}).Where("vehicle_id = ? AND deleted_at IS NULL", vehicleID)
	if from != nil {
		q = q.Where("recorded_at >= ?", *from)
	}
	if to != nil {
		q = q.Where("recorded_at <= ?", *to)
	}

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var es []Entity
	if err := q.Order("recorded_at desc").Offset(page.Offset()).Limit(page.Size).Find(&es).Error; err != nil {
		return nil, 0, err
	}

	out := make([]Model, 0, len(es))
	for _, e := range es {
		out = append(out, Make(e))
	}
	return out, int(total), nil
}
