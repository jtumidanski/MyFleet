package oidc

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/jtumidanski/myfleet/apps/auth-service/internal/session"
)

var testSecret = []byte("test-state-secret")

func testLogger() logrus.FieldLogger {
	l := logrus.New()
	l.SetOutput(nil)
	return l
}

// cookieRequest returns a callback request carrying the cookies set on rec.
func cookieRequest(rec *httptest.ResponseRecorder) *http.Request {
	req := httptest.NewRequest(http.MethodGet, "/auth/callback", nil)
	for _, c := range rec.Result().Cookies() {
		req.AddCookie(c)
	}
	return req
}

// storedReturnPath decodes the return path a handler actually WROTE into the
// state cookie. Reading it back through verifyStateCookie would not do: that
// re-sanitizes on read, so it reports "" for a hostile value whether or not the
// handler sanitized on write. Only the raw payload pins the write side.
func storedReturnPath(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	cookies := rec.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("expected exactly one cookie, got %d", len(cookies))
	}
	raw, err := base64.RawURLEncoding.DecodeString(cookies[0].Value)
	if err != nil {
		t.Fatalf("decode cookie: %v", err)
	}
	parts := strings.Split(string(raw), "|")
	if len(parts) != 5 {
		t.Fatalf("expected 5 cookie fields, got %d", len(parts))
	}
	decoded, err := base64.RawURLEncoding.DecodeString(parts[3])
	if err != nil {
		t.Fatalf("decode return path field %q: %v", parts[3], err)
	}
	return string(decoded)
}

