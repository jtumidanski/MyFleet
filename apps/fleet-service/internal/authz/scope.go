// Package authz is the fleet-scoped authorization spine (design §9). Every
// resource handler calls RequireSameFleet then a role guard.
package authz

import (
	"github.com/jtumidanski/myfleet/packages/shared-go/auth"
	"github.com/jtumidanski/myfleet/packages/shared-go/server"
)

// RequireSameFleet returns 404 (not 403) when the resource is in another fleet,
// so cross-fleet existence is never leaked (design §9, PRD §5.1).
func RequireSameFleet(id auth.Identity, resourceFleetID string) error {
	if id.ActiveFleetID == "" || id.ActiveFleetID != resourceFleetID {
		return server.ErrNotFound
	}
	return nil
}

// RequireWrite allows member|owner; viewer is read-only (403).
func RequireWrite(id auth.Identity) error {
	if id.Role == "member" || id.Role == "owner" {
		return nil
	}
	return server.ErrForbidden
}

// RequireOwner allows only owner (403 otherwise). Callers should additionally
// confirm against the membership table for owner-only mutations (stale-claim guard).
func RequireOwner(id auth.Identity) error {
	if id.Role == "owner" {
		return nil
	}
	return server.ErrForbidden
}
