package admin

import (
	"fmt"

	"gorm.io/gorm"

	"github.com/jtumidanski/myfleet/packages/shared-go/server"
)

// ReassignRequest re-homes a set of media objects to another fleet. It is the
// body of POST /internal/admin/reassign-fleet, issued by fleet-service when a
// platform admin transfers a vehicle between fleets.
type ReassignRequest struct {
	MediaIDs           []string `json:"media_ids"`
	DestinationFleetID string   `json:"destination_fleet_id"`
}

// Reassign rewrites media_objects.fleet_id for the named objects and returns
// the read-back count.
//
// Rewriting media_objects alone re-homes the whole media subtree: variants and
// the variant-failure ledger key on media_object_id, not on fleet_id.
//
// It is IDEMPOTENT, achieved exactly the way Stamp achieves it — the count is
// READ BACK after the update rather than taken from RowsAffected. A replay
// updates zero rows because of the `fleet_id <> ?` guard, but must still report
// the same number, or fleet-service's compensating reverse call could not tell
// "already done" from "did nothing" (FR-XFER-MEDIA-4).
//
// Unknown ids simply do not match and are ignored, matching the purge path's
// tolerance. Soft-deleted objects are left alone: a pending-purge object is on
// its way out and must not be dragged into a fleet that never had it.
func Reassign(tx *gorm.DB, mediaIDs []string, destFleetID string) (map[string]int, error) {
	upd := `UPDATE media.media_objects SET fleet_id = ?
	         WHERE id IN ? AND fleet_id <> ? AND deleted_at IS NULL`
	if err := tx.Exec(upd, destFleetID, mediaIDs, destFleetID).Error; err != nil {
		return nil, fmt.Errorf("reassign media objects: %w", err)
	}
	var n int64
	cnt := `SELECT count(*) FROM media.media_objects
	         WHERE id IN ? AND fleet_id = ? AND deleted_at IS NULL`
	if err := tx.Raw(cnt, mediaIDs, destFleetID).Scan(&n).Error; err != nil {
		return nil, fmt.Errorf("count reassigned media objects: %w", err)
	}
	return map[string]int{"media_objects": int(n)}, nil
}

// reassignRootFrom validates the request body, or returns 422 — the same
// treatment rootFrom gives a purge body.
func reassignRootFrom(body ReassignRequest) error {
	if len(body.MediaIDs) == 0 {
		return server.Detailed(server.ErrValidation, "media_ids is required")
	}
	if body.DestinationFleetID == "" {
		return server.Detailed(server.ErrValidation, "destination_fleet_id is required")
	}
	return nil
}
