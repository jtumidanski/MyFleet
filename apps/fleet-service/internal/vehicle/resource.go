package vehicle

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/sirupsen/logrus"
	"gorm.io/gorm"

	"github.com/jtumidanski/myfleet/packages/shared-go/auth"
	"github.com/jtumidanski/myfleet/packages/shared-go/server"

	"github.com/jtumidanski/myfleet/apps/fleet-service/internal/authz"
)

// PrimaryImageSetter handles setting the primary image for a vehicle, updating
// both the vehiclemedia rows and mirroring into vehicles.primary_image_media_id.
// Satisfied by *vehiclemedia.Processor.
type PrimaryImageSetter interface {
	SetPrimary(vehicleID, mediaID string) error
}

// InitializeRoutes wires the JWT-protected vehicle endpoints.
// ownerCheck is injected for the authoritative DB owner recheck on restore (stale-claim guard).
// primaryImage is injected to delegate PUT /vehicles/{id}/primary-image to the vehiclemedia domain.
func InitializeRoutes(log logrus.FieldLogger, db *gorm.DB, ownerCheck OwnerChecker, primaryImage PrimaryImageSetter) func(chi.Router) {
	proc := NewProcessor(log, NewProvider(db), NewAdministrator(db))
	return func(r chi.Router) {
		// GET /fleets/{id}/vehicles — list vehicles (fleet-paged)
		r.Get("/fleets/{id}/vehicles", func(w http.ResponseWriter, req *http.Request) {
			identity := auth.IdentityFromContext(req.Context())
			fleetID := chi.URLParam(req, "id")
			if err := authz.RequireSameFleet(identity, fleetID); err != nil {
				server.WriteError(w, err)
				return
			}
			page := server.ParsePage(req)
			ms, total, err := proc.ListByFleet(fleetID, page)
			if err != nil {
				server.WriteError(w, err)
				return
			}
			server.WriteJSON(w, http.StatusOK, server.Document{
				Data: TransformSlice(ms),
				Meta: page.Meta(total),
			})
		})

		// POST /fleets/{id}/vehicles — create a vehicle
		r.Post("/fleets/{id}/vehicles", server.RegisterInputHandler(func(w http.ResponseWriter, req *http.Request, attrs struct {
			Nickname       string `json:"nickname"`
			Make           string `json:"make"`
			Model          string `json:"model"`
			Trim           string `json:"trim"`
			Year           int    `json:"year"`
			VIN            string `json:"vin"`
			CurrentMileage int    `json:"currentMileage"`
			Notes          string `json:"notes"`
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
			m, err := NewBuilder().
				SetFleetID(fleetID).
				SetNickname(attrs.Nickname).
				SetMake(attrs.Make).
				SetModel(attrs.Model).
				SetTrim(attrs.Trim).
				SetYear(attrs.Year).
				SetVIN(attrs.VIN).
				SetCurrentMileage(attrs.CurrentMileage).
				SetNotes(attrs.Notes).
				Build()
			if err != nil {
				server.WriteError(w, err)
				return
			}
			created, err := proc.Create(m)
			if err != nil {
				server.WriteError(w, err)
				return
			}
			server.WriteJSON(w, http.StatusCreated, server.Document{Data: Transform(created)})
		}))

		// GET /vehicles/{id}
		r.Get("/vehicles/{id}", func(w http.ResponseWriter, req *http.Request) {
			identity := auth.IdentityFromContext(req.Context())
			id := chi.URLParam(req, "id")
			m, err := proc.GetByID(id)
			if err != nil {
				server.WriteError(w, err)
				return
			}
			if err := authz.RequireSameFleet(identity, m.FleetID()); err != nil {
				server.WriteError(w, err)
				return
			}
			server.WriteJSON(w, http.StatusOK, server.Document{Data: Transform(m)})
		})

		// PATCH /vehicles/{id} — partial update
		r.Patch("/vehicles/{id}", server.RegisterInputHandler(func(w http.ResponseWriter, req *http.Request, attrs struct {
			Nickname       *string `json:"nickname"`
			CurrentMileage *int    `json:"currentMileage"`
			Notes          *string `json:"notes"`
		}) {
			identity := auth.IdentityFromContext(req.Context())
			id := chi.URLParam(req, "id")
			// Fetch first to get fleetID for authz, then mutate.
			current, err := proc.GetByID(id)
			if err != nil {
				server.WriteError(w, err)
				return
			}
			if err := authz.RequireSameFleet(identity, current.FleetID()); err != nil {
				server.WriteError(w, err)
				return
			}
			if err := authz.RequireWrite(identity); err != nil {
				server.WriteError(w, err)
				return
			}
			updated, err := proc.Update(id, func(m Model) Model {
				if attrs.Nickname != nil {
					m = m.WithNickname(*attrs.Nickname)
				}
				if attrs.CurrentMileage != nil {
					m = m.WithCurrentMileage(*attrs.CurrentMileage)
				}
				if attrs.Notes != nil {
					m = m.WithNotes(*attrs.Notes)
				}
				return m
			})
			if err != nil {
				server.WriteError(w, err)
				return
			}
			server.WriteJSON(w, http.StatusOK, server.Document{Data: Transform(updated)})
		}))

		// DELETE /vehicles/{id} — soft-delete
		r.Delete("/vehicles/{id}", func(w http.ResponseWriter, req *http.Request) {
			identity := auth.IdentityFromContext(req.Context())
			id := chi.URLParam(req, "id")
			current, err := proc.GetByID(id)
			if err != nil {
				server.WriteError(w, err)
				return
			}
			if err := authz.RequireSameFleet(identity, current.FleetID()); err != nil {
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

		// POST /vehicles/{id}/restore — owner-only; 410 if past purge window
		r.Post("/vehicles/{id}/restore", func(w http.ResponseWriter, req *http.Request) {
			identity := auth.IdentityFromContext(req.Context())
			id := chi.URLParam(req, "id")
			// Fetch including deleted to get fleetID for authz.
			current, err := proc.GetByIDIncludingDeleted(id)
			if err != nil {
				server.WriteError(w, server.ErrNotFound)
				return
			}
			if err := authz.RequireSameFleet(identity, current.FleetID()); err != nil {
				server.WriteError(w, err)
				return
			}
			// Token-level gate (fast path)
			if err := authz.RequireOwner(identity); err != nil {
				server.WriteError(w, err)
				return
			}
			// Authoritative DB check (stale-claim guard, design §9)
			if err := ownerCheck.RequireOwnerInFleet(current.FleetID(), identity.UserID); err != nil {
				server.WriteError(w, err)
				return
			}
			restored, err := proc.Restore(id)
			if err != nil {
				server.WriteError(w, err)
				return
			}
			server.WriteJSON(w, http.StatusOK, server.Document{Data: Transform(restored)})
		})

		// PUT /vehicles/{id}/primary-image — set primary image.
		// Delegates to vehiclemedia.Processor.SetPrimary which: clears all is_primary
		// rows, sets exactly one is_primary=true, and mirrors to vehicles row.
		r.Put("/vehicles/{id}/primary-image", server.RegisterInputHandler(func(w http.ResponseWriter, req *http.Request, attrs struct {
			MediaID string `json:"mediaId"`
		}) {
			identity := auth.IdentityFromContext(req.Context())
			id := chi.URLParam(req, "id")
			current, err := proc.GetByID(id)
			if err != nil {
				server.WriteError(w, err)
				return
			}
			if err := authz.RequireSameFleet(identity, current.FleetID()); err != nil {
				server.WriteError(w, err)
				return
			}
			if err := authz.RequireWrite(identity); err != nil {
				server.WriteError(w, err)
				return
			}
			if err := primaryImage.SetPrimary(id, attrs.MediaID); err != nil {
				server.WriteError(w, err)
				return
			}
			// Re-fetch vehicle to reflect updated primary_image_media_id.
			updated, err := proc.GetByID(id)
			if err != nil {
				server.WriteError(w, err)
				return
			}
			server.WriteJSON(w, http.StatusOK, server.Document{Data: Transform(updated)})
		}))
	}
}
