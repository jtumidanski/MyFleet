package mediavariant

import (
	"errors"

	"gorm.io/gorm"
)

// Provider is the read-only interface for media-variant data access.
type Provider interface {
	ListByMediaObject(mediaObjectID string) ([]Model, error)
	// GetByMediaObjectAndVariant returns the named variant, or found=false when
	// the worker has not produced it (or never will, for non-image media). A
	// miss is a normal outcome, not an error.
	GetByMediaObjectAndVariant(mediaObjectID string, v Variant) (Model, bool, error)
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

func (p *dbProvider) GetByMediaObjectAndVariant(mediaObjectID string, v Variant) (Model, bool, error) {
	var e Entity
	err := p.db.Where("media_object_id = ? AND variant = ?", mediaObjectID, string(v)).First(&e).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return Model{}, false, nil
	}
	if err != nil {
		return Model{}, false, err
	}
	return Make(e), true, nil
}
