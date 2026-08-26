package admin

import (
	"errors"
	"net/http"
	"unicode"

	"github.com/go-chi/chi/v5"
	"github.com/sirupsen/logrus"

	"github.com/jtumidanski/myfleet/apps/fleet-service/internal/authz"
	"github.com/jtumidanski/myfleet/packages/shared-go/auth"
	"github.com/jtumidanski/myfleet/packages/shared-go/server"
	"github.com/jtumidanski/myfleet/packages/shared-go/telemetry"
)

// errInternal is what a failed admin read renders. It carries no detail on
// purpose: server.WriteError copies err.Error() into the response title, so
// returning the underlying error would publish database internals to the client.
var errInternal = errors.New("internal server error")

// isClientError reports whether an error is the caller's fault. Those are not
// incidents and must not be logged at error level — an operator typing a wrong
// confirmation phrase is the system working.
func isClientError(err error) bool {
	return errors.Is(err, server.ErrValidation) || errors.Is(err, server.ErrConflict) ||
		errors.Is(err, server.ErrNotFound) || errors.Is(err, server.ErrForbidden)
}

// errTransferNotAdmin is the 403 both transfer routes write when the caller is
// not a platform administrator.
//
// The /admin tree's other routes write a bare server.ErrForbidden, whose title
// is "forbidden" and whose detail is empty. The transfer contract requires
// EVERY 4xx to carry an actionable sentence, because FR-XFER-UI-7 shows the
// detail to the operator verbatim and an empty one leaves them unable to tell
// "you lack the role" from "the service is broken". It deliberately names no
// vehicle and no fleet: the caller has just been told they may not see them.
var errTransferNotAdmin = server.Detailed(server.ErrForbidden,
	"platform administrator privileges are required to transfer a vehicle between fleets")

// errDestinationMalformed is the 422 for a destination_fleet_id that could not
// name any fleet.
//
// Without it such a value falls through to a 404, which invites the operator to
// go looking for a fleet that never could have existed. The processor only
// distinguishes "absent" (errDestinationRequired) and "the fleet it is already
// in" (errSameFleet), so structural rejection belongs here, at the edge.
var errDestinationMalformed = server.Detailed(server.ErrValidation,
	"destination_fleet_id is malformed; a fleet id contains no whitespace or control characters")

// maxFleetIDLen bounds a destination id. Fleet ids are UUIDs (36 characters) in
// production and short readable strings in the test fixtures; the limit is
// generous on purpose, because its job is to reject values that are obviously
// not identifiers, not to re-specify the id format.
//
// Deliberately NOT a UUID-format check. Every other admin route treats ids as
// opaque strings, and a format assertion here would be the only place in the
// tree that disagrees.
const maxFleetIDLen = 64

// checkDestinationFleetID rejects a destination id no fleet could have.
//
// An empty id is NOT this function's business: it is legal on the preview (the
// "pick a destination" state of the dialog, where the source-side counts are
// still computable) and the processor answers it on the transfer with its own
// "required" sentence.
func checkDestinationFleetID(id string) error {
	if id == "" {
		return nil
	}
	if len(id) > maxFleetIDLen {
		return errDestinationMalformed
	}
	for _, r := range id {
		if unicode.IsSpace(r) || !unicode.IsPrint(r) {
			return errDestinationMalformed
		}
	}
	return nil
}

// mapStoreError translates the package's sentinels into HTTP-shaped errors.
func mapStoreError(err error) error {
	if errors.Is(err, ErrOperationNotFound) {
		return server.ErrNotFound
	}
	return err
}

