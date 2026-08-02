package invite

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/sirupsen/logrus"
	"gorm.io/gorm"

	"github.com/jtumidanski/myfleet/packages/shared-go/server"
	"github.com/jtumidanski/myfleet/packages/shared-go/telemetry"

	"github.com/jtumidanski/myfleet/apps/fleet-service/internal/fleet"
)

// FleetNamer resolves a fleet's display name for the invite email's subject and
// body. A narrow read-only seam rather than the fleet processor, mirroring how
// OwnerChecker keeps this package decoupled. Satisfied by fleet.Provider.
type FleetNamer interface {
	GetByID(id string) (fleet.Model, error)
}

// InitializeInternalRoutes wires the network-restricted internal endpoint (no JWT).
// Register this initializer WITHOUT JWT middleware.
//
// GET /internal/invites/{inviteID} → InternalResponse or 404.
//
// SECURITY: this endpoint serves the invite TOKEN, a bearer credential. It is
// kept off the public internet by the priority-200 internal-deny rule in the
// main overlay's ingressroute, which already matches every path under
// /api/fleet/internal. TestInternalRouteAbsentFromJWTTree proves it is not
// reachable through the JWT router either (FR-INT-4).
func InitializeInternalRoutes(log logrus.FieldLogger, db *gorm.DB, fleets FleetNamer) func(chi.Router) {
	prov := NewProvider(db)
	proc := NewProcessor(log, prov)
	return func(r chi.Router) {
		r.Get("/internal/invites/{inviteID}", func(w http.ResponseWriter, req *http.Request) {
			inviteID := chi.URLParam(req, "inviteID")
			// The caller is another service, which forwards the correlation id of
			// the request that made it call. Without this field a failed invite
			// email cannot be joined back to the create that triggered it — the
			// whole reason the header is propagated.
			correlationID := telemetry.CorrelationIDFromContext(req.Context())
			if inviteID == "" {
				server.WriteError(w, server.ErrValidation)
				return
			}

			// Returned even when accepted or expired (FR-INT-3) — the consumer
			// decides whether to send, and needs to tell "stale" apart from
			// "deleted" to avoid pointless retries.
			inv, err := proc.GetByID(req.Context(), inviteID)
			if err != nil {
				if errors.Is(err, server.ErrNotFound) {
					server.WriteError(w, server.ErrNotFound)
					return
				}
				log.WithError(err).WithFields(logrus.Fields{
					"invite_id":      inviteID,
					"correlation_id": correlationID,
				}).Error("internal get invite")
				server.WriteError(w, err)
				return
			}

			// An unresolvable fleet degrades to an empty name rather than a 500;
			// the email then uses the generic subject.
			var fleetName string
			f, err := fleets.GetByID(inv.FleetID())
			if err != nil {
				log.WithError(err).WithFields(logrus.Fields{
					"invite_id":      inviteID,
					"fleet_id":       inv.FleetID(),
					"correlation_id": correlationID,
				}).Warn("could not resolve fleet name for invite email")
			} else {
				fleetName = f.Name()
			}

			server.WriteJSON(w, http.StatusOK, TransformInternal(inv, fleetName))
		})
	}
}
