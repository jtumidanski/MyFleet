package vehiclemedia

import "gorm.io/gorm"

// Administrator is the write interface for vehicle media data access.
// SetPrimary must run in a transaction: unset all, then set one.
type Administrator interface {
	Insert(Model) (Model, error)
	SetIsPrimary(id string, isPrimary bool) error
	UpdateVehiclePrimaryImage(vehicleID, mediaID string) error
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

// SetIsPrimary updates is_primary for a single vehicle_media row.
func (a *dbAdministrator) SetIsPrimary(id string, isPrimary bool) error {
	return a.db.Model(&Entity{}).Where("id = ?", id).Update("is_primary", isPrimary).Error
}

// UpdateVehiclePrimaryImage mirrors the chosen media_id into fleet.vehicles.
func (a *dbAdministrator) UpdateVehiclePrimaryImage(vehicleID, mediaID string) error {
	return a.db.Exec(
		"UPDATE fleet.vehicles SET primary_image_media_id = ? WHERE id = ?",
		mediaID, vehicleID,
	).Error
}
