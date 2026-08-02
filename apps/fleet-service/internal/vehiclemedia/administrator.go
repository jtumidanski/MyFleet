package vehiclemedia

import (
	"errors"

	"gorm.io/gorm"
)

// Administrator is the write interface for vehicle media data access.
type Administrator interface {
	Insert(Model) (Model, error)
	// SetPrimaryAtomic clears is_primary on clearIDs, sets is_primary=true on
	// targetID, and mirrors targetMediaID into fleet.vehicles — all in one
	// database transaction. A partial failure rolls back entirely.
	SetPrimaryAtomic(vehicleID, targetID, targetMediaID string, clearIDs []string) error
	// SoftDelete stamps deleted_at on one row and, when that row was the
	// primary, promotes the next surviving photo (or clears the mirror when
	// none remains) in the same database transaction.
	SoftDelete(vehicleID, id string, wasPrimary bool) error
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

// SoftDelete removes one media reference from a vehicle.
//
// The row is stamped rather than dropped so the gallery's read path
// (`deleted_at IS NULL`) stops returning it while the underlying media object,
// which media-service owns and purges on its own schedule, is not orphaned by
// a hard delete here.
//
// The primary promotion is part of the same transaction on purpose: leaving
// fleet.vehicles.primary_image_media_id pointing at a removed reference is what
// makes a vehicle card render a photo the gallery no longer lists. The
// successor is the oldest surviving row by sort_order, matching the order the
// gallery itself displays.
func (a *dbAdministrator) SoftDelete(vehicleID, id string, wasPrimary bool) error {
	return a.db.Transaction(func(tx *gorm.DB) error {
		res := tx.Model(&Entity{}).
			Where("id = ? AND deleted_at IS NULL", id).
			Update("deleted_at", gorm.Expr("NOW()"))
		if res.Error != nil {
			return res.Error
		}
		// Zero rows means a concurrent request already removed it. Promoting a
		// successor off the back of a delete that did not happen would move the
		// primary photo for no reason.
		if res.RowsAffected == 0 {
			return ErrNotFound
		}
		if !wasPrimary {
			return nil
		}

		var successor Entity
		err := tx.Where("vehicle_id = ? AND deleted_at IS NULL", vehicleID).
			Order("sort_order ASC, created_at ASC").
			First(&successor).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// Last photo removed: the vehicle falls back to the placeholder.
			return tx.Exec(
				"UPDATE fleet.vehicles SET primary_image_media_id = NULL WHERE id = ?",
				vehicleID,
			).Error
		}

		if err := tx.Model(&Entity{}).
			Where("id = ?", successor.ID).
			Update("is_primary", true).Error; err != nil {
			return err
		}
		return tx.Exec(
			"UPDATE fleet.vehicles SET primary_image_media_id = ? WHERE id = ?",
			successor.MediaID, vehicleID,
		).Error
	})
}
