package admin

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/sirupsen/logrus"
	"gorm.io/gorm"

	"github.com/jtumidanski/myfleet/packages/shared-go/server"
)

// ObjectRemover is the slice of storage.Client this package needs. Declaring the
// port here rather than importing the concrete client keeps the dependency
// one-way and makes the reap testable without MinIO.
type ObjectRemover interface {
	RemoveObject(ctx context.Context, key string) error
}

// PurgeRequest is the one body shape both downstream services accept.
type PurgeRequest struct {
	OperationID string   `json:"operation_id"`
	Scope       Scope    `json:"scope"`
	FleetIDs    []string `json:"fleet_ids,omitempty"`
	MediaIDs    []string `json:"media_ids,omitempty"`
}

type affectedResponse struct {
	Affected map[string]int `json:"affected"`
}

type statsResponse struct {
	MediaObjects int `json:"media_objects"`
}

// InitializeInternalRoutes wires media-service's slice of the purge protocol.
// Register WITHOUT JWT middleware.
//
// SECURITY: these routes have no authentication and they DELETE DATA. The
// priority-200 internal-deny rule matching ^/+api/+media[^/]*/*internal in
// deploy/k8s/overlays/main/ingressroute.yaml is what keeps them off the public
// internet. It predates this task because /internal/media already existed —
// confirm it still matches before shipping; never assume it (design F2).
func InitializeInternalRoutes(log logrus.FieldLogger, db *gorm.DB, store ObjectRemover) func(chi.Router) {
	return func(r chi.Router) {
		r.Get("/internal/admin/stats", func(w http.ResponseWriter, req *http.Request) {
			var n int64
			if err := db.Raw(`SELECT count(*) FROM media.media_objects WHERE deleted_at IS NULL`).
				Scan(&n).Error; err != nil {
				log.WithError(err).Error("internal admin media count")
				server.WriteError(w, err)
				return
			}
			server.WriteJSON(w, http.StatusOK, statsResponse{MediaObjects: int(n)})
		})

		r.Post("/internal/admin/purge", func(w http.ResponseWriter, req *http.Request) {
			var body PurgeRequest
			if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
				server.WriteError(w, server.ErrValidation)
				return
			}
			root, err := rootFrom(body)
			if err != nil {
				server.WriteError(w, err)
				return
			}
			var affected map[string]int
			if terr := db.Transaction(func(tx *gorm.DB) error {
				var serr error
				affected, serr = Stamp(tx, root, body.OperationID, time.Now().UTC())
				return serr
			}); terr != nil {
				log.WithError(terr).WithField("operation_id", body.OperationID).Error("media admin stamp")
				server.WriteError(w, terr)
				return
			}
			server.WriteJSON(w, http.StatusOK, affectedResponse{Affected: affected})
		})

		// SECURITY: like its neighbours this route has no authentication, and
		// it is a HIGHER-value target than they are — it can move any media
		// object into any fleet, which is a read-access grant rather than a
		// deletion. It is kept off the public internet by two independent
		// mechanisms: the priority-200 internal-deny rule matching
		// ^/+api/+media[^/]*/*internal (an ipAllowList of 255.255.255.255/32,
		// which no client can match), and media-stripprefix stripping only
		// /api, so a public /api/media/internal/... would arrive here as
		// /media/internal/... and match no registered route.
		// tools/check-manifests.sh asserts the first of those on both
		// entrypoints and runs as part of `make ci`.
		r.Post("/internal/admin/reassign-fleet", func(w http.ResponseWriter, req *http.Request) {
			var body ReassignRequest
			if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
				server.WriteError(w, server.ErrValidation)
				return
			}
			if err := reassignRootFrom(body); err != nil {
				server.WriteError(w, err)
				return
			}
			var affected map[string]int
			if terr := db.Transaction(func(tx *gorm.DB) error {
				var rerr error
				affected, rerr = Reassign(tx, body.MediaIDs, body.SourceFleetID, body.DestinationFleetID)
				return rerr
			}); terr != nil {
				log.WithError(terr).WithField("destination_fleet_id", body.DestinationFleetID).
					Error("media admin reassign")
				server.WriteError(w, terr)
				return
			}
			server.WriteJSON(w, http.StatusOK, affectedResponse{Affected: affected})
		})

		r.Delete("/internal/admin/purge/{opId}", func(w http.ResponseWriter, req *http.Request) {
			opID := chi.URLParam(req, "opId")
			if terr := db.Transaction(func(tx *gorm.DB) error { return Restore(tx, opID) }); terr != nil {
				log.WithError(terr).WithField("operation_id", opID).Error("media admin restore")
				server.WriteError(w, terr)
				return
			}
			affected, err := CountByOperation(db, opID)
			if err != nil {
				server.WriteError(w, err)
				return
			}
			server.WriteJSON(w, http.StatusOK, affectedResponse{Affected: affected})
		})

		r.Post("/internal/admin/reap/{opId}", func(w http.ResponseWriter, req *http.Request) {
			opID := chi.URLParam(req, "opId")

			// Objects first, rows second. The rows are the only record of which
			// objects exist, so deleting them first would strand the bytes in
			// the bucket with nothing left pointing at them.
			keys, err := ReapableObjectKeys(db, opID)
			if err != nil {
				log.WithError(err).WithField("operation_id", opID).Error("list reapable objects")
				server.WriteError(w, err)
				return
			}
			failed := map[string]bool{}
			for _, k := range keys {
				if rerr := store.RemoveObject(req.Context(), k.Key); rerr != nil {
					// An object already absent is NOT an error
					// (FR-ADMIN-RESTORE-5) — storage.Client.RemoveObject is
					// idempotent on a missing key, so anything that reaches here
					// is a real failure. Keep the owning row so the next tick
					// retries.
					log.WithError(rerr).WithFields(logrus.Fields{
						"operation_id": opID, "object_key": k.Key,
					}).Warn("remove minio object during admin reap failed")
					failed[k.MediaObjectID] = true
				}
			}

			// Spare the objects whose bytes are still in the bucket, and their
			// variants with them — WITHOUT detaching them from the operation.
			// They keep purge_operation_id so the next tick can find them again;
			// clearing it would strand the row and its bytes permanently, since
			// an admin stamp leaves purge_after NULL and the ordinary sweep
			// therefore never sees it either.
			spare := make([]string, 0, len(failed))
			for id := range failed {
				spare = append(spare, id)
			}

			var deleted map[string]int
			if terr := db.Transaction(func(tx *gorm.DB) error {
				var rerr error
				deleted, rerr = ReapSparing(tx, opID, spare)
				return rerr
			}); terr != nil {
				log.WithError(terr).WithField("operation_id", opID).Error("media admin reap")
				server.WriteError(w, terr)
				return
			}

			if len(failed) > 0 {
				// Report failure so fleet-service leaves the operation pending
				// and the next hourly tick retries the survivors.
				server.WriteError(w, server.Detailed(server.ErrConflict,
					"some stored objects could not be removed; their rows were kept for retry"))
				return
			}
			server.WriteJSON(w, http.StatusOK, affectedResponse{Affected: deleted})
		})
	}
}

// rootFrom validates the request body into a Root, or returns 422.
func rootFrom(body PurgeRequest) (Root, error) {
	if body.OperationID == "" {
		return Root{}, server.Detailed(server.ErrValidation, "operation_id is required")
	}
	switch body.Scope {
	case ScopeSystem:
		return Root{Scope: ScopeSystem}, nil
	case ScopeFleet:
		if len(body.FleetIDs) == 0 {
			return Root{}, server.Detailed(server.ErrValidation, "fleet scope requires fleet_ids")
		}
		return Root{Scope: ScopeFleet, FleetIDs: body.FleetIDs}, nil
	case ScopeMediaIDs:
		if len(body.MediaIDs) == 0 {
			return Root{}, server.Detailed(server.ErrValidation, "media_ids scope requires media_ids")
		}
		return Root{Scope: ScopeMediaIDs, MediaIDs: body.MediaIDs}, nil
	}
	return Root{}, server.Detailed(server.ErrValidation, "unsupported scope")
}
