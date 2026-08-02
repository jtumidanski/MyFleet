package session

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/golang-jwt/jwt/v5"
	"github.com/sirupsen/logrus"
)

// newRefreshRouter mounts the real public routes over a store seeded with one
// valid refresh token, and returns the router plus that token's raw value.
func newRefreshRouter(t *testing.T, resolve PrincipalResolver) (chi.Router, *Processor, string) {
	t.Helper()
	store := newFakeStore()
	proc := newTestProcessor(store)

	raw := "valid-refresh-token"
	store.seed(NewBuilder().
		SetUserID("user-1").
		SetTokenHash(HashRefresh(raw)).
		SetExpiresAt(time.Now().Add(refreshTTL)).
		Build())

	log := logrus.New()
	log.SetOutput(io.Discard)

	r := chi.NewRouter()
	r.Group(InitializePublicRoutes(log, proc, resolve, false))
	return r, proc, raw
}

func postRefresh(r chi.Router, raw string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/auth/refresh", nil)
	req.AddCookie(&http.Cookie{Name: RefreshCookieName, Value: raw})
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

func refreshCookieCleared(rec *httptest.ResponseRecorder) bool {
	for _, c := range rec.Result().Cookies() {
		if c.Name == RefreshCookieName && c.Value == "" && c.MaxAge < 0 {
			return true
		}
	}
	return false
}

// TestRefresh_mintsAccessTokenCarryingEmailClaim is the direct, user-visible
// proof of the fix. Before it, this route minted `"email": ""` on every call,
// and since the SPA refreshes on any 401 and access tokens live 15 minutes, all
// but the first few minutes of every session ran on an email-less token.
func TestRefresh_mintsAccessTokenCarryingEmailClaim(t *testing.T) {
	resolve := func(context.Context, string) (Principal, error) {
		return Principal{UserID: "user-1", Email: "a@b.com", ActiveFleetID: "fleet-9", Role: "owner"}, nil
	}
	r, proc, raw := newRefreshRouter(t, resolve)

	rec := postRefresh(r, raw)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}

	var body struct {
		Data struct {
			Attributes struct {
				AccessToken  string `json:"accessToken"`
				RefreshToken string `json:"refreshToken"`
			} `json:"attributes"`
		} `json:"data"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}

	claims := jwt.MapClaims{}
	if _, err := jwt.ParseWithClaims(body.Data.Attributes.AccessToken, claims, func(*jwt.Token) (any, error) {
		return proc.ks.Private().Public(), nil
	}); err != nil {
		t.Fatalf("parse minted access token: %v", err)
	}
	for claim, want := range map[string]string{
		"sub": "user-1", "email": "a@b.com", "active_fleet_id": "fleet-9", "role": "owner",
	} {
		if claims[claim] != want {
			t.Fatalf("claim %s = %v, want %q", claim, claims[claim], want)
		}
	}
	if body.Data.Attributes.RefreshToken == raw {
		t.Fatal("refresh token must rotate")
	}
}

// TestRefresh_failsClosedAndClearsCookieWhenTheResolverErrors covers FR-5. The
// cookie clearing is a deliberate behaviour change: today's resolver-error path
// returns 401 WITHOUT clearing, unlike every other 401 in this handler, so a
// session whose user row is gone keeps re-presenting a credential that will 401
// forever.
func TestRefresh_failsClosedAndClearsCookieWhenTheResolverErrors(t *testing.T) {
	resolve := func(context.Context, string) (Principal, error) {
		return Principal{}, errors.New("user row is gone")
	}
	r, _, raw := newRefreshRouter(t, resolve)

	rec := postRefresh(r, raw)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
	if !refreshCookieCleared(rec) {
		t.Fatalf("refresh cookie must be cleared on resolver error; cookies = %v", rec.Result().Cookies())
	}

	// No token of any kind may be issued: an incomplete identity must never be
	// minted, and no new refresh cookie may replace the cleared one.
	var body struct {
		Data any `json:"data"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Data != nil {
		t.Fatalf("response carried a data member: %v", body.Data)
	}
	for _, c := range rec.Result().Cookies() {
		if c.Name == RefreshCookieName && c.Value != "" {
			t.Fatalf("a refresh cookie was still set: %q", c.Value)
		}
	}
}
