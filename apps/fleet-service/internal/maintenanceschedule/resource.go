package maintenanceschedule

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/sirupsen/logrus"
	"gorm.io/gorm"

	"github.com/jtumidanski/myfleet/packages/shared-go/auth"
	"github.com/jtumidanski/myfleet/packages/shared-go/server"
	"github.com/jtumidanski/myfleet/packages/shared-go/telemetry"

	"github.com/jtumidanski/myfleet/apps/fleet-service/internal/authz"
	"github.com/jtumidanski/myfleet/apps/fleet-service/internal/vehicle"
)

// VehicleAccessor resolves a vehicle for fleet-scoping schedule routes and for
// reading the current mileage baseline. Satisfied by *vehicle.Processor.
type VehicleAccessor interface {
	GetByID(id string) (vehicle.Model, error)
}

// InitializeRoutes wires the JWT-protected maintenance schedule endpoints,
// including the completion action and upcoming/overdue queues.
//
// vehicleAccessor resolves the owning vehicle's fleetID + current mileage for
// authz and completion. completion runs the cross-domain completion transaction.
func InitializeRoutes(log logrus.FieldLogger, db *gorm.DB, vehicleAccessor VehicleAccessor, completion CompletionDeps) func(chi.Router) {
	proc := NewProcessor(log, NewProvider(db), NewAdministrator(db))
	return func(r chi.Router) {
		// GET /vehicles/{id}/maintenance-schedules — list (paged).
		r.Get("/vehicles/{id}/maintenance-schedules", func(w http.ResponseWriter, req *http.Request) {
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
				log.WithError(err).Error("list maintenance schedules")
				server.WriteError(w, err)
				return
			}
			server.WriteJSON(w, http.StatusOK, server.Document{
				Data: TransformSlice(ms),
				Meta: page.Meta(total),
			})
		})

		// POST /vehicles/{id}/maintenance-schedules — define a schedule.
		r.Post("/vehicles/{id}/maintenance-schedules", server.RegisterInputHandler(func(w http.ResponseWriter, req *http.Request, attrs struct {
			CategoryID     string `json:"categoryId"`
			RecurrenceType string `json:"recurrenceType"`
			IntervalMonths int    `json:"intervalMonths"`
			IntervalMiles  int    `json:"intervalMiles"`
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
			m, err := NewBuilder().
				SetVehicleID(vehicleID).
				SetCategoryID(attrs.CategoryID).
				SetRecurrenceType(attrs.RecurrenceType).
				SetIntervalMonths(attrs.IntervalMonths).
				SetIntervalMiles(attrs.IntervalMiles).
				Build()
			if err != nil {
				server.WriteError(w, err)
				return
			}
			created, err := proc.Create(m)
			if err != nil {
				log.WithError(err).Error("create maintenance schedule")
				server.WriteError(w, err)
				return
			}
			server.WriteJSON(w, http.StatusCreated, server.Document{Data: Transform(created)})
		}))

		// GET /maintenance-schedules/{id}
		r.Get("/maintenance-schedules/{id}", func(w http.ResponseWriter, req *http.Request) {
			identity := auth.IdentityFromContext(req.Context())
			id := chi.URLParam(req, "id")
			m, err := proc.GetByID(id)
			if err != nil {
				server.WriteError(w, err)
				return
			}
			if err := requireScheduleFleet(identity, vehicleAccessor, m); err != nil {
				server.WriteError(w, err)
				return
			}
			server.WriteJSON(w, http.StatusOK, server.Document{Data: Transform(m)})
		})

		// PATCH /maintenance-schedules/{id} — partial update.
		r.Patch("/maintenance-schedules/{id}", server.RegisterInputHandler(func(w http.ResponseWriter, req *http.Request, attrs struct {
			RecurrenceType *string `json:"recurrenceType"`
			IntervalMonths *int    `json:"intervalMonths"`
			IntervalMiles  *int    `json:"intervalMiles"`
			Active         *bool   `json:"active"`
		}) {
			identity := auth.IdentityFromContext(req.Context())
			id := chi.URLParam(req, "id")
			current, err := proc.GetByID(id)
			if err != nil {
				server.WriteError(w, err)
				return
			}
			if err := requireScheduleFleet(identity, vehicleAccessor, current); err != nil {
				server.WriteError(w, err)
				return
			}
			if err := authz.RequireWrite(identity); err != nil {
				server.WriteError(w, err)
				return
			}
			updated, err := proc.Update(id, func(m Model) Model {
				rt := m.RecurrenceType()
				months := m.IntervalMonths()
				miles := m.IntervalMiles()
				if attrs.RecurrenceType != nil {
					rt = *attrs.RecurrenceType
				}
				if attrs.IntervalMonths != nil {
					months = *attrs.IntervalMonths
				}
				if attrs.IntervalMiles != nil {
					miles = *attrs.IntervalMiles
				}
				m = m.WithRecurrence(rt, months, miles)
				if attrs.Active != nil {
					m = m.WithActive(*attrs.Active)
				}
				return m
			})
			if err != nil {
				log.WithError(err).Error("update maintenance schedule")
				server.WriteError(w, err)
				return
			}
			server.WriteJSON(w, http.StatusOK, server.Document{Data: Transform(updated)})
		}))

		// DELETE /maintenance-schedules/{id}
		r.Delete("/maintenance-schedules/{id}", func(w http.ResponseWriter, req *http.Request) {
			identity := auth.IdentityFromContext(req.Context())
			id := chi.URLParam(req, "id")
			current, err := proc.GetByID(id)
			if err != nil {
				server.WriteError(w, err)
				return
			}
			if err := requireScheduleFleet(identity, vehicleAccessor, current); err != nil {
				server.WriteError(w, err)
				return
			}
			if err := authz.RequireWrite(identity); err != nil {
				server.WriteError(w, err)
				return
			}
			if err := proc.Delete(id); err != nil {
				server.WriteError(w, err)
				return
			}
			w.WriteHeader(http.StatusNoContent)
		})

		// POST /maintenance-schedules/{id}/complete — completion flow (§10.3).
		r.Post("/maintenance-schedules/{id}/complete", server.RegisterInputHandler(func(w http.ResponseWriter, req *http.Request, attrs struct {
			Date          string `json:"date"`
			LatestMileage *int   `json:"latestMileage"`
		}) {
			identity := auth.IdentityFromContext(req.Context())
			id := chi.URLParam(req, "id")
			sched, err := proc.GetByID(id)
			if err != nil {
				server.WriteError(w, err)
				return
			}
			v, err := vehicleAccessor.GetByID(sched.VehicleID())
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

			date := time.Now().UTC()
			if attrs.Date != "" {
				date, err = time.Parse(time.RFC3339, attrs.Date)
				if err != nil {
					server.WriteError(w, server.ErrValidation)
					return
				}
			}
			miles := v.CurrentMileage()
			if attrs.LatestMileage != nil {
				miles = *attrs.LatestMileage
			}

			out, err := completion.CompleteInTransaction(log, CompletionInput{
				ScheduleID:    id,
				VehicleID:     sched.VehicleID(),
				CategoryID:    sched.CategoryID(),
				FleetID:       v.FleetID(),
				ActorUserID:   identity.UserID,
				TraceID:       telemetry.CorrelationIDFromContext(req.Context()),
				Date:          date,
				LatestMileage: miles,
			})
			if err != nil {
				log.WithError(err).Error("complete maintenance schedule")
				server.WriteError(w, err)
				return
			}
			server.WriteJSON(w, http.StatusCreated, server.Document{
				Data: server.Resource{
					Type:       "maintenanceCompletions",
					ID:         id,
					Attributes: map[string]any{"maintenanceRecordId": out.MaintenanceRecordID},
				},
			})
		}))

		// GET /fleets/{id}/maintenance/upcoming — live upcoming queue (paged).
		r.Get("/fleets/{id}/maintenance/upcoming", queueHandler(log, proc, "upcoming"))
		// GET /fleets/{id}/maintenance/overdue — live overdue queue (paged).
		r.Get("/fleets/{id}/maintenance/overdue", queueHandler(log, proc, "overdue"))
	}
}

