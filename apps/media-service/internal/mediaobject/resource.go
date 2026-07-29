package mediaobject

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/sirupsen/logrus"

	"gorm.io/gorm"

	"github.com/jtumidanski/myfleet/packages/shared-go/auth"
	"github.com/jtumidanski/myfleet/packages/shared-go/server"
)

// requireWrite allows member|owner; viewer is read-only (403). media-service
// trusts the token's role claim (design §9).
func requireWrite(id auth.Identity) error {
	if id.Role == "member" || id.Role == "owner" {
		return nil
	}
	return server.ErrForbidden
}

// InitializeRoutes wires the JWT-protected media-object endpoints. Paths are
// bare (the gateway strips /api/media). Event emission uses the transactional
// outbox (design A8); no Kafka producer is needed here.
func InitializeRoutes(log logrus.FieldLogger, db *gorm.DB, st Presigner) func(chi.Router) {
	proc := NewProcessor(log, NewProvider(db), NewAdministrator(db), st)
	return func(r chi.Router) {
		// POST /media — init upload: create row (uploaded) + presigned PUT URL.
		r.Post("/media", server.RegisterInputHandler(func(w http.ResponseWriter, req *http.Request, attrs struct {
			ContentType      string `json:"contentType"`
			OriginalFilename string `json:"originalFilename"`
		},
		) {
			identity := auth.IdentityFromContext(req.Context())
			if identity.ActiveFleetID == "" {
				server.WriteError(w, server.ErrForbidden)
				return
			}
			if err := requireWrite(identity); err != nil {
				server.WriteError(w, err)
				return
			}
			m, uploadURL, err := proc.InitUpload(identity.ActiveFleetID, identity.UserID, attrs.ContentType, attrs.OriginalFilename)
			if err != nil {
				log.WithError(err).Error("media init upload failed")
				server.WriteError(w, err)
				return
			}
			server.WriteJSON(w, http.StatusCreated, server.Document{Data: TransformWithUploadURL(m, uploadURL)})
		}))

		// POST /media/{id}/confirm — uploaded→processing + publish media.uploaded.
		r.Post("/media/{id}/confirm", func(w http.ResponseWriter, req *http.Request) {
			identity := auth.IdentityFromContext(req.Context())
			id := chi.URLParam(req, "id")
			if err := requireWrite(identity); err != nil {
				server.WriteError(w, err)
				return
			}
			m, err := proc.Confirm(req.Context(), id, identity.ActiveFleetID)
			if err != nil {
				server.WriteError(w, err)
				return
			}
			server.WriteJSON(w, http.StatusOK, server.Document{Data: Transform(m)})
		})

		// GET /media/{id} — metadata, fleet-scoped.
		r.Get("/media/{id}", func(w http.ResponseWriter, req *http.Request) {
			identity := auth.IdentityFromContext(req.Context())
			id := chi.URLParam(req, "id")
			m, err := proc.GetByID(id, identity.ActiveFleetID)
			if err != nil {
				server.WriteError(w, err)
				return
			}
			server.WriteJSON(w, http.StatusOK, server.Document{Data: Transform(m)})
		})

		// GET /media/{id}/download — short-lived presigned GET after authz.
		r.Get("/media/{id}/download", func(w http.ResponseWriter, req *http.Request) {
			identity := auth.IdentityFromContext(req.Context())
			id := chi.URLParam(req, "id")
			m, err := proc.GetByID(id, identity.ActiveFleetID)
			if err != nil {
				server.WriteError(w, err)
				return
			}
			url, err := proc.DownloadURL(id, identity.ActiveFleetID)
			if err != nil {
				server.WriteError(w, err)
				return
			}
			server.WriteJSON(w, http.StatusOK, server.Document{Data: TransformWithDownloadURL(m, url)})
		})

		// DELETE /media/{id} — soft delete (deleted_at + purge_after = +5d).
		r.Delete("/media/{id}", func(w http.ResponseWriter, req *http.Request) {
			identity := auth.IdentityFromContext(req.Context())
			id := chi.URLParam(req, "id")
			if err := requireWrite(identity); err != nil {
				server.WriteError(w, err)
				return
			}
			if err := proc.SoftDelete(id, identity.ActiveFleetID); err != nil {
				server.WriteError(w, err)
				return
			}
			w.WriteHeader(http.StatusNoContent)
		})
	}
}
