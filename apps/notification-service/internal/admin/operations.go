package admin

import (
	"fmt"
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
	out := make(map[string]int, len(Manifest))
	for _, t := range Manifest {
		res := tx.Exec("DELETE FROM "+t.Table+" WHERE purge_operation_id = ?", opID)
		if res.Error != nil {
			return nil, fmt.Errorf("reap %s: %w", t.Table, res.Error)
		}
		out[t.Key] = int(res.RowsAffected)
	}
	return out, nil
}
