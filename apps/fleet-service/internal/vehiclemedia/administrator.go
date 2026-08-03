package vehiclemedia

import (
	"errors"
	"time"

	"gorm.io/gorm"
)

// Administrator is the write interface for vehicle media data access.
type Administrator interface {
	Insert(Model) (Model, error)
	// SetPrimaryAtomic clears is_primary on clearIDs, sets is_primary=true on
	// targetID, and mirrors targetMediaID into fleet.vehicles — all in one
	// database transaction. A partial failure rolls back entirely.
	SetPrimaryAtomic(vehicleID, targetID, targetMediaID string, clearIDs []string) error
	// SoftDelete stamps deleted_at on one vehicle's media reference and, when
	// the vehicle's primary pointed at it, promotes the next surviving photo
	// (or clears the mirror when none remains) in the same database
	// transaction. Scoped by (vehicleID, mediaID) so it cannot reach another
	// vehicle's row. Returns ErrNotFound when there is no live match.
	SoftDelete(vehicleID, mediaID string) error
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
//
// The mirror step writes fleet.vehicles directly rather than going through
// vehicle.Administrator.SetPrimaryImage. That is the documented raw-query
// exception for a circular dependency: the vehicle domain already imports
// vehiclemedia (vehicle/resource.go delegates primary-image handling to this
// processor), so taking the vehicle administrator as a dependency here would
// close an import cycle. The write must also share THIS transaction — a
// cross-domain call could not be rolled back with the is_primary flips — which
// is the reason a cross-domain read-only exception is not sufficient.
// SoftDelete below writes the same column for the same reason.
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
// Everything, INCLUDING the decision about whether a successor is needed,
// happens inside one transaction and is derived from stored state. An earlier
// version took a `wasPrimary` flag read by the caller before the transaction
// opened; a concurrent SetPrimary between that read and this write could then
// either leave fleet.vehicles.primary_image_media_id pointing at a row this
// call just removed, or promote a second photo on top of one SetPrimary had
// just chosen. The mirror column is the single source of truth instead.
//
// Portability note: deleted_at is stamped with a Go timestamp, not SQL NOW().
// Every DB-backed test in this service runs on SQLite, which has no NOW(), and
// the rest of the codebase already writes UTC from Go.
func (a *dbAdministrator) SoftDelete(vehicleID, mediaID string) error {
	return a.db.Transaction(func(tx *gorm.DB) error {
		// is_primary is cleared alongside deleted_at so a dead row can never be
		// mistaken for the primary by any query that forgets the deleted_at
		// predicate.
		res := tx.Model(&Entity{}).
			Where("vehicle_id = ? AND media_id = ? AND deleted_at IS NULL", vehicleID, mediaID).
			Updates(map[string]any{"deleted_at": time.Now().UTC(), "is_primary": false})
		if res.Error != nil {
			return res.Error
		}
		// Zero rows means no live reference for this (vehicle, media) pair —
		// either it never existed, it belongs to another vehicle, or a
		// concurrent request already removed it. All three are 404s, and none
		// of them should touch the vehicle's primary.
		if res.RowsAffected == 0 {
			return ErrNotFound
		}

		// Only the vehicle whose primary actually pointed at the removed photo
		// needs a successor. Reading it here, after the delete and inside the
		// transaction, is what makes that decision race-free.
		var currentPrimary *string
		if err := tx.Raw(
			"SELECT primary_image_media_id FROM fleet.vehicles WHERE id = ?", vehicleID,
		).Scan(&currentPrimary).Error; err != nil {
			return err
		}
		if currentPrimary == nil || *currentPrimary != mediaID {
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
