package activity

import (
	"errors"
	"time"

	"gorm.io/gorm"

	"github.com/jtumidanski/myfleet/packages/shared-go/server"
)

// Provider is the read-only interface for activity-event data access.
type Provider interface {
	// ListByFleet returns a page of events for a fleet, newest first.
	ListByFleet(fleetID string, page server.Page) ([]Model, int, error)
	// ListByVehicle returns a page of events for a vehicle, newest first.
	ListByVehicle(vehicleID string, page server.Page) ([]Model, int, error)
	// LastActivityByVehicle returns the most-recent event time for a vehicle, or
	// the zero time when the vehicle has no recorded activity.
	LastActivityByVehicle(vehicleID string) (time.Time, error)
}

type dbProvider struct{ db *gorm.DB }

// NewProvider returns a read-only Provider backed by the given database.
func NewProvider(db *gorm.DB) Provider { return &dbProvider{db: db} }

func (p *dbProvider) ListByFleet(fleetID string, page server.Page) ([]Model, int, error) {
	return p.list(p.db.Where("fleet_id = ?", fleetID), page)
}

func (p *dbProvider) ListByVehicle(vehicleID string, page server.Page) ([]Model, int, error) {
	return p.list(p.db.Where("vehicle_id = ?", vehicleID), page)
}

func (p *dbProvider) list(scope *gorm.DB, page server.Page) ([]Model, int, error) {
	var total int64
	if err := scope.Session(&gorm.Session{}).Model(&Entity{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var es []Entity
	if err := scope.Session(&gorm.Session{}).Model(&Entity{}).
		Order("created_at desc").Offset(page.Offset()).Limit(page.Size).Find(&es).Error; err != nil {
		return nil, 0, err
	}
	out := make([]Model, 0, len(es))
	for _, e := range es {
		out = append(out, Make(e))
	}
	return out, int(total), nil
}

func (p *dbProvider) LastActivityByVehicle(vehicleID string) (time.Time, error) {
	var e Entity
	err := p.db.Model(&Entity{}).
		Where("vehicle_id = ?", vehicleID).
		Order("created_at desc").
		Limit(1).
		First(&e).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return time.Time{}, nil
		}
		return time.Time{}, err
	}
	return e.CreatedAt, nil
}
