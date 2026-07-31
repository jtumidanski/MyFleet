package mediaobject

import (
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

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
func InitializeRoutes(log logrus.FieldLogger, db *gorm.DB, st ObjectStore, maxUploadBytes int64, allow Allowlist) func(chi.Router) {
	proc := NewProcessor(log, NewProvider(db), NewAdministrator(db), st, allow)
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
		r.Get("/media/{id}/content", func(w http.ResponseWriter, req *http.Request) {
			identity := auth.IdentityFromContext(req.Context())
			id := chi.URLParam(req, "id")
			m, rc, err := proc.Content(req.Context(), id, identity.ActiveFleetID)
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
			// The Content-Type is re-resolved through the allowlist on every
			// read rather than trusting the stored value, so shrinking the
			// allowlist retroactively downgrades already-stored objects and
			// rows created before the allowlist existed are covered too
			// (design D15, PRD FR-DL-4).
			ct, class := allow.Resolve(m.ContentType())
			w.Header().Set("Content-Type", ct)
			// nosniff on EVERY response, both classes (PRD FR-DL-1). Together
			// with attachment on documents this is what prevents an uploaded
			// file from executing in the application's origin; neither alone
			// is sufficient.
			w.Header().Set("X-Content-Type-Options", "nosniff")
			w.Header().Set("Content-Disposition",
				ContentDisposition(class, m.OriginalFilename(), m.ID()))
			if size := m.Size(); size > 0 {
				w.Header().Set("Content-Length", strconv.FormatInt(size, 10))
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

// MaxInternalLookupIDs bounds a single /internal/media lookup. The endpoint is
// unauthenticated, so its input must be bounded; fleet-service's own
// maintenancerecord.MaxDocuments (10) is the authoritative per-record cap and
// this is the defensive ceiling behind it.
const MaxInternalLookupIDs = 50

// InitializeInternalRoutes wires the network-restricted internal endpoint.
// Register this initializer WITHOUT JWT middleware, exactly as fleet-service
// does for membership.InitializeInternalRoutes.
//
// GET /internal/media?fleet_id=<uuid>&ids=<id>,<id>,… returns the requested IDs
// that are active AND belong to fleet_id. fleet-service compares the returned
// set against the requested set to prove a record's documentMediaIds are its
// caller's own media (design D6, PRD FR-DOC-6). A missing ID is
// indistinguishable between "does not exist", "was deleted" and "belongs to
// another fleet" — which is the non-disclosure property api-contracts §3 asks
// for, and it falls out of the shape rather than needing handler-side care.
//
// SECURITY: this route has no authentication. The priority-200 `internal-deny`
// rule in deploy/k8s/overlays/main/ingressroute.yaml is what keeps it off the
// public internet (design D20). Without it this is an unauthenticated
// cross-fleet media-existence oracle. The two ship together; never separately.
func InitializeInternalRoutes(log logrus.FieldLogger, db *gorm.DB) func(chi.Router) {
	prov := NewProvider(db)
	return func(r chi.Router) {
		r.Get("/internal/media", func(w http.ResponseWriter, req *http.Request) {
			fleetID := req.URL.Query().Get("fleet_id")
			if fleetID == "" {
				server.WriteError(w, server.ErrValidation)
				return
			}

			ids := splitInternalIDs(req.URL.Query().Get("ids"))
			if len(ids) > MaxInternalLookupIDs {
				server.WriteError(w, server.ErrValidation)
				return
			}
			if len(ids) == 0 {
				server.WriteJSON(w, http.StatusOK, InternalMediaResponse{Media: []InternalMedia{}})
				return
			}

			ms, err := prov.ListActiveByFleetAndIDs(fleetID, ids)
			if err != nil {
				log.WithError(err).Error("internal media lookup")
				server.WriteError(w, err)
				return
			}
			server.WriteJSON(w, http.StatusOK, InternalMediaResponse{Media: TransformInternalMedia(ms)})
		})
	}
}

// splitInternalIDs parses the comma-separated ids parameter, dropping empty
// segments so a trailing comma or a doubled separator is not a lookup for "".
func splitInternalIDs(raw string) []string {
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if id := strings.TrimSpace(p); id != "" {
			out = append(out, id)
		}
	}
	return out
}
