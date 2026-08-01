package mediavariant

import (
	"context"
	"errors"

	"gorm.io/gorm"

	"github.com/jtumidanski/myfleet/packages/shared-go/database"
)

// ErrNotFound is the package-local not-found error. A miss is a normal outcome,
// not a fault: variants do not exist until the processing worker has run, and
// never exist for non-image media. The composition-root adapter translates it
// into mediaobject.VariantLookup's found=false so mediaobject never has to know
// this package's sentinel.
var ErrNotFound = errors.New("media variant not found")

// Provider is the read-only interface for media-variant data access.
type Provider interface {
	ListByMediaObject(mediaObjectID string) ([]Model, error)
	// GetByMediaObjectAndVariant returns the named variant, or ErrNotFound when
	// the worker has not produced it (or never will, for non-image media).
	//
	// ctx is the caller's request context: the read is part of serving an HTTP
	// response, so a client that disconnects must cancel the query rather than
	// leaving it running against a bare connection.
	GetByMediaObjectAndVariant(ctx context.Context, mediaObjectID string, v Variant) (Model, error)
}

type dbProvider struct{ db *gorm.DB }

// NewProvider returns a read-only Provider backed by the given database.
func NewProvider(db *gorm.DB) Provider { return &dbProvider{db: db} }

func (p *dbProvider) ListByMediaObject(mediaObjectID string) ([]Model, error) {
	return database.SliceQuery(func() ([]Model, error) {
		var es []Entity
		if err := p.db.Where("media_object_id = ?", mediaObjectID).Find(&es).Error; err != nil {
			return nil, err
		}
		out := make([]Model, 0, len(es))
		for _, e := range es {
			out = append(out, Make(e))
		}
		return out, nil
	})()
}

func (p *dbProvider) GetByMediaObjectAndVariant(ctx context.Context, mediaObjectID string, v Variant) (Model, error) {
	return database.Query(func() (Model, error) {
		var e Entity
		err := p.db.WithContext(ctx).
			Where("media_object_id = ? AND variant = ?", mediaObjectID, string(v)).
			First(&e).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return Model{}, ErrNotFound
		}
		if err != nil {
			return Model{}, err
		}
		return Make(e), nil
	})()
}
