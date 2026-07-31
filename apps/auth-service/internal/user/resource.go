package user

import (
	"errors"
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

// InitializeRoutes wires GET /auth/me (design §8.1, FR-AUTH-3). Active fleet/role
// are read from the validated token's Identity; profile from the DB.
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
				Meta: map[string]any{"activeFleetId": id.ActiveFleetID, "role": id.Role},
			})
		})
	}
}
