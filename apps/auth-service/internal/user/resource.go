package user

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/sirupsen/logrus"
	"gorm.io/gorm"

	"github.com/jtumidanski/myfleet/packages/shared-go/auth"
	"github.com/jtumidanski/myfleet/packages/shared-go/server"
)

// errInternal is what a failed lookup renders. It carries no detail on purpose:
// server.WriteError copies err.Error() into the response title, so returning the
// underlying error would publish database internals to any authenticated caller.
var errInternal = errors.New("internal server error")

// errThemeValidation names the offending field and the accepted values without
// echoing the caller's input (PRD §5.2, FR-SEC-2). It wraps server.ErrValidation
// so StatusFor still yields 422, and the message is a compile-time constant so
// no attacker-supplied string can reach the response title.
//
// Note the pair: user.ErrInvalidTheme is the DOMAIN error the processor
// returns; errThemeValidation is the TRANSPORT envelope rendered for it.
var errThemeValidation = fmt.Errorf("%w: themePreference must be one of light, dark, system",
	server.ErrValidation)

// maxUserIDs bounds both the response size and the work done per request
// (SEC-3). Applied AFTER de-duplication, so a repeated id cannot trip it.
const maxUserIDs = 100

// Both messages are compile-time constants: no caller-supplied string reaches
// the response, matching errThemeValidation above.
var (
	errIDsRequired = fmt.Errorf("%w: ids is required", server.ErrValidation)
	errTooManyIDs  = fmt.Errorf("%w: ids accepts at most 100 values", server.ErrValidation)
)

// FleetMemberGatherer returns the user ids of the active members of a fleet.
//
// Injected as a function value so the user package never imports the
// fleet-service membership client — the same constraint that produced
// PrincipalResolver (see auth-service/cmd/main.go Decision 1). It is also what
// makes the scoping rules unit-testable without standing up fleet-service.
type FleetMemberGatherer func(ctx context.Context, fleetID string) ([]string, error)

// parseUserIDs splits, trims and de-duplicates the `ids` query parameter.
// De-duplication precedes the cap so `?ids=a,a,a,…` cannot inflate the work
// done or trip the limit by itself.
func parseUserIDs(raw string) ([]string, error) {
	seen := make(map[string]bool)
	out := make([]string, 0)
	for _, part := range strings.Split(raw, ",") {
		v := strings.TrimSpace(part)
		if v == "" || seen[v] {
			continue
		}
		seen[v] = true
		out = append(out, v)
	}
	if len(out) == 0 {
		return nil, errIDsRequired
	}
	if len(out) > maxUserIDs {
		return nil, errTooManyIDs
	}
	return out, nil
}

// intersect keeps only the requested ids that are also fleet members, in the
// order they were requested. This is the whole of the scoping rule (FR-1.2):
// an id outside the set is dropped silently, so it is indistinguishable from an
// id that does not exist (SEC-1).
func intersect(requested, allowed []string) []string {
	set := make(map[string]bool, len(allowed))
	for _, a := range allowed {
		set[a] = true
	}
	out := make([]string, 0, len(requested))
	for _, r := range requested {
		if set[r] {
			out = append(out, r)
		}
	}
	return out
}

