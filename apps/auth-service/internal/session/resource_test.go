package session

import (
	"context"
	"crypto/rsa"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/golang-jwt/jwt/v5"
	"github.com/sirupsen/logrus"
	logrustest "github.com/sirupsen/logrus/hooks/test"

	"github.com/jtumidanski/myfleet/apps/auth-service/internal/jwks"
	"github.com/jtumidanski/myfleet/packages/shared-go/server"
)

// newRefreshRouter mounts the real public routes over a store seeded with one
// valid refresh token, and returns the router, the processor, the store, a hook
// holding everything the handler logged, and that token's raw value. The store
// is returned so a test can inject a RevokeFamily failure and inspect what was
// revoked; the hook so a test can assert the level AND message a path logged at.
func newRefreshRouter(t *testing.T, resolve PrincipalResolver) (chi.Router, *Processor, *fakeStore, *logrustest.Hook, string) {
	t.Helper()
	store := newFakeStore()
	return mountRefresh(t, store, newTestProcessor(store), resolve)
}

// mountRefresh is newRefreshRouter with the store and processor supplied, so a
// test can mount the real routes over a processor whose MintAccess cannot
// succeed. It returns the same five values newRefreshRouter does.
func mountRefresh(t *testing.T, store *fakeStore, proc *Processor, resolve PrincipalResolver) (chi.Router, *Processor, *fakeStore, *logrustest.Hook, string) {
	t.Helper()

	raw := "valid-refresh-token"
	store.seed(NewBuilder().
		SetUserID("user-1").
		SetTokenHash(HashRefresh(raw)).
		SetExpiresAt(time.Now().Add(refreshTTL)).
		Build())

	log, hook := logrustest.NewNullLogger()

	// WriteError logs every 5xx through the package-level error logger, which
	// otherwise falls back to logrus' stderr standard logger. Point it at the
	// same null logger so the logout-failure test does not print a fault over an
	// otherwise quiet `go test` run, and restore the default after. Its entries
	// land in the hook too, under WriteError's own message — never one the
	// handler writes — so the level/message assertions stay unambiguous.
	server.SetErrorLogger(log)
	t.Cleanup(func() { server.SetErrorLogger(nil) })

	r := chi.NewRouter()
	r.Group(InitializePublicRoutes(log, proc, resolve, false))
	return r, proc, store, hook, raw
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

// refreshCookieSet returns the non-clearing refresh cookie the response sets,
// or nil. Distinct from refreshCookieCleared: "no cookie at all" and "a cookie
// carrying the rotated token" are different outcomes with different
// consequences on the user's next attempt.
func refreshCookieSet(rec *httptest.ResponseRecorder) *http.Cookie {
	for _, c := range rec.Result().Cookies() {
		if c.Name == RefreshCookieName && c.Value != "" {
			return c
		}
	}
	return nil
}

// TestRefresh_mintsAccessTokenCarryingEmailClaim is the direct, user-visible
// proof of the fix. Before it, this route minted `"email": ""` on every call,
// and since the SPA refreshes on any 401 and access tokens live 15 minutes, all
// but the first few minutes of every session ran on an email-less token.
func TestRefresh_mintsAccessTokenCarryingEmailClaim(t *testing.T) {
	resolve := func(context.Context, string) (Principal, error) {
		return Principal{UserID: "user-1", Email: "a@b.com", ActiveFleetID: "fleet-9", Role: "owner"}, nil
	}
	r, proc, _, _, raw := newRefreshRouter(t, resolve)

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
	r, _, _, hook, raw := newRefreshRouter(t, resolve)

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

	if got := rec.Header().Get("Retry-After"); got != "" {
		t.Fatalf("Retry-After = %q on the permanent path; a dead credential is not something to retry", got)
	}
	if !loggedAtLevel(hook, logrus.ErrorLevel, "resolve principal on refresh") {
		t.Fatalf("the permanent failure must still log at error: %+v", hook.AllEntries())
	}
}

// loggedAtLevel reports whether the handler wrote exactly this message at
// exactly this level. Level AND message, because FR-REFRESH-6 is about an
// outage not reading as a wave of authentication failures — same text at a
// different level, or a different text at the same level, both fail that.
func loggedAtLevel(hook *logrustest.Hook, level logrus.Level, msg string) bool {
	for _, e := range hook.AllEntries() {
		if e.Level == level && e.Message == msg {
			return true
		}
	}
	return false
}

// transientResolver stands in for a fleet-service outage, in the exact shape
// membership.Client produces.
func transientResolver() PrincipalResolver {
	return func(context.Context, string) (Principal, error) {
		return Principal{}, fmt.Errorf("%w: active membership lookup failed with status 500",
			server.ErrServiceUnavailable)
	}
}

// TestRefresh_transientResolverFailureKeepsTheSessionAlive is the whole task in
// one test. Asserting the status ALONE would pass while still logging the user
// out — which is precisely the failure mode the acceptance criteria call out —
// so this asserts the three things that actually decide the outcome: the
// status, the Retry-After, and the cookie the browser is left holding.
func TestRefresh_transientResolverFailureKeepsTheSessionAlive(t *testing.T) {
	r, proc, _, hook, raw := newRefreshRouter(t, transientResolver())

	rec := postRefresh(r, raw)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503: %s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Retry-After"); got != "5" {
		t.Fatalf("Retry-After = %q, want \"5\"", got)
	}
	if refreshCookieCleared(rec) {
		t.Fatal("the refresh cookie was cleared — someone else's outage must not end this session")
	}

	// FR-REFRESH-5, the highest-risk item in the task. Rotate has ALREADY
	// consumed the presented token and committed a new one to the store. If the
	// browser is left holding the old value, its next attempt is a replay, and
	// Processor.Rotate answers a replay by revoking the whole family — the exact
	// logout this branch exists to prevent. So the rotated value must be written.
	c := refreshCookieSet(rec)
	if c == nil {
		t.Fatal("no refresh cookie was set: the browser would keep the token Rotate already consumed " +
			"and trip reuse detection on its next attempt")
	}
	if c.Value == raw {
		t.Fatalf("the cookie still carries the CONSUMED token %q", raw)
	}

	// Acceptance criterion 4, asserted against stored state rather than against
	// the response: the value the browser now holds must still rotate cleanly
	// once fleet-service recovers.
	if _, userID, err := proc.Rotate(c.Value); err != nil {
		t.Fatalf("re-presenting the cookie written alongside the 503 failed: %v — "+
			"the retry after recovery would log the user out", err)
	} else if userID != "user-1" {
		t.Fatalf("userID = %q, want user-1", userID)
	}

	// FR-REFRESH-6: an outage is greppable as one and does not inflate the
	// error rate.
	if !loggedAtLevel(hook, logrus.WarnLevel, "resolve principal on refresh: upstream unavailable") {
		t.Fatalf("the transient failure must log at warn with its own message: %+v", hook.AllEntries())
	}
	if loggedAtLevel(hook, logrus.ErrorLevel, "resolve principal on refresh") {
		t.Fatal("the transient failure also logged the permanent path's error line")
	}
}

