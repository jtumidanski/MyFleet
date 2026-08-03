package session

import (
	"context"
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

	"github.com/jtumidanski/myfleet/packages/shared-go/server"
)

// newRefreshRouter mounts the real public routes over a store seeded with one
// valid refresh token, and returns the router, the processor, a hook holding
// everything the handler logged, and that token's raw value.
func newRefreshRouter(t *testing.T, resolve PrincipalResolver) (chi.Router, *Processor, *logrustest.Hook, string) {
	t.Helper()
	store := newFakeStore()
	proc := newTestProcessor(store)

	raw := "valid-refresh-token"
	store.seed(NewBuilder().
		SetUserID("user-1").
		SetTokenHash(HashRefresh(raw)).
		SetExpiresAt(time.Now().Add(refreshTTL)).
		Build())

	log, hook := logrustest.NewNullLogger()

	r := chi.NewRouter()
	r.Group(InitializePublicRoutes(log, proc, resolve, false))
	return r, proc, hook, raw
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
	r, _, hook, raw := newRefreshRouter(t, resolve)

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
	r, proc, hook, raw := newRefreshRouter(t, transientResolver())

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
	if c.MaxAge < 0 {
		t.Fatalf("MaxAge = %d — this is the clearing form of the cookie", c.MaxAge)
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
	r, _, _, raw := newRefreshRouter(t, transientResolver())

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

// TestRefresh_transientFailureCannotReviveARevokedToken: a 503 must never
// become a way to keep a dead session alive. Rotate decides token validity and
// runs BEFORE the resolver, so a revoked cookie still ends at 401-and-clear
// however unavailable fleet-service is.
func TestRefresh_transientFailureCannotReviveARevokedToken(t *testing.T) {
	r, _, _, raw := newRefreshRouter(t, transientResolver())

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
