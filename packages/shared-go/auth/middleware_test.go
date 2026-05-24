package auth

import (
	"crypto/rand"
	"crypto/rsa"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
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
	mw := jwtWithKeyfunc(func(*jwt.Token) (any, error) { return nil, nil })
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
	mw := jwtWithKeyfunc(func(*jwt.Token) (any, error) { return &key.PublicKey, nil })
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