func TestSafeReturnPath_acceptsSiteRelativePaths(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"/invites/abc123/accept", "/invites/abc123/accept"},
		{"/vehicles?sort=name", "/vehicles?sort=name"},
		{"/", "/"},
		// A fragment would collide with the "#access_token=" the callback
		// appends, so it is dropped rather than carried through.
		{"/settings#members", "/settings"},
		// 512 bytes is the documented ceiling and must still be accepted.
		{"/" + strings.Repeat("a", maxReturnPathLen-1), "/" + strings.Repeat("a", maxReturnPathLen-1)},
		// Dot segments and duplicate slashes are collapsed rather than rejected.
		// What matters is that the result can no longer resolve to an authority:
		// both of these read as "//evil.example" to a browser if left alone.
		{"/.//evil.example", "/evil.example"},
		{"/x/../..//evil.example", "/evil.example"},
		{"/invites/abc123/accept/", "/invites/abc123/accept"},
	} {
		t.Run(tc.in, func(t *testing.T) {
			if got := safeReturnPath(tc.in); got != tc.want {
				t.Errorf("safeReturnPath(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestSafeReturnPath_rejectsOffSiteAndMalformed(t *testing.T) {
	for _, in := range []string{
		"",
		"https://evil.example/steal",
		"//evil.example/steal",       // protocol-relative
		"/\\evil.example/steal",      // browsers treat /\ as protocol-relative too
		"javascript:alert(1)",        // scheme
		"vehicles",                   // not site-relative
		"/vehicles\r\nSet-Cookie: x", // header injection attempt
		// Same-origin but routed to the services, not the SPA: the access-token
		// fragment would be stranded on a JSON 401.
		"/api/fleet/vehicles",
		"/" + strings.Repeat("a", maxReturnPathLen),
	} {
		t.Run(in, func(t *testing.T) {
			if got := safeReturnPath(in); got != "" {
				t.Errorf("safeReturnPath(%q) = %q, want rejected", in, got)
			}
		})
	}
}

func TestStateCookie_roundTripsReturnPath(t *testing.T) {
	rec := httptest.NewRecorder()
	setStateCookie(rec, testSecret, "st", "nn", "/invites/abc123/accept", false)

	nonce, returnPath, ok := verifyStateCookie(cookieRequest(rec), testSecret, "st")
	if !ok {
		t.Fatal("state cookie should verify")
	}
	if nonce != "nn" {
		t.Fatalf("nonce = %q, want %q", nonce, "nn")
	}
	if returnPath != "/invites/abc123/accept" {
		t.Fatalf("returnPath = %q, want /invites/abc123/accept", returnPath)
	}
}

func TestStateCookie_roundTripsEmptyReturnPath(t *testing.T) {
	rec := httptest.NewRecorder()
	setStateCookie(rec, testSecret, "st", "nn", "", false)

	nonce, returnPath, ok := verifyStateCookie(cookieRequest(rec), testSecret, "st")
	if !ok || nonce != "nn" || returnPath != "" {
		t.Fatalf("empty-path cookie: nonce=%q path=%q ok=%v", nonce, returnPath, ok)
	}
}

func TestStateCookie_rejectsTamperedReturnPath(t *testing.T) {
	rec := httptest.NewRecorder()
	setStateCookie(rec, testSecret, "st", "nn", "/vehicles", false)

	raw, err := base64.RawURLEncoding.DecodeString(rec.Result().Cookies()[0].Value)
	if err != nil {
		t.Fatalf("decode cookie: %v", err)
	}
	parts := strings.Split(string(raw), "|")
	parts[3] = base64.RawURLEncoding.EncodeToString([]byte("https://evil.example"))
	tampered := base64.RawURLEncoding.EncodeToString([]byte(strings.Join(parts, "|")))

	req := httptest.NewRequest(http.MethodGet, "/auth/callback", nil)
	req.AddCookie(&http.Cookie{Name: stateCookieName, Value: tampered})

	if _, _, ok := verifyStateCookie(req, testSecret, "st"); ok {
		t.Fatal("a tampered return path must fail the signature check")
	}
}

// A login started on the previous build must still complete after this one
// deploys, so the pre-return-path cookie format stays acceptable for a release.
func TestStateCookie_acceptsLegacyFourFieldFormat(t *testing.T) {
	exp := itoa(time.Now().Add(stateTTL).Unix())
	payload := "st|nn|" + exp
	value := base64.RawURLEncoding.EncodeToString([]byte(payload + "|" + sign(testSecret, payload)))

	req := httptest.NewRequest(http.MethodGet, "/auth/callback", nil)
	req.AddCookie(&http.Cookie{Name: stateCookieName, Value: value})

	nonce, returnPath, ok := verifyStateCookie(req, testSecret, "st")
	if !ok {
		t.Fatal("legacy 4-field cookie should still verify during rollout")
	}
	if nonce != "nn" || returnPath != "" {
		t.Fatalf("legacy cookie: nonce=%q returnPath=%q", nonce, returnPath)
	}
}

func TestStateCookie_rejectsForgedLegacyFormat(t *testing.T) {
	exp := itoa(time.Now().Add(stateTTL).Unix())
	payload := "st|nn|" + exp
	value := base64.RawURLEncoding.EncodeToString([]byte(payload + "|" + sign([]byte("wrong-secret"), payload)))

	req := httptest.NewRequest(http.MethodGet, "/auth/callback", nil)
	req.AddCookie(&http.Cookie{Name: stateCookieName, Value: value})

	if _, _, ok := verifyStateCookie(req, testSecret, "st"); ok {
		t.Fatal("legacy tolerance must not skip the signature check")
	}
}

// Drives the real handler: it must sanitize return_to and persist the result.
// NewProcessor is a pure struct constructor and AuthCodeURL is string building,
// so no network is involved.
func TestLoginHandler_storesSanitizedReturnPath(t *testing.T) {
	d := Dependencies{
		OIDC:        NewProcessor("client-id", "client-secret", "http://auth.local/auth/callback"),
		StateSecret: testSecret,
	}

	for _, tc := range []struct{ name, query, want string }{
		{"site-relative path is kept", "?return_to=%2Finvites%2Fabc123%2Faccept", "/invites/abc123/accept"},
		{"off-site url is dropped", "?return_to=https%3A%2F%2Fevil.example", ""},
		{"protocol-relative url is dropped", "?return_to=%2F%2Fevil.example", ""},
		{"absent param yields no return path", "", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/auth/login/google"+tc.query, nil)
			loginHandler(testLogger(), d)(rec, req)

			if rec.Code != http.StatusFound {
				t.Fatalf("status = %d, want %d", rec.Code, http.StatusFound)
			}

			// The state the handler signed into the cookie is the one it sent to
			// Google, so read it back off the redirect to verify the cookie.
			loc, err := rec.Result().Location()
			if err != nil {
				t.Fatalf("redirect location: %v", err)
			}
			state := loc.Query().Get("state")
			if state == "" {
				t.Fatal("redirect carries no state")
			}
			if _, _, ok := verifyStateCookie(cookieRequest(rec), testSecret, state); !ok {
				t.Fatal("handler's state cookie should verify")
			}

			if got := storedReturnPath(t, rec); got != tc.want {
				t.Errorf("stored returnPath = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestCallbackDestination_prefersReturnPath(t *testing.T) {
	// Distinct values so transposed arguments cannot coincidentally pass.
	d := Dependencies{AppBaseURL: "http://app.local", HomePath: "/home", OnboardingPath: "/onboarding"}

	for _, tc := range []struct{ name, fleetID, returnPath, want string }{
		// A fleetless invitee must land on the accept route, not onboarding.
		{"return path wins for fleetless user", "", "/invites/abc123/accept", "http://app.local/invites/abc123/accept"},
		{"return path wins for fleeted user", "f1", "/vehicles", "http://app.local/vehicles"},
		{"fleetless default is onboarding", "", "", "http://app.local/onboarding"},
		{"fleeted default is home", "f1", "", "http://app.local/home"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			principal := session.Principal{UserID: "u1", ActiveFleetID: tc.fleetID}
			if got := destination(d, principal, tc.returnPath); got != tc.want {
				t.Errorf("destination(%q, %q) = %q, want %q", tc.fleetID, tc.returnPath, got, tc.want)
			}
		})
	}
}
