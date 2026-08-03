package variantfailures

import "gorm.io/gorm"

// DeleteForMediaObject hard-deletes the ledger rows for a media object.
//
// db may be a transaction — the purge passes the tx so the ledger rows, the
// variant rows and the media-object row all go together (FR-PURGE-4).
func DeleteForMediaObject(db *gorm.DB, mediaObjectID string) error {
	return db.Exec(`DELETE FROM media.media_variant_failures WHERE media_object_id = ?`,
		mediaObjectID).Error
}

// DeleteOrphaned removes ledger rows whose media object no longer exists and
// returns how many rows went. The ledger carries no object_key, so there are no
// bytes to reclaim first (FR-RECON-4) and this is a single statement.
//
// limit bounds MEDIA OBJECTS, not rows (design A-2). Grouping by media object
// keeps an object's ledger atomic rather than half-reconciled, and still bounds
// the transaction on a first deployment against a large accumulated backlog.
//
// The DELETE target carries no alias: `DELETE FROM t AS f` is Postgres-only and
// SQLite — the whole test harness — rejects it, so the sub-query does the
// aliasing instead. This is the portability trap fleet-service's
// admin/orphans.go already documents.
func DeleteOrphaned(db *gorm.DB, limit int) (int, error) {
	q := `DELETE FROM media.media_variant_failures
	      WHERE media_object_id IN (
	          SELECT media_object_id FROM media.media_variant_failures f
	           WHERE NOT EXISTS (SELECT 1 FROM media.media_objects o WHERE o.id = f.media_object_id)
	           GROUP BY media_object_id
	           LIMIT ?)`
	res := db.Exec(q, limit)
	if res.Error != nil {
		return 0, res.Error
	}
	return int(res.RowsAffected), nil
}
