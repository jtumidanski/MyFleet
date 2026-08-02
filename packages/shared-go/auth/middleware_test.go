package auth

import (
	"crypto/rand"
	"crypto/rsa"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/sirupsen/logrus"
	"github.com/sirupsen/logrus/hooks/test"
)

func signTestToken(t *testing.T, key *rsa.PrivateKey, claims jwt.MapClaims) string {
	t.Helper()
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	s, err := tok.SignedString(key)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestJWT_rejectsMissingToken(t *testing.T) {
	mw := JWT(func(*jwt.Token) (any, error) { return nil, nil })
	rec := httptest.NewRecorder()
	mw(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})).
		ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("want 401 got %d", rec.Code)
	}
}

func TestJWT_parsesIdentityFromValidToken(t *testing.T) {
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	tokenStr := signTestToken(t, key, jwt.MapClaims{
		"sub":             "user-1",
		"email":           "a@b.com",
		"active_fleet_id": "fleet-9",
		"role":            "owner",
		"exp":             time.Now().Add(time.Hour).Unix(),
	})
	var seen Identity
	mw := JWT(func(*jwt.Token) (any, error) { return &key.PublicKey, nil })
	h := mw(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		seen = IdentityFromContext(r.Context())
	}))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+tokenStr)
	h.ServeHTTP(httptest.NewRecorder(), req)
	if seen.UserID != "user-1" || seen.ActiveFleetID != "fleet-9" || seen.Role != "owner" {
		t.Fatalf("identity not parsed: %+v", seen)
	}
}

// TestJWT_warnsOnEmptyEmailClaimAndStillCallsNext is the guard that keeps this
// observability change from quietly becoming enforcement. The warn exists so a
// regression of the empty-claim class surfaces in the logs before a user
// reports a rejected invite; it must never cost the request.
func TestJWT_warnsOnEmptyEmailClaimAndStillCallsNext(t *testing.T) {
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	tokenStr := signTestToken(t, key, jwt.MapClaims{
		"sub":             "user-1",
		"email":           "",
		"active_fleet_id": "fleet-9",
		"role":            "owner",
		"exp":             time.Now().Add(time.Hour).Unix(),
	})

	log, hook := test.NewNullLogger()
	called := false
	mw := JWT(func(*jwt.Token) (any, error) { return &key.PublicKey, nil }, WithLogger(log))
	h := mw(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true }))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+tokenStr)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if !called {
		t.Fatal("an empty email claim must not block the request — this is observability, not enforcement")
	}
	if rec.Code == http.StatusUnauthorized {
		t.Fatalf("status = %d, want the request to proceed", rec.Code)
	}

	entries := hook.AllEntries()
	if len(entries) != 1 {
		t.Fatalf("log entries = %d, want exactly 1", len(entries))
	}
	e := entries[0]
	if e.Level != logrus.WarnLevel {
		t.Fatalf("level = %v, want warn", e.Level)
	}
	if e.Data["sub"] != "user-1" {
		t.Fatalf("sub field = %v, want user-1", e.Data["sub"])
	}
	if _, ok := e.Data["correlation_id"]; !ok {
		t.Fatal("warn must carry a correlation_id field so the line joins to the request")
	}
	// Never the raw token (Global Constraints, PRD §8).
	if strings.Contains(e.Message, tokenStr) {
		t.Fatal("log message must not contain the raw token")
	}
	for k, v := range e.Data {
		if s, ok := v.(string); ok && strings.Contains(s, tokenStr) {
			t.Fatalf("log field %q must not contain the raw token", k)
		}
	}
}

func TestJWT_doesNotWarnWhenEmailIsPresent(t *testing.T) {
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	tokenStr := signTestToken(t, key, jwt.MapClaims{
		"sub":   "user-1",
		"email": "a@b.com",
		"exp":   time.Now().Add(time.Hour).Unix(),
	})

	log, hook := test.NewNullLogger()
	mw := JWT(func(*jwt.Token) (any, error) { return &key.PublicKey, nil }, WithLogger(log))
	h := mw(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+tokenStr)
	h.ServeHTTP(httptest.NewRecorder(), req)

	if n := len(hook.AllEntries()); n != 0 {
		t.Fatalf("log entries = %d, want 0 for a healthy token", n)
	}
}
