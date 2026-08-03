package session

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/sirupsen/logrus"

	"github.com/jtumidanski/myfleet/packages/shared-go/server"
)

// PrincipalResolver resolves the COMPLETE session.Principal for a user — email
// from the local users table, active fleet and role from fleet-service.
//
// It is injected (Decision 1) so neither session nor oidc imports the concrete
// membership client, and it is the single construction site for Principal: a
// call site that assembles its own can omit a field and mint a token missing a
// claim, which is exactly what the refresh path did with Email.
type PrincipalResolver func(ctx context.Context, userID string) (Principal, error)

// RefreshCookieName is the cookie carrying the opaque rotating refresh token.
const RefreshCookieName = "refresh_token"

// InitializePublicRoutes wires POST /auth/refresh and POST /auth/logout. These
// are PUBLIC (no JWT middleware): the refresh token itself authenticates the
// caller. The processor must already be wired with a store via WithStore.
// cookieSecure controls the Secure flag on the refresh cookie (false for local
// plaintext HTTP, true in production).
func InitializePublicRoutes(log logrus.FieldLogger, proc *Processor, resolve PrincipalResolver, cookieSecure bool) func(chi.Router) {
	return func(r chi.Router) {
		r.Post("/auth/refresh", refreshHandler(log, proc, resolve, cookieSecure))
		r.Post("/auth/logout", logoutHandler(log, proc, cookieSecure))
	}
}

func refreshHandler(log logrus.FieldLogger, proc *Processor, resolve PrincipalResolver, cookieSecure bool) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		raw := readRefreshToken(req)
		if raw == "" {
			server.WriteError(w, server.ErrUnauthorized)
			return
		}

		newRaw, userID, err := proc.Rotate(raw)
		if err != nil {
			// Reuse, expiry, or unknown token all surface as unauthorized; the
			// processor has already revoked the family on reuse.
			if err != ErrNotFound && err != ErrTokenReuse && err != ErrTokenExpired {
				log.WithError(err).Error("rotate refresh token")
			}
			clearRefreshCookie(w, cookieSecure)
			server.WriteError(w, server.ErrUnauthorized)
			return
		}

		principal, err := resolve(req.Context(), userID)
		if err != nil {
			// Fail closed (FR-5): a token with incomplete identity is never
			// minted. Clear the cookie too — unlike this path's previous bare
			// 401 — so a session whose user row is gone stops re-presenting a
			// credential that can only 401.
			log.WithError(err).Error("resolve principal on refresh")
			clearRefreshCookie(w, cookieSecure)
			server.WriteError(w, server.ErrUnauthorized)
			return
		}

		access, err := proc.MintAccess(principal)
		if err != nil {
			log.WithError(err).Error("mint access on refresh")
			server.WriteError(w, server.ErrUnauthorized)
			return
		}

		SetRefreshCookie(w, newRaw, cookieSecure)
		server.WriteJSON(w, http.StatusOK, server.Document{
			Data: Transform(Issued{Access: access, Refresh: newRaw}),
		})
	}
}

func logoutHandler(log logrus.FieldLogger, proc *Processor, cookieSecure bool) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		raw := readRefreshToken(req)
		if raw != "" {
			if err := proc.Logout(raw); err != nil {
				// Kept alongside WriteError's own 5xx log line: this one names
				// the operation, that one only knows the status. One redundant
				// line beats an operator grepping for "logout" and finding
				// nothing.
				log.WithError(err).Error("logout revoke family")
				// Cookie FIRST. SetCookie appends a header and WriteError
				// flushes them, so the reverse order drops the Set-Cookie
				// silently. Clearing the browser's copy is strictly
				// risk-reducing even when the family survives in the database.
				clearRefreshCookie(w, cookieSecure)
				// The raw error is safe to pass through: StatusFor maps
				// anything without a matching sentinel to 500, and WriteError
				// redacts the text of every 5xx body. session.ErrNotFound is
				// this package's own sentinel, not server.ErrNotFound, so it
				// could not be mapped to a 404 by accident even if it reached
				// here — which it cannot, since Processor.Logout collapses it
				// to nil.
				server.WriteError(w, err)
				return
			}
		}
		clearRefreshCookie(w, cookieSecure)
		w.WriteHeader(http.StatusNoContent)
	}
}

// readRefreshToken extracts the raw refresh token from the cookie, falling back
// to a JSON body {"refreshToken": "..."}.
func readRefreshToken(req *http.Request) string {
	if c, err := req.Cookie(RefreshCookieName); err == nil && c.Value != "" {
		return c.Value
	}
	if req.Body == nil {
		return ""
	}
	var body struct {
		RefreshToken string `json:"refreshToken"`
	}
	if err := json.NewDecoder(req.Body).Decode(&body); err == nil {
		return body.RefreshToken
	}
	return ""
}

// SetRefreshCookie writes the rotating refresh token as an HttpOnly cookie.
// secure controls the Secure flag (false for local plaintext HTTP).
func SetRefreshCookie(w http.ResponseWriter, raw string, secure bool) {
	http.SetCookie(w, &http.Cookie{
		Name:     RefreshCookieName,
		Value:    raw,
		Path:     "/",
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
		Expires:  time.Now().Add(refreshTTL),
	})
}

func clearRefreshCookie(w http.ResponseWriter, secure bool) {
	http.SetCookie(w, &http.Cookie{
		Name:     RefreshCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
}
