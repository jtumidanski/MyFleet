package membership

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/sirupsen/logrus"
	"gorm.io/gorm"

	"github.com/jtumidanski/myfleet/packages/shared-go/auth"
	"github.com/jtumidanski/myfleet/packages/shared-go/server"

	"github.com/jtumidanski/myfleet/apps/fleet-service/internal/authz"
)

// membershipProc bundles the full-featured provider for handlers that need it.
type membershipProc struct {
	proc *Processor
	prov Provider
	adm  Administrator
}

// InitializeRoutes wires the JWT-protected membership endpoints under a fleet.
func InitializeRoutes(log logrus.FieldLogger, db *gorm.DB) func(chi.Router) {
	prov := NewProvider(db)
	adm := NewAdministrator(db)
	proc := NewProcessor(log, prov)
	mp := &membershipProc{proc: proc, prov: prov, adm: adm}
	return func(r chi.Router) {
		// GET /fleets/{id}/members — list fleet memberships (fleet-scoped)
		r.Get("/fleets/{id}/members", func(w http.ResponseWriter, req *http.Request) {
			identity := auth.IdentityFromContext(req.Context())
			fleetID := chi.URLParam(req, "id")
			if err := authz.RequireSameFleet(identity, fleetID); err != nil {
				server.WriteError(w, err)
				return
			}
			ms, err := mp.prov.ListByFleetID(fleetID)
			if err != nil {
				server.WriteError(w, err)
				return
			}
			server.WriteJSON(w, http.StatusOK, server.Document{Data: TransformSlice(ms)})
		})

		// DELETE /fleets/{id}/members/{userId} — owner-only; sole-owner guard
		r.Delete("/fleets/{id}/members/{userId}", func(w http.ResponseWriter, req *http.Request) {
			identity := auth.IdentityFromContext(req.Context())
			fleetID := chi.URLParam(req, "id")
			targetUserID := chi.URLParam(req, "userId")

			if err := authz.RequireSameFleet(identity, fleetID); err != nil {
				server.WriteError(w, err)
				return
			}
			// Token-level gate (fast path)
			if err := authz.RequireOwner(identity); err != nil {
				server.WriteError(w, err)
				return
			}
			// Authoritative DB check: confirm actor is still owner (stale-token guard)
			actorMem, err := mp.prov.GetByFleetAndUser(fleetID, identity.UserID)
			if err != nil {
				if errors.Is(err, ErrNotFound) {
					server.WriteError(w, server.ErrForbidden)
					return
				}
				server.WriteError(w, err)
				return
			}
			if actorMem.Role() != "owner" {
				server.WriteError(w, server.ErrForbidden)
				return
			}

			// Fetch the target membership
			targetMem, err := mp.prov.GetByFleetAndUser(fleetID, targetUserID)
			if err != nil {
				if errors.Is(err, ErrNotFound) {
					server.WriteError(w, server.ErrNotFound)
					return
				}
				server.WriteError(w, err)
				return
			}

			// Sole-owner self-removal guard
			if err := mp.proc.ValidateRemoval(fleetID, identity.UserID, targetUserID, targetMem.Role()); err != nil {
				server.WriteError(w, err)
				return
			}

			if err := mp.adm.Delete(targetMem.ID()); err != nil {
				server.WriteError(w, err)
				return
			}
			w.WriteHeader(http.StatusNoContent)
		})
	}
}

// InitializeInternalRoutes wires the network-restricted internal endpoint (no JWT).
// Register this initializer WITHOUT JWT middleware.
// GET /internal/memberships/active?user_id= → {fleet_id, role} or 404.
func InitializeInternalRoutes(log logrus.FieldLogger, db *gorm.DB) func(chi.Router) {
	prov := NewProvider(db)
	return func(r chi.Router) {
		r.Get("/internal/memberships/active", func(w http.ResponseWriter, req *http.Request) {
			userID := req.URL.Query().Get("user_id")
			if userID == "" {
				server.WriteError(w, server.ErrValidation)
				return
			}
			m, err := prov.GetActiveByUserID(userID)
			if err != nil {
				if errors.Is(err, ErrNotFound) {
					server.WriteError(w, server.ErrNotFound)
					return
				}
				server.WriteError(w, err)
				return
			}
			server.WriteJSON(w, http.StatusOK, ActiveResponse{FleetID: m.FleetID(), Role: m.Role()})
		})
	}
}
