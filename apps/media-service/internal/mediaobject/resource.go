package mediaobject

import (
	"errors"
	"io"
	"net/http"
	"strconv"

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

// classifyUploadError maps the error http.MaxBytesReader produces once a body
// exceeds its cap to the 413 sentinel; every other error passes through
// unchanged so the caller's existing error handling (404/409/500, ...) still
// applies. Split out as its own function so the mapping is unit-testable
// without standing up a full HTTP round trip.
func classifyUploadError(err error) error {
	var maxErr *http.MaxBytesError
	if errors.As(err, &maxErr) {
		return server.ErrRequestEntityTooLarge
	}
	return err
}

// InitializeRoutes wires the JWT-protected media-object endpoints. Paths
// carry their own /media segment (the gateway strips only /api). Event
// emission uses the transactional outbox (design A8); no Kafka producer is
// needed here.
func InitializeRoutes(log logrus.FieldLogger, db *gorm.DB, st ObjectStore, variants VariantLookup, maxUploadBytes int64) func(chi.Router) {
	proc := NewProcessor(log, NewProvider(db), NewAdministrator(db), st, variants)
	return func(r chi.Router) {
		// POST /media — init upload: create the row in the uploaded state.
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
			m, err := proc.InitUpload(identity.ActiveFleetID, identity.UserID, attrs.ContentType, attrs.OriginalFilename)
			if err != nil {
				log.WithError(err).Error("media init upload failed")
				server.WriteError(w, err)
				return
			}
			server.WriteJSON(w, http.StatusCreated, server.Document{Data: Transform(m)})
		}))

		// PUT /media/{id}/content — proxy the raw bytes to object storage.
		// Bounded by maxUploadBytes so a client cannot stream unbounded data;
		// Cloudflare also caps request bodies at the edge for the public host.
		r.Put("/media/{id}/content", func(w http.ResponseWriter, req *http.Request) {
			identity := auth.IdentityFromContext(req.Context())
			id := chi.URLParam(req, "id")
			if err := requireWrite(identity); err != nil {
				server.WriteError(w, err)
				return
			}
			// A client that advertises more than the cap has told us up front
			// the body is too large: answer 413 immediately, before any bytes
			// are read and before a multipart upload is opened against object
			// storage. Downgrading this to the unknown-length sentinel instead
			// would hand the SDK a body it must size-guess for, which is both
			// wasteful and (historically) the path that let one request
			// allocate more memory than the pod is allowed.
			if req.ContentLength > maxUploadBytes {
				server.WriteError(w, server.ErrRequestEntityTooLarge)
				return
			}

			body := http.MaxBytesReader(w, req.Body, maxUploadBytes)
			defer func() { _ = body.Close() }()

			// Content-Length is advisory here: MaxBytesReader is the real
			// bound on bytes actually read. A chunked body has no
			// Content-Length and arrives as -1; that is legitimate, and the
			// store bounds the SDK's buffer for it (storage.uploadPartSize).
			size := req.ContentLength
			if size < 0 {
				size = -1
			}
			m, err := proc.StoreContent(req.Context(), id, identity.ActiveFleetID, body, size)
			if err != nil {
				if mapped := classifyUploadError(err); errors.Is(mapped, server.ErrRequestEntityTooLarge) {
					server.WriteError(w, mapped)
					return
				}
				log.WithError(err).Error("media content upload failed")
				server.WriteError(w, err)
				return
			}
			server.WriteJSON(w, http.StatusOK, server.Document{Data: Transform(m)})
		})

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

		// GET /media/{id}/content — stream the bytes after authz. Proxied, not
		// presigned: MinIO is a shared cluster service and is never exposed
		// outside the cluster.
		//
		// The optional ?variant= parameter selects a stored rendition
		// (thumbnail/display); omitting it serves the original, byte-for-byte
		// as before. An unrecognised value is a 400, never a silent fallback —
		// a typo would otherwise ship multi-megabyte responses undetected.
		r.Get("/media/{id}/content", func(w http.ResponseWriter, req *http.Request) {
			identity := auth.IdentityFromContext(req.Context())
			id := chi.URLParam(req, "id")
			v, err := ParseContentVariant(req.URL.Query().Get("variant"))
			if err != nil {
				server.WriteError(w, err)
				return
			}
			info, rc, err := proc.Content(req.Context(), id, identity.ActiveFleetID, v)
			if err != nil {
				server.WriteError(w, err)
				return
			}
			defer func() { _ = rc.Close() }()

			// proc.Content has already proven the object is readable (the
			// store issues its GET before returning), so committing 200 here
			// no longer risks answering an empty body for bytes that were
			// never stored. A copy that fails *after* this point is a genuine
			// mid-stream failure; the status is on the wire and cannot be
			// changed, so it is logged and the client sees a short read.
			//
			// Headers come from ContentInfo, i.e. from the bytes about to be
			// written — a variant is re-encoded and has its own content type,
			// and its length is unknown, so Content-Length is omitted for it
			// rather than describing the original.
			if info.ContentType != "" {
				w.Header().Set("Content-Type", info.ContentType)
			}
			if info.Size > 0 {
				w.Header().Set("Content-Length", strconv.FormatInt(info.Size, 10))
			}
			// Per-fleet authorized bytes — never store in a shared cache.
			w.Header().Set("Cache-Control", "private, max-age=300")
			w.WriteHeader(http.StatusOK)
			if _, err := io.Copy(w, rc); err != nil {
				// Headers are already written, so the status cannot be changed;
				// log and let the client see a truncated body.
				log.WithError(err).Warn("media content stream interrupted")
			}
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