// InitializeInternalRoutes wires the network-restricted internal endpoint (no
// JWT). Register WITHOUT JWT middleware. Feeds notification-service's reminder
// safety-net (design §11): all currently upcoming/overdue schedules across ALL
// fleets, with live-computed DueState.
func InitializeInternalRoutes(log logrus.FieldLogger, db *gorm.DB) func(chi.Router) {
	proc := NewProcessor(log, NewProvider(db), NewAdministrator(db))
	return func(r chi.Router) {
		// GET /internal/maintenance/due → all non-ok schedules across all fleets.
		r.Get("/internal/maintenance/due", func(w http.ResponseWriter, req *http.Request) {
			entries, err := proc.DueAcrossAllFleets(time.Now().UTC())
			if err != nil {
				log.WithError(err).Error("internal maintenance due feed")
				server.WriteError(w, err)
				return
			}
			server.WriteJSON(w, http.StatusOK, TransformInternalDue(entries))
		})
	}
}

// queueHandler builds a fleet-scoped, live-computed queue handler for the given
// due-state (design A5). DueState is computed on read from current mileage + now.
func queueHandler(log logrus.FieldLogger, proc *Processor, state string) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		identity := auth.IdentityFromContext(req.Context())
		fleetID := chi.URLParam(req, "id")
		if err := authz.RequireSameFleet(identity, fleetID); err != nil {
			server.WriteError(w, err)
			return
		}
		entries, err := proc.Queue(fleetID, state, time.Now().UTC())
		if err != nil {
			log.WithError(err).Error("maintenance queue")
			server.WriteError(w, err)
			return
		}
		page := server.ParsePage(req)
		total := len(entries)
		// Page the already-filtered slice in memory.
		start := page.Offset()
		if start > total {
			start = total
		}
		end := start + page.Size
		if end > total {
			end = total
		}
		server.WriteJSON(w, http.StatusOK, server.Document{
			Data: TransformQueue(entries[start:end]),
			Meta: page.Meta(total),
		})
	}
}

// requireScheduleFleet resolves a schedule's vehicle and enforces same-fleet
// access (404 no-leak). Used by GET/PATCH/DELETE on a schedule by ID.
func requireScheduleFleet(identity auth.Identity, vehicleAccessor VehicleAccessor, m Model) error {
	v, err := vehicleAccessor.GetByID(m.VehicleID())
	if err != nil {
		return err
	}
	return authz.RequireSameFleet(identity, v.FleetID())
}
