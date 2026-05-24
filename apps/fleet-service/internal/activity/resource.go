package activity

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/sirupsen/logrus"
	"gorm.io/gorm"

	"github.com/jtumidanski/myfleet/packages/shared-go/auth"
	"github.com/jtumidanski/myfleet/packages/shared-go/server"

	"github.com/jtumidanski/myfleet/apps/fleet-service/internal/authz"
	"github.com/jtumidanski/myfleet/apps/fleet-service/internal/vehicle"
)

// VehicleAccessor resolves a vehicle for fleet-scoping the per-vehicle timeline.
// Satisfied by *vehicle.Processor.
type VehicleAccessor interface {
	GetByID(id string) (vehicle.Model, error)
}

// InitializeRoutes wires the JWT-protected, read-only activity-feed endpoints.
// vehicleAccessor resolves the owning vehicle's fleetID for the /vehicles route.
func InitializeRoutes(log logrus.FieldLogger, db *gorm.DB, vehicleAccessor VehicleAccessor) func(chi.Router) {
	proc := NewProcessor(log, NewProvider(db))
	return func(r chi.Router) {
		// GET /fleets/{id}/activity — fleet activity feed (paged, newest first).
		r.Get("/fleets/{id}/activity", func(w http.ResponseWriter, req *http.Request) {
			identity := auth.IdentityFromContext(req.Context())
			fleetID := chi.URLParam(req, "id")
			if err := authz.RequireSameFleet(identity, fleetID); err != nil {
				server.WriteError(w, err)
				return
			}
			page := server.ParsePage(req)
			ms, total, err := proc.ListByFleet(fleetID, page)
			if err != nil {
				log.WithError(err).Error("list fleet activity")
				server.WriteError(w, err)
				return
			}
			resources, err := TransformSlice(ms)
			if err != nil {
				log.WithError(err).Error("transform fleet activity")
				server.WriteError(w, err)
				return
			}
			server.WriteJSON(w, http.StatusOK, server.Document{Data: resources, Meta: page.Meta(total)})
		})

		// GET /vehicles/{id}/activity — per-vehicle timeline (paged, newest first).
		r.Get("/vehicles/{id}/activity", func(w http.ResponseWriter, req *http.Request) {
			identity := auth.IdentityFromContext(req.Context())
			vehicleID := chi.URLParam(req, "id")
			// Resolve the vehicle's fleet for the fleet-scoped authz (404 no-leak).
			v, err := vehicleAccessor.GetByID(vehicleID)
			if err != nil {
				server.WriteError(w, err)
				return
			}
			if err := authz.RequireSameFleet(identity, v.FleetID()); err != nil {
				server.WriteError(w, err)
				return
			}
			page := server.ParsePage(req)
			ms, total, err := proc.ListByVehicle(vehicleID, page)
			if err != nil {
				log.WithError(err).Error("list vehicle activity")
				server.WriteError(w, err)
				return
			}
			resources, err := TransformSlice(ms)
			if err != nil {
				log.WithError(err).Error("transform vehicle activity")
				server.WriteError(w, err)
				return
			}
			server.WriteJSON(w, http.StatusOK, server.Document{Data: resources, Meta: page.Meta(total)})
		})
	}
}
