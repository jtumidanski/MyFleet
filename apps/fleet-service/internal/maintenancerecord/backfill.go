package maintenancerecord

import (
	"time"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
	"gorm.io/gorm"

	"github.com/jtumidanski/myfleet/apps/fleet-service/internal/mileage"
)

type backfillCandidate struct {
	ID              string
	VehicleID       string
	Mileage         int
	PerformedAt     time.Time
	CreatedByUserID string
}

// BackfillMileage gives every live maintenance record that has a reading and
// no mileage history row one (source=maintenance), then advances each
// vehicle's odometer to the highest reading inserted, never regressing it.
// It is idempotent: a second run selects nothing.
func BackfillMileage(db *gorm.DB, log logrus.FieldLogger) error {
	return db.Transaction(func(tx *gorm.DB) error {
		var candidates []backfillCandidate
		// The NOT EXISTS deliberately ignores m.deleted_at: a record whose
		// entry was retracted must never be re-inserted. The CAST is needed
		// because source_ref_id is text but maintenance_records.id is uuid in
		// Postgres, which has no text = uuid operator.
		if err := tx.Raw(`
			SELECT r.id, r.vehicle_id, r.mileage, r.performed_at, r.created_by_user_id
			FROM fleet.maintenance_records r
			JOIN fleet.vehicles v ON v.id = r.vehicle_id AND v.deleted_at IS NULL
			WHERE r.deleted_at IS NULL AND r.mileage > 0
			  AND NOT EXISTS (SELECT 1 FROM fleet.mileage_records m
			                  WHERE m.source = ? AND m.source_ref_id = CAST(r.id AS TEXT))`,
			mileage.SourceMaintenance).Scan(&candidates).Error; err != nil {
			return err
		}

		rows := make([]mileage.Entity, 0, len(candidates))
		maxByVehicle := make(map[string]int)
		now := time.Now().UTC()
		for _, c := range candidates {
			rows = append(rows, mileage.Entity{
				ID:              uuid.NewString(),
				VehicleID:       c.VehicleID,
				Mileage:         c.Mileage,
				RecordedAt:      c.PerformedAt,
				Source:          mileage.SourceMaintenance,
				SourceRefID:     c.ID,
				CreatedByUserID: c.CreatedByUserID,
				CreatedAt:       now,
			})
			if c.Mileage > maxByVehicle[c.VehicleID] {
				maxByVehicle[c.VehicleID] = c.Mileage
			}
		}
		if len(rows) > 0 {
			if err := tx.CreateInBatches(rows, 500).Error; err != nil {
				return err
			}
		}

		var advanced int64
		for vehicleID, max := range maxByVehicle {
			res := tx.Exec(`UPDATE fleet.vehicles SET current_mileage = ?
				WHERE id = ? AND deleted_at IS NULL AND current_mileage < ?`, max, vehicleID, max)
			if res.Error != nil {
				return res.Error
			}
			advanced += res.RowsAffected
		}

		log.WithFields(logrus.Fields{
			"rows_inserted":     len(rows),
			"vehicles_advanced": advanced,
		}).Info("backfilled maintenance mileage")
		return nil
	})
}
