package oidc

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/sirupsen/logrus"

	"github.com/jtumidanski/myfleet/apps/auth-service/internal/session"
	"github.com/jtumidanski/myfleet/apps/auth-service/internal/user"
)

const stateCookieName = "oidc_state"

// stateTTL bounds how long a login attempt's state/nonce cookie is valid.
const stateTTL = 10 * time.Minute

// Dependencies bundles everything the callback orchestration needs. The
// membership resolver is injected (Decision 1) so this package never imports
// the concrete membership client.
type Dependencies struct {
	OIDC        *Processor
	Users       *user.Processor
	Sessions    *session.Processor
	Resolve     session.MembershipResolver
	StateSecret []byte
	// AppBaseURL is the SPA origin the browser is redirected back to.
	AppBaseURL string
	// HomePath / OnboardingPath are relative paths under AppBaseURL.
	HomePath       string
	OnboardingPath string
	// CookieSecure controls the Secure flag on cookies this package sets. It is
	// false for local plaintext HTTP (Traefik :80) and true in production.
	CookieSecure bool
}

// InitializeRoutes wires GET /auth/login/google and GET /auth/callback. Both are
// PUBLIC (no JWT middleware).
func InitializeRoutes(log logrus.FieldLogger, d Dependencies) func(chi.Router) {
	return func(r chi.Router) {
		r.Get("/auth/login/google", loginHandler(log, d))
		r.Get("/auth/callback", callbackHandler(log, d))
	}
}

func loginHandler(log logrus.FieldLogger, d Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		state := uuid.NewString()
		nonce := uuid.NewString()
		// Persist state+nonce in a signed, short-lived cookie for CSRF/replay
		// defense; verified on callback.
		setStateCookie(w, d.StateSecret, state, nonce, d.CookieSecure)
		http.Redirect(w, req, d.OIDC.AuthCodeURL(state, nonce), http.StatusFound)
	}
}

func callbackHandler(log logrus.FieldLogger, d Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		code := req.URL.Query().Get("code")
		state := req.URL.Query().Get("state")
		if code == "" || state == "" {
			http.Error(w, "missing code or state", http.StatusBadRequest)
			return
		}
		wantNonce, ok := verifyStateCookie(req, d.StateSecret, state)
		if !ok {
			http.Error(w, "invalid state", http.StatusBadRequest)
			return
		}
		clearStateCookie(w, d.CookieSecure)

		ctx := req.Context()
		rawIDToken, err := d.OIDC.Exchange(ctx, code)
		if err != nil {
			log.WithError(err).Error("oidc code exchange")
			http.Error(w, "authentication failed", http.StatusBadGateway)
			return
		}
		profile, gotNonce, err := d.OIDC.Verify(ctx, rawIDToken)
		if err != nil {
			log.WithError(err).Error("oidc id_token verification")
			http.Error(w, "authentication failed", http.StatusUnauthorized)
			return
		}
		// idtoken.Validate does not check the nonce; bind the id_token to this
		// login attempt by comparing its nonce to the one in the state cookie.
		if gotNonce == "" || !hmac.Equal([]byte(gotNonce), []byte(wantNonce)) {
			log.Error("oidc nonce mismatch")
			http.Error(w, "authentication failed", http.StatusUnauthorized)
			return
		}

		u, err := d.Users.ProvisionFromGoogle(profile)
		if err != nil {
			log.WithError(err).Error("provision user from google")
			http.Error(w, "authentication failed", http.StatusInternalServerError)
			return
		}

		fleetID, role, err := d.Resolve(ctx, u.ID())
		if err != nil {
			log.WithError(err).Error("resolve membership on callback")
			http.Error(w, "authentication failed", http.StatusInternalServerError)
			return
		}

		access, err := d.Sessions.MintAccess(session.Principal{
			UserID:        u.ID(),
			Email:         u.Email(),
			ActiveFleetID: fleetID,
			Role:          role,
		})
		if err != nil {
			log.WithError(err).Error("mint access on callback")
			http.Error(w, "authentication failed", http.StatusInternalServerError)
			return
		}
		refresh, err := d.Sessions.IssueRefresh(u.ID())
		if err != nil {
			log.WithError(err).Error("issue refresh on callback")
			http.Error(w, "authentication failed", http.StatusInternalServerError)
			return
		}

		// Refresh token stays in an HttpOnly cookie (unreadable by JS). The
		// access token is delivered in the URL fragment so the SPA can read it
		// in JS and send it as `Authorization: Bearer`; an HttpOnly access
		// cookie would be invisible to the SPA and break the API client.
		session.SetRefreshCookie(w, refresh, d.CookieSecure)

		// New users without a fleet go to onboarding; everyone else lands home.
		dest := d.AppBaseURL + d.HomePath
		if fleetID == "" {
			dest = d.AppBaseURL + d.OnboardingPath
		}
		dest += "#access_token=" + url.QueryEscape(access)
		http.Redirect(w, req, dest, http.StatusFound)
	}
}

// --- signed state cookie helpers ---

// setStateCookie stores "state|nonce|exp" with an HMAC signature.
func setStateCookie(w http.ResponseWriter, secret []byte, state, nonce string, secure bool) {
	exp := time.Now().Add(stateTTL).Unix()
	payload := state + "|" + nonce + "|" + itoa(exp)
	value := payload + "|" + sign(secret, payload)
	http.SetCookie(w, &http.Cookie{
		Name:     stateCookieName,
		Value:    base64.RawURLEncoding.EncodeToString([]byte(value)),
		Path:     "/",
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
		Expires:  time.Now().Add(stateTTL),
	})
}

// verifyStateCookie validates the cookie signature, expiry, and state match,
// returning the embedded nonce on success.
func verifyStateCookie(req *http.Request, secret []byte, state string) (nonce string, ok bool) {
	c, err := req.Cookie(stateCookieName)
	if err != nil {
		return "", false
	}
	raw, err := base64.RawURLEncoding.DecodeString(c.Value)
	if err != nil {
		return "", false
	}
	parts := strings.Split(string(raw), "|")
	if len(parts) != 4 {
		return "", false
	}
	gotState, gotNonce, expStr, sig := parts[0], parts[1], parts[2], parts[3]
	payload := gotState + "|" + gotNonce + "|" + expStr
	if !hmac.Equal([]byte(sig), []byte(sign(secret, payload))) {
		return "", false
	}
	if exp, perr := atoi(expStr); perr != nil || time.Now().Unix() > exp {
		return "", false
	}
	if gotState != state {
		return "", false
	}
	return gotNonce, true
}

func clearStateCookie(w http.ResponseWriter, secure bool) {
	http.SetCookie(w, &http.Cookie{
		Name:     stateCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
}

func sign(secret []byte, payload string) string {
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }

func atoi(s string) (int64, error) { return strconv.ParseInt(s, 10, 64) }
