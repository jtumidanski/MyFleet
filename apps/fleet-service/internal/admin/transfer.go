package admin

import (
	"fmt"
	"time"

	"gorm.io/gorm"
)

// TransferSpec is the resolved, validated input to a vehicle transfer — the
// analogue of Root for the purge path.
//
// Now is a parameter rather than SQL now() because the entire test harness is
// SQLite, which has no now(). The production call site passes time.Now().UTC().
type TransferSpec struct {
	VehicleID     string
	SourceFleetID string
	DestFleetID   string
	// Label is the confirmation phrase (FR-XFER-CONF-2): the vehicle's
	// nickname, or "{year} {make} {model}" when it has none.
	Label       string
	ActorUserID string
	Now         time.Time
}

// TransferStep is one table the transfer counts and, sometimes, rewrites.
//
// Where and Set are used by BOTH CountTransfer and ApplyTransfer, which is what
// makes the blast-radius preview and the rows actually touched provably equal
// rather than equal by discipline — the same property admin.Count/Stamp have.
type TransferStep struct {
	// Key is the name used in affected_counts JSON and in the console's
	// blast-radius panel. It is API surface: renaming one is breaking.
	Key   string
	Table string
	// Where selects this table's rows for the moved vehicle. It binds exactly
	// one argument: the vehicle id.
	Where string
	// Set is the assignment ApplyTransfer writes, binding exactly one
	// argument: the destination fleet id.
	//
	// EMPTY MEANS COUNT-ONLY. Those tables key on vehicle_id alone, so they
	// follow the car for free and the transfer must not rewrite them
	// (FR-XFER-MOVE-2). The absence is the enforcement, and
	// TestTransferPlan_countOnlyStepsHaveNoSetClause pins it.
	Set string
}

// TransferPlan is the hand-enumerated list of tables a vehicle transfer
// touches, mirroring Manifest's role for a purge.
//
// The full fleet-service table inventory, with the transfer question answered
// for each (design §2.1):
//
//	fleet.vehicles                       rewritten explicitly, not a step (see ApplyTransfer)
//	fleet.activity_events                rewritten where vehicle_id matches
//	fleet.maintenance_categories         find-or-create in the destination (ResolveCategories)
//	fleet.dashboard_widgets              source-fleet rows pinned to the vehicle are DELETED (PruneWidgets)
//	fleet.maintenance_records            category_id remapped only
//	fleet.maintenance_schedules          category_id remapped only
//	fleet.maintenance_record_documents   untouched; source of receipt media ids
//	fleet.fuel_logs                      untouched
//	fleet.mileage_records                untouched
//	fleet.vehicle_media                  untouched; source of photo media ids
//	fleet.dashboards                     untouched (fleet-scoped, not vehicle-scoped)
//	fleet.fleets, fleet_memberships, fleet_invites  untouched (PRD non-goals)
//	fleet.purge_operations               untouched
//	fleet.admin_audit_events             one row appended
//	media.media_objects                  delegated to media-service
//	notification.notifications           delegated to notification-service
//
// If a future table gains a fleet_id or a vehicle reference, answer the
// transfer question here at the same time arch_test.go forces you to answer the
// purge question in Manifest.
var TransferPlan = []TransferStep{
	{Key: "maintenance_records", Table: "fleet.maintenance_records", Where: "vehicle_id = ?"},
	{Key: "maintenance_schedules", Table: "fleet.maintenance_schedules", Where: "vehicle_id = ?"},
	{Key: "fuel_logs", Table: "fleet.fuel_logs", Where: "vehicle_id = ?"},
	{Key: "mileage_records", Table: "fleet.mileage_records", Where: "vehicle_id = ?"},
	{Key: "vehicle_media", Table: "fleet.vehicle_media", Where: "vehicle_id = ?"},
	{
		// The car's timeline follows the car (FR-XFER-MOVE-3). Rows with a NULL
		// vehicle_id describe the FLEET and are never matched by this
		// predicate, so FR-XFER-MOVE-4 holds by construction.
		Key: "activity_events", Table: "fleet.activity_events",
		Where: "vehicle_id = ?", Set: "fleet_id = ?",
	},
}

// CountTransfer returns, per plan key, how many LIVE rows the transfer covers.
// It is the preview's only source for these keys and uses COUNT aggregates —
// no counted row is ever loaded (PRD §8 Performance).
func CountTransfer(db *gorm.DB, vehicleID string) (map[string]int, error) {
	out := make(map[string]int, len(TransferPlan))
	for _, s := range TransferPlan {
		var n int64
		q := "SELECT count(*) FROM " + s.Table + " WHERE (" + s.Where + ") AND deleted_at IS NULL"
		if err := db.Raw(q, vehicleID).Scan(&n).Error; err != nil {
			return nil, fmt.Errorf("count transfer %s: %w", s.Table, err)
		}
		out[s.Key] = int(n)
	}
	return out, nil
}

// VehicleMediaIDs is the set of media objects that must be re-homed with the
// vehicle (FR-XFER-MEDIA-2). Media objects carry a fleet_id, but "the media
// belonging to this vehicle" is a fact only fleet-service holds.
//
// Three sources, unioned:
//
//  1. the vehicle's photos (fleet.vehicle_media),
//  2. its primary image — a plain NOT NULL string column where "none" is the
//     EMPTY STRING, so it is filtered with a not-equal-to-empty test rather
//     than with IS NOT NULL,
//  3. receipts and attachments on its maintenance records
//     (fleet.maintenance_record_documents, the only attachment table).
//
// UNION, not UNION ALL: the primary image is usually also a vehicle_media row,
// and sending media-service the same id twice would double-count the read-back.
func VehicleMediaIDs(db *gorm.DB, vehicleID string) ([]string, error) {
	var ids []string
	q := `
		SELECT media_id FROM fleet.vehicle_media
		 WHERE vehicle_id = ? AND deleted_at IS NULL
		   AND media_id IS NOT NULL AND media_id <> ''
		UNION
		SELECT primary_image_media_id FROM fleet.vehicles
		 WHERE id = ? AND primary_image_media_id IS NOT NULL
		   AND primary_image_media_id <> ''
		UNION
		SELECT d.media_id FROM fleet.maintenance_record_documents d
		  JOIN fleet.maintenance_records m ON m.id = d.maintenance_record_id
		 WHERE m.vehicle_id = ? AND d.deleted_at IS NULL AND m.deleted_at IS NULL
		   AND d.media_id IS NOT NULL AND d.media_id <> ''`
	if err := db.Raw(q, vehicleID, vehicleID, vehicleID).Scan(&ids).Error; err != nil {
		return nil, fmt.Errorf("resolve transfer media ids: %w", err)
	}
	return ids, nil
}
