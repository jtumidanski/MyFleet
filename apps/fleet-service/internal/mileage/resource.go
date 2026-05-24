package mileage

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/sirupsen/logrus"
	"gorm.io/gorm"

	"github.com/jtumidanski/myfleet/packages/shared-go/auth"
	"github.com/jtumidanski/myfleet/packages/shared-go/server"

	"github.com/jtumidanski/myfleet/apps/fleet-service/internal/authz"
	"github.com/jtumidanski/myfleet/apps/fleet-service/internal/vehicle"
)

// VehicleAccessor resolves a vehicle for fleet-scoping mileage routes.
// Satisfied by *vehicle.Processor.
type VehicleAccessor interface {
	GetByID(id string) (vehicle.Model, error)
}

// InitializeRoutes wires the JWT-protected mileage endpoints.
// vehicleAccessor is used to resolve fleetID and currentMileage from a vehicleID.
// updater is injected for OnAppend (satisfied by vehicle.Administrator).
func InitializeRoutes(log logrus.FieldLogger, db *gorm.DB, vehicleAccessor VehicleAccessor, updater VehicleMileageUpdater) func(chi.Router) {
	proc := NewProcessor(log, updater)
	adm := NewAdministrator(db)
	prov := NewProvider(db)
	return func(r chi.Router) {
		// GET /vehicles/{id}/mileage — list mileage records (chronological, paged)
		// Supports ?from= and ?to= as RFC3339 range filters on recorded_at.
		r.Get("/vehicles/{id}/mileage", func(w http.ResponseWriter, req *http.Request) {
			identity := auth.IdentityFromContext(req.Context())
			vehicleID := chi.URLParam(req, "id")

			v, err := vehicleAccessor.GetByID(vehicleID)
			if err != nil {
				server.WriteError(w, err)
				return
			}
			if err := authz.RequireSameFleet(identity, v.FleetID()); err != nil {
				server.WriteError(w, err)
				return
			}

			var from, to *time.Time
			if s := req.URL.Query().Get("from"); s != "" {
				t, err := time.Parse(time.RFC3339, s)
				if err != nil {
					server.WriteError(w, server.ErrValidation)
					return
				}
				from = &t
			}
			if s := req.URL.Query().Get("to"); s != "" {
				t, err := time.Parse(time.RFC3339, s)
				if err != nil {
					server.WriteError(w, server.ErrValidation)
					return
				}
				to = &t
			}

			page := server.ParsePage(req)
			ms, total, err := prov.ListByVehicle(vehicleID, from, to, page)
			if err != nil {
				server.WriteError(w, err)
				return
			}
			server.WriteJSON(w, http.StatusOK, server.Document{
				Data: TransformSlice(ms),
				Meta: page.Meta(total),
			})
		})

		// POST /vehicles/{id}/mileage — append a manual mileage record
		r.Post("/vehicles/{id}/mileage", server.RegisterInputHandler(func(w http.ResponseWriter, req *http.Request, attrs struct {
			Mileage    int    `json:"mileage"`
			RecordedAt string `json:"recordedAt"`
		}) {
			identity := auth.IdentityFromContext(req.Context())
			vehicleID := chi.URLParam(req, "id")

			// Resolve vehicle for fleet-scoping authz and current mileage baseline.
			v, err := vehicleAccessor.GetByID(vehicleID)
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

			if attrs.Mileage <= 0 {
				server.WriteError(w, server.ErrValidation)
				return
			}
			recordedAt := time.Now().UTC()
			if attrs.RecordedAt != "" {
				recordedAt, err = time.Parse(time.RFC3339, attrs.RecordedAt)
				if err != nil {
					server.WriteError(w, server.ErrValidation)
					return
				}
			}

			rec := NewBuilder().
				SetVehicleID(vehicleID).
				SetMileage(attrs.Mileage).
				SetRecordedAt(recordedAt).
				SetSource("manual").
				SetCreatedByUserID(identity.UserID).
				Build()

			created, err := adm.Insert(rec)
			if err != nil {
				server.WriteError(w, err)
				return
			}

			// Call OnAppend to advance or flag based on currentMileage.
			flagged, err := proc.OnAppend(created, v.CurrentMileage())
			if err != nil {
				server.WriteError(w, err)
				return
			}

			server.WriteJSON(w, http.StatusCreated, server.Document{
				Data: Transform(created.WithFlagged(flagged)),
			})
		}))
	}
}
