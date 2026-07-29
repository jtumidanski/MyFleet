package maintenancerecord

import (
	"errors"

	"gorm.io/gorm"

	"github.com/jtumidanski/myfleet/packages/shared-go/server"
)

var ErrNotFound = errors.New("maintenance record not found")

// Provider is the read-only interface for maintenance record data access.
type Provider interface {
	GetByID(id string) (Model, error)
	ListByVehicle(vehicleID string, page server.Page) ([]Model, int, error)
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
	var docs []DocumentEntity
	if err := p.db.Where("maintenance_record_id = ?", e.ID).Find(&docs).Error; err != nil {
		return Model{}, err
	}
	return Make(e, docs), nil
}

func (p *dbProvider) ListByVehicle(vehicleID string, page server.Page) ([]Model, int, error) {
	q := p.db.Model(&Entity{}).Where("vehicle_id = ? AND deleted_at IS NULL", vehicleID)

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var es []Entity
	if err := p.db.Where("vehicle_id = ? AND deleted_at IS NULL", vehicleID).
		Order("performed_at desc").Offset(page.Offset()).Limit(page.Size).Find(&es).Error; err != nil {
		return nil, 0, err
	}

	out := make([]Model, 0, len(es))
	for _, e := range es {
		var docs []DocumentEntity
		if err := p.db.Where("maintenance_record_id = ?", e.ID).Find(&docs).Error; err != nil {
			return nil, 0, err
		}
		out = append(out, Make(e, docs))
	}
	return out, int(total), nil
}
