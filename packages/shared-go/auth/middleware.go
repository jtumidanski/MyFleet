package auth

import (
	"net/http"
	"strings"

	"github.com/golang-jwt/jwt/v5"
	"github.com/sirupsen/logrus"

	"github.com/jtumidanski/myfleet/packages/shared-go/server"
	"github.com/jtumidanski/myfleet/packages/shared-go/telemetry"
)

// Option configures the JWT middleware. Options are variadic so every existing
// single-argument JWT(keyfn) call site keeps compiling untouched; services with
// no Identity.Email consumer are deliberately left on that form.
type Option func(*jwtConfig)

type jwtConfig struct{ log logrus.FieldLogger }

// WithLogger attaches a logger so the middleware can report an identity claim
// that validated but arrived empty. With no logger the middleware logs nothing
// and behaves exactly as it did before options existed.
func WithLogger(l logrus.FieldLogger) Option { return func(c *jwtConfig) { c.log = l } }

// JWT validates RS256 tokens via JWKS and puts an Identity on context (design §9).
func JWT(keyfn jwt.Keyfunc, opts ...Option) func(http.Handler) http.Handler {
	var cfg jwtConfig
	for _, o := range opts {
		o(&cfg)
	}
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
				PlatformAdmin: boolean(claims["platform_admin"]),
			}
			// A token that validates but carries no email is the signature of a
			// minting path that built a partial principal — the defect this task
			// fixed, which previously surfaced only as an unexplained 409 on
			// invite acceptance. Log it and proceed: observability, not
			// enforcement. Never the raw token, never an email address.
			if cfg.log != nil && id.Email == "" {
				cfg.log.WithFields(logrus.Fields{
					"sub":            id.UserID,
					"correlation_id": telemetry.CorrelationIDFromContext(r.Context()),
				}).Warn("access token missing email claim")
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

// boolean mirrors str: anything that is not a JSON boolean — absent, a string,
// a number — reads as false. Failing closed here means a hand-rolled or
// half-migrated token can never grant the platform tier (FR-ADMIN-AUTH-5).
func boolean(v any) bool {
	b, ok := v.(bool)
	return ok && b
}
