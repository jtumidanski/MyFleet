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
	MediaIDs []string `json:"media_ids"`
	// SourceFleetID is the fleet the named objects are expected to be in right
	// now. It is REQUIRED, and it is the ownership predicate: without it this
	// endpoint would move any named media object into any fleet regardless of
	// who owns it, and fleet-service cannot vouch for the ids it sends —
	// vehicle_media rows are attached by fleet members with no cross-service
	// ownership check, so a third fleet's media id can reach here.
	SourceFleetID      string `json:"source_fleet_id"`
	DestinationFleetID string `json:"destination_fleet_id"`
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
//
// srcFleetID is the OWNERSHIP PREDICATE, and it is why an id fleet-service
// should never have sent cannot do damage: an object belonging to a third
// fleet does not satisfy `fleet_id = ?` and stays exactly where it is. The
// caller's id set is not trusted, because it cannot be — the attach path that
// produces those ids takes an arbitrary media id from a fleet member.
//
// The `fleet_id <> ?` destination guard is kept alongside it. With the source
// predicate present it is implied whenever source and destination differ, but
// it costs nothing and keeps the "a replay updates zero rows" property stated
// above true on its own terms.
func Reassign(tx *gorm.DB, mediaIDs []string, srcFleetID, destFleetID string) (map[string]int, error) {
	upd := `UPDATE media.media_objects SET fleet_id = ?
	         WHERE id IN ? AND fleet_id = ? AND fleet_id <> ? AND deleted_at IS NULL`
	if err := tx.Exec(upd, destFleetID, mediaIDs, srcFleetID, destFleetID).Error; err != nil {
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
	if body.SourceFleetID == "" {
		// Required, not defaulted. An empty source fleet id would match no row
		// at all, so the request would silently succeed having moved nothing —
		// and the caller would read the read-back count as "already done".
		return server.Detailed(server.ErrValidation, "source_fleet_id is required")
	}
	if body.DestinationFleetID == "" {
		return server.Detailed(server.ErrValidation, "destination_fleet_id is required")
	}
	return nil
}
