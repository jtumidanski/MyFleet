package mediaobject

import (
	"time"

	"gorm.io/gorm"
)

// Administrator is the write interface for media-object data access. All
// mutations (insert, status updates, soft-delete) go here.
type Administrator interface {
	Insert(Model) (Model, error)
	Update(Model) (Model, error)
	SoftDelete(id string) (Model, error)
}

type dbAdministrator struct{ db *gorm.DB }

// NewAdministrator returns an Administrator backed by the given database.
func NewAdministrator(db *gorm.DB) Administrator { return &dbAdministrator{db: db} }

func (a *dbAdministrator) Insert(m Model) (Model, error) {
	e := m.ToEntity()
	if err := a.db.Create(&e).Error; err != nil {
		return Model{}, err
	}
	return Make(e), nil
}

func (a *dbAdministrator) Update(m Model) (Model, error) {
	e := m.ToEntity()
	if err := a.db.Save(&e).Error; err != nil {
		return Model{}, err
	}
	return Make(e), nil
}

// SoftDelete stamps deleted_at and purge_after on the media object.
func (a *dbAdministrator) SoftDelete(id string) (Model, error) {
	var e Entity
	if err := a.db.First(&e, "id = ? AND deleted_at IS NULL", id).Error; err != nil {
		return Model{}, err
	}
	now := time.Now().UTC()
	purge := ComputePurgeAfter(now)
	if err := a.db.Model(&e).Updates(map[string]any{
		"deleted_at":  now,
		"purge_after": purge,
	}).Error; err != nil {
		return Model{}, err
	}
	e.DeletedAt = &now
	e.PurgeAfter = &purge
	return Make(e), nil
}
