package mediavariant

import "gorm.io/gorm"

// Orphan is a variant row whose media object no longer exists, paired with the
// bytes it is the only remaining record of.
type Orphan struct {
	ID        string
	ObjectKey string
}

// ObjectKeysForMediaObject returns every stored key for a media object,
// including rows a purge has soft-deleted.
//
// There is deliberately no deleted_at predicate. A purge hard-deletes the media
// object, so a soft-deleted variant of it is not recoverable state — it is state
// whose bytes would leak if this query skipped it (FR-PURGE-1). It also admits
// one odd row: a variant carrying a purge_operation_id whose parent object does
// not, the residue of a spared or partially-reaped admin operation. Deleting it
// is still correct, because the object it describes is being hard-deleted in the
// same transaction and the row could only become an orphan.
//
// Keys come from the object_key column, never recomputed from
// storage.ObjectKey(fleetID, mediaObjectID, kind+ext): the column is the record
// of what was actually written, and a recomputed key that disagreed with a
// historical naming scheme would silently skip the bytes (FR-PURGE-2).
func ObjectKeysForMediaObject(db *gorm.DB, mediaObjectID string) ([]string, error) {
	var keys []string
	err := db.Raw(`SELECT object_key FROM media.media_variants WHERE media_object_id = ?`,
		mediaObjectID).Scan(&keys).Error
	if err != nil {
		return nil, err
	}
	return keys, nil
}

// DeleteForMediaObject hard-deletes every variant row for a media object.
//
// db may be a transaction — pass the tx so the variant rows, the ledger rows and
// the media-object row all go in one statement group (FR-PURGE-4).
func DeleteForMediaObject(db *gorm.DB, mediaObjectID string) error {
	return db.Exec(`DELETE FROM media.media_variants WHERE media_object_id = ?`, mediaObjectID).Error
}

// ListOrphaned returns up to limit variant rows whose media object no longer
// exists, with the object key each one is the last record of.
//
// The test is the ABSENCE of the parent row, never deleted_at (FR-RECON-2). A
// variant belonging to a soft-deleted-but-recoverable media object is not an
// orphan, and deleting it would silently break restore — both the user-facing
// five-day window and the admin console's cancel.
//
// The DELETE target in the sibling query carries no alias for portability
// (`DELETE FROM t AS c` is Postgres-only), but a SELECT alias is fine on both.
func ListOrphaned(db *gorm.DB, limit int) ([]Orphan, error) {
	var out []Orphan
	q := `SELECT id, object_key FROM media.media_variants v
	      WHERE NOT EXISTS (SELECT 1 FROM media.media_objects o WHERE o.id = v.media_object_id)
	      LIMIT ?`
	if err := db.Raw(q, limit).Scan(&out).Error; err != nil {
		return nil, err
	}
	return out, nil
}

// DeleteByID hard-deletes one variant row, used after its bytes are gone.
//
// One row at a time, deliberately: reconciliation removes each orphan's bytes
// individually, and a batched delete would either lose the ability to spare
// exactly the row whose removal failed or need the failed set threaded back
// through, for no benefit at the per-tick cap.
func DeleteByID(db *gorm.DB, id string) error {
	return db.Exec(`DELETE FROM media.media_variants WHERE id = ?`, id).Error
}
