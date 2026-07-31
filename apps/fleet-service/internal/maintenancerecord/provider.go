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
	// ListByVehicle returns a page of a vehicle's records, newest first.
	//
	// categoryIDs is a three-state filter (design D3):
	//   nil            → no filter
	//   empty non-nil  → match nothing
	//   populated      → category_id IN (…)
	//
	// The empty case is not a corner: `IN ()` is not valid SQL, and skipping
	// the clause when the slice is empty would silently degrade a filtered
	// request into an unfiltered one.
	ListByVehicle(vehicleID string, categoryIDs []string, page server.Page) ([]Model, int, error)
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

func (p *dbProvider) ListByVehicle(vehicleID string, categoryIDs []string, page server.Page) ([]Model, int, error) {
	if categoryIDs != nil && len(categoryIDs) == 0 {
		return []Model{}, 0, nil
	}

	count := p.db.Model(&Entity{}).Where("vehicle_id = ? AND deleted_at IS NULL", vehicleID)
	find := p.db.Model(&Entity{}).Where("vehicle_id = ? AND deleted_at IS NULL", vehicleID)
	if categoryIDs != nil {
		count = count.Where("category_id IN ?", categoryIDs)
		find = find.Where("category_id IN ?", categoryIDs)
	}

	var total int64
	if err := count.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var es []Entity
	if err := find.Order("performed_at desc").Offset(page.Offset()).Limit(page.Size).
		Find(&es).Error; err != nil {
		return nil, 0, err
	}

	// One query for the whole page's documents, grouped in memory (design D21).
	// This loop used to issue one query per record — 26 for a 25-record page.
	// It was harmless while no record had documents; this task makes
	// attachments the point, so it stops being harmless. Bounded by page size.
	ids := make([]string, 0, len(es))
	for _, e := range es {
		ids = append(ids, e.ID)
	}
	byRecord := make(map[string][]DocumentEntity, len(ids))
	if len(ids) > 0 {
		var docs []DocumentEntity
		if err := p.db.Where("maintenance_record_id IN ?", ids).Find(&docs).Error; err != nil {
			return nil, 0, err
		}
		for _, d := range docs {
			byRecord[d.MaintenanceRecordID] = append(byRecord[d.MaintenanceRecordID], d)
		}
	}

	out := make([]Model, 0, len(es))
	for _, e := range es {
		out = append(out, Make(e, byRecord[e.ID]))
	}
	return out, int(total), nil
}
