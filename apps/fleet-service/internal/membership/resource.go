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

// InitializeRoutes wires the JWT-protected membership endpoints under a fleet.
func InitializeRoutes(log logrus.FieldLogger, db *gorm.DB) func(chi.Router) {
	prov := NewProvider(db)
	adm := NewAdministrator(db)
	proc := NewProcessor(log, prov)
	return func(r chi.Router) {
		// GET /fleets/{id}/members — list fleet memberships (fleet-scoped)
		r.Get("/fleets/{id}/members", func(w http.ResponseWriter, req *http.Request) {
			identity := auth.IdentityFromContext(req.Context())
			fleetID := chi.URLParam(req, "id")
			if err := authz.RequireSameFleet(identity, fleetID); err != nil {
				server.WriteError(w, err)
				return
			}
			ms, err := proc.ListMembers(fleetID)
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
			// Authoritative DB check via processor (stale-claim guard, design §9)
			if err := proc.RequireOwnerInFleet(fleetID, identity.UserID); err != nil {
				server.WriteError(w, err)
				return
			}

			// Fetch the target membership via processor
			targetMem, err := proc.GetMember(fleetID, targetUserID)
			if err != nil {
				if errors.Is(err, server.ErrNotFound) {
					server.WriteError(w, server.ErrNotFound)
					return
				}
				server.WriteError(w, err)
				return
			}

			// Sole-owner self-removal guard
			if err := proc.ValidateRemoval(fleetID, identity.UserID, targetUserID, targetMem.Role()); err != nil {
				server.WriteError(w, err)
				return
			}

			if err := adm.Delete(targetMem.ID()); err != nil {
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
