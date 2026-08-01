package maintenancecategory

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/sirupsen/logrus"
	"gorm.io/gorm"

	"github.com/jtumidanski/myfleet/packages/shared-go/server"
)

// InitializeRoutes wires the JWT-protected maintenance category endpoints.
// Categories are global/system data (not fleet-scoped); any authenticated
// caller may list them.
func InitializeRoutes(log logrus.FieldLogger, db *gorm.DB) func(chi.Router) {
	proc := NewProcessor(log, NewProvider(db))
	return func(r chi.Router) {
		// GET /maintenance-categories — list categories (paged).
		r.Get("/maintenance-categories", func(w http.ResponseWriter, req *http.Request) {
			// An unrecognised kind is 422, not a silent empty list
			// (PRD FR-KIND-4). 422 rather than 400 because shared-go has no 400
			// sentinel and ErrValidation is the established mapping.
			kind, err := ParseKind(req.URL.Query().Get("kind"))
			if err != nil {
				server.WriteError(w, err)
				return
			}

			page := server.ParsePage(req)
			ms, total, err := proc.List(kind, page)
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
	}
}
