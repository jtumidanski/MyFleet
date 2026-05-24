package invite

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/sirupsen/logrus"
	"gorm.io/gorm"

	"github.com/jtumidanski/myfleet/packages/shared-go/auth"
	"github.com/jtumidanski/myfleet/packages/shared-go/events"
	"github.com/jtumidanski/myfleet/packages/shared-go/server"

	"github.com/jtumidanski/myfleet/apps/fleet-service/internal/authz"
	"github.com/jtumidanski/myfleet/apps/fleet-service/internal/membership"
)

const defaultExpiry = 7 * 24 * time.Hour

// ownerChecker is satisfied by membership.Provider for the authoritative owner check.
type ownerChecker interface {
	GetByFleetAndUser(fleetID, userID string) (membership.Model, error)
}

// InitializeRoutes wires the JWT-protected invite endpoints.
func InitializeRoutes(log logrus.FieldLogger, db *gorm.DB, producer events.Producer) func(chi.Router) {
	prov := NewProvider(db)
	adm := NewAdministrator(db)
	proc := NewProcessor(log)
	memProv := membership.NewProvider(db)

	return func(r chi.Router) {
		// POST /fleets/{id}/invites — owner-only; creates an invite with a unique token
		r.Post("/fleets/{id}/invites", server.RegisterInputHandler(func(w http.ResponseWriter, req *http.Request, attrs struct {
			Email string `json:"email"`
			Role  string `json:"role"`
		}) {
			identity := auth.IdentityFromContext(req.Context())
			fleetID := chi.URLParam(req, "id")

			if err := authz.RequireSameFleet(identity, fleetID); err != nil {
				server.WriteError(w, err)
				return
			}
			// Token-level owner gate
			if err := authz.RequireOwner(identity); err != nil {
				server.WriteError(w, err)
				return
			}
			// Authoritative DB check: confirm actor is still owner
			actorMem, err := memProv.GetByFleetAndUser(fleetID, identity.UserID)
			if err != nil {
				if errors.Is(err, membership.ErrNotFound) {
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

			if attrs.Email == "" || attrs.Role == "" {
				server.WriteError(w, server.ErrValidation)
				return
			}

			token, err := generateToken()
			if err != nil {
				server.WriteError(w, err)
				return
			}

			m := NewBuilder().
				SetFleetID(fleetID).
				SetEmail(attrs.Email).
				SetRole(attrs.Role).
				SetToken(token).
				SetExpiresAt(time.Now().Add(defaultExpiry)).
				SetInvitedByUserID(identity.UserID).
				Build()

			created, err := adm.Insert(m)
			if err != nil {
				server.WriteError(w, err)
				return
			}
			server.WriteJSON(w, http.StatusCreated, server.Document{Data: Transform(created)})
		}))

		// GET /fleets/{id}/invites — list invites (fleet-scoped)
		r.Get("/fleets/{id}/invites", func(w http.ResponseWriter, req *http.Request) {
			identity := auth.IdentityFromContext(req.Context())
			fleetID := chi.URLParam(req, "id")

			if err := authz.RequireSameFleet(identity, fleetID); err != nil {
				server.WriteError(w, err)
				return
			}
			ms, err := prov.ListByFleetID(fleetID)
			if err != nil {
				server.WriteError(w, err)
				return
			}
			server.WriteJSON(w, http.StatusOK, server.Document{Data: TransformSlice(ms)})
		})

		// DELETE /invites/{id} — owner-only
		r.Delete("/invites/{id}", func(w http.ResponseWriter, req *http.Request) {
			identity := auth.IdentityFromContext(req.Context())
			id := chi.URLParam(req, "id")

			inv, err := prov.GetByID(id)
			if err != nil {
				if errors.Is(err, ErrNotFound) {
					server.WriteError(w, server.ErrNotFound)
					return
				}
				server.WriteError(w, err)
				return
			}

			if err := authz.RequireSameFleet(identity, inv.FleetID()); err != nil {
				server.WriteError(w, err)
				return
			}
			if err := authz.RequireOwner(identity); err != nil {
				server.WriteError(w, err)
				return
			}
			// Authoritative DB check
			actorMem, err := memProv.GetByFleetAndUser(inv.FleetID(), identity.UserID)
			if err != nil {
				if errors.Is(err, membership.ErrNotFound) {
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

			if err := adm.Delete(id); err != nil {
				server.WriteError(w, err)
				return
			}
			w.WriteHeader(http.StatusNoContent)
		})

		// POST /invites/{token}/accept — look up by token; validate; create membership + stamp accepted_at
		r.Post("/invites/{token}/accept", func(w http.ResponseWriter, req *http.Request) {
			identity := auth.IdentityFromContext(req.Context())
			token := chi.URLParam(req, "token")

			inv, err := prov.GetByToken(token)
			if err != nil {
				if errors.Is(err, ErrNotFound) {
					server.WriteError(w, server.ErrNotFound)
					return
				}
				server.WriteError(w, err)
				return
			}

			if err := proc.ValidateAccept(inv, identity.Email); err != nil {
				server.WriteError(w, err)
				return
			}

			updated, err := adm.Accept(inv, identity.UserID, producer)
			if err != nil {
				server.WriteError(w, err)
				return
			}
			server.WriteJSON(w, http.StatusOK, server.Document{Data: Transform(updated)})
		})
	}
}

// generateToken returns a cryptographically random 32-byte hex string.
func generateToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
