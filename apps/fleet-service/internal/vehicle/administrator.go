package vehicle

import (
	"time"

	"gorm.io/gorm"
)

// Administrator is the write interface for vehicle data access.
// All mutations (inserts, updates, soft-delete, restore, primary-image) go here.
type Administrator interface {
	Insert(Model) (Model, error)
	Update(Model) (Model, error)
	SoftDelete(id string) (Model, error)
	RestoreRow(id string) (Model, error)
	SetPrimaryImage(id, mediaID string) (Model, error)
	GetByIDIncludingDeleted(id string) (Model, error)
	UpdateCurrentMileage(vehicleID string, mileage int) error
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

// SoftDelete stamps deleted_at and purge_after on the vehicle.
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

// RestoreRow clears deleted_at and purge_after from a row unconditionally.
// The processor is responsible for calling IsPurgeable before invoking this.
func (a *dbAdministrator) RestoreRow(id string) (Model, error) {
	var e Entity
	if err := a.db.First(&e, "id = ?", id).Error; err != nil {
		return Model{}, err
	}
	if err := a.db.Model(&e).Updates(map[string]any{
		"deleted_at":  nil,
		"purge_after": nil,
	}).Error; err != nil {
		return Model{}, err
	}
	e.DeletedAt = nil
	e.PurgeAfter = nil
	return Make(e), nil
}

// SetPrimaryImage mirrors the chosen media_id into vehicles.primary_image_media_id.
func (a *dbAdministrator) SetPrimaryImage(id, mediaID string) (Model, error) {
	var e Entity
	if err := a.db.First(&e, "id = ? AND deleted_at IS NULL", id).Error; err != nil {
		return Model{}, err
	}
	if err := a.db.Model(&e).Update("primary_image_media_id", mediaID).Error; err != nil {
		return Model{}, err
	}
	e.PrimaryImageMediaID = mediaID
	return Make(e), nil
}

// GetByIDIncludingDeleted fetches a vehicle regardless of soft-delete status.
func (a *dbAdministrator) GetByIDIncludingDeleted(id string) (Model, error) {
	var e Entity
	if err := a.db.First(&e, "id = ?", id).Error; err != nil {
		return Model{}, err
	}
	return Make(e), nil
}

// UpdateCurrentMileage sets fleet.vehicles.current_mileage for the given vehicle.
// Called by the mileage domain's OnAppend hook to mirror the latest record.
func (a *dbAdministrator) UpdateCurrentMileage(vehicleID string, mileage int) error {
	return a.db.Model(&Entity{}).
		Where("id = ? AND deleted_at IS NULL", vehicleID).
		Update("current_mileage", mileage).Error
}
