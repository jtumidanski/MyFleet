package invite

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/sirupsen/logrus"
	"gorm.io/gorm"

	"github.com/jtumidanski/myfleet/packages/shared-go/auth"
	"github.com/jtumidanski/myfleet/packages/shared-go/server"
	"github.com/jtumidanski/myfleet/packages/shared-go/telemetry"

	"github.com/jtumidanski/myfleet/apps/fleet-service/internal/authz"
	"github.com/jtumidanski/myfleet/apps/fleet-service/internal/membership"
)

const defaultExpiry = 7 * 24 * time.Hour

// Limits carries the abuse-control knobs (FR-RATE-1…4). Both are enforced
// server-side in the domain layer; the UI disabling a button is a convenience,
// not the control.
type Limits struct {
	CreatePerWindow int
	CreateWindow    time.Duration
	ResendCooldown  time.Duration
}

// OwnerChecker performs the authoritative DB-level owner check (stale-claim guard,
// design §9). Satisfied by *membership.Processor.
type OwnerChecker interface {
	RequireOwnerInFleet(fleetID, userID string) error
}

// InitializeRoutes wires the JWT-protected invite endpoints.
// ownerCheck is injected for the authoritative DB owner recheck on mutations.
func InitializeRoutes(log logrus.FieldLogger, db *gorm.DB, ownerCheck OwnerChecker, record ActivityRecorder, emit InvitedEmitter, emitCreated CreatedEmitter, limits Limits) func(chi.Router) {
	prov := NewProvider(db)
	adm := NewAdministrator(db).WithActivityRecorder(record).WithEmitter(emit).WithCreatedEmitter(emitCreated)
	proc := NewProcessor(log, prov)

	return func(r chi.Router) {
		// POST /fleets/{id}/invites — owner-only; creates an invite with a unique token
		r.Post("/fleets/{id}/invites", server.RegisterInputHandler(func(w http.ResponseWriter, req *http.Request, attrs struct {
			Email string `json:"email"`
			Role  string `json:"role"`
		},
		) {
			identity := auth.IdentityFromContext(req.Context())
			fleetID := chi.URLParam(req, "id")

			if err := authz.RequireSameFleet(identity, fleetID); err != nil {
				server.WriteError(w, err)
				return
			}
			// Token-level owner gate (fast path)
			if err := authz.RequireOwner(identity); err != nil {
				server.WriteError(w, err)
				return
			}
			// Authoritative DB check via processor (stale-claim guard, design §9)
			if err := ownerCheck.RequireOwnerInFleet(fleetID, identity.UserID); err != nil {
				server.WriteError(w, err)
				return
			}

			// Role is copied verbatim onto the membership created at accept
			// time, so an unrecognised value would mint a membership whose
			// role no authz gate understands. Validate against the vocabulary
			// membership owns.
			if !membership.IsValidRole(attrs.Role) {
				server.WriteError(w, server.ErrValidation)
				return
			}
			// A newline in an address must fail HERE, not be discovered by the
			// SMTP layer hours later (PRD §8 Security).
			if err := ValidateInviteEmail(attrs.Email); err != nil {
				server.WriteError(w, err)
				return
			}

			// Per-fleet creation window (FR-RATE-1). Checked before minting a
			// token so a throttled request costs no entropy and no DB write.
			if err := proc.CheckCreateLimit(fleetID, limits.CreatePerWindow, limits.CreateWindow, time.Now()); err != nil {
				server.WriteError(w, err)
				return
			}

			token, err := generateToken()
			if err != nil {
				server.WriteError(w, err)
				return
			}

			m := NewBuilder().
				SetFleetID(fleetID).
				SetEmail(attrs.Email).
				SetRole(attrs.Role).
				SetToken(token).
				SetExpiresAt(time.Now().Add(defaultExpiry)).
				SetInvitedByUserID(identity.UserID).
				Build()

			created, err := adm.Insert(m, telemetry.CorrelationIDFromContext(req.Context()))
			if err != nil {
				server.WriteError(w, err)
				return
			}
			server.WriteJSON(w, http.StatusCreated, server.Document{Data: Transform(created)})
		}))

		// GET /fleets/{id}/invites — list invites (fleet-scoped)
		r.Get("/fleets/{id}/invites", func(w http.ResponseWriter, req *http.Request) {
			identity := auth.IdentityFromContext(req.Context())
			fleetID := chi.URLParam(req, "id")

			if err := authz.RequireSameFleet(identity, fleetID); err != nil {
				server.WriteError(w, err)
				return
			}
			ms, err := proc.ListByFleet(fleetID)
			if err != nil {
				server.WriteError(w, err)
				return
			}
			server.WriteJSON(w, http.StatusOK, server.Document{Data: TransformSlice(ms)})
		})

		// DELETE /invites/{id} — owner-only
		r.Delete("/invites/{id}", func(w http.ResponseWriter, req *http.Request) {
			identity := auth.IdentityFromContext(req.Context())
			id := chi.URLParam(req, "id")

			inv, err := proc.GetByID(id)
			if err != nil {
				if errors.Is(err, server.ErrNotFound) {
					server.WriteError(w, server.ErrNotFound)
					return
				}
				server.WriteError(w, err)
				return
			}

			if err := authz.RequireSameFleet(identity, inv.FleetID()); err != nil {
				server.WriteError(w, err)
				return
			}
			// Token-level owner gate (fast path)
			if err := authz.RequireOwner(identity); err != nil {
				server.WriteError(w, err)
				return
			}
			// Authoritative DB check via processor (stale-claim guard, design §9)
			if err := ownerCheck.RequireOwnerInFleet(inv.FleetID(), identity.UserID); err != nil {
				server.WriteError(w, err)
				return
			}

			if err := adm.Delete(id); err != nil {
				server.WriteError(w, err)
				return
			}
			w.WriteHeader(http.StatusNoContent)
		})

		// POST /fleets/{fleetId}/invites/{inviteId}/resend — owner-only.
		// Rotates the token and resets expiry (FR-RSND-1…5): resend is used when
		// the previous link never arrived or expired, so invalidating it costs
		// nothing and bounds the lifetime of a token that leaked into a mailbox.
		r.Post("/fleets/{fleetId}/invites/{inviteId}/resend", func(w http.ResponseWriter, req *http.Request) {
			identity := auth.IdentityFromContext(req.Context())
			fleetID := chi.URLParam(req, "fleetId")
			inviteID := chi.URLParam(req, "inviteId")

			inv, err := proc.GetByID(inviteID)
			if err != nil {
				server.WriteError(w, err)
				return
			}

			if err := authz.RequireSameFleet(identity, fleetID); err != nil {
				server.WriteError(w, err)
				return
			}
			// Path-pair mismatch → 403, not 404: the caller proved membership of
			// the fleet they named but named an invite belonging to another one.
			if inv.FleetID() != fleetID {
				server.WriteError(w, server.ErrForbidden)
				return
			}
			// Token-level owner gate (fast path)
			if err := authz.RequireOwner(identity); err != nil {
				server.WriteError(w, err)
				return
			}
			// Authoritative DB check via processor (stale-claim guard, design §9)
			if err := ownerCheck.RequireOwnerInFleet(fleetID, identity.UserID); err != nil {
				server.WriteError(w, err)
				return
			}

			// Accepted BEFORE cooldown, so an accepted invite never reports a
			// cooldown it could never satisfy (FR-RSND-3).
			if inv.AcceptedAt() != nil {
				server.WriteError(w, server.ErrConflict)
				return
			}
			now := time.Now()
			if err := proc.CheckResendCooldown(inv, limits.ResendCooldown, now); err != nil {
				server.WriteError(w, err)
				return
			}

			token, err := generateToken()
			if err != nil {
				server.WriteError(w, err)
				return
			}
			updated, err := adm.Resend(inv, token, now.Add(defaultExpiry), now,
				telemetry.CorrelationIDFromContext(req.Context()))
			if err != nil {
				server.WriteError(w, err)
				return
			}
			server.WriteJSON(w, http.StatusOK, server.Document{Data: Transform(updated)})
		})

		// POST /invites/{token}/accept — look up by token; validate; create membership + stamp accepted_at
		r.Post("/invites/{token}/accept", func(w http.ResponseWriter, req *http.Request) {
			identity := auth.IdentityFromContext(req.Context())
			token := chi.URLParam(req, "token")

			inv, err := proc.GetByToken(token)
			if err != nil {
				if errors.Is(err, server.ErrNotFound) {
					server.WriteError(w, server.ErrNotFound)
					return
				}
				server.WriteError(w, err)
				return
			}

			if err := proc.ValidateAccept(inv, identity.Email); err != nil {
				server.WriteError(w, err)
				return
			}

			updated, err := adm.Accept(inv, identity.UserID, telemetry.CorrelationIDFromContext(req.Context()))
			if err != nil {
				server.WriteError(w, err)
				return
			}
			server.WriteJSON(w, http.StatusOK, server.Document{Data: Transform(updated)})
		})
	}
}

// generateToken returns a cryptographically random 32-byte hex string.
func generateToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
