package mediaobject

import (
	"errors"

	"gorm.io/gorm"
)

// ErrNotFound is the package-local not-found error; the processor maps it to
// server.ErrNotFound / server.ErrGone as appropriate.
var ErrNotFound = errors.New("media object not found")

// Provider is the read-only interface for media-object data access.
type Provider interface {
	GetByID(id string) (Model, error)
	GetByIDIncludingDeleted(id string) (Model, error)
	// ListActiveByFleetAndIDs returns the subset of ids that are active (not
	// soft-deleted) AND belong to fleetID. fleetID is a filter, never a trusted
	// assertion: the result set is never widened on the caller's say-so.
	ListActiveByFleetAndIDs(fleetID string, ids []string) ([]Model, error)
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
	return Make(e), nil
}

func (p *dbProvider) GetByIDIncludingDeleted(id string) (Model, error) {
	var e Entity
	if err := p.db.First(&e, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return Model{}, ErrNotFound
		}
		return Model{}, err
	}
	return Make(e), nil
}

func (p *dbProvider) ListActiveByFleetAndIDs(fleetID string, ids []string) ([]Model, error) {
	if len(ids) == 0 {
		return []Model{}, nil
	}
	var es []Entity
	if err := p.db.Where("fleet_id = ? AND deleted_at IS NULL AND id IN ?", fleetID, ids).
		Find(&es).Error; err != nil {
		return nil, err
	}
	out := make([]Model, 0, len(es))
	for _, e := range es {
		out = append(out, Make(e))
	}
	return out, nil
}