// nullIfEmpty renders an absent token claim as JSON `null` rather than `""`.
//
// auth.Identity holds every claim as a Go string, so "the user has no fleet"
// and "the user's fleet id is the empty string" are indistinguishable by the
// time they reach here — and `""` marshals to a value the client reads as
// present. A fleetless user then passed the SPA's `activeFleetId === null`
// guard and failed every page's `!activeFleetId` check, landing on
// "No fleet selected" with onboarding unreachable. The absent claim is a
// distinct state and the wire must say so.
func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// InitializeRoutes wires GET /auth/me (design §8.1, FR-AUTH-3), PATCH /auth/me
// (PRD §5.2) and GET /auth/users (task-014 §3).
//
// members resolves the caller's fleet roster for the /auth/users scoping rule.
func InitializeRoutes(log logrus.FieldLogger, db *gorm.DB, members FleetMemberGatherer) func(chi.Router) {
	proc := NewProcessor(log, NewProvider(db), NewAdministrator(db))
	return func(r chi.Router) {
		r.Get("/auth/me", func(w http.ResponseWriter, req *http.Request) {
			id := auth.IdentityFromContext(req.Context())
			// id.UserID is the JWT `sub` claim, which session.Processor sets to
			// our internal user id — NOT Google's sub. It must be looked up by
			// primary key. Calling GetBySub here matched it against google_sub,
			// never found the row, and 404'd every authenticated request, which
			// the SPA treats as logged-out — an unbreakable login loop.
			m, err := proc.GetByID(id.UserID)
			if err != nil {
				// Distinguish "no such user" from "the lookup failed". Mapping
				// everything to 404 meant a dropped connection or a pool timeout
				// logged the user out — apps/web AuthContext clears the access
				// token on any /auth/me error — and emitted no signal at all,
				// which is much of why the google_sub bug above needed a
				// production SELECT to find.
				if errors.Is(err, ErrNotFound) {
					server.WriteError(w, server.ErrNotFound)
					return
				}
				log.WithError(err).WithField("user_id", id.UserID).Error("auth/me lookup failed")
				// Deliberately not WriteError(w, err): the envelope puts
				// err.Error() in the title, which would leak database internals
				// to the client. errInternal renders a bare 500.
				server.WriteError(w, errInternal)
				return
			}
			server.WriteJSON(w, http.StatusOK, server.Document{
				Data: Transform(m),
				Meta: map[string]any{
					"activeFleetId": nullIfEmpty(id.ActiveFleetID),
					"role":          nullIfEmpty(id.Role),
				},
			})
		})

		// PATCH /auth/me — updates the caller's own preferences (PRD §5.2).
		//
		// The target user is id.UserID, the validated JWT `sub`. There is no
		// path parameter, no body field and no query parameter carrying a user
		// identifier, so horizontal privilege escalation is not a check that
		// could be forgotten but a shape that cannot express the attack
		// (FR-SEC-1).
		r.Patch("/auth/me", server.RegisterInputHandler(func(w http.ResponseWriter, req *http.Request, attrs struct {
			ThemePreference string `json:"themePreference"`
		},
		) {
			id := auth.IdentityFromContext(req.Context())
			m, err := proc.UpdateTheme(id.UserID, attrs.ThemePreference)
			if err != nil {
				// Client errors are not incidents — do not log them (FR-OBS-1).
				if errors.Is(err, ErrInvalidTheme) {
					server.WriteError(w, errThemeValidation)
					return
				}
				if errors.Is(err, ErrNotFound) {
					server.WriteError(w, server.ErrNotFound)
					return
				}
				log.WithError(err).WithField("user_id", id.UserID).Error("auth/me theme update failed")
				// Deliberately not WriteError(w, err): the envelope puts
				// err.Error() in the title, which would leak database internals
				// (FR-SEC-3).
				server.WriteError(w, errInternal)
				return
			}
			// No meta block: active fleet and role are token-derived and
			// unaffected by this call (PRD §5.2).
			server.WriteJSON(w, http.StatusOK, server.Document{Data: Transform(m)})
		}))

		// GET /auth/users?ids=a,b,c — resolve display names for a batch of user
		// ids, scoped to the caller's active fleet.
		//
		// Registered in this JWT-protected group, so SEC-2 is satisfied by
		// PLACEMENT rather than by a check that could be forgotten.
		//
		// Omission is the only failure mode: an id in another fleet, an id with
		// no users row, and a malformed id all produce the same 200 with the id
		// absent. There is deliberately no response shape meaning "that user
		// exists but you may not see them" (SEC-1).
		r.Get("/auth/users", func(w http.ResponseWriter, req *http.Request) {
			requested, err := parseUserIDs(req.URL.Query().Get("ids"))
			if err != nil {
				// Client errors are not incidents — do not log them.
				server.WriteError(w, err)
				return
			}

			id := auth.IdentityFromContext(req.Context())
			if id.ActiveFleetID == "" {
				// FR-1.3. Short-circuit before the hop: there is no fleet to ask
				// about, and the answer is the same either way.
				server.WriteJSON(w, http.StatusOK, server.Document{Data: []server.Resource{}})
				return
			}

			memberIDs, err := members(req.Context(), id.ActiveFleetID)
			if err != nil {
				// A 500, NOT an empty 200 (D4): an empty array would make a
				// fleet-service outage indistinguishable from a fleet with no
				// members, and only one of those shows up in metrics.
				log.WithError(err).Error("auth/users fleet member lookup failed")
				server.WriteError(w, errInternal)
				return
			}

			allowed := intersect(requested, memberIDs)
			ms, err := proc.ListByIDs(allowed)
			if err != nil {
				log.WithError(err).Error("auth/users lookup failed")
				// Deliberately not WriteError(w, err): the envelope puts
				// err.Error() in the title, which would leak database internals.
				server.WriteError(w, errInternal)
				return
			}

			// TransformSlice returns make([]server.Resource, 0, …), so an empty
			// result marshals as [] and never as null.
			//
			// Attributes also carries themePreference — another member's UI
			// preference, not sensitive but not the caller's business either.
			// Reusing Transform as-is is deliberate: a second transform to strip
			// one cosmetic field would fork the "keep rest.go and
			// types/models/user.ts in step" contract for no security gain.
			server.WriteJSON(w, http.StatusOK, server.Document{Data: TransformSlice(ms)})
		})
	}
}
