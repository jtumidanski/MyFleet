package mileage

import (
	"net/http"
	"strings"
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

// parseSources parses the comma-separated ?source= value: entries are trimmed,
// empties dropped, duplicates removed. An absent or empty value yields nil (no
// filter); any entry that is not a valid source is server.ErrValidation.
func parseSources(raw string) ([]string, error) {
	var out []string
	seen := map[string]bool{}
	for _, part := range strings.Split(raw, ",") {
		s := strings.TrimSpace(part)
		if s == "" || seen[s] {
			continue
		}
		if !ValidSource(s) {
			return nil, server.ErrValidation
		}
		seen[s] = true
		out = append(out, s)
	}
	return out, nil
}

// InitializeRoutes wires the JWT-protected mileage endpoints.
// vehicleAccessor is used to resolve fleetID and currentMileage from a vehicleID.
// adm appends readings and advances the vehicle odometer in one transaction.
func InitializeRoutes(log logrus.FieldLogger, db *gorm.DB, vehicleAccessor VehicleAccessor, adm Administrator) func(chi.Router) {
	proc := NewProcessor(log, db, NewProvider(db), adm)
	return func(r chi.Router) {
		// GET /vehicles/{id}/mileage — list mileage records (newest first, paged)
		// Supports ?from= and ?to= as RFC3339 range filters on recorded_at, and
		// ?source= as a comma-separated list of fuel|maintenance|manual.
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

			sources, err := parseSources(req.URL.Query().Get("source"))
			if err != nil {
				server.WriteError(w, server.ErrValidation)
				return
			}

			page := server.ParsePage(req)
			ms, total, err := proc.ListByVehicle(vehicleID, ListFilter{From: from, To: to, Sources: sources}, page)
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
		},
		) {
			identity := auth.IdentityFromContext(req.Context())
			vehicleID := chi.URLParam(req, "id")

			// Resolve vehicle for fleet-scoping authz.
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
				SetSource(SourceManual).
				SetCreatedByUserID(identity.UserID).
				Build()

			created, err := proc.Append(rec)
			if err != nil {
				server.WriteError(w, err)
				return
			}

			server.WriteJSON(w, http.StatusCreated, server.Document{
				Data: Transform(created),
			})
		}))
	}
}
