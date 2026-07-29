package vehiclemedia

import "gorm.io/gorm"

// Administrator is the write interface for vehicle media data access.
type Administrator interface {
	Insert(Model) (Model, error)
	// SetPrimaryAtomic clears is_primary on clearIDs, sets is_primary=true on
	// targetID, and mirrors targetMediaID into fleet.vehicles — all in one
	// database transaction. A partial failure rolls back entirely.
	SetPrimaryAtomic(vehicleID, targetID, targetMediaID string, clearIDs []string) error
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

// SetPrimaryAtomic performs clear + set + mirror inside a single transaction.
func (a *dbAdministrator) SetPrimaryAtomic(vehicleID, targetID, targetMediaID string, clearIDs []string) error {
	return a.db.Transaction(func(tx *gorm.DB) error {
		// Clear is_primary on all previously-primary rows (other than target).
		if len(clearIDs) > 0 {
			if err := tx.Model(&Entity{}).
				Where("id IN ?", clearIDs).
				Update("is_primary", false).Error; err != nil {
				return err
			}
		}

		// Set is_primary on the target row.
		if err := tx.Model(&Entity{}).
			Where("id = ?", targetID).
			Update("is_primary", true).Error; err != nil {
			return err
		}

		// Mirror into fleet.vehicles.primary_image_media_id.
		return tx.Exec(
			"UPDATE fleet.vehicles SET primary_image_media_id = ? WHERE id = ?",
			targetMediaID, vehicleID,
		).Error
	})
}
