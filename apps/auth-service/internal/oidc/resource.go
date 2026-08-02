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

// maxReturnPathLen bounds the post-login return path carried through the OAuth
// dance, so a hostile link cannot stuff an unbounded value into the cookie.
const maxReturnPathLen = 512

// Dependencies bundles everything the callback orchestration needs. The
// principal resolver is injected (Decision 1) so this package never imports the
// concrete membership client, and so this handler never constructs a Principal
// of its own.
type Dependencies struct {
	OIDC        *Processor
	Users       *user.Processor
	Sessions    *session.Processor
	Resolve     session.PrincipalResolver
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
		// Where to land after the dance, e.g. the invite-accept route the user
		// clicked before being bounced to login. It rides in the SIGNED state
		// cookie rather than the OAuth state parameter so it cannot be swapped
		// mid-flight, and it is sanitized here so only site-relative paths
		// survive (open-redirect guard).
		returnPath := safeReturnPath(req.URL.Query().Get("return_to"))
		// Persist state+nonce in a signed, short-lived cookie for CSRF/replay
		// defense; verified on callback.
		setStateCookie(w, d.StateSecret, state, nonce, returnPath, d.CookieSecure)
		http.Redirect(w, req, d.OIDC.AuthCodeURL(state, nonce), http.StatusFound)
	}
}

// safeReturnPath reduces a caller-supplied return target to a site-relative
// path under AppBaseURL, or "" if it is anything else. Anything with a scheme
// or authority — including the protocol-relative "//host" and "/\host" forms
// browsers resolve off-site — is rejected, so this can never become an open
// redirect. The fragment is dropped because the callback appends its own.
func safeReturnPath(raw string) string {
	if raw == "" || len(raw) > maxReturnPathLen {
		return ""
	}
	if !strings.HasPrefix(raw, "/") || strings.HasPrefix(raw, "//") || strings.HasPrefix(raw, "/\\") {
		return ""
	}
	for _, r := range raw {
		if r < 0x20 || r == 0x7f {
			return ""
		}
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "" || u.Host != "" || u.Opaque != "" {
		return ""
	}
	return u.RequestURI()
}

// destination resolves where the callback sends the browser: an explicit return
// path wins, otherwise fleetless users go to onboarding and everyone else home.
func destination(d Dependencies, fleetID, returnPath string) string {
	if returnPath != "" {
		return d.AppBaseURL + returnPath
	}
	if fleetID == "" {
		return d.AppBaseURL + d.OnboardingPath
	}
	return d.AppBaseURL + d.HomePath
}

func callbackHandler(log logrus.FieldLogger, d Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		code := req.URL.Query().Get("code")
		state := req.URL.Query().Get("state")
		if code == "" || state == "" {
			http.Error(w, "missing code or state", http.StatusBadRequest)
			return
		}
		wantNonce, returnPath, ok := verifyStateCookie(req, d.StateSecret, state)
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

		// The resolver reads the row ProvisionFromGoogle just wrote, by primary
		// key. Safe: there is a single primary Postgres and no read replica in
		// deploy/k8s, and the write has committed by the time ProvisionFromGoogle
		// returns. This is the first place a read replica would break.
		principal, err := d.Resolve(ctx, u.ID())
		if err != nil {
			log.WithError(err).Error("resolve principal on callback")
			http.Error(w, "authentication failed", http.StatusInternalServerError)
			return
		}

		access, err := d.Sessions.MintAccess(principal)
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

		// A return path from the login request wins; otherwise new users without
		// a fleet go to onboarding and everyone else lands home. membership.Client
		// maps fleet-service's 404 to an empty fleet id, so that is how a
		// brand-new user is recognised.
		dest := destination(d, principal.ActiveFleetID, returnPath) + "#access_token=" + url.QueryEscape(access)
		http.Redirect(w, req, dest, http.StatusFound)
	}
}

// --- signed state cookie helpers ---

// setStateCookie stores "state|nonce|exp|b64(returnPath)" with an HMAC
// signature. The return path is base64'd so a path containing the "|" field
// separator cannot forge extra fields, and signed so it cannot be swapped for
// an off-site destination after the fact.
func setStateCookie(w http.ResponseWriter, secret []byte, state, nonce, returnPath string, secure bool) {
	exp := time.Now().Add(stateTTL).Unix()
	payload := state + "|" + nonce + "|" + itoa(exp) + "|" + base64.RawURLEncoding.EncodeToString([]byte(returnPath))
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
// returning the embedded nonce and post-login return path on success. The
// return path is re-sanitized after the signature check: the value was signed
// by this service, but a sanitizer that tightens later must not be bypassed by
// a cookie minted by an older build.
func verifyStateCookie(req *http.Request, secret []byte, state string) (nonce, returnPath string, ok bool) {
	c, err := req.Cookie(stateCookieName)
	if err != nil {
		return "", "", false
	}
	raw, err := base64.RawURLEncoding.DecodeString(c.Value)
	if err != nil {
		return "", "", false
	}
	parts := strings.Split(string(raw), "|")
	if len(parts) != 5 {
		return "", "", false
	}
	gotState, gotNonce, expStr, encPath, sig := parts[0], parts[1], parts[2], parts[3], parts[4]
	payload := gotState + "|" + gotNonce + "|" + expStr + "|" + encPath
	if !hmac.Equal([]byte(sig), []byte(sign(secret, payload))) {
		return "", "", false
	}
	if exp, perr := atoi(expStr); perr != nil || time.Now().Unix() > exp {
		return "", "", false
	}
	if gotState != state {
		return "", "", false
	}
	pathBytes, err := base64.RawURLEncoding.DecodeString(encPath)
	if err != nil {
		return "", "", false
	}
	return gotNonce, safeReturnPath(string(pathBytes)), true
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
