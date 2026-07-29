package jwks

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/jtumidanski/myfleet/packages/shared-go/server"
)

// InitializeRoutes serves GET /.well-known/jwks.json (public; no JWT required).
func InitializeRoutes(ks *KeySet) func(chi.Router) {
	return func(r chi.Router) {
		r.Get("/.well-known/jwks.json", func(w http.ResponseWriter, _ *http.Request) {
			server.WriteJSON(w, http.StatusOK, ks.PublicJWKS())
		})
	}
}
