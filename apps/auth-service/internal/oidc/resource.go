package oidc

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"net/http"
	"net/url"
	"path"
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
// principal resolver is injected (Decision 1) so this package never imports the
// concrete membership client, and so this handler never constructs a Principal
// of its own.
type Dependencies struct {
	OIDC        Authenticator
	Users       UserProvisioner
	Sessions    TokenIssuer
	Resolve     session.PrincipalResolver
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
	// Collapse dot segments and duplicate slashes BEFORE the authority re-check:
	// "/.//evil.example" and "/x/..//evil.example" both resolve to "//evil.example"
	// in the browser, which the prefix test above cannot see. Clearing RawPath
	// makes RequestURI re-encode from the cleaned value.
	u.Path = path.Clean(u.Path)
	u.RawPath = ""
	if !strings.HasPrefix(u.Path, "/") || strings.HasPrefix(u.Path, "//") {
		return ""
	}
	// /api/* is same-origin but routed to the services, not the SPA: landing
	// there strands the access-token fragment on a JSON 401.
	if strings.HasPrefix(u.Path, "/api/") {
		return ""
	}
	return u.RequestURI()
}

// destination resolves where the callback sends the browser: an explicit return
// path wins, otherwise fleetless users go to onboarding and everyone else home.
// Takes the Principal rather than a fleet-id string so the two arguments cannot
// be silently transposed at the call site.
func destination(d Dependencies, principal session.Principal, returnPath string) string {
	if returnPath != "" {
		return d.AppBaseURL + returnPath
	}
	if principal.ActiveFleetID == "" {
		return d.AppBaseURL + d.OnboardingPath
	}
	return d.AppBaseURL + d.HomePath
}

func callbackHandler(log logrus.FieldLogger, d Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		// Google reports a declined consent screen — and its own
		// misconfigurations — on `error`, with no `code`. Checked first so a
		// cancel is not misreported as a missing-code fault.
		if oauthErr := req.URL.Query().Get("error"); oauthErr != "" {
			// Info, not Error: declining consent is a normal outcome and must
			// not inflate the error rate. Logged all the same, because a spike
			// in access_denied is a UX signal.
			log.WithField("oauth_error", oauthErr).Info("oidc callback returned provider error")
			if oauthErr == "access_denied" {
				failLogin(w, req, d, errCancelled)
				return
			}
			// invalid_scope / invalid_request / server_error are our
			// misconfigurations, not the user's choice (design §3.3).
			failLogin(w, req, d, errAuthFailed)
			return
		}
		code := req.URL.Query().Get("code")
		state := req.URL.Query().Get("state")
		if code == "" || state == "" {
			failLogin(w, req, d, errInvalidState)
			return
		}
		wantNonce, returnPath, ok := verifyStateCookie(req, d.StateSecret, state)
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

		// The resolver reads the row ProvisionFromGoogle just wrote, by primary
		// key. Safe: there is a single primary Postgres and no read replica in
		// deploy/k8s, and the write has committed by the time ProvisionFromGoogle
		// returns. This is the first place a read replica would break.
		principal, err := d.Resolve(ctx, u.ID())
		if err != nil {
			log.WithError(err).Error("resolve principal on callback")
			failLogin(w, req, d, errServerError)
			return
		}

		access, err := d.Sessions.MintAccess(principal)
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

		// A return path from the login request wins; otherwise new users without
		// a fleet go to onboarding and everyone else lands home. membership.Client
		// maps fleet-service's 404 to an empty fleet id, so that is how a
		// brand-new user is recognised.
		dest := destination(d, principal, returnPath) + "#access_token=" + url.QueryEscape(access)
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
	// 4 fields is the pre-return-path format. Accepted so a login already in
	// flight when this version deploys still completes — a rolling deploy fails
	// in BOTH directions otherwise, for the whole rollout rather than one TTL.
	// Removable one release after this ships.
	if len(parts) == 4 {
		parts = []string{parts[0], parts[1], parts[2], "", parts[3]}
	}
	if len(parts) != 5 {
		return "", "", false
	}
	gotState, gotNonce, expStr, encPath, sig := parts[0], parts[1], parts[2], parts[3], parts[4]
	payload := gotState + "|" + gotNonce + "|" + expStr + "|" + encPath
	if !hmac.Equal([]byte(sig), []byte(sign(secret, payload))) {
		// A legacy cookie was signed without the trailing return-path field, so
		// its signature only matches the three-field payload. An empty encPath
		// is the only case that can be legacy; a current empty-path cookie
		// matched above.
		legacy := gotState + "|" + gotNonce + "|" + expStr
		if encPath != "" || !hmac.Equal([]byte(sig), []byte(sign(secret, legacy))) {
			return "", "", false
		}
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
