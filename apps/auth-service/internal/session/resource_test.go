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

	"github.com/jtumidanski/myfleet/packages/shared-go/server"
)

// newRefreshRouter mounts the real public routes over a store seeded with one
// valid refresh token, and returns the router, the processor, the store and
// that token's raw value. The store is returned so a test can inject a
// RevokeFamily failure and inspect what was revoked.
func newRefreshRouter(t *testing.T, resolve PrincipalResolver) (chi.Router, *Processor, *fakeStore, string) {
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

	// WriteError logs every 5xx through the package-level error logger, which
	// otherwise falls back to logrus' stderr standard logger. Point it at the
	// same discarded output so the logout-failure test does not print a fault
	// over an otherwise quiet `go test` run, and restore the default after.
	server.SetErrorLogger(log)
	t.Cleanup(func() { server.SetErrorLogger(nil) })

	r := chi.NewRouter()
	r.Group(InitializePublicRoutes(log, proc, resolve, false))
	return r, proc, store, raw
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
	r, proc, _, raw := newRefreshRouter(t, resolve)

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
	r, _, _, raw := newRefreshRouter(t, resolve)

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

// postLogout posts to the logout route carrying raw as the refresh cookie. An
// empty raw sends no cookie at all — the "nothing to revoke" case.
func postLogout(r chi.Router, raw string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/auth/logout", nil)
	if raw != "" {
		req.AddCookie(&http.Cookie{Name: RefreshCookieName, Value: raw})
	}
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

// unusedResolver satisfies InitializePublicRoutes for the logout tests. The
// logout route never resolves a principal, so reaching this is a bug in the
// test, not a scenario.
func unusedResolver(context.Context, string) (Principal, error) {
	return Principal{}, errors.New("the logout route must not resolve a principal")
}

// TestLogout_returns500WhenTheFamilyRevokeFails is the load-bearing test of
// this task. Today the handler logs the failure and returns 204 regardless, so
// a failed revocation and a successful one are indistinguishable to any client
// — no client-side status check, however correct, can observe the difference.
func TestLogout_returns500WhenTheFamilyRevokeFails(t *testing.T) {
	r, _, store, raw := newRefreshRouter(t, unusedResolver)
	store.revokeErr = errors.New("db down")

	rec := postLogout(r, raw)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500: %s", rec.Code, rec.Body.String())
	}
	if len(store.revokedFamilies) != 1 {
		t.Fatalf("RevokeFamily calls = %v, want exactly one attempt", store.revokedFamilies)
	}

	var body struct {
		Errors []struct {
			Status string `json:"status"`
			Title  string `json:"title"`
		} `json:"errors"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Errors) != 1 {
		t.Fatalf("errors = %v, want exactly one JSON:API error", body.Errors)
	}
	if body.Errors[0].Status != "500" {
		t.Errorf("errors[0].status = %q, want \"500\"", body.Errors[0].Status)
	}
	// The store's error text must never reach the client (SEC-09): WriteError
	// replaces the title of every 5xx with the fixed one.
	if body.Errors[0].Title != server.InternalErrorTitle {
		t.Errorf("errors[0].title = %q, want %q", body.Errors[0].Title, server.InternalErrorTitle)
	}

	// The ordering trap. clearRefreshCookie must run BEFORE WriteError, because
	// SetCookie appends a header and WriteError flushes them. Swap the two lines
	// and Go drops the Set-Cookie without complaint, leaving the browser holding
	// a cookie for a family that is still live — the worst outcome available.
	if !refreshCookieCleared(rec) {
		t.Fatalf("the refresh cookie must still be cleared on the failure path; cookies = %v",
			rec.Result().Cookies())
	}
}

// TestLogout_succeedsAndClearsCookie pins the unchanged happy path (FR-SRV-5):
// 204, the family actually revoked, the browser's cookie cleared.
func TestLogout_succeedsAndClearsCookie(t *testing.T) {
	r, _, store, raw := newRefreshRouter(t, unusedResolver)

	rec := postLogout(r, raw)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204: %s", rec.Code, rec.Body.String())
	}
	if len(store.revokedFamilies) != 1 {
		t.Fatalf("RevokeFamily calls = %v, want exactly one", store.revokedFamilies)
	}
	revoked, err := store.FindByHash(HashRefresh(raw))
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	if !revoked.IsRevoked() {
		t.Fatal("the presented token's family must be revoked after a successful logout")
	}
	if !refreshCookieCleared(rec) {
		t.Fatalf("refresh cookie must be cleared on success; cookies = %v", rec.Result().Cookies())
	}
}

// TestLogout_returns204WithNoRefreshToken — FR-SRV-4. revokeErr is armed so the
// 204 is evidence the processor was never reached, not merely that it happened
// to succeed.
func TestLogout_returns204WithNoRefreshToken(t *testing.T) {
	r, _, store, _ := newRefreshRouter(t, unusedResolver)
	store.revokeErr = errors.New("RevokeFamily must not be called without a token")

	rec := postLogout(r, "")

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204: %s", rec.Code, rec.Body.String())
	}
	if len(store.revokedFamilies) != 0 {
		t.Fatalf("nothing may be revoked when no token was presented; calls = %v", store.revokedFamilies)
	}
	if !refreshCookieCleared(rec) {
		t.Fatalf("refresh cookie must be cleared even with nothing to revoke; cookies = %v",
			rec.Result().Cookies())
	}
}

// TestLogout_returns204ForAnUnknownToken — the other half of FR-SRV-4. An
// expired or fabricated token is a no-op success, not a fault: Processor.Logout
// maps ErrNotFound to nil. revokeErr is armed to prove the lookup
// short-circuits before any revoke is attempted.
func TestLogout_returns204ForAnUnknownToken(t *testing.T) {
	r, _, store, _ := newRefreshRouter(t, unusedResolver)
	store.revokeErr = errors.New("RevokeFamily must not be called for an unknown token")

	rec := postLogout(r, "never-issued-token")

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204: %s", rec.Code, rec.Body.String())
	}
	if len(store.revokedFamilies) != 0 {
		t.Fatalf("an unknown token must revoke nothing; calls = %v", store.revokedFamilies)
	}
	if !refreshCookieCleared(rec) {
		t.Fatalf("refresh cookie must be cleared for an unknown token; cookies = %v",
			rec.Result().Cookies())
	}
}
