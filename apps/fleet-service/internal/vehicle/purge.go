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

// PurgeExpired hard-deletes rows past purge_after. Run under database.WithLeaderLock
// for multi-replica safety (design A9).
func PurgeExpired(db *gorm.DB) error {
	return db.Exec("DELETE FROM fleet.vehicles WHERE purge_after IS NOT NULL AND purge_after < now()").Error
}
