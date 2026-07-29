package preferences

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/sirupsen/logrus"
	"gorm.io/gorm"

	"github.com/jtumidanski/myfleet/packages/shared-go/auth"
	"github.com/jtumidanski/myfleet/packages/shared-go/server"
)

// InitializeRoutes wires the JWT-protected notification-preference endpoints.
// Paths are bare (the gateway strips /api/notifications). Every endpoint is
// scoped to the token's user id.
func InitializeRoutes(log logrus.FieldLogger, db *gorm.DB) func(chi.Router) {
	proc := NewProcessor(log, NewProvider(db), NewAdministrator(db))
	return func(r chi.Router) {
		// GET /notification-preferences — the caller's stored preference rows.
		r.Get("/notification-preferences", func(w http.ResponseWriter, req *http.Request) {
			identity := auth.IdentityFromContext(req.Context())
			if identity.UserID == "" {
				server.WriteError(w, server.ErrUnauthorized)
				return
			}
			ms, err := proc.List(identity.UserID)
			if err != nil {
				log.WithError(err).Error("list notification preferences")
				server.WriteError(w, err)
				return
			}
			server.WriteJSON(w, http.StatusOK, server.Document{Data: TransformSlice(ms)})
		})

		// PUT /notification-preferences — upsert one (type, inAppEnabled) toggle.
		r.Put("/notification-preferences", server.RegisterInputHandler(func(w http.ResponseWriter, req *http.Request, attrs struct {
			Type         string `json:"type"`
			InAppEnabled *bool  `json:"inAppEnabled"`
		},
		) {
			identity := auth.IdentityFromContext(req.Context())
			if identity.UserID == "" {
				server.WriteError(w, server.ErrUnauthorized)
				return
			}
			if attrs.Type == "" || attrs.InAppEnabled == nil {
				server.WriteError(w, server.ErrValidation)
				return
			}
			m, err := proc.Set(identity.UserID, attrs.Type, *attrs.InAppEnabled)
			if err != nil {
				log.WithError(err).Error("upsert notification preference")
				server.WriteError(w, err)
				return
			}
			server.WriteJSON(w, http.StatusOK, server.Document{Data: Transform(m)})
		}))
	}
}
