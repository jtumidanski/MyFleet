package vehiclemedia

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

// InitializeRoutes wires the vehicle media endpoints.
// vehicleProc is used to resolve fleetID from a vehicleID for same-fleet authz.
func InitializeRoutes(log logrus.FieldLogger, db *gorm.DB, vehicleProc VehicleGetter) func(chi.Router) {
	proc := NewProcessor(log, NewProvider(db), NewAdministrator(db))
	return func(r chi.Router) {
		// GET /vehicles/{id}/media — list media refs for a vehicle
		r.Get("/vehicles/{id}/media", func(w http.ResponseWriter, req *http.Request) {
			identity := auth.IdentityFromContext(req.Context())
			vehicleID := chi.URLParam(req, "id")

			v, err := vehicleProc.GetByID(vehicleID)
			if err != nil {
				server.WriteError(w, err)
				return
			}
			if err := authz.RequireSameFleet(identity, v.FleetID()); err != nil {
				server.WriteError(w, err)
				return
			}
			ms, err := proc.ListByVehicle(vehicleID)
			if err != nil {
				server.WriteError(w, err)
				return
			}
			server.WriteJSON(w, http.StatusOK, server.Document{Data: TransformSlice(ms)})
		})

		// POST /vehicles/{id}/media — add a media reference
		r.Post("/vehicles/{id}/media", server.RegisterInputHandler(func(w http.ResponseWriter, req *http.Request, attrs struct {
			MediaID   string `json:"mediaId"`
			SortOrder int    `json:"sortOrder"`
		}) {
			identity := auth.IdentityFromContext(req.Context())
			vehicleID := chi.URLParam(req, "id")

			v, err := vehicleProc.GetByID(vehicleID)
			if err != nil {
				server.WriteError(w, err)
				return
			}
			if err := authz.RequireSameFleet(identity, v.FleetID()); err != nil {
				server.WriteError(w, err)
				return
			}
			if err := authz.RequireWrite(identity); err != nil {
				server.WriteError(w, err)
				return
			}
			if attrs.MediaID == "" {
				server.WriteError(w, server.ErrValidation)
				return
			}
			m := NewBuilder().
				SetVehicleID(vehicleID).
				SetMediaID(attrs.MediaID).
				SetSortOrder(attrs.SortOrder).
				Build()
			created, err := proc.AddMedia(m)
			if err != nil {
				server.WriteError(w, err)
				return
			}
			server.WriteJSON(w, http.StatusCreated, server.Document{Data: Transform(created)})
		}))

		// PUT /vehicles/{id}/primary-image — handled by vehicle resource; this domain
		// owns the vehiclemedia side (SetPrimary across rows + mirror).
		// Route is registered here as an alternative entry point when vehiclemedia
		// is mounted separately; vehicle/resource.go delegates via vehiclemedia.Processor.
	}
}

// VehicleGetter is the subset of vehicle.Processor used for fleet-ID resolution.
type VehicleGetter interface {
	GetByID(id string) (vehicle.Model, error)
}
