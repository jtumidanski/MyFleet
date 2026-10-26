package mileage

import (
	"time"

	"gorm.io/gorm"
)

// Administrator is the write interface for mileage data access.
// Append-only except RetractTx, which soft-deletes a parent's rows when the
// parent is deleted.
type Administrator interface {
	// AppendTx inserts the reading and advances the vehicle odometer within tx.
	AppendTx(tx *gorm.DB, m Model) (Model, error)
	// RetractTx soft-deletes the matching readings and recomputes the odometer within tx.
	RetractTx(tx *gorm.DB, vehicleID, source, sourceRefID string) error
}

type dbAdministrator struct{ db *gorm.DB }

// NewAdministrator returns an Administrator backed by the given database.
func NewAdministrator(db *gorm.DB) Administrator { return &dbAdministrator{db: db} }

func (a *dbAdministrator) AppendTx(tx *gorm.DB, m Model) (Model, error) {
	return appendTx(tx, m)
}

func (a *dbAdministrator) RetractTx(tx *gorm.DB, vehicleID, source, sourceRefID string) error {
	return retractTx(tx, vehicleID, source, sourceRefID)
}

// appendTx inserts the reading and advances the vehicle's current_mileage only
// when the reading is >= the current value. The returned model is flagged when
// the conditional update matched no row, i.e. the reading is lower than the
// current odometer (or the vehicle is soft-deleted). The reading is kept either way.
func appendTx(tx *gorm.DB, m Model) (Model, error) {
	e := m.ToEntity()
	if err := tx.Create(&e).Error; err != nil {
		return Model{}, err
	}
	res := tx.Table("fleet.vehicles").
		Where("id = ? AND deleted_at IS NULL AND current_mileage <= ?", e.VehicleID, e.Mileage).
		Update("current_mileage", e.Mileage)
	if res.Error != nil {
		return Model{}, res.Error
	}
	return Make(e).WithFlagged(res.RowsAffected == 0), nil
}

// retractTx soft-deletes live readings matching (vehicleID, source, sourceRefID)
// and, if any were deleted, recomputes the vehicle's current_mileage as the max
// of the remaining live readings. With no live readings left the odometer is
// unchanged.
//
// Concurrency: under Postgres READ COMMITTED, if a concurrent append holds the
// vehicle row lock when the recompute UPDATE starts, the UPDATE blocks; once the
// other transaction commits, Postgres re-checks the WHERE against the new row
// version but the SET subquery still runs on the statement's original snapshot,
// which lacks the concurrent reading, so the odometer could be lowered wrongly.
// Locking the vehicle row in a separate statement first makes the recompute
// start after any competing writer commits, with a fresh snapshot. SQLite
// serializes writers, so the lock is skipped there. The lock is taken only after
// something was retracted, so records without mileage never touch the vehicle row.
func retractTx(tx *gorm.DB, vehicleID, source, sourceRefID string) error {
	res := tx.Table("fleet.mileage_records").
		Where("vehicle_id = ? AND source = ? AND source_ref_id = ? AND deleted_at IS NULL", vehicleID, source, sourceRefID).
		Update("deleted_at", time.Now().UTC())
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return nil
	}
	if tx.Name() != "sqlite" {
		var id string
		if err := tx.Raw("SELECT id FROM fleet.vehicles WHERE id = ? FOR UPDATE", vehicleID).Scan(&id).Error; err != nil {
			return err
		}
	}
	return tx.Exec(`UPDATE fleet.vehicles
		SET current_mileage = (SELECT MAX(mileage) FROM fleet.mileage_records
			WHERE vehicle_id = ? AND deleted_at IS NULL)
		WHERE id = ? AND deleted_at IS NULL
		AND EXISTS (SELECT 1 FROM fleet.mileage_records WHERE vehicle_id = ? AND deleted_at IS NULL)`,
		vehicleID, vehicleID, vehicleID).Error
}