// InitializeRoutes wires the /admin tree. Register it in its OWN chi group with
// authmw.JWT and nothing else shared — the separation from the ordinary
// authenticated group is the safety argument for the whole cross-fleet API, and
// arch_test.go enforces it (risks.md R7).
//
// Every handler starts with RequirePlatformAdmin and nothing else. In
// particular there is no RequireSameFleet anywhere below: these endpoints are
// deliberately fleet-agnostic, and none of them may require an active fleet
// (FR-ADMIN-AUTH-9).
func InitializeRoutes(log logrus.FieldLogger, proc *Processor) func(chi.Router) {
	return func(r chi.Router) {
		r.Get("/admin/stats", func(w http.ResponseWriter, req *http.Request) {
			if !authorized(w, req) {
				return
			}
			s, err := proc.Stats(req.Context())
			if err != nil {
				log.WithError(err).Error("admin stats")
				server.WriteError(w, errInternal)
				return
			}
			server.WriteJSON(w, http.StatusOK, server.Document{Data: TransformStats(s)})
		})

		r.Get("/admin/fleets", func(w http.ResponseWriter, req *http.Request) {
			if !authorized(w, req) {
				return
			}
			deleted, err := ParseDeletedFilter(req.URL.Query().Get("deleted"))
			if err != nil {
				server.WriteError(w, err)
				return
			}
			page := server.ParsePage(req)
			got, err := proc.ListFleets(req.Context(), req.URL.Query().Get("q"), deleted, page)
			if err != nil {
				log.WithError(err).Error("admin list fleets")
				server.WriteError(w, errInternal)
				return
			}
			server.WriteJSON(w, http.StatusOK, server.Document{
				Data: TransformFleets(got.Rows),
				Meta: listMeta(page, got.Total, got.Warnings),
			})
		})

		r.Get("/admin/fleets/{fleetId}", func(w http.ResponseWriter, req *http.Request) {
			if !authorized(w, req) {
				return
			}
			got, err := proc.GetFleet(req.Context(), chi.URLParam(req, "fleetId"))
			if err != nil {
				if isClientError(err) {
					server.WriteError(w, err)
					return
				}
				log.WithError(err).Error("admin get fleet")
				server.WriteError(w, errInternal)
				return
			}
			server.WriteJSON(w, http.StatusOK, server.Document{Data: TransformFleetDetail(got)})
		})

		r.Get("/admin/users", func(w http.ResponseWriter, req *http.Request) {
			if !authorized(w, req) {
				return
			}
			page := server.ParsePage(req)
			got, err := proc.ListUsers(req.Context(), page)
			if err != nil {
				log.WithError(err).Error("admin list users")
				server.WriteError(w, errInternal)
				return
			}
			server.WriteJSON(w, http.StatusOK, server.Document{
				Data: TransformUsers(got.Rows),
				Meta: listMeta(page, got.Total, got.Warnings),
			})
		})

		r.Get("/admin/purge-operations", func(w http.ResponseWriter, req *http.Request) {
			if !authorized(w, req) {
				return
			}
			page := server.ParsePage(req)
			ops, total, err := proc.ListOperations(req.URL.Query().Get("status"), page)
			if err != nil {
				log.WithError(err).Error("admin list purge operations")
				server.WriteError(w, errInternal)
				return
			}
			server.WriteJSON(w, http.StatusOK, server.Document{
				Data: TransformOperations(ops),
				Meta: listMeta(page, total, nil),
			})
		})

		r.Get("/admin/purge-operations/{id}", func(w http.ResponseWriter, req *http.Request) {
			if !authorized(w, req) {
				return
			}
			op, err := proc.GetOperation(chi.URLParam(req, "id"))
			if err != nil {
				if mapped := mapStoreError(err); isClientError(mapped) {
					server.WriteError(w, mapped)
					return
				}
				log.WithError(err).Error("admin get purge operation")
				server.WriteError(w, errInternal)
				return
			}
			server.WriteJSON(w, http.StatusOK, server.Document{Data: TransformOperation(op)})
		})

		r.Post("/admin/purge-operations", server.RegisterInputHandler(
			func(w http.ResponseWriter, req *http.Request, attrs struct {
				Scope        string `json:"scope"`
				TargetType   string `json:"target_type"`
				TargetID     string `json:"target_id"`
				Confirmation string `json:"confirmation"`
			},
			) {
				identity := auth.IdentityFromContext(req.Context())
				if err := authz.RequirePlatformAdmin(identity); err != nil {
					server.WriteError(w, err)
					return
				}
				op, err := proc.Create(req.Context(), CreateInput{
					Scope:         Scope(attrs.Scope),
					TargetType:    attrs.TargetType,
					TargetID:      attrs.TargetID,
					Confirmation:  attrs.Confirmation,
					ActorUserID:   identity.UserID,
					ActorEmail:    identity.Email,
					CorrelationID: telemetry.CorrelationIDFromContext(req.Context()),
				})
				if err != nil {
					// Client errors are not incidents — do not log them.
					if isClientError(err) {
						server.WriteError(w, err)
						return
					}
					log.WithError(err).Error("create purge operation")
					server.WriteError(w, errInternal)
					return
				}
				server.WriteJSON(w, http.StatusCreated,
					server.Document{Data: TransformOperation(op)})
			}))

		r.Delete("/admin/purge-operations/{id}", func(w http.ResponseWriter, req *http.Request) {
			identity, ok := authorizedIdentity(w, req)
			if !ok {
				return
			}
			op, err := proc.Cancel(req.Context(), chi.URLParam(req, "id"), actorFrom(req, identity))
			if err != nil {
				if mapped := mapStoreError(err); isClientError(mapped) {
					server.WriteError(w, mapped)
					return
				}
				log.WithError(err).Error("cancel purge operation")
				server.WriteError(w, errInternal)
				return
			}
			server.WriteJSON(w, http.StatusOK, server.Document{Data: TransformOperation(op)})
		})

		r.Post("/admin/purge-operations/{id}/retry", func(w http.ResponseWriter, req *http.Request) {
			identity, ok := authorizedIdentity(w, req)
			if !ok {
				return
			}
			op, err := proc.Retry(req.Context(), chi.URLParam(req, "id"), actorFrom(req, identity))
			if err != nil {
				if mapped := mapStoreError(err); isClientError(mapped) {
					server.WriteError(w, mapped)
					return
				}
				log.WithError(err).Error("retry purge operation")
				server.WriteError(w, errInternal)
				return
			}
			server.WriteJSON(w, http.StatusOK, server.Document{Data: TransformOperation(op)})
		})

		// PreviewVehicleTransfer performs NO authorization and NO eligibility
		// check of its own — it will happily describe a soft-deleted or
		// pending-purge vehicle. This guard is therefore the only thing standing
		// between an ordinary signed-in caller and one household's vehicle label
		// plus the names of two fleets they have no membership in. It is not
		// belt-and-braces; it is the belt.
		r.Get("/admin/vehicles/{vehicleId}/transfer-preview", func(w http.ResponseWriter, req *http.Request) {
			if _, ok := authorizedFor(w, req, errTransferNotAdmin); !ok {
				return
			}
			// destination_fleet_id is optional: without it the response omits
			// the destination fields and categories_to_create, which cannot be
			// computed without knowing where the car is going.
			destFleetID := req.URL.Query().Get("destination_fleet_id")
			if err := checkDestinationFleetID(destFleetID); err != nil {
				server.WriteError(w, err)
				return
			}
			pv, err := proc.PreviewVehicleTransfer(req.Context(),
				chi.URLParam(req, "vehicleId"), destFleetID)
			if err != nil {
				if isClientError(err) {
					server.WriteError(w, err)
					return
				}
				log.WithError(err).Error("admin vehicle transfer preview")
				server.WriteError(w, errInternal)
				return
			}
			server.WriteJSON(w, http.StatusOK, server.Document{Data: TransformTransferPreview(pv)})
		})

		r.Post("/admin/vehicles/{vehicleId}/transfer", server.RegisterInputHandler(
			func(w http.ResponseWriter, req *http.Request, attrs struct {
				DestinationFleetID string `json:"destination_fleet_id"`
				Confirmation       string `json:"confirmation"`
			},
			) {
				// Same gate as the preview, before the body is looked at for
				// anything but decoding: a caller who may not transfer must not
				// learn from the response whether the vehicle or the
				// destination exists.
				identity, ok := authorizedFor(w, req, errTransferNotAdmin)
				if !ok {
					return
				}
				if err := checkDestinationFleetID(attrs.DestinationFleetID); err != nil {
					server.WriteError(w, err)
					return
				}
				res, err := proc.TransferVehicle(req.Context(), TransferInput{
					VehicleID:     chi.URLParam(req, "vehicleId"),
					DestFleetID:   attrs.DestinationFleetID,
					Confirmation:  attrs.Confirmation,
					ActorUserID:   identity.UserID,
					ActorEmail:    identity.Email,
					CorrelationID: telemetry.CorrelationIDFromContext(req.Context()),
				})
				if err != nil {
					// A 503 is deliberately NOT a client error: it is an
					// incident, already logged where it happened, so it is
					// passed through rather than being relogged as a generic
					// transaction failure. Its detail survives the 5xx
					// redaction — server.WriteError renders an authored
					// Detailed sentence on a 503, and only on a 503 — which is
					// what tells the operator the transfer was rolled back
					// whole rather than half-applied.
					if isClientError(err) {
						server.WriteError(w, err)
						return
					}
					if errors.Is(err, server.ErrServiceUnavailable) {
						server.WriteError(w, err)
						return
					}
					log.WithError(err).Error("transfer vehicle")
					server.WriteError(w, errInternal)
					return
				}
				server.WriteJSON(w, http.StatusOK, server.Document{
					Data: TransformTransfer(res),
					Meta: TransferMeta(res),
				})
			}))

		r.Get("/admin/audit-events", func(w http.ResponseWriter, req *http.Request) {
			if !authorized(w, req) {
				return
			}
			page := server.ParsePage(req)
			events, total, err := proc.ListAuditEvents(AuditFilter{
				Action: req.URL.Query().Get("action"),
				Actor:  req.URL.Query().Get("actor"),
			}, page)
			if err != nil {
				log.WithError(err).Error("admin list audit events")
				server.WriteError(w, errInternal)
				return
			}
			server.WriteJSON(w, http.StatusOK, server.Document{
				Data: TransformAuditEvents(events),
				Meta: listMeta(page, total, nil),
			})
		})
	}
}

