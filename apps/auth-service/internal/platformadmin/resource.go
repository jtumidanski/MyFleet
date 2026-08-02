package platformadmin

import (
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/sirupsen/logrus"
	"gorm.io/gorm"

	"github.com/jtumidanski/myfleet/packages/shared-go/server"
)

// MaxInternalLookupIDs bounds a single /internal/admin/users lookup. The
// endpoint is unauthenticated, so its input must be bounded; fleet-service
// chunks larger member sets. Mirrors mediaobject.MaxInternalLookupIDs.
const MaxInternalLookupIDs = 50

// InternalUser is one resolved user in the internal lookup response.
type InternalUser struct {
	ID          string     `json:"id"`
	Email       string     `json:"email"`
	DisplayName string     `json:"display_name"`
	CreatedAt   time.Time  `json:"created_at"`
	LastLoginAt *time.Time `json:"last_login_at"`
}

// internalUsersResponse carries Total so the paginated mode can report a page
// count. In the ids mode Total is simply the number resolved.
type internalUsersResponse struct {
	Users []InternalUser `json:"users"`
	Total int            `json:"total"`
}

type internalStatsResponse struct {
	Users int `json:"users"`
}

// InitializeInternalRoutes wires the network-restricted admin endpoints.
// Register this initializer WITHOUT JWT middleware, exactly as fleet-service
// does for membership.InitializeInternalRoutes.
//
// SECURITY: these routes have no authentication. auth-stripprefix strips only
// `/api`, and every public auth route lives under `/auth/…`, so a public request
// would have to be `/api/internal/…` — which matches no `/api/*` router and
// falls through to the SPA catch-all. That makes this surface safe BY ACCIDENT,
// which is one prefix change away from not safe, so the priority-200
// internal-deny rule in deploy/k8s/overlays/main/ingressroute.yaml ships with
// it. The two go together; never separately (design F2).
func InitializeInternalRoutes(log logrus.FieldLogger, db *gorm.DB) func(chi.Router) {
	prov := NewProvider(db)
	return func(r chi.Router) {
		// GET /internal/admin/stats → { "users": N }
		r.Get("/internal/admin/stats", func(w http.ResponseWriter, req *http.Request) {
			var n int64
			if err := db.Raw(`SELECT count(*) FROM auth.users`).Scan(&n).Error; err != nil {
				log.WithError(err).Error("internal admin user count")
				server.WriteError(w, err)
				return
			}
			server.WriteJSON(w, http.StatusOK, internalStatsResponse{Users: int(n)})
		})

		// GET /internal/admin/users — two modes on one route.
		//
		//   ?ids=a,b,c              resolve a known set (fleet detail's member
		//                           names). A missing id is simply absent rather
		//                           than an error: fleet-service resolves names
		//                           best-effort and warns if the lookup came
		//                           back short (FR-ADMIN-FLEET-5).
		//   ?page[number]=&page[size]=  the paginated directory behind
		//                           /admin/users (FR-ADMIN-FLEET-6).
		//
		// One route rather than two because the shape returned is identical and
		// the caller's intent is unambiguous from the parameters.
		r.Get("/internal/admin/users", func(w http.ResponseWriter, req *http.Request) {
			if raw := req.URL.Query().Get("ids"); raw != "" || req.URL.Query().Get("page[size]") == "" {
				ids := splitIDs(raw)
				if len(ids) > MaxInternalLookupIDs {
					server.WriteError(w, server.ErrValidation)
					return
				}
				if len(ids) == 0 {
					server.WriteJSON(w, http.StatusOK, internalUsersResponse{Users: []InternalUser{}})
					return
				}
				var rows []InternalUser
				if err := db.Raw(`SELECT id, email, display_name, created_at, last_login_at
				                  FROM auth.users WHERE id IN ?`, ids).Scan(&rows).Error; err != nil {
					log.WithError(err).Error("internal admin user lookup")
					server.WriteError(w, err)
					return
				}
				if rows == nil {
					rows = []InternalUser{}
				}
				server.WriteJSON(w, http.StatusOK, internalUsersResponse{Users: rows, Total: len(rows)})
				return
			}

			page := server.ParsePage(req)
			var total int64
			if err := db.Raw(`SELECT count(*) FROM auth.users`).Scan(&total).Error; err != nil {
				log.WithError(err).Error("internal admin user count")
				server.WriteError(w, err)
				return
			}
			var rows []InternalUser
			if err := db.Raw(`SELECT id, email, display_name, created_at, last_login_at
			                  FROM auth.users ORDER BY created_at DESC LIMIT ? OFFSET ?`,
				page.Size, page.Offset()).Scan(&rows).Error; err != nil {
				log.WithError(err).Error("internal admin user list")
				server.WriteError(w, err)
				return
			}
			if rows == nil {
				rows = []InternalUser{}
			}
			server.WriteJSON(w, http.StatusOK, internalUsersResponse{Users: rows, Total: int(total)})
		})

		// GET /internal/admin/platform-admins/{userId} → 200 | 404.
		//
		// This is the stale-claim re-verification (FR-ADMIN-AUTH-7): the claim
		// is stamped at mint time, so a revoked admin holds a valid token for up
		// to 15 minutes. fleet-service calls this before an irreversible purge
		// and fails closed on an error.
		r.Get("/internal/admin/platform-admins/{userId}", func(w http.ResponseWriter, req *http.Request) {
			userID := chi.URLParam(req, "userId")
			ok, err := prov.IsAdmin(userID)
			if err != nil {
				log.WithError(err).Error("internal platform-admin lookup")
				server.WriteError(w, err)
				return
			}
			if !ok {
				server.WriteError(w, server.ErrNotFound)
				return
			}
			w.WriteHeader(http.StatusOK)
		})
	}
}

// splitIDs parses the comma-separated ids parameter, dropping empty segments so
// a trailing comma is not a lookup for "".
func splitIDs(raw string) []string {
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if id := strings.TrimSpace(p); id != "" {
			out = append(out, id)
		}
	}
	return out
}
