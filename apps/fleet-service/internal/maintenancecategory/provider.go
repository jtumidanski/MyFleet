package maintenancecategory

import (
	"gorm.io/gorm"

	"github.com/jtumidanski/myfleet/packages/shared-go/server"
)

// Provider is the read-only interface for maintenance category data access.
type Provider interface {
	// List returns a page of categories visible to fleetID: system rows plus
	// that fleet's own. An empty kind means no filter.
	List(kind Kind, fleetID string, page server.Page) ([]Model, int, error)
	// IDsByKind returns every visible category ID of a kind. It always returns
	// a non-nil slice, because the record provider reads nil as "no filter"
	// and empty-non-nil as "match nothing" (design D3).
	IDsByKind(kind Kind, fleetID string) ([]string, error)
}

type dbProvider struct{ db *gorm.DB }

// NewProvider returns a read-only Provider backed by the given database.
func NewProvider(db *gorm.DB) Provider { return &dbProvider{db: db} }

// visibleTo scopes a query to system rows plus one fleet's own.
func visibleTo(q *gorm.DB, fleetID string) *gorm.DB {
	return q.Where("fleet_id IS NULL OR fleet_id = ?", fleetID)
}

func (p *dbProvider) List(kind Kind, fleetID string, page server.Page) ([]Model, int, error) {
	// Two independent query builders: reusing one after Count() carries the
	// aggregate's state into the Find.
	count := visibleTo(p.db.Model(&Entity{}), fleetID)
	find := visibleTo(p.db.Model(&Entity{}), fleetID)
	if kind != "" {
		count = count.Where("kind = ?", string(kind))
		find = find.Where("kind = ?", string(kind))
	}

	var total int64
	if err := count.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var es []Entity
	if err := find.Order("name asc").Offset(page.Offset()).Limit(page.Size).
		Find(&es).Error; err != nil {
		return nil, 0, err
	}
	out := make([]Model, 0, len(es))
	for _, e := range es {
		out = append(out, Make(e))
	}
	return out, int(total), nil
}

func (p *dbProvider) IDsByKind(kind Kind, fleetID string) ([]string, error) {
	var ids []string
	if err := visibleTo(p.db.Model(&Entity{}), fleetID).
		Where("kind = ?", string(kind)).
		Pluck("id", &ids).Error; err != nil {
		return nil, err
	}
	if ids == nil {
		ids = []string{}
	}
	return ids, nil
}
