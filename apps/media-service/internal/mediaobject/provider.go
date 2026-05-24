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
