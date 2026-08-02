package maintenancecategory

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/sirupsen/logrus"
	"gorm.io/gorm"

	"github.com/jtumidanski/myfleet/apps/fleet-service/internal/authz"
	"github.com/jtumidanski/myfleet/packages/shared-go/auth"
	"github.com/jtumidanski/myfleet/packages/shared-go/server"
)

// InitializeRoutes wires the JWT-protected maintenance category endpoints.
// Categories are no longer purely global: a category is visible to a caller
// when it is system-defined (NULL fleet_id) or scoped to the caller's own
// active fleet; any authenticated caller may list them.
func InitializeRoutes(log logrus.FieldLogger, db *gorm.DB) func(chi.Router) {
	proc := NewProcessor(log, NewProvider(db))
	return func(r chi.Router) {
		// GET /maintenance-categories — list categories (paged).
		r.Get("/maintenance-categories", func(w http.ResponseWriter, req *http.Request) {
			identity := auth.IdentityFromContext(req.Context())

			// An unrecognised kind is 422, not a silent empty list
			// (PRD FR-KIND-4). 422 rather than 400 because shared-go has no 400
			// sentinel and ErrValidation is the established mapping.
			kind, err := ParseKind(req.URL.Query().Get("kind"))
			if err != nil {
				server.WriteError(w, err)
				return
			}

			page := server.ParsePage(req)
			ms, total, err := proc.List(kind, identity.ActiveFleetID, page)
			if err != nil {
				log.WithError(err).Error("list maintenance categories")
				server.WriteError(w, err)
				return
			}
			server.WriteJSON(w, http.StatusOK, server.Document{
				Data: TransformSlice(ms),
				Meta: page.Meta(total),
			})
		})

		// POST /maintenance-categories — create a free-form category scoped to
		// the caller's fleet. Idempotent: an existing case-insensitive match is
		// returned instead of a duplicate.
		r.Post("/maintenance-categories", func(w http.ResponseWriter, req *http.Request) {
			identity := auth.IdentityFromContext(req.Context())
			if err := authz.RequireWrite(identity); err != nil {
				server.WriteError(w, err)
				return
			}
			// No active fleet means there is nothing to scope the row to.
			if identity.ActiveFleetID == "" {
				server.WriteError(w, server.ErrValidation)
				return
			}

			var body struct {
				Data struct {
					Attributes CreateAttributes `json:"attributes"`
				} `json:"data"`
			}
			if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
				server.WriteError(w, server.ErrValidation)
				return
			}

			kind, err := ParseKind(body.Data.Attributes.Kind)
			if err != nil || kind == "" {
				server.WriteError(w, server.ErrValidation)
				return
			}

			m, err := proc.Create(identity.ActiveFleetID, body.Data.Attributes.Name, kind)
			if err != nil {
				log.WithError(err).Error("create maintenance category")
				server.WriteError(w, err)
				return
			}
			server.WriteJSON(w, http.StatusCreated, server.Document{Data: Transform(m)})
		})
	}
}
