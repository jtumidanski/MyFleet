package admin

import (
	"errors"
	"net/http"

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
