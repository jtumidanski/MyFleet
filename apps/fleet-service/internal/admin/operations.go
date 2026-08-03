package admin

import (
	"fmt"
	"time"

	"gorm.io/gorm"
)

// Count returns, per manifest key, how many LIVE rows a purge at this root
// would take. It is the blast-radius panel's only source (FR-ADMIN-UI-9).
//
// Count and Stamp share the same Where, which is what makes the displayed
// figures and the affected rows provably equal rather than equal by discipline.
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

// Stamp soft-deletes every row the root resolves to, marking it with opID, and
// returns the per-table counts now carried by the operation.
//
// It writes deleted_at and purge_operation_id and NOTHING ELSE. In particular it
// never writes purge_after: both legacy sweeps (vehicle.PurgeExpired and
// media-service's ListPurgeable) key on that column, so leaving it NULL is what
// makes an admin-stamped row invisible to them (design F3).
//
// now is a parameter rather than SQL now() because the entire test harness is
// SQLite, which has no now(). The single production call site passes
// time.Now().UTC().
//
// The counts are read back AFTER the update, keyed on purge_operation_id rather
// than on rows affected. That is what makes a replay return the SAME numbers as
// the first call instead of zeros: the UPDATE guards on deleted_at IS NULL, so a
// replay touches nothing, but affected_counts must still be correct
// (FR-ADMIN-PURGE-10).
func Stamp(tx *gorm.DB, root Root, opID string, now time.Time) (map[string]int, error) {
	for _, t := range Manifest {
		pred, args := t.Where(root)
		if pred == "" {
			continue
		}
		q := "UPDATE " + t.Table +
			" SET deleted_at = ?, purge_operation_id = ?" +
			" WHERE (" + pred + ") AND deleted_at IS NULL"
		full := append([]any{now, opID}, args...)
		if err := tx.Exec(q, full...).Error; err != nil {
			return nil, fmt.Errorf("stamp %s: %w", t.Table, err)
		}
	}
	return CountByOperation(tx, opID)
}

// CountByOperation returns the per-table rows currently carrying opID.
func CountByOperation(db *gorm.DB, opID string) (map[string]int, error) {
	out := make(map[string]int, len(Manifest))
	for _, t := range Manifest {
		var n int64
		q := "SELECT count(*) FROM " + t.Table + " WHERE purge_operation_id = ?"
		if err := db.Raw(q, opID).Scan(&n).Error; err != nil {
			return nil, fmt.Errorf("count by operation %s: %w", t.Table, err)
		}
		out[t.Key] = int(n)
	}
	return out, nil
}

// Restore clears the soft-delete on every row carrying opID.
//
// It does not use Where at all. Keying purely on purge_operation_id is what
// makes restore scope-independent, order-independent, idempotent — and, most
// importantly, incapable of resurrecting a row a user deleted before the purge:
// such a row has a NULL purge_operation_id and no operation's restore can match
// it (FR-ADMIN-RESTORE-3).
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

// Reap hard-deletes every row carrying opID and returns per-table counts
// removed. Like Restore it keys only on the operation id, so it is idempotent:
// a second call deletes nothing and reports zeros.
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

// StampReversedForTest runs Stamp's updates in the exact reverse of the
// manifest's order. It exists so TestStamp_isOrderIndependent can prove design
// §3.4's rule holds rather than assuming it; production code must call Stamp.
func StampReversedForTest(tx *gorm.DB, root Root, opID string, now time.Time) error {
	for i := len(Manifest) - 1; i >= 0; i-- {
		t := Manifest[i]
		pred, args := t.Where(root)
		if pred == "" {
			continue
		}
		q := "UPDATE " + t.Table +
			" SET deleted_at = ?, purge_operation_id = ?" +
			" WHERE (" + pred + ") AND deleted_at IS NULL"
		if err := tx.Exec(q, append([]any{now, opID}, args...)...).Error; err != nil {
			return fmt.Errorf("stamp %s: %w", t.Table, err)
		}
	}
	return nil
}
