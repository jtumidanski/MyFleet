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

// errRoleValidation is the TRANSPORT envelope for the domain's ErrInvalidRole.
// It names the field and the accepted values without echoing the caller's
// input in the JSON:API `detail` field, and it is a compile-time constant so
// no attacker-supplied string can reach the response. server.Detailed is what
// populates `detail` — the same pairing invite.ErrAlreadyAccepted etc. use —
// unlike a fmt.Errorf("%w: ...") wrap, which WriteError renders only into
// `title` because it doesn't implement Detail() string.
var errRoleValidation = server.Detailed(server.ErrValidation, "role must be one of owner, member, viewer")

// InitializeRoutes wires the JWT-protected membership endpoints under a fleet.
//
// rec is the activity recorder run inside the role-change and removal
// transactions (FR-5.2). Pass nil in tests that do not exercise the feed.
func InitializeRoutes(log logrus.FieldLogger, db *gorm.DB, rec ActivityRecorder) func(chi.Router) {
	prov := NewProvider(db)
	adm := NewAdministrator(db).WithActivityRecorder(rec)
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

		// PATCH /fleets/{id}/members/{userId} — change a membership's role.
		// Owner-only at BOTH layers; zero-owner guard in ValidateRoleChange.
		r.Patch("/fleets/{id}/members/{userId}", server.RegisterInputHandler(func(w http.ResponseWriter, req *http.Request, attrs struct {
			Role string `json:"role"`
		},
		) {
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
			// Authoritative DB check (stale-claim guard, SEC-5)
			if err := proc.RequireOwnerInFleet(fleetID, identity.UserID); err != nil {
				server.WriteError(w, err)
				return
			}

			target, err := proc.ValidateRoleChange(fleetID, targetUserID, attrs.Role)
			if err != nil {
				// Client errors are not incidents — do not log them.
				if errors.Is(err, ErrInvalidRole) {
					server.WriteError(w, errRoleValidation)
					return
				}
				server.WriteError(w, err)
				return
			}

			updated, err := adm.UpdateRole(target, attrs.Role, identity.UserID)
			if err != nil {
				log.WithError(err).Error("membership role update failed")
				server.WriteError(w, err)
				return
			}
			server.WriteJSON(w, http.StatusOK, server.Document{Data: Transform(updated)})
		}))

		// DELETE /fleets/{id}/members/{userId} — owner-only for OTHERS;
		// self-removal needs no role (FR-3.1). Sole-owner guard unchanged.
		r.Delete("/fleets/{id}/members/{userId}", func(w http.ResponseWriter, req *http.Request) {
			identity := auth.IdentityFromContext(req.Context())
			fleetID := chi.URLParam(req, "id")
			targetUserID := chi.URLParam(req, "userId")

			// OUTSIDE the isSelf branch on purpose: this is what makes a
			// cross-fleet id 404 rather than leaking existence, and self-ness
			// must not be able to bypass it. identity.UserID == targetUserID is
			// necessary but not sufficient.
			if err := authz.RequireSameFleet(identity, fleetID); err != nil {
				server.WriteError(w, err)
				return
			}

			// One predicate, two consequences: it relaxes the guard here and
			// picks member.left over member.removed in Administrator.Remove, so
			// the authorization decision and the audit trail cannot disagree.
			isSelf := identity.UserID == targetUserID
			if !isSelf {
				// Token-level gate (fast path)
				if err := authz.RequireOwner(identity); err != nil {
					server.WriteError(w, err)
					return
				}
				// Authoritative DB check (stale-claim guard, SEC-5)
				if err := proc.RequireOwnerInFleet(fleetID, identity.UserID); err != nil {
					server.WriteError(w, err)
					return
				}
			}

			targetMem, err := proc.GetMember(fleetID, targetUserID)
			if err != nil {
				if errors.Is(err, server.ErrNotFound) {
					server.WriteError(w, server.ErrNotFound)
					return
				}
				server.WriteError(w, err)
				return
			}

			// Sole-owner self-removal guard (FR-3.2, unchanged)
			if err := proc.ValidateRemoval(fleetID, identity.UserID, targetUserID, targetMem.Role()); err != nil {
				server.WriteError(w, err)
				return
			}

			if err := adm.Remove(targetMem, identity.UserID); err != nil {
				log.WithError(err).Error("membership removal failed")
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
	proc := NewProcessor(log, prov)
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

		// GET /internal/fleets/{fleetID}/members → active members [{user_id, role}].
		// Network-restricted (no JWT); notification-service uses this to resolve a
		// fleet's recipients without a cross-service DB join (D2).
		r.Get("/internal/fleets/{fleetID}/members", func(w http.ResponseWriter, req *http.Request) {
			fleetID := chi.URLParam(req, "fleetID")
			if fleetID == "" {
				server.WriteError(w, server.ErrValidation)
				return
			}
			ms, err := proc.ListActiveMembers(fleetID)
			if err != nil {
				log.WithError(err).Error("internal list active members")
				server.WriteError(w, err)
				return
			}
			server.WriteJSON(w, http.StatusOK, TransformInternalMembers(ms))
		})
	}
}
