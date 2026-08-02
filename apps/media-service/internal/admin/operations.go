package admin

import (
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
)

// Count returns per-key LIVE row counts for the root.
func Count(db *gorm.DB, root Root) (map[string]int, error) {
	out := make(map[string]int, len(Manifest))
	for _, t := range Manifest {
		pred, args := t.Where(root)
		if pred == "" {
			continue
		}
		var n int64
		q := "SELECT count(*) FROM " + t.Table + " WHERE (" + pred + ") AND deleted_at IS NULL"
		if err := db.Raw(q, args...).Scan(&n).Error; err != nil {
			return nil, fmt.Errorf("count %s: %w", t.Table, err)
		}
		out[t.Key] = int(n)
	}
	return out, nil
}

// CountByOperation returns per-key rows currently carrying opID.
func CountByOperation(db *gorm.DB, opID string) (map[string]int, error) {
	out := make(map[string]int, len(Manifest))
	for _, t := range Manifest {
		var n int64
		if err := db.Raw("SELECT count(*) FROM "+t.Table+" WHERE purge_operation_id = ?", opID).
			Scan(&n).Error; err != nil {
			return nil, fmt.Errorf("count by operation %s: %w", t.Table, err)
		}
		out[t.Key] = int(n)
	}
	return out, nil
}

// Stamp soft-deletes the root's rows under opID and returns the counts the
// operation now carries.
//
// It never writes purge_after. media-service runs its OWN 24-hour sweep keyed on
// that column, which hard-deletes rows and their MinIO objects; leaving it NULL
// is what keeps an admin-stamped, still-cancellable object out of that sweep's
// reach (design F3).
//
// The counts are read back after the update rather than summed from rows
// affected, so a replay returns the same numbers instead of zeros
// (FR-ADMIN-PURGE-10).
func Stamp(tx *gorm.DB, root Root, opID string, now time.Time) (map[string]int, error) {
	for _, t := range Manifest {
		pred, args := t.Where(root)
		if pred == "" {
			continue
		}
		q := "UPDATE " + t.Table + " SET deleted_at = ?, purge_operation_id = ?" +
			" WHERE (" + pred + ") AND deleted_at IS NULL"
		if err := tx.Exec(q, append([]any{now, opID}, args...)...).Error; err != nil {
			return nil, fmt.Errorf("stamp %s: %w", t.Table, err)
		}
	}
	return CountByOperation(tx, opID)
}

// Restore clears the soft-delete on every row carrying opID.
func Restore(tx *gorm.DB, opID string) error {
	for _, t := range Manifest {
		q := "UPDATE " + t.Table +
			" SET deleted_at = NULL, purge_operation_id = NULL WHERE purge_operation_id = ?"
		if err := tx.Exec(q, opID).Error; err != nil {
			return fmt.Errorf("restore %s: %w", t.Table, err)
		}
	}
	return nil
}

// Reap hard-deletes every row carrying opID.
func Reap(tx *gorm.DB, opID string) (map[string]int, error) {
	return ReapSparing(tx, opID, nil)
}

// ReapSparing hard-deletes every row carrying opID except those belonging to the
// named media objects, which keep BOTH their soft-delete and their
// purge_operation_id.
//
// Keeping the id is the whole point. An earlier version spared a row by NULLing
// purge_operation_id, which detached it from the only handle anything has on
// it: the next reap keys on that column, restore keys on that column, and the
// purge_after sweep never sees it because an admin stamp deliberately leaves
// purge_after NULL. The row and its bytes were stranded permanently. A row we
// could not finish with must stay attached to the operation that owns it.
func ReapSparing(tx *gorm.DB, opID string, spareMediaObjectIDs []string) (map[string]int, error) {
	out := make(map[string]int, len(Manifest))
	for _, t := range Manifest {
		q := "DELETE FROM " + t.Table + " WHERE purge_operation_id = ?"
		args := []any{opID}
		if len(spareMediaObjectIDs) > 0 {
			// media_objects are spared by their own id; their variants by the
			// object they belong to, so a spared object never loses the
			// variants that describe it.
			switch t.Table {
			case "media.media_objects":
				q += " AND id NOT IN ?"
			case "media.media_variants":
				q += " AND media_object_id NOT IN ?"
			}
			if strings.HasSuffix(q, "NOT IN ?") {
				args = append(args, spareMediaObjectIDs)
			}
		}
		res := tx.Exec(q, args...)
		if res.Error != nil {
			return nil, fmt.Errorf("reap %s: %w", t.Table, res.Error)
		}
		out[t.Key] = int(res.RowsAffected)
	}
	return out, nil
}

// ObjectKey pairs a stored object with the row that owns it, so a failed removal
// can spare exactly that row.
type ObjectKey struct {
	MediaObjectID string
	Key           string
}

// ReapableObjectKeys returns every MinIO key belonging to opID — the media
// objects' own keys and their variants' — grouped by owning media object.
//
// It must be called BEFORE Reap: the rows are the only record of which objects
// exist, so deleting them first would strand the bytes in the bucket with
// nothing left pointing at them.
func ReapableObjectKeys(db *gorm.DB, opID string) ([]ObjectKey, error) {
	var out []ObjectKey
	q := `SELECT id AS media_object_id, object_key AS key
	      FROM media.media_objects WHERE purge_operation_id = ?
	      UNION ALL
	      SELECT media_object_id AS media_object_id, object_key AS key
	      FROM media.media_variants WHERE purge_operation_id = ?`
	if err := db.Raw(q, opID, opID).Scan(&out).Error; err != nil {
		return nil, fmt.Errorf("list reapable object keys: %w", err)
	}
	return out, nil
}
