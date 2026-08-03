package admin

import (
	"fmt"

	"gorm.io/gorm"
)

// DeleteVehicleChildren hard-deletes every row beneath the given vehicles. It
// does NOT delete the vehicles themselves — the caller owns that, because the
// caller is what decided they should go.
//
// It is injected into vehicle.PurgeExpired rather than called from it, so the
// vehicle package does not import this one and the dependency arrow keeps
// pointing one way.
//
// Documents go first and explicitly: they hang off maintenance_records, not off
// vehicles, so deleting the records first would strand them exactly the way the
// original defect stranded everything else.
func DeleteVehicleChildren(tx *gorm.DB, vehicleIDs []string) error {
	if len(vehicleIDs) == 0 {
		return nil
	}
	docs := `DELETE FROM fleet.maintenance_record_documents
	         WHERE maintenance_record_id IN (
	             SELECT id FROM fleet.maintenance_records WHERE vehicle_id IN ?)`
	if err := tx.Exec(docs, vehicleIDs).Error; err != nil {
		return fmt.Errorf("delete maintenance_record_documents for vehicles: %w", err)
	}
	for _, t := range Manifest {
		if t.Orphan == nil || t.Orphan.ParentTable != "fleet.vehicles" {
			continue
		}
		q := "DELETE FROM " + t.Table + " WHERE " + t.Orphan.Column + " IN ?"
		if err := tx.Exec(q, vehicleIDs).Error; err != nil {
			return fmt.Errorf("delete %s for vehicles: %w", t.Table, err)
		}
	}
	return nil
}

// DeleteOrphans hard-deletes rows whose parent no longer exists, using each
// manifest target's OrphanRule. It returns per-key counts removed and is a no-op
// on a clean database.
//
// This is the cleanup for rows the pre-cascade vehicle sweep already stranded
// (PRD §11b). It runs at every startup under an advisory lock and logs what it
// removed; on a healthy database — which is every boot after the first — it
// removes nothing and says so, so it stays a cheap no-op guard rather than a
// one-shot migration that only the first deploy needed.
//
// The DELETE target carries no alias: `DELETE FROM t AS c` is Postgres-only and
// SQLite (the whole test harness) rejects it, so the correlated sub-query
// qualifies the column with the full table name instead.
//
// Manifest is written child-to-parent, but orphan chains can run two deep
// (maintenance_record_documents -> maintenance_records -> vehicles): on the
// first pass, a maintenance_record row that only exists because its own vehicle
// is gone has not been removed yet when maintenance_record_documents is
// checked, so its document looks (wrongly) like it still has a live parent. A
// single walk of Manifest would leave that document behind. Looping to a fixed
// point — re-walking Manifest until a full pass removes nothing — resolves any
// depth of chain in one call regardless of manifest order, which matters
// because a caller running this at startup needs it to leave a clean database
// in one call, not a partially-cleaned one that requires a second deploy to
// finish.
func DeleteOrphans(db *gorm.DB) (map[string]int, error) {
	out := make(map[string]int, len(Manifest))
	// Bounded by len(Manifest): no orphan chain can be deeper than the number of
	// purgeable tables, so this can never loop forever even on unexpected data.
	for pass := 0; pass < len(Manifest); pass++ {
		progressed := false
		for _, t := range Manifest {
			if t.Orphan == nil {
				continue
			}
			q := "DELETE FROM " + t.Table + " WHERE NOT EXISTS (" +
				"SELECT 1 FROM " + t.Orphan.ParentTable + " p WHERE p.id = " +
				t.Table + "." + t.Orphan.Column + ")"
			res := db.Exec(q)
			if res.Error != nil {
				return nil, fmt.Errorf("delete orphans in %s: %w", t.Table, res.Error)
			}
			if n := int(res.RowsAffected); n > 0 {
				out[t.Key] += n
				progressed = true
			}
		}
		if !progressed {
			break
		}
	}
	return out, nil
}
