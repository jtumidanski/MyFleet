package notification

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/sirupsen/logrus"
	"gorm.io/gorm"

	"github.com/jtumidanski/myfleet/packages/shared-go/auth"
	"github.com/jtumidanski/myfleet/packages/shared-go/server"
)

// InitializeRoutes wires the JWT-protected notification endpoints. Paths are
// bare (the gateway strips /api/notifications). Every endpoint is scoped to the
// token's user id — a user only ever sees and mutates their OWN notifications,
// so no fleet-membership recheck is needed (design §8.4 / §9).
func InitializeRoutes(log logrus.FieldLogger, db *gorm.DB, prefs Prefs) func(chi.Router) {
	proc := NewProcessor(log, NewAdministrator(db), prefs).
		WithReads(NewProvider(db), NewAdministrator(db))
	return func(r chi.Router) {
		// GET /notifications?read=&type= — the caller's own notifications (paged).
		r.Get("/notifications", func(w http.ResponseWriter, req *http.Request) {
			identity := auth.IdentityFromContext(req.Context())
			if identity.UserID == "" {
				server.WriteError(w, server.ErrUnauthorized)
				return
			}
			filter := ListFilter{Type: req.URL.Query().Get("type")}
			switch req.URL.Query().Get("read") {
			case "true":
				t := true
				filter.Read = &t
			case "false":
				f := false
				filter.Read = &f
			}
			page := server.ParsePage(req)
			ms, total, err := proc.List(identity.UserID, filter, page)
			if err != nil {
				log.WithError(err).Error("list notifications")
				server.WriteError(w, err)
				return
			}
			server.WriteJSON(w, http.StatusOK, server.Document{
				Data: TransformSlice(ms),
				Meta: page.Meta(total),
			})
		})

		// POST /notifications/{id}/read — mark a single notification read.
		r.Post("/notifications/{id}/read", func(w http.ResponseWriter, req *http.Request) {
			identity := auth.IdentityFromContext(req.Context())
			if identity.UserID == "" {
				server.WriteError(w, server.ErrUnauthorized)
				return
			}
			id := chi.URLParam(req, "id")
			if err := proc.MarkRead(identity.UserID, id); err != nil {
				server.WriteError(w, err)
				return
			}
			w.WriteHeader(http.StatusNoContent)
		})

		// POST /notifications/read-all — mark all of the caller's unread read.
		r.Post("/notifications/read-all", func(w http.ResponseWriter, req *http.Request) {
			identity := auth.IdentityFromContext(req.Context())
			if identity.UserID == "" {
				server.WriteError(w, server.ErrUnauthorized)
				return
			}
			if err := proc.MarkAllRead(identity.UserID); err != nil {
				log.WithError(err).Error("mark all notifications read")
				server.WriteError(w, err)
				return
			}
			w.WriteHeader(http.StatusNoContent)
		})
	}
}
