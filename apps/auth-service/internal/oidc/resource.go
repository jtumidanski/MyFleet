package oidc

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"net/http"
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
		setStateCookie(w, d.StateSecret, state, nonce)
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
		if _, ok := verifyStateCookie(req, d.StateSecret, state); !ok {
			http.Error(w, "invalid state", http.StatusBadRequest)
			return
		}
		clearStateCookie(w)

		ctx := req.Context()
		rawIDToken, err := d.OIDC.Exchange(ctx, code)
		if err != nil {
			log.WithError(err).Error("oidc code exchange")
			http.Error(w, "authentication failed", http.StatusBadGateway)
			return
		}
		profile, err := d.OIDC.Verify(ctx, rawIDToken)
		if err != nil {
			log.WithError(err).Error("oidc id_token verification")
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

		session.SetRefreshCookie(w, refresh)
		setAccessCookie(w, access)

		// New users without a fleet go to onboarding; everyone else lands home.
		dest := d.AppBaseURL + d.HomePath
		if fleetID == "" {
			dest = d.AppBaseURL + d.OnboardingPath
		}
		http.Redirect(w, req, dest, http.StatusFound)
	}
}

// --- signed state cookie helpers ---

// setStateCookie stores "state|nonce|exp" with an HMAC signature.
func setStateCookie(w http.ResponseWriter, secret []byte, state, nonce string) {
	exp := time.Now().Add(stateTTL).Unix()
	payload := state + "|" + nonce + "|" + itoa(exp)
	value := payload + "|" + sign(secret, payload)
	http.SetCookie(w, &http.Cookie{
		Name:     stateCookieName,
		Value:    base64.RawURLEncoding.EncodeToString([]byte(value)),
		Path:     "/",
		HttpOnly: true,
		Secure:   true,
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

func clearStateCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     stateCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
}

func setAccessCookie(w http.ResponseWriter, access string) {
	http.SetCookie(w, &http.Cookie{
		Name:     "access_token",
		Value:    access,
		Path:     "/",
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	})
}

func sign(secret []byte, payload string) string {
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }

func atoi(s string) (int64, error) { return strconv.ParseInt(s, 10, 64) }
