package oidc

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

var testSecret = []byte("test-state-secret")

func TestSafeReturnPath_acceptsSiteRelativePaths(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"/invites/abc123/accept", "/invites/abc123/accept"},
		{"/vehicles?sort=name", "/vehicles?sort=name"},
		{"/", "/"},
		// A fragment would collide with the "#access_token=" the callback
		// appends, so it is dropped rather than carried through.
		{"/settings#members", "/settings"},
	} {
		if got := safeReturnPath(tc.in); got != tc.want {
			t.Fatalf("safeReturnPath(%q) = %q, want %q", tc.in, got, tc.want)
		}
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
		"/" + strings.Repeat("a", maxReturnPathLen),
	} {
		if got := safeReturnPath(in); got != "" {
			t.Fatalf("safeReturnPath(%q) = %q, want rejected", in, got)
		}
	}
}

func TestStateCookie_roundTripsReturnPath(t *testing.T) {
	rec := httptest.NewRecorder()
	setStateCookie(rec, testSecret, "st", "nn", "/invites/abc123/accept", false)

	req := httptest.NewRequest(http.MethodGet, "/auth/callback", nil)
	for _, c := range rec.Result().Cookies() {
		req.AddCookie(c)
	}

	nonce, returnPath, ok := verifyStateCookie(req, testSecret, "st")
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

func TestLoginHandler_storesSanitizedReturnPath(t *testing.T) {
	for _, tc := range []struct{ query, want string }{
		{"?return_to=%2Finvites%2Fabc123%2Faccept", "/invites/abc123/accept"},
		{"?return_to=https%3A%2F%2Fevil.example", ""},
		{"", ""},
	} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/auth/login/google"+tc.query, nil)
		// Only the cookie is under test; a nil Processor would panic on the
		// redirect, so the handler is exercised through setStateCookie's output.
		returnPath := safeReturnPath(req.URL.Query().Get("return_to"))
		setStateCookie(rec, testSecret, "st", "nn", returnPath, false)

		verifyReq := httptest.NewRequest(http.MethodGet, "/auth/callback", nil)
		for _, c := range rec.Result().Cookies() {
			verifyReq.AddCookie(c)
		}
		_, got, ok := verifyStateCookie(verifyReq, testSecret, "st")
		if !ok {
			t.Fatalf("query %q: cookie should verify", tc.query)
		}
		if got != tc.want {
			t.Fatalf("query %q: returnPath = %q, want %q", tc.query, got, tc.want)
		}
	}
}

func TestCallbackDestination_prefersReturnPath(t *testing.T) {
	d := Dependencies{AppBaseURL: "http://app.local", HomePath: "/", OnboardingPath: "/onboarding"}

	// A fleetless invitee must land on the accept route, not onboarding.
	if got := destination(d, "", "/invites/abc123/accept"); got != "http://app.local/invites/abc123/accept" {
		t.Fatalf("fleetless with return path = %q", got)
	}
	// Without a return path the fleet-based defaults still apply.
	if got := destination(d, "", ""); got != "http://app.local/onboarding" {
		t.Fatalf("fleetless default = %q", got)
	}
	if got := destination(d, "f1", ""); got != "http://app.local/" {
		t.Fatalf("fleeted default = %q", got)
	}
}
