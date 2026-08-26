package admin

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/sirupsen/logrus"
	"gorm.io/gorm"

	"github.com/jtumidanski/myfleet/packages/shared-go/server"
)

// PurgeRequest is the one body shape both downstream services accept. The
// media_ids field of media-service's twin has no equivalent here: notifications
// are reachable from a fleet id alone.
type PurgeRequest struct {
	OperationID string   `json:"operation_id"`
	Scope       Scope    `json:"scope"`
	FleetIDs    []string `json:"fleet_ids,omitempty"`
}

type affectedResponse struct {
	Affected map[string]int `json:"affected"`
}

type statsResponse struct {
	Notifications int `json:"notifications"`
}

// InitializeInternalRoutes wires notification-service's slice of the purge
// protocol. Register WITHOUT JWT middleware.
//
// SECURITY: these routes have no authentication and they DELETE DATA, and
// unlike fleet-service and media-service this service is not safe by accident.
// notifications-stripprefix strips the FULL /api/notifications prefix, so a
// public request to /api/notifications/internal/admin/purge would arrive here as
// /internal/admin/purge. The priority-200 internal-deny rule matching
// ^/+api/+notifications[^/]*/*internal in
// deploy/k8s/overlays/main/ingressroute.yaml is the only thing keeping them off
// the public internet. The two ship together; never separately (design F2).
func InitializeInternalRoutes(log logrus.FieldLogger, db *gorm.DB) func(chi.Router) {
	return func(r chi.Router) {
		r.Get("/internal/admin/stats", func(w http.ResponseWriter, req *http.Request) {
			var n int64
			if err := db.Raw(`SELECT count(*) FROM notification.notifications WHERE deleted_at IS NULL`).
				Scan(&n).Error; err != nil {
				log.WithError(err).Error("internal admin notification count")
				server.WriteError(w, err)
				return
			}
			server.WriteJSON(w, http.StatusOK, statsResponse{Notifications: int(n)})
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
				log.WithError(terr).WithField("operation_id", body.OperationID).Error("notification admin stamp")
				server.WriteError(w, terr)
				return
			}
			server.WriteJSON(w, http.StatusOK, affectedResponse{Affected: affected})
		})

		// SECURITY: unauthenticated, like its neighbours, and this service is
		// the one that is not safe by accident — notifications-stripprefix
		// strips the FULL /api/notifications prefix, so the priority-200
		// internal-deny rule matching ^/+api/+notifications[^/]*/*internal is
		// the ONLY thing keeping this off the public internet.
		// tools/check-manifests.sh asserts it on both entrypoints and runs as
		// part of `make ci`.
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
				affected, rerr = Reassign(tx, body.VehicleIDs, body.DestinationFleetID)
				return rerr
			}); terr != nil {
				log.WithError(terr).WithField("destination_fleet_id", body.DestinationFleetID).
					Error("notification admin reassign")
				server.WriteError(w, terr)
				return
			}
			server.WriteJSON(w, http.StatusOK, affectedResponse{Affected: affected})
		})

		r.Delete("/internal/admin/purge/{opId}", func(w http.ResponseWriter, req *http.Request) {
			opID := chi.URLParam(req, "opId")
			if terr := db.Transaction(func(tx *gorm.DB) error { return Restore(tx, opID) }); terr != nil {
				log.WithError(terr).WithField("operation_id", opID).Error("notification admin restore")
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
			var deleted map[string]int
			if terr := db.Transaction(func(tx *gorm.DB) error {
				var rerr error
				deleted, rerr = Reap(tx, opID)
				return rerr
			}); terr != nil {
				log.WithError(terr).WithField("operation_id", opID).Error("notification admin reap")
				server.WriteError(w, terr)
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
	}
	return Root{}, server.Detailed(server.ErrValidation, "unsupported scope")
}
