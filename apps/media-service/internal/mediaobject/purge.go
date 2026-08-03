package mediaobject

import (
	"time"

	"gorm.io/gorm"
)

const recoveryWindow = 5 * 24 * time.Hour

// ComputePurgeAfter returns the time after which a soft-deleted media object may
// be hard-deleted (deletedAt + 5 days), mirroring the vehicle recovery window.
func ComputePurgeAfter(deletedAt time.Time) time.Time { return deletedAt.Add(recoveryWindow) }

// IsPurgeable reports whether a soft-deleted media object has passed its window.
func IsPurgeable(purgeAfter *time.Time) bool {
	return purgeAfter != nil && time.Now().After(*purgeAfter)
}

// ListPurgeable returns soft-deleted objects whose purge window has elapsed.
// The purge job uses these to remove both the rows and the MinIO objects.
//
// purge_operation_id IS NULL keeps this sweep off rows an admin purge stamped.
// An admin stamp never writes purge_after, so such rows could not match anyway
// — this is the explicit belt to that structural brace (FR-ADMIN-RESTORE-7),
// and it is what makes the guarantee survive someone later "helpfully" setting
// purge_after in the admin path.
func ListPurgeable(db *gorm.DB) ([]Model, error) {
	var es []Entity
	if err := db.Where("purge_after IS NOT NULL AND purge_after < CURRENT_TIMESTAMP AND purge_operation_id IS NULL").
		Find(&es).Error; err != nil {
		return nil, err
	}
	out := make([]Model, 0, len(es))
	for _, e := range es {
		out = append(out, Make(e))
	}
	return out, nil
}

// DeleteRow hard-deletes a single media-object row by ID (purge job).
//
// db may be a transaction — *gorm.DB is what db.Transaction hands its callback,
// and the sweep passes the tx so the media row goes in the same transaction as
// the variant and ledger rows that describe it (FR-PURGE-4). Do not bind this to
// an outer handle.
func DeleteRow(db *gorm.DB, id string) error {
	return db.Exec("DELETE FROM media.media_objects WHERE id = ?", id).Error
}
