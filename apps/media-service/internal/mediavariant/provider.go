package mediavariant

import "gorm.io/gorm"

// Provider is the read-only interface for media-variant data access.
type Provider interface {
	ListByMediaObject(mediaObjectID string) ([]Model, error)
}

type dbProvider struct{ db *gorm.DB }

// NewProvider returns a read-only Provider backed by the given database.
func NewProvider(db *gorm.DB) Provider { return &dbProvider{db: db} }

func (p *dbProvider) ListByMediaObject(mediaObjectID string) ([]Model, error) {
	var es []Entity
	if err := p.db.Where("media_object_id = ?", mediaObjectID).Find(&es).Error; err != nil {
		return nil, err
	}
	out := make([]Model, 0, len(es))
	for _, e := range es {
		out = append(out, Make(e))
	}
	return out, nil
}
