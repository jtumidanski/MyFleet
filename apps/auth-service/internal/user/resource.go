package user

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/sirupsen/logrus"
	"gorm.io/gorm"

	"github.com/jtumidanski/myfleet/packages/shared-go/auth"
	"github.com/jtumidanski/myfleet/packages/shared-go/server"
)

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
				server.WriteError(w, server.ErrNotFound)
				return
			}
			server.WriteJSON(w, http.StatusOK, server.Document{
				Data: Transform(m),
				Meta: map[string]any{"activeFleetId": id.ActiveFleetID, "role": id.Role},
			})
		})
	}
}
