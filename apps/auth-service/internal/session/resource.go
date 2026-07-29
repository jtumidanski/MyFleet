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

// MembershipResolver resolves a user's active fleet and role for token minting.
// It is injected (Decision 1) so the session package never imports the concrete
// membership client, avoiding an import cycle.
type MembershipResolver func(ctx context.Context, userID string) (fleetID, role string, err error)

// RefreshCookieName is the cookie carrying the opaque rotating refresh token.
const RefreshCookieName = "refresh_token"

// InitializePublicRoutes wires POST /auth/refresh and POST /auth/logout. These
// are PUBLIC (no JWT middleware): the refresh token itself authenticates the
// caller. The processor must already be wired with a store via WithStore.
// cookieSecure controls the Secure flag on the refresh cookie (false for local
// plaintext HTTP, true in production).
func InitializePublicRoutes(log logrus.FieldLogger, proc *Processor, resolve MembershipResolver, cookieSecure bool) func(chi.Router) {
	return func(r chi.Router) {
		r.Post("/auth/refresh", refreshHandler(log, proc, resolve, cookieSecure))
		r.Post("/auth/logout", logoutHandler(log, proc, cookieSecure))
	}
}

func refreshHandler(log logrus.FieldLogger, proc *Processor, resolve MembershipResolver, cookieSecure bool) http.HandlerFunc {
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

		fleetID, role, err := resolve(req.Context(), userID)
		if err != nil {
			log.WithError(err).Error("resolve membership on refresh")
			server.WriteError(w, server.ErrUnauthorized)
			return
		}

		access, err := proc.MintAccess(Principal{UserID: userID, ActiveFleetID: fleetID, Role: role})
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
				log.WithError(err).Error("logout revoke family")
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
