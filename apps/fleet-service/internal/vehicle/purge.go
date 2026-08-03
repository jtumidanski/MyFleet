package vehicle

import (
	"time"

	"gorm.io/gorm"
)

const recoveryWindow = 5 * 24 * time.Hour

// ComputePurgeAfter returns the time after which a soft-deleted vehicle may be
// hard-deleted (deletedAt + 5 days).
func ComputePurgeAfter(deletedAt time.Time) time.Time { return deletedAt.Add(recoveryWindow) }

// IsPurgeable reports whether a soft-deleted vehicle has passed its recovery window.
func IsPurgeable(purgeAfter *time.Time) bool {
	return purgeAfter != nil && time.Now().After(*purgeAfter)
}

// PurgeExpired hard-deletes vehicles past purge_after together with every row
// beneath them. Run under database.WithLeaderLock for multi-replica safety.
//
// It used to be a bare DELETE over fleet.vehicles. There are no foreign keys
// anywhere in this schema, so nothing cascaded and every mileage record, fuel
// log, maintenance record, schedule and vehicle-media row belonging to a purged
// vehicle was left referencing a vehicle_id that no longer existed — invisible
// to the product, because every read is vehicle-scoped, and accumulating
// forever (PRD §11).
//
// deleteChildren is injected so this package does not import the admin manifest.
// The composition root passes admin.DeleteVehicleChildren.
//
// purge_operation_id IS NULL keeps this sweep off rows an admin purge stamped:
// those belong to a cancellable operation and the admin reaper owns their
// lifecycle (FR-ADMIN-RESTORE-7).
func PurgeExpired(db *gorm.DB, deleteChildren func(tx *gorm.DB, vehicleIDs []string) error) error {
	return db.Transaction(func(tx *gorm.DB) error {
		var ids []string
		if err := tx.Raw(`SELECT id FROM fleet.vehicles
		                  WHERE purge_after IS NOT NULL AND purge_after < ?
		                    AND purge_operation_id IS NULL`, time.Now().UTC()).
			Scan(&ids).Error; err != nil {
			return err
		}
		if len(ids) == 0 {
			return nil
		}
		if err := deleteChildren(tx, ids); err != nil {
			return err
		}
		return tx.Exec("DELETE FROM fleet.vehicles WHERE id IN ?", ids).Error
	})
}
