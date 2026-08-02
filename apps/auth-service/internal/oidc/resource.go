package oidc

import (
	"context"
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

// Authenticator is the OIDC surface the callback needs. Declared here, at the
// consumer, so the handler can be exercised without a live Google endpoint
// (design §3.2). *Processor satisfies it implicitly.
type Authenticator interface {
	AuthCodeURL(state, nonce string) string
	Exchange(ctx context.Context, code string) (string, error)
	Verify(ctx context.Context, rawIDToken string) (user.GoogleProfile, string, error)
}

// UserProvisioner is the single user-store operation the callback performs.
type UserProvisioner interface {
	ProvisionFromGoogle(gp user.GoogleProfile) (user.Model, error)
}

// TokenIssuer mints the pair the browser leaves with.
type TokenIssuer interface {
	MintAccess(pr session.Principal) (string, error)
	IssueRefresh(userID string) (string, error)
}

// Dependencies bundles everything the callback orchestration needs. The
// membership resolver is injected (Decision 1) so this package never imports
// the concrete membership client.
type Dependencies struct {
	OIDC        Authenticator
	Users       UserProvisioner
	Sessions    TokenIssuer
	Resolve     session.MembershipResolver
	StateSecret []byte
	// AppBaseURL is the SPA origin the browser is redirected back to.
	AppBaseURL string
	// HomePath / OnboardingPath / LoginPath are relative paths under AppBaseURL.
	HomePath       string
	OnboardingPath string
	LoginPath      string
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

// loginErrorCode is the coarse, browser-visible outcome of a failed callback.
// Deliberately not derived from the underlying error: nothing about the
// failure's internals reaches the SPA (FR-ERR-4). A typed constant set means
// the compiler, not review, enforces the closed vocabulary.
type loginErrorCode string

const (
	errCancelled    loginErrorCode = "cancelled"
	errInvalidState loginErrorCode = "invalid_state"
	errAuthFailed   loginErrorCode = "auth_failed"
	errServerError  loginErrorCode = "server_error"
)

// failLogin returns the browser to the SPA's login page carrying a coarse
// reason, instead of dead-ending on a plaintext error body.
//
// The state cookie is cleared on every path (FR-ERR-8): each of these exits
// abandons the attempt, and the page the user lands on offers "Try again", so a
// stale signed state must not survive to collide with the next attempt's. The
// clear must precede http.Redirect, which calls WriteHeader — headers set after
// that are dropped silently.
//
// The location is composed entirely from server configuration plus a constant,
// so there is no open-redirect surface (FR-ERR-9). It is deliberately NOT
// query-escaped: escaping would turn the "#" into "%23" and put the code in the
// path rather than the fragment.
func failLogin(w http.ResponseWriter, req *http.Request, d Dependencies, code loginErrorCode) {
	clearStateCookie(w, d.CookieSecure)
	http.Redirect(w, req, d.AppBaseURL+d.LoginPath+"#error="+string(code), http.StatusFound)
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
			failLogin(w, req, d, errInvalidState)
			return
		}
		wantNonce, ok := verifyStateCookie(req, d.StateSecret, state)
		if !ok {
			failLogin(w, req, d, errInvalidState)
			return
		}
		clearStateCookie(w, d.CookieSecure)

		ctx := req.Context()
		rawIDToken, err := d.OIDC.Exchange(ctx, code)
		if err != nil {
			log.WithError(err).Error("oidc code exchange")
			failLogin(w, req, d, errAuthFailed)
			return
		}
		profile, gotNonce, err := d.OIDC.Verify(ctx, rawIDToken)
		if err != nil {
			log.WithError(err).Error("oidc id_token verification")
			failLogin(w, req, d, errAuthFailed)
			return
		}
		// idtoken.Validate does not check the nonce; bind the id_token to this
		// login attempt by comparing its nonce to the one in the state cookie.
		if gotNonce == "" || !hmac.Equal([]byte(gotNonce), []byte(wantNonce)) {
			log.Error("oidc nonce mismatch")
			failLogin(w, req, d, errAuthFailed)
			return
		}

		u, err := d.Users.ProvisionFromGoogle(profile)
		if err != nil {
			log.WithError(err).Error("provision user from google")
			failLogin(w, req, d, errServerError)
			return
		}

		fleetID, role, err := d.Resolve(ctx, u.ID())
		if err != nil {
			log.WithError(err).Error("resolve membership on callback")
			failLogin(w, req, d, errServerError)
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
			failLogin(w, req, d, errServerError)
			return
		}
		refresh, err := d.Sessions.IssueRefresh(u.ID())
		if err != nil {
			log.WithError(err).Error("issue refresh on callback")
			failLogin(w, req, d, errServerError)
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
