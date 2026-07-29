package maintenancecategory

import (
	"gorm.io/gorm"

	"github.com/jtumidanski/myfleet/packages/shared-go/server"
)

// Provider is the read-only interface for maintenance category data access.
type Provider interface {
	List(page server.Page) ([]Model, int, error)
}

type dbProvider struct{ db *gorm.DB }

// NewProvider returns a read-only Provider backed by the given database.
func NewProvider(db *gorm.DB) Provider { return &dbProvider{db: db} }

func (p *dbProvider) List(page server.Page) ([]Model, int, error) {
	var total int64
	if err := p.db.Model(&Entity{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var es []Entity
	if err := p.db.Order("name asc").Offset(page.Offset()).Limit(page.Size).Find(&es).Error; err != nil {
		return nil, 0, err
	}
	out := make([]Model, 0, len(es))
	for _, e := range es {
		out = append(out, Make(e))
	}
	return out, int(total), nil
}