// authorizedIdentity is the guard every admin handler runs first. It returns the
// caller's identity, or writes the error and reports false.
func authorizedIdentity(w http.ResponseWriter, req *http.Request) (auth.Identity, bool) {
	id := auth.IdentityFromContext(req.Context())
	if err := authz.RequirePlatformAdmin(id); err != nil {
		server.WriteError(w, err)
		return auth.Identity{}, false
	}
	return id, true
}

// authorized is authorizedIdentity for the handlers that do not need the
// identity itself.
func authorized(w http.ResponseWriter, req *http.Request) bool {
	_, ok := authorizedIdentity(w, req)
	return ok
}

// authorizedFor is authorizedIdentity with a route-specific refusal. The gate
// is identical — authz.RequirePlatformAdmin, same 403 — but the caller supplies
// the error, so a route whose contract promises a detail on every 4xx can keep
// that promise without changing the sentinel every other admin route writes.
func authorizedFor(w http.ResponseWriter, req *http.Request, forbidden error) (auth.Identity, bool) {
	id := auth.IdentityFromContext(req.Context())
	if err := authz.RequirePlatformAdmin(id); err != nil {
		server.WriteError(w, forbidden)
		return auth.Identity{}, false
	}
	return id, true
}

func actorFrom(req *http.Request, id auth.Identity) Actor {
	return Actor{
		UserID:        id.UserID,
		Email:         id.Email,
		CorrelationID: telemetry.CorrelationIDFromContext(req.Context()),
	}
}

// listMeta carries pagination and any degradation warnings. Warnings live in
// meta rather than in the error envelope because the response IS successful —
// it is simply missing a part the client should be told about (FR-ADMIN-STATS-5).
func listMeta(page server.Page, total int, warnings []string) map[string]any {
	m := map[string]any{"page": page.Meta(total)}
	if len(warnings) > 0 {
		m["warnings"] = warnings
	}
	return m
}
