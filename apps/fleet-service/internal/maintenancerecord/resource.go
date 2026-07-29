package maintenancerecord

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

// VehicleAccessor resolves a vehicle for fleet-scoping maintenance record routes.
// Satisfied by *vehicle.Processor.
type VehicleAccessor interface {
	GetByID(id string) (vehicle.Model, error)
}

// InitializeRoutes wires the JWT-protected maintenance record endpoints.
// vehicleAccessor resolves the owning vehicle's fleetID for authz.
func InitializeRoutes(log logrus.FieldLogger, db *gorm.DB, vehicleAccessor VehicleAccessor) func(chi.Router) {
	proc := NewProcessor(log, NewProvider(db), NewAdministrator(db))
	return func(r chi.Router) {
		// GET /vehicles/{id}/maintenance-records — list (paged, newest first).
		r.Get("/vehicles/{id}/maintenance-records", func(w http.ResponseWriter, req *http.Request) {
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
				log.WithError(err).Error("list maintenance records")
				server.WriteError(w, err)
				return
			}
			server.WriteJSON(w, http.StatusOK, server.Document{
				Data: TransformSlice(ms),
				Meta: page.Meta(total),
			})
		})

		// POST /vehicles/{id}/maintenance-records — log a maintenance record.
		r.Post("/vehicles/{id}/maintenance-records", server.RegisterInputHandler(func(w http.ResponseWriter, req *http.Request, attrs struct {
			CategoryID       string   `json:"categoryId"`
			PerformedAt      string   `json:"performedAt"`
			Mileage          int      `json:"mileage"`
			Cost             float64  `json:"cost"`
			Vendor           string   `json:"vendor"`
			Notes            string   `json:"notes"`
			DocumentMediaIDs []string `json:"documentMediaIds"`
		},
		) {
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

			performedAt := time.Now().UTC()
			if attrs.PerformedAt != "" {
				performedAt, err = time.Parse(time.RFC3339, attrs.PerformedAt)
				if err != nil {
					server.WriteError(w, server.ErrValidation)
					return
				}
			}

			m, err := NewBuilder().
				SetVehicleID(vehicleID).
				SetCategoryID(attrs.CategoryID).
				SetPerformedAt(performedAt).
				SetMileage(attrs.Mileage).
				SetCost(attrs.Cost).
				SetVendor(attrs.Vendor).
				SetNotes(attrs.Notes).
				SetCreatedByUserID(identity.UserID).
				SetDocumentMediaIDs(attrs.DocumentMediaIDs).
				Build()
			if err != nil {
				server.WriteError(w, err)
				return
			}
			created, err := proc.Create(m)
			if err != nil {
				log.WithError(err).Error("create maintenance record")
				server.WriteError(w, err)
				return
			}
			server.WriteJSON(w, http.StatusCreated, server.Document{Data: Transform(created)})
		}))

		// GET /maintenance-records/{id}
		r.Get("/maintenance-records/{id}", func(w http.ResponseWriter, req *http.Request) {
			identity := auth.IdentityFromContext(req.Context())
			id := chi.URLParam(req, "id")
			m, err := proc.GetByID(id)
			if err != nil {
				server.WriteError(w, err)
				return
			}
			v, err := vehicleAccessor.GetByID(m.VehicleID())
			if err != nil {
				server.WriteError(w, err)
				return
			}
			if err := authz.RequireSameFleet(identity, v.FleetID()); err != nil {
				server.WriteError(w, err)
				return
			}
			server.WriteJSON(w, http.StatusOK, server.Document{Data: Transform(m)})
		})

		// PATCH /maintenance-records/{id} — partial update.
		r.Patch("/maintenance-records/{id}", server.RegisterInputHandler(func(w http.ResponseWriter, req *http.Request, attrs struct {
			PerformedAt *string  `json:"performedAt"`
			Mileage     *int     `json:"mileage"`
			Cost        *float64 `json:"cost"`
			Vendor      *string  `json:"vendor"`
			Notes       *string  `json:"notes"`
		},
		) {
			identity := auth.IdentityFromContext(req.Context())
			id := chi.URLParam(req, "id")
			current, err := proc.GetByID(id)
			if err != nil {
				server.WriteError(w, err)
				return
			}
			v, err := vehicleAccessor.GetByID(current.VehicleID())
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

			var parsedAt *time.Time
			if attrs.PerformedAt != nil {
				t, perr := time.Parse(time.RFC3339, *attrs.PerformedAt)
				if perr != nil {
					server.WriteError(w, server.ErrValidation)
					return
				}
				parsedAt = &t
			}

			updated, err := proc.Update(id, func(m Model) Model {
				if parsedAt != nil {
					m = m.WithPerformedAt(*parsedAt)
				}
				if attrs.Mileage != nil {
					m = m.WithMileage(*attrs.Mileage)
				}
				if attrs.Cost != nil {
					m = m.WithCost(*attrs.Cost)
				}
				if attrs.Vendor != nil {
					m = m.WithVendor(*attrs.Vendor)
				}
				if attrs.Notes != nil {
					m = m.WithNotes(*attrs.Notes)
				}
				return m
			})
			if err != nil {
				log.WithError(err).Error("update maintenance record")
				server.WriteError(w, err)
				return
			}
			server.WriteJSON(w, http.StatusOK, server.Document{Data: Transform(updated)})
		}))

		// DELETE /maintenance-records/{id} — soft-delete.
		r.Delete("/maintenance-records/{id}", func(w http.ResponseWriter, req *http.Request) {
			identity := auth.IdentityFromContext(req.Context())
			id := chi.URLParam(req, "id")
			current, err := proc.GetByID(id)
			if err != nil {
				server.WriteError(w, err)
				return
			}
			v, err := vehicleAccessor.GetByID(current.VehicleID())
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
			if err := proc.SoftDelete(id); err != nil {
				server.WriteError(w, err)
				return
			}
			w.WriteHeader(http.StatusNoContent)
		})
	}
}
