package invite

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/sirupsen/logrus"
	"gorm.io/gorm"

	"github.com/jtumidanski/myfleet/packages/shared-go/auth"
	"github.com/jtumidanski/myfleet/packages/shared-go/server"
	"github.com/jtumidanski/myfleet/packages/shared-go/telemetry"

	"github.com/jtumidanski/myfleet/apps/fleet-service/internal/authz"
	"github.com/jtumidanski/myfleet/apps/fleet-service/internal/fleet"
)

// OwnerChecker performs the authoritative DB-level owner check (stale-claim guard,
// design §9). Satisfied by *membership.Processor.
type OwnerChecker interface {
	RequireOwnerInFleet(fleetID, userID string) error
}

// InitializeRoutes wires the JWT-protected invite endpoints.
// ownerCheck is injected for the authoritative DB owner recheck on mutations.
//
// The handlers below authorize, call ONE domain operation and render. Token
// minting, expiry computation, model construction, the rate limits and the
// accept/resend invariants all live in the processor; the administrator is
// reachable only through it.
func InitializeRoutes(log logrus.FieldLogger, db *gorm.DB, ownerCheck OwnerChecker, record ActivityRecorder, emit InvitedEmitter, emitCreated CreatedEmitter, limits Limits) func(chi.Router) {
	proc := NewProcessor(log, NewProvider(db)).
		WithAdministrator(NewAdministrator(db).
			WithActivityRecorder(record).
			WithEmitter(emit).
			WithCreatedEmitter(emitCreated)).
		WithLimits(limits)
	// Read-only, and only to name the fleet an invitee is being asked to join
	// — they hold no membership yet, so they cannot resolve the id themselves.
	fleets := fleet.NewProvider(db)

	return func(r chi.Router) {
		// POST /fleets/{id}/invites — owner-only; creates an invite with a unique token
		r.Post("/fleets/{id}/invites", server.RegisterInputHandler(func(w http.ResponseWriter, req *http.Request, attrs CreateRequest) {
			identity := auth.IdentityFromContext(req.Context())
			fleetID := chi.URLParam(req, "id")
			traceID := telemetry.CorrelationIDFromContext(req.Context())

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
				// ErrForbidden is the routine "not an owner" outcome; anything
				// else is a DB fault an operator needs to see.
				if !errors.Is(err, server.ErrForbidden) {
					log.WithError(err).WithFields(logrus.Fields{
						"fleet_id": fleetID,
						"user_id":  identity.UserID,
						"trace_id": traceID,
					}).Error("check invite owner")
				}
				server.WriteError(w, err)
				return
			}

			created, err := proc.Create(req.Context(), fleetID, attrs.Email, attrs.Role, identity.UserID, traceID)
			if err != nil {
				// ErrValidation (bad role or address) and ErrTooManyRequests
				// (the per-fleet window, FR-RATE-1) are routine client
				// outcomes. Anything else is the count query, the entropy
				// source or the insert failing — an operator's problem.
				if !errors.Is(err, server.ErrValidation) && !errors.Is(err, server.ErrTooManyRequests) {
					log.WithError(err).WithFields(logrus.Fields{
						"fleet_id": fleetID,
						"trace_id": traceID,
					}).Error("create invite")
				}
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
			ms, err := proc.ListByFleet(req.Context(), fleetID)
			if err != nil {
				log.WithError(err).WithField("fleet_id", fleetID).Error("list invites")
				server.WriteError(w, err)
				return
			}
			server.WriteJSON(w, http.StatusOK, server.Document{Data: TransformSlice(ms)})
		})

		// GET /invites/pending — the invites waiting for the CALLER.
		//
		// The discovery path for someone invited before they had an account.
		// Nothing delivers invites (task-009-smtp-invite-delivery is specced
		// but unimplemented), so without this an invitee logs in, has no fleet,
		// and is offered only "create a fleet" — the invite addressed to them
		// is unreachable unless the owner separately sent them the link.
		//
		// Scoped entirely by identity.Email, the validated `email` claim. There
		// is no path, query or body parameter naming a user: enumerating
		// someone else's invites is not a check that could be forgotten but a
		// shape the route cannot express.
		//
		// Registered before /invites/{id} routes is unnecessary — chi matches
		// the static segment ahead of the wildcard regardless — but no GET
		// /invites/{id} exists in the first place.
		r.Get("/invites/pending", func(w http.ResponseWriter, req *http.Request) {
			identity := auth.IdentityFromContext(req.Context())

			ms, err := proc.ListRedeemableForEmail(req.Context(), identity.Email)
			if err != nil {
				log.WithError(err).WithField("correlation_id",
					telemetry.CorrelationIDFromContext(req.Context())).
					Error("list pending invites")
				server.WriteError(w, err)
				return
			}

			out := make([]server.Resource, 0, len(ms))
			for _, m := range ms {
				// A soft-deleted fleet returns ErrNotFound here, and an invite
				// into a fleet that no longer exists is not something to offer
				// — accepting it would mint a membership in a dead fleet. Drop
				// it from the listing rather than failing the whole request for
				// the sake of one stale row.
				f, ferr := fleets.GetByID(m.FleetID())
				if ferr != nil {
					log.WithError(ferr).WithField("fleet_id", m.FleetID()).
						Warn("pending invite names a fleet that cannot be read; omitting it")
					continue
				}
				out = append(out, TransformPending(m, f.Name()))
			}
			server.WriteJSON(w, http.StatusOK, server.Document{Data: out})
		})

		// DELETE /invites/{id} — owner-only
		r.Delete("/invites/{id}", func(w http.ResponseWriter, req *http.Request) {
			identity := auth.IdentityFromContext(req.Context())
			id := chi.URLParam(req, "id")

			inv, err := proc.GetByID(req.Context(), id)
			if err != nil {
				if errors.Is(err, server.ErrNotFound) {
					server.WriteError(w, server.ErrNotFound)
					return
				}
				log.WithError(err).WithField("invite_id", id).Error("get invite")
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
				if !errors.Is(err, server.ErrForbidden) {
					log.WithError(err).WithFields(logrus.Fields{
						"invite_id": id,
						"fleet_id":  inv.FleetID(),
						"user_id":   identity.UserID,
					}).Error("check invite owner")
				}
				server.WriteError(w, err)
				return
			}

			if err := proc.Delete(req.Context(), id); err != nil {
				log.WithError(err).WithFields(logrus.Fields{
					"invite_id": id,
					"fleet_id":  inv.FleetID(),
				}).Error("delete invite")
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
			traceID := telemetry.CorrelationIDFromContext(req.Context())

			inv, err := proc.GetByID(req.Context(), inviteID)
			if err != nil {
				if !errors.Is(err, server.ErrNotFound) {
					log.WithError(err).WithFields(logrus.Fields{
						"invite_id": inviteID,
						"trace_id":  traceID,
					}).Error("get invite")
				}
				server.WriteError(w, err)
				return
			}

			if err := authz.RequireSameFleet(identity, fleetID); err != nil {
				server.WriteError(w, err)
				return
			}
			// Path-pair mismatch → 404, not 403 (authz.RequireSameFleet's
			// convention, mirrored here by hand since the invite came from
			// proc.GetByID rather than a second RequireSameFleet call): the
			// invite belongs to a different fleet than the one named in the
			// path, and that must look identical to a nonexistent invite so
			// cross-fleet existence is never leaked.
			if inv.FleetID() != fleetID {
				server.WriteError(w, server.ErrNotFound)
				return
			}
			// Token-level owner gate (fast path)
			if err := authz.RequireOwner(identity); err != nil {
				server.WriteError(w, err)
				return
			}
			// Authoritative DB check via processor (stale-claim guard, design §9)
			if err := ownerCheck.RequireOwnerInFleet(fleetID, identity.UserID); err != nil {
				if !errors.Is(err, server.ErrForbidden) {
					log.WithError(err).WithFields(logrus.Fields{
						"invite_id": inviteID,
						"fleet_id":  fleetID,
						"user_id":   identity.UserID,
						"trace_id":  traceID,
					}).Error("check invite owner")
				}
				server.WriteError(w, err)
				return
			}

			updated, err := proc.Resend(req.Context(), inv, traceID)
			if err != nil {
				// ErrConflict is either the already-accepted invariant
				// (FR-RSND-3, checked before the cooldown so an accepted invite
				// never reports a cooldown it could never satisfy) or the TOCTOU
				// race the administrator's UPDATE catches; ErrTooManyRequests is
				// the cooldown itself (FR-RATE-2). All routine. Anything else is
				// the entropy source or the UPDATE failing.
				if !errors.Is(err, server.ErrConflict) && !errors.Is(err, server.ErrTooManyRequests) {
					log.WithError(err).WithFields(logrus.Fields{
						"invite_id": inviteID,
						"fleet_id":  fleetID,
						"trace_id":  traceID,
					}).Error("resend invite")
				}
				server.WriteError(w, err)
				return
			}
			server.WriteJSON(w, http.StatusOK, server.Document{Data: Transform(updated)})
		})

		// POST /invites/{token}/accept — look up by token; validate; create membership + stamp accepted_at
		r.Post("/invites/{token}/accept", func(w http.ResponseWriter, req *http.Request) {
			identity := auth.IdentityFromContext(req.Context())
			token := chi.URLParam(req, "token")
			traceID := telemetry.CorrelationIDFromContext(req.Context())

			inv, err := proc.GetByToken(req.Context(), token)
			if err != nil {
				if errors.Is(err, server.ErrNotFound) {
					server.WriteError(w, server.ErrNotFound)
					return
				}
				// The token itself must never reach a log field or message
				// (PRD §8 Security) — this branch logs no identifying field at
				// all, only the correlation id, since the lookup by definition
				// failed to resolve one.
				log.WithError(err).WithField("trace_id", traceID).Error("get invite by token")
				server.WriteError(w, err)
				return
			}

			updated, err := proc.Accept(req.Context(), inv, identity.UserID, identity.Email, traceID)
			if err != nil {
				// Already-accepted and expired are ordinary user outcomes the
				// response body explains; logging them adds noise. The next two
				// are worth being greppable: a mismatch is either a genuine
				// wrong-account attempt or a regression of the empty-email-claim
				// defect, and an unusable invite is a corrupt row an operator
				// should chase. Anything else is the accept transaction failing.
				//
				// Invite id and correlation id only: never inv.Email(), never
				// identity.Email (PRD FR-10/§8). The invite id joins to the row
				// for an operator who already has database access, so the line
				// itself discloses nothing.
				fields := logrus.Fields{
					"invite_id": inv.ID(),
					"fleet_id":  inv.FleetID(),
					"trace_id":  traceID,
				}
				switch {
				case errors.Is(err, ErrEmailMismatch):
					log.WithFields(fields).Warn("invite accept rejected: email mismatch")
				case errors.Is(err, ErrInviteUnusable):
					log.WithFields(fields).Error("invite accept rejected: invite row has no email address")
				case errors.Is(err, ErrAlreadyAccepted), errors.Is(err, ErrInviteExpired):
					// Routine; the 409 body says which.
				default:
					log.WithError(err).WithFields(fields).
						WithField("user_id", identity.UserID).Error("accept invite")
				}
				server.WriteError(w, err)
				return
			}
			server.WriteJSON(w, http.StatusOK, server.Document{Data: Transform(updated)})
		})
	}
}