// TestRefresh_transientFailureMintsNothingAndDisclosesNothing: failing closed is
// unchanged — a token with incomplete identity is never minted — and the body
// of a 503 on a PUBLIC endpoint tells an unauthenticated caller only that
// something is unavailable.
func TestRefresh_transientFailureMintsNothingAndDisclosesNothing(t *testing.T) {
	r, _, _, _, raw := newRefreshRouter(t, transientResolver())

	rec := postRefresh(r, raw)

	// Snapshot the raw body BEFORE decoding: json.Decoder.Decode drains the
	// *bytes.Buffer, so reading rec.Body.String() afterward would return "" and
	// the leak loop below would pass vacuously no matter what the body said.
	rawBody := rec.Body.String()

	var body struct {
		Data   any `json:"data"`
		Errors []struct {
			Status string `json:"status"`
			Code   string `json:"code"`
			Title  string `json:"title"`
			Detail string `json:"detail"`
		} `json:"errors"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Data != nil {
		t.Fatalf("a 503 carried a data member: %v — no token may be minted on this path", body.Data)
	}
	if len(body.Errors) != 1 {
		t.Fatalf("errors = %d, want 1", len(body.Errors))
	}
	if body.Errors[0].Status != "503" || body.Errors[0].Code != "service_unavailable" {
		t.Fatalf("status/code = %q/%q, want 503/service_unavailable — the SPA keys on the code",
			body.Errors[0].Status, body.Errors[0].Code)
	}
	if body.Errors[0].Title != server.InternalErrorTitle {
		t.Fatalf("title = %q, want %q — a 5xx title must not describe the fault",
			body.Errors[0].Title, server.InternalErrorTitle)
	}
	if body.Errors[0].Detail != "" {
		t.Fatalf("detail = %q, want empty", body.Errors[0].Detail)
	}
	for _, secret := range []string{"membership", "lookup", "user-1"} {
		if strings.Contains(rawBody, secret) {
			t.Fatalf("the 503 body leaked %q: %s", secret, rawBody)
		}
	}
}

// brokenSignerProcessor is newTestProcessor with a key set that cannot sign, so
// MintAccess — and ONLY MintAccess — fails. Rotate runs first and runs normally,
// which is precisely the state the mint-failure exit has to cope with: the
// presented token is already spent.
func brokenSignerProcessor(store *fakeStore) *Processor {
	ks := jwks.NewKeySet(&rsa.PrivateKey{}, "kid-1")
	return NewProcessor(logrus.New(), ks, "myfleet-auth", "myfleet").WithStore(store, store)
}

// TestRefresh_mintFailureStillWritesTheRotatedCookie pins the last exit in this
// handler that Rotate's consumption reaches. The transient branch was fixed to
// write the rotated cookie; this one wrote neither it nor a clear, so the
// browser kept the token Rotate had already consumed. Its next refresh is then
// a replay, and Processor.Rotate answers a replay by revoking the whole family —
// signing the user out of every device over a local signing fault.
func TestRefresh_mintFailureStillWritesTheRotatedCookie(t *testing.T) {
	store := newFakeStore()
	resolve := func(context.Context, string) (Principal, error) {
		return Principal{UserID: "user-1", Email: "a@b.com", ActiveFleetID: "fleet-9", Role: "owner"}, nil
	}
	r, proc, _, hook, raw := mountRefresh(t, store, brokenSignerProcessor(store), resolve)

	rec := postRefresh(r, raw)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401 — the mint must actually have failed: %s", rec.Code, rec.Body.String())
	}
	if !loggedAtLevel(hook, logrus.ErrorLevel, "mint access on refresh") {
		t.Fatalf("the test did not reach the mint-failure branch: %+v", hook.AllEntries())
	}

	c := refreshCookieSet(rec)
	if c == nil {
		t.Fatal("no refresh cookie was set on the mint-failure exit: the browser would keep the token " +
			"Rotate already consumed and trip family revocation on its next attempt")
	}
	if c.Value == raw {
		t.Fatalf("the cookie still carries the CONSUMED token %q", raw)
	}

	// Asserted against stored state, not the response: the value the browser now
	// holds must rotate cleanly once the signing fault is fixed.
	if _, userID, err := proc.Rotate(c.Value); err != nil {
		t.Fatalf("re-presenting the cookie written alongside the mint failure failed: %v — "+
			"the next attempt would revoke the family", err)
	} else if userID != "user-1" {
		t.Fatalf("userID = %q, want user-1", userID)
	}
}

// TestRefresh_transientFailureCannotReviveARevokedToken: a 503 must never
// become a way to keep a dead session alive. Rotate decides token validity and
// runs BEFORE the resolver, so a revoked cookie still ends at 401-and-clear
// however unavailable fleet-service is.
func TestRefresh_transientFailureCannotReviveARevokedToken(t *testing.T) {
	r, _, _, _, raw := newRefreshRouter(t, transientResolver())

	// Spend the token first: the second presentation is a replay.
	postRefresh(r, raw)
	rec := postRefresh(r, raw)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401 — a replayed token must not reach the transient branch", rec.Code)
	}
	if !refreshCookieCleared(rec) {
		t.Fatalf("a replayed token must still clear the cookie; cookies = %v", rec.Result().Cookies())
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
	r, _, store, _, raw := newRefreshRouter(t, unusedResolver)
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
	r, _, store, _, raw := newRefreshRouter(t, unusedResolver)

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
	r, _, store, _, _ := newRefreshRouter(t, unusedResolver)
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
	r, _, store, _, _ := newRefreshRouter(t, unusedResolver)
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
