package dashboard

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/sirupsen/logrus"
	"gorm.io/gorm"

	"github.com/jtumidanski/myfleet/packages/shared-go/auth"
	"github.com/jtumidanski/myfleet/packages/shared-go/server"

	"github.com/jtumidanski/myfleet/apps/fleet-service/internal/authz"
	"github.com/jtumidanski/myfleet/apps/fleet-service/internal/maintenanceschedule"
	"github.com/jtumidanski/myfleet/apps/fleet-service/internal/vehicle"
)

// ScheduleStateReader reads active schedules for a fleet (for overview counts).
// Satisfied by *maintenanceschedule.Processor.
type ScheduleStateReader interface {
	ListActiveByFleet(fleetID string) ([]maintenanceschedule.QueueRow, error)
}

// LastActivityReader returns the most-recent activity time for a vehicle.
// Satisfied by *activity.Processor.
type LastActivityReader interface {
	LastActivityByVehicle(vehicleID string) (time.Time, error)
}

// VehicleListReader lists vehicles for a fleet (for overview status derivation).
// Satisfied by *vehicle.Processor.
type VehicleListReader interface {
	GetByID(id string) (vehicle.Model, error)
	ListByFleet(fleetID string, page server.Page) ([]vehicle.Model, int, error)
}

// InitializeRoutes wires the JWT-protected dashboard endpoints.
func InitializeRoutes(
	log logrus.FieldLogger,
	db *gorm.DB,
	schedules ScheduleStateReader,
	activity LastActivityReader,
	vehicles VehicleListReader,
) func(chi.Router) {
	proc := NewProcessor(log, NewProvider(db))
	adm := NewAdministrator(db)
	agg := NewAggregateProvider(db)

	return func(r chi.Router) {
		// GET /fleets/{id}/dashboard — per-user dashboard layout.
		r.Get("/fleets/{id}/dashboard", func(w http.ResponseWriter, req *http.Request) {
			identity := auth.IdentityFromContext(req.Context())
			fleetID := chi.URLParam(req, "id")

			if err := authz.RequireSameFleet(identity, fleetID); err != nil {
				server.WriteError(w, err)
				return
			}

			d, err := proc.GetDashboard(fleetID, identity.UserID)
			if err != nil {
				log.WithError(err).Error("get dashboard")
				server.WriteError(w, err)
				return
			}
			server.WriteJSON(w, http.StatusOK, server.Document{Data: Transform(d)})
		})

		// PUT /fleets/{id}/dashboard — replace widget layout (per-user).
		r.Put("/fleets/{id}/dashboard", server.RegisterInputHandler(func(w http.ResponseWriter, req *http.Request, attrs struct {
			Widgets []WidgetInput `json:"widgets"`
		}) {
			identity := auth.IdentityFromContext(req.Context())
			fleetID := chi.URLParam(req, "id")

			if err := authz.RequireSameFleet(identity, fleetID); err != nil {
				server.WriteError(w, err)
				return
			}
			if err := authz.RequireWrite(identity); err != nil {
				server.WriteError(w, err)
				return
			}

			d, err := proc.ReplaceDashboard(adm, fleetID, identity.UserID, attrs.Widgets)
			if err != nil {
				log.WithError(err).Error("replace dashboard")
				server.WriteError(w, err)
				return
			}
			server.WriteJSON(w, http.StatusOK, server.Document{Data: Transform(d)})
		}))

		// GET /fleets/{id}/dashboard/spend-by-vehicle?from=&to=
		r.Get("/fleets/{id}/dashboard/spend-by-vehicle", func(w http.ResponseWriter, req *http.Request) {
			identity := auth.IdentityFromContext(req.Context())
			fleetID := chi.URLParam(req, "id")

			if err := authz.RequireSameFleet(identity, fleetID); err != nil {
				server.WriteError(w, err)
				return
			}

			from, to, err := parseWindow(req)
			if err != nil {
				server.WriteError(w, server.ErrValidation)
				return
			}

			rows, err := agg.SpendByVehicle(fleetID, from, to)
			if err != nil {
				log.WithError(err).Error("spend-by-vehicle aggregate")
				server.WriteError(w, err)
				return
			}
			server.WriteJSON(w, http.StatusOK, server.Document{Data: rows})
		})

		// GET /vehicles/{id}/dashboard/mileage-trends
		r.Get("/vehicles/{id}/dashboard/mileage-trends", func(w http.ResponseWriter, req *http.Request) {
			identity := auth.IdentityFromContext(req.Context())
			vehicleID := chi.URLParam(req, "id")

			v, err := vehicles.GetByID(vehicleID)
			if err != nil {
				server.WriteError(w, err)
				return
			}
			if err := authz.RequireSameFleet(identity, v.FleetID()); err != nil {
				server.WriteError(w, err)
				return
			}

			from, to, err := parseWindow(req)
			if err != nil {
				server.WriteError(w, server.ErrValidation)
				return
			}

			rows, err := agg.MileageTrends(vehicleID, from, to)
			if err != nil {
				log.WithError(err).Error("mileage-trends aggregate")
				server.WriteError(w, err)
				return
			}
			server.WriteJSON(w, http.StatusOK, server.Document{Data: rows})
		})

		// GET /fleets/{id}/dashboard/overview
		r.Get("/fleets/{id}/dashboard/overview", func(w http.ResponseWriter, req *http.Request) {
			identity := auth.IdentityFromContext(req.Context())
			fleetID := chi.URLParam(req, "id")

			if err := authz.RequireSameFleet(identity, fleetID); err != nil {
				server.WriteError(w, err)
				return
			}

			overview, err := computeOverview(fleetID, vehicles, schedules, activity)
			if err != nil {
				log.WithError(err).Error("dashboard overview")
				server.WriteError(w, err)
				return
			}
			server.WriteJSON(w, http.StatusOK, server.Document{Data: overview})
		})
	}
}

// parseWindow parses optional ?from= and ?to= query parameters (RFC3339).
// Returns zero times when the parameters are absent (unbounded).
func parseWindow(req *http.Request) (from, to time.Time, err error) {
	if s := req.URL.Query().Get("from"); s != "" {
		from, err = time.Parse(time.RFC3339, s)
		if err != nil {
			return
		}
	}
	if s := req.URL.Query().Get("to"); s != "" {
		to, err = time.Parse(time.RFC3339, s)
		if err != nil {
			return
		}
	}
	return
}
