package user

import (
	"errors"
	"fmt"
	"net/http"

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

// InitializeRoutes wires GET /auth/me (design §8.1, FR-AUTH-3) and
// PATCH /auth/me (PRD §5.2). Active fleet/role are read from the validated
// token's Identity; profile from the DB.
func InitializeRoutes(log logrus.FieldLogger, db *gorm.DB) func(chi.Router) {
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
	}
}
