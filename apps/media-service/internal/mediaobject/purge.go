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
func ListPurgeable(db *gorm.DB) ([]Model, error) {
	var es []Entity
	if err := db.Where("purge_after IS NOT NULL AND purge_after < now()").Find(&es).Error; err != nil {
		return nil, err
	}
	out := make([]Model, 0, len(es))
	for _, e := range es {
		out = append(out, Make(e))
	}
	return out, nil
}

// DeleteRow hard-deletes a single media-object row by ID (purge job).
func DeleteRow(db *gorm.DB, id string) error {
	return db.Exec("DELETE FROM media.media_objects WHERE id = ?", id).Error
}
