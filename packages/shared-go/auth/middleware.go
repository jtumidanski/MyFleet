package auth

import (
	"net/http"
	"strings"

	"github.com/golang-jwt/jwt/v5"

	"github.com/jtumidanski/myfleet/packages/shared-go/server"
)

// JWT validates RS256 tokens via JWKS and puts an Identity on context (design §9).
func JWT(keyfn jwt.Keyfunc) func(http.Handler) http.Handler { return jwtWithKeyfunc(keyfn) }

func jwtWithKeyfunc(keyfn jwt.Keyfunc) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			raw := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
			if raw == "" || raw == r.Header.Get("Authorization") {
				server.WriteError(w, server.ErrUnauthorized)
				return
			}
			claims := jwt.MapClaims{}
			tok, err := jwt.ParseWithClaims(raw, claims, keyfn, jwt.WithValidMethods([]string{"RS256"}))
			if err != nil || !tok.Valid {
				server.WriteError(w, server.ErrUnauthorized)
				return
			}
			id := Identity{
				UserID:        str(claims["sub"]),
				Email:         str(claims["email"]),
				ActiveFleetID: str(claims["active_fleet_id"]),
				Role:          str(claims["role"]),
			}
			next.ServeHTTP(w, r.WithContext(WithIdentity(r.Context(), id)))
		})
	}
}

// RequireRole returns 403 if the caller's role is not in allowed (design §9).
func RequireRole(allowed ...string) func(http.Handler) http.Handler {
	set := map[string]bool{}
	for _, a := range allowed {
		set[a] = true
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !set[IdentityFromContext(r.Context()).Role] {
				server.WriteError(w, server.ErrForbidden)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func str(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}
