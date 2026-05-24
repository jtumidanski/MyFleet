package fleet

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/sirupsen/logrus"
	"gorm.io/gorm"

	"github.com/jtumidanski/myfleet/packages/shared-go/auth"
	"github.com/jtumidanski/myfleet/packages/shared-go/server"

	"github.com/jtumidanski/myfleet/apps/fleet-service/internal/authz"
)

// OnboardingAdmin is the subset of the membership administrator used for fleet
// onboarding (create fleet + owner membership in one transaction).
type OnboardingAdmin interface {
	CreateFleetWithOwner(db *gorm.DB, fleetName, userID string) (Model, error)
}

// InitializeRoutes wires the fleet REST endpoints. The onboardAdmin is the
// membership administrator which owns the cross-domain onboarding transaction.
func InitializeRoutes(log logrus.FieldLogger, db *gorm.DB, onboardAdmin OnboardingAdmin) func(chi.Router) {
	proc := NewProcessor(log, NewProvider(db), NewAdministrator(db))
	return func(r chi.Router) {
		// POST /fleets — onboarding: create fleet + owner membership in one tx
		r.Post("/fleets", server.RegisterInputHandler(func(w http.ResponseWriter, req *http.Request, attrs struct {
			Name string `json:"name"`
		}) {
			identity := auth.IdentityFromContext(req.Context())
			if identity.UserID == "" {
				server.WriteError(w, server.ErrUnauthorized)
				return
			}
			m, err := onboardAdmin.CreateFleetWithOwner(db, attrs.Name, identity.UserID)
			if err != nil {
				server.WriteError(w, err)
				return
			}
			server.WriteJSON(w, http.StatusCreated, server.Document{Data: Transform(m)})
		}))

		// GET /fleets/{id}
		r.Get("/fleets/{id}", func(w http.ResponseWriter, req *http.Request) {
			identity := auth.IdentityFromContext(req.Context())
			id := chi.URLParam(req, "id")
			if err := authz.RequireSameFleet(identity, id); err != nil {
				server.WriteError(w, err)
				return
			}
			m, err := proc.GetByID(id)
			if err != nil {
				server.WriteError(w, err)
				return
			}
			server.WriteJSON(w, http.StatusOK, server.Document{Data: Transform(m)})
		})

		// PATCH /fleets/{id} — rename, owner-only (token gate + authoritative DB check)
		r.Patch("/fleets/{id}", server.RegisterInputHandler(func(w http.ResponseWriter, req *http.Request, attrs struct {
			Name string `json:"name"`
		}) {
			identity := auth.IdentityFromContext(req.Context())
			id := chi.URLParam(req, "id")
			if err := authz.RequireSameFleet(identity, id); err != nil {
				server.WriteError(w, err)
				return
			}
			if err := authz.RequireOwner(identity); err != nil {
				server.WriteError(w, err)
				return
			}
			m, err := proc.Rename(id, attrs.Name)
			if err != nil {
				server.WriteError(w, err)
				return
			}
			server.WriteJSON(w, http.StatusOK, server.Document{Data: Transform(m)})
		}))
	}
}
