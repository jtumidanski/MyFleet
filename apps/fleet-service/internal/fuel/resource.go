package fuel

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/sirupsen/logrus"
	"gorm.io/gorm"

	"github.com/jtumidanski/myfleet/packages/shared-go/auth"
	"github.com/jtumidanski/myfleet/packages/shared-go/events"
	"github.com/jtumidanski/myfleet/packages/shared-go/server"

	"github.com/jtumidanski/myfleet/apps/fleet-service/internal/authz"
	"github.com/jtumidanski/myfleet/apps/fleet-service/internal/vehicle"
)

// VehicleAccessor resolves a vehicle for fleet-scoping fuel routes.
// Satisfied by *vehicle.Processor.
type VehicleAccessor interface {
	GetByID(id string) (vehicle.Model, error)
}

// InitializeRoutes wires the JWT-protected fuel log endpoints.
// vehicleAccessor resolves the owning vehicle's fleetID + currentMileage for
// authz and for the OnAppend mileage rule.
func InitializeRoutes(log logrus.FieldLogger, db *gorm.DB, vehicleAccessor VehicleAccessor) func(chi.Router) {
	proc := NewProcessor(log, NewProvider(db))
	adm := NewAdministrator(db)
	loggingDeps := NewLoggingDeps(db, events.NoopProducer{})
	return func(r chi.Router) {
		// GET /vehicles/{id}/fuel-logs — list fuel logs (newest first, paged).
		r.Get("/vehicles/{id}/fuel-logs", func(w http.ResponseWriter, req *http.Request) {
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

			page := server.ParsePage(req)
			ms, total, err := proc.ListByVehicle(vehicleID, page)
			if err != nil {
				log.WithError(err).Error("list fuel logs")
				server.WriteError(w, err)
				return
			}
			server.WriteJSON(w, http.StatusOK, server.Document{
				Data: TransformSlice(ms),
				Meta: page.Meta(total),
			})
		})

		// POST /vehicles/{id}/fuel-logs — log a fuel entry.
		// DerivePrice fills the missing of {pricePerGallon, totalCost} (design §10.5).
		// Orchestration: fuel insert + mileage insert + current_mileage advance in ONE tx.
		r.Post("/vehicles/{id}/fuel-logs", server.RegisterInputHandler(func(w http.ResponseWriter, req *http.Request, attrs struct {
			Date           string  `json:"date"`
			Mileage        int     `json:"mileage"`
			Gallons        float64 `json:"gallons"`
			TotalCost      float64 `json:"totalCost"`
			PricePerGallon float64 `json:"pricePerGallon"`
		}) {
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
			if err := authz.RequireWrite(identity); err != nil {
				server.WriteError(w, err)
				return
			}

			// Parse date; default to now.
			date := time.Now().UTC()
			if attrs.Date != "" {
				date, err = time.Parse(time.RFC3339, attrs.Date)
				if err != nil {
					server.WriteError(w, server.ErrValidation)
					return
				}
			}

			// Derive the missing price/total (422 if neither is provided or gallons<=0).
			derived, err := DerivePrice(attrs.PricePerGallon, attrs.TotalCost, attrs.Gallons)
			if err != nil {
				server.WriteError(w, err)
				return
			}

			m, err := NewBuilder().
				SetVehicleID(vehicleID).
				SetDate(date).
				SetMileage(attrs.Mileage).
				SetGallons(attrs.Gallons).
				SetTotalCost(derived.TotalCost).
				SetPricePerGallon(derived.PricePerGallon).
				SetCreatedByUserID(identity.UserID).
				Build()
			if err != nil {
				server.WriteError(w, err)
				return
			}

			// Run the cross-domain transaction: fuel + mileage in one tx.
			created, err := loggingDeps.LogInTransaction(log, LogInput{
				FuelLog: m,
				FleetID: v.FleetID(),
			})
			if err != nil {
				log.WithError(err).Error("log fuel entry")
				server.WriteError(w, err)
				return
			}
			server.WriteJSON(w, http.StatusCreated, server.Document{Data: Transform(created)})
		}))

		// GET /fuel-logs/{id}
		r.Get("/fuel-logs/{id}", func(w http.ResponseWriter, req *http.Request) {
			identity := auth.IdentityFromContext(req.Context())
			id := chi.URLParam(req, "id")
			m, err := proc.GetByID(id)
			if err != nil {
				server.WriteError(w, err)
				return
			}
			if err := requireFuelFleet(identity, vehicleAccessor, m); err != nil {
				server.WriteError(w, err)
				return
			}
			server.WriteJSON(w, http.StatusOK, server.Document{Data: Transform(m)})
		})

		// PATCH /fuel-logs/{id} — partial update.
		r.Patch("/fuel-logs/{id}", server.RegisterInputHandler(func(w http.ResponseWriter, req *http.Request, attrs struct {
			Date           *string  `json:"date"`
			Mileage        *int     `json:"mileage"`
			Gallons        *float64 `json:"gallons"`
			TotalCost      *float64 `json:"totalCost"`
			PricePerGallon *float64 `json:"pricePerGallon"`
		}) {
			identity := auth.IdentityFromContext(req.Context())
			id := chi.URLParam(req, "id")
			current, err := proc.GetByID(id)
			if err != nil {
				server.WriteError(w, err)
				return
			}
			if err := requireFuelFleet(identity, vehicleAccessor, current); err != nil {
				server.WriteError(w, err)
				return
			}
			if err := authz.RequireWrite(identity); err != nil {
				server.WriteError(w, err)
				return
			}

			updated := current
			if attrs.Date != nil {
				d, perr := time.Parse(time.RFC3339, *attrs.Date)
				if perr != nil {
					server.WriteError(w, server.ErrValidation)
					return
				}
				updated = updated.WithDate(d)
			}
			if attrs.Mileage != nil {
				updated = updated.WithMileage(*attrs.Mileage)
			}
			if attrs.Gallons != nil {
				updated = updated.WithGallons(*attrs.Gallons)
			}
			// Re-derive price when any of gallons/total/price changes.
			gallons := updated.Gallons()
			total := updated.TotalCost()
			price := updated.PricePerGallon()
			if attrs.TotalCost != nil {
				total = *attrs.TotalCost
			}
			if attrs.PricePerGallon != nil {
				price = *attrs.PricePerGallon
			}
			if attrs.TotalCost != nil || attrs.PricePerGallon != nil || attrs.Gallons != nil {
				derived, derr := DerivePrice(price, total, gallons)
				if derr != nil {
					server.WriteError(w, derr)
					return
				}
				updated = updated.WithTotalCost(derived.TotalCost).WithPricePerGallon(derived.PricePerGallon)
			}

			saved, err := adm.Update(updated)
			if err != nil {
				log.WithError(err).Error("update fuel log")
				server.WriteError(w, err)
				return
			}
			server.WriteJSON(w, http.StatusOK, server.Document{Data: Transform(saved)})
		}))

		// DELETE /fuel-logs/{id} — soft-delete.
		r.Delete("/fuel-logs/{id}", func(w http.ResponseWriter, req *http.Request) {
			identity := auth.IdentityFromContext(req.Context())
			id := chi.URLParam(req, "id")
			current, err := proc.GetByID(id)
			if err != nil {
				server.WriteError(w, err)
				return
			}
			if err := requireFuelFleet(identity, vehicleAccessor, current); err != nil {
				server.WriteError(w, err)
				return
			}
			if err := authz.RequireWrite(identity); err != nil {
				server.WriteError(w, err)
				return
			}
			if err := adm.SoftDelete(id); err != nil {
				server.WriteError(w, err)
				return
			}
			w.WriteHeader(http.StatusNoContent)
		})
	}
}

// requireFuelFleet resolves a fuel log's vehicle and enforces same-fleet access
// (404 no-leak). Used by GET/PATCH/DELETE on a fuel log by ID.
func requireFuelFleet(identity auth.Identity, vehicleAccessor VehicleAccessor, m Model) error {
	v, err := vehicleAccessor.GetByID(m.VehicleID())
	if err != nil {
		return err
	}
	return authz.RequireSameFleet(identity, v.FleetID())
}
