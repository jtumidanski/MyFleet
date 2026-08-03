package maintenancerecord

import (
	"context"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/sirupsen/logrus"
	"gorm.io/gorm"

	"github.com/jtumidanski/myfleet/packages/shared-go/auth"
	"github.com/jtumidanski/myfleet/packages/shared-go/server"

	"github.com/jtumidanski/myfleet/apps/fleet-service/internal/authz"
	"github.com/jtumidanski/myfleet/apps/fleet-service/internal/maintenancecategory"
	"github.com/jtumidanski/myfleet/apps/fleet-service/internal/vehicle"
)

// VehicleAccessor resolves a vehicle for fleet-scoping maintenance record routes.
// Satisfied by *vehicle.Processor.
type VehicleAccessor interface {
	GetByID(id string) (vehicle.Model, error)
}

// CategoryAccessor resolves the category IDs of a kind so the record list can
// be filtered by kind without importing another domain's data access. It
// mirrors VehicleAccessor exactly; satisfied by *maintenancecategory.Processor.
//
// Importing maintenancecategory.Kind/ParseKind for the type keeps parsing and
// the permitted value set in the domain that owns them, so the category and
// record endpoints cannot drift on what ?kind= accepts (design D2).
type CategoryAccessor interface {
	IDsByKind(kind maintenancecategory.Kind, fleetID string) ([]string, error)
}

// DocumentValidator proves every documentMediaId belongs to the caller's active
// fleet (PRD FR-DOC-6). Satisfied by *mediaclient.Client. A nil value is legal
// and means "no validator wired" — used by unit tests and by any future caller
// that has already validated.
type DocumentValidator interface {
	ValidateOwnership(ctx context.Context, fleetID string, mediaIDs []string) error
}

// InitializeRoutes wires the JWT-protected maintenance record endpoints.
// vehicleAccessor resolves the owning vehicle's fleetID for authz;
// categoryAccessor backs the ?kind= filter; docs validates attachment
// ownership on create and may be nil.
func InitializeRoutes(
	log logrus.FieldLogger,
	db *gorm.DB,
	vehicleAccessor VehicleAccessor,
	categoryAccessor CategoryAccessor,
	docs DocumentValidator,
) func(chi.Router) {
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

			kind, err := maintenancecategory.ParseKind(req.URL.Query().Get("kind"))
			if err != nil {
				server.WriteError(w, err)
				return
			}

			// nil means "no filter"; a resolved-but-empty set means "match
			// nothing". Never collapse the two (design D3).
			var categoryIDs []string
			if kind != "" {
				categoryIDs, err = categoryAccessor.IDsByKind(kind, v.FleetID())
				if err != nil {
					log.WithError(err).Error("resolve category ids by kind")
					server.WriteError(w, err)
					return
				}
				if categoryIDs == nil {
					categoryIDs = []string{}
				}
			}

			page := server.ParsePage(req)
			ms, total, err := proc.ListByVehicle(vehicleID, categoryIDs, page)
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
			Description      string   `json:"description"`
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

			// performedAt is required (PRD FR-REC-5). This used to default
			// to time.Now().UTC() when empty; a maintenance log with a
			// silently-guessed date is worse than one that refuses to save.
			// The builder rejects a zero time as well, so the invariant is
			// enforced twice on purpose: the handler gives the accurate
			// status code, the builder guarantees no code path can construct
			// a dateless record.
			if attrs.PerformedAt == "" {
				server.WriteError(w, server.ErrValidation)
				return
			}
			performedAt, err := time.Parse(time.RFC3339, attrs.PerformedAt)
			if err != nil {
				server.WriteError(w, server.ErrValidation)
				return
			}

			// Enforce the per-record cap before the cross-service call, not
			// just at Build() below. Otherwise an oversized ID list builds a
			// multi-megabyte ids= query string and media-service's own
			// MaxInternalLookupIDs rejects it with a plain-status error that
			// StatusFor maps to 500 — an opaque server error for what is
			// really a client mistake. The Build()-time check stays too:
			// that one is the invariant guarantee, this one is about status
			// code and not shipping an absurd URL.
			if len(attrs.DocumentMediaIDs) > MaxDocuments {
				server.WriteError(w, server.ErrValidation)
				return
			}

			// Prove the attachments are the caller's own BEFORE anything is
			// written, so a rejection leaves nothing to roll back
			// (PRD FR-DOC-6). Skipped entirely when there are no
			// attachments, so the common case makes no cross-service call.
			if len(attrs.DocumentMediaIDs) > 0 && docs != nil {
				if err := docs.ValidateOwnership(req.Context(), identity.ActiveFleetID, attrs.DocumentMediaIDs); err != nil {
					log.WithError(err).Warn("attachment ownership validation failed")
					server.WriteError(w, err)
					return
				}
			}

			m, err := NewBuilder().
				SetVehicleID(vehicleID).
				SetCategoryID(attrs.CategoryID).
				SetDescription(attrs.Description).
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
			// CategoryID is editable: the record drawer offers the same
			// category picker the create form does, and omitting it here made
			// that control silently discard the user's choice. A cleared value
			// is rejected by Validate (categoryID is an invariant), not
			// written through.
			CategoryID  *string  `json:"categoryId"`
			Description *string  `json:"description"`
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
				if attrs.CategoryID != nil {
					m = m.WithCategoryID(*attrs.CategoryID)
				}
				if attrs.Description != nil {
					m = m.WithDescription(*attrs.Description)
				}
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
