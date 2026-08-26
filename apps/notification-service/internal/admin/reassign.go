package admin

import (
	"fmt"

	"gorm.io/gorm"

	"github.com/jtumidanski/myfleet/packages/shared-go/server"
)

// ReassignRequest re-points the fleet_id on notifications about a set of
// vehicles. It is the body of POST /internal/admin/reassign-fleet, issued by
// fleet-service when a platform admin transfers a vehicle between fleets.
//
// It takes VEHICLE ids rather than notification ids because the vehicle ->
// notification relationship is this service's to know, not fleet-service's.
type ReassignRequest struct {
	VehicleIDs         []string `json:"vehicle_ids"`
	DestinationFleetID string   `json:"destination_fleet_id"`
}

// Reassign rewrites notifications.fleet_id for the named vehicles and returns
// the read-back count.
//
// notifications.fleet_id is a stored, denormalised routing column — the same
// stored-not-derived staleness fleet.activity_events has. A stale value breaks
// no read, since notifications are per-user, but it would mis-scope a later
// fleet-scoped admin purge selecting on that column, which is exactly why the
// column is indexed.
//
// Idempotent for the same reason and by the same means as media-service's twin:
// the count is READ BACK, not taken from RowsAffected, so a replay reports the
// same number instead of the zero rows its UPDATE touches. Unknown vehicle ids
// simply do not match. Soft-deleted rows are left alone: a pending-purge
// notification is on its way out and must not be dragged into a fleet that
// never had it.
func Reassign(tx *gorm.DB, vehicleIDs []string, destFleetID string) (map[string]int, error) {
	upd := `UPDATE notification.notifications SET fleet_id = ?
	         WHERE vehicle_id IN ? AND fleet_id <> ? AND deleted_at IS NULL`
	if err := tx.Exec(upd, destFleetID, vehicleIDs, destFleetID).Error; err != nil {
		return nil, fmt.Errorf("reassign notifications: %w", err)
	}
	var n int64
	cnt := `SELECT count(*) FROM notification.notifications
	         WHERE vehicle_id IN ? AND fleet_id = ? AND deleted_at IS NULL`
	if err := tx.Raw(cnt, vehicleIDs, destFleetID).Scan(&n).Error; err != nil {
		return nil, fmt.Errorf("count reassigned notifications: %w", err)
	}
	return map[string]int{"notifications": int(n)}, nil
}

// reassignRootFrom validates the request body, or returns 422 — the same
// treatment rootFrom gives a purge body.
func reassignRootFrom(body ReassignRequest) error {
	if len(body.VehicleIDs) == 0 {
		return server.Detailed(server.ErrValidation, "vehicle_ids is required")
	}
	if body.DestinationFleetID == "" {
		return server.Detailed(server.ErrValidation, "destination_fleet_id is required")
	}
	return nil
}
