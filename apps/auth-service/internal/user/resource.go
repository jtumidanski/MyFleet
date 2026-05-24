package user

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"

	"github.com/jtumidanski/myfleet/packages/shared-go/auth"
	"github.com/jtumidanski/myfleet/packages/shared-go/server"
)

// InitializeRoutes wires GET /auth/me (design §8.1, FR-AUTH-3). Active fleet/role
// are read from the validated token's Identity; profile from the DB.
func InitializeRoutes(db *gorm.DB) func(chi.Router) {
	store := NewStore(db)
	return func(r chi.Router) {
		r.Get("/auth/me", func(w http.ResponseWriter, req *http.Request) {
			id := auth.IdentityFromContext(req.Context())
			m, err := store.GetBySub(id.UserID) // sub == user id in our tokens
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
