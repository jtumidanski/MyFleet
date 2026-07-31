package user

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/sirupsen/logrus"
	"gorm.io/gorm"

	"github.com/jtumidanski/myfleet/packages/shared-go/auth"
)

// The regression guard for the login loop, at the layer where the bug actually
// lived. provider_test.go pins what each lookup means, but it cannot catch this
// handler calling the wrong one — reintroducing `proc.GetBySub(id.UserID)` here
// leaves every provider test green. This test drives the real chi route against
// a real database, so the wiring itself is covered.
//
// Symptom when this fails: GET /auth/me 404s for a perfectly valid token, the
// SPA reads that as logged-out, and the user is bounced back to the login page
// forever — having just completed a successful Google round-trip.

// newAuthRouter builds the real router over a seeded database and returns both,
// so a test can drive several requests against ONE dataset — PATCH then GET, to
// prove the write actually landed rather than merely echoing its own input.
func newAuthRouter(t *testing.T) (chi.Router, *gorm.DB) {
	t.Helper()
	db := newTestDB(t)
	seedUser(t, db)

	log := logrus.New()
	log.SetOutput(io.Discard)

	r := chi.NewRouter()
	r.Group(InitializeRoutes(log, db))
	return r, db
}

// serve drives one request with a validated Identity in context, standing in
// for the JWT middleware the real router mounts upstream.
func serve(r chi.Router, method, path, body, userID string) *httptest.ResponseRecorder {
	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, path, reader)
	req = req.WithContext(auth.WithIdentity(req.Context(), auth.Identity{
		UserID:        userID,
		Email:         "a@b.com",
		ActiveFleetID: "fleet-1",
		Role:          "owner",
	}))

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

func serveMe(t *testing.T, userID string) *httptest.ResponseRecorder {
	t.Helper()
	r, _ := newAuthRouter(t)
	return serve(r, http.MethodGet, "/auth/me", "", userID)
}

func TestAuthMe_resolvesTheUserIDCarriedByTheToken(t *testing.T) {
	rec := serveMe(t, testUserID)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /auth/me = %d, want 200. The token's `sub` is our user id; "+
			"looking it up by google_sub 404s and logs the user straight back out. Body: %s",
			rec.Code, strings.TrimSpace(rec.Body.String()))
	}
	if !strings.Contains(rec.Body.String(), "a@b.com") {
		t.Fatalf("GET /auth/me body did not contain the user's email: %s", rec.Body.String())
	}
}

// A Google sub is not a user id: presenting one must not resolve a user.
func TestAuthMe_doesNotResolveAGoogleSub(t *testing.T) {
	if rec := serveMe(t, testGoogleSub); rec.Code == http.StatusOK {
		t.Fatalf("GET /auth/me returned 200 for a Google sub; the handler must "+
			"resolve user ids only. Body: %s", strings.TrimSpace(rec.Body.String()))
	}
}

// FR-TEST-3 / PRD §5.1: the attribute is always present, so a client never has
// to guess a default.
func TestAuthMe_includesThemePreference(t *testing.T) {
	rec := serveMe(t, testUserID)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /auth/me = %d, want 200. Body: %s", rec.Code, strings.TrimSpace(rec.Body.String()))
	}
	if !strings.Contains(rec.Body.String(), `"themePreference":"system"`) {
		t.Fatalf("GET /auth/me omitted themePreference. Body: %s", rec.Body.String())
	}
}

// FR-DATA-4 end to end: a row holding a value the allow-list does not know —
// written before the column existed, or edited out of band — surfaces as
// `system` rather than leaking an out-of-range value to the SPA.
func TestAuthMe_normalisesAnEmptyStoredTheme(t *testing.T) {
	r, db := newAuthRouter(t)
	if err := db.Exec("UPDATE auth.users SET theme_preference = '' WHERE id = ?", testUserID).Error; err != nil {
		t.Fatalf("blank the stored theme: %v", err)
	}

	rec := serve(r, http.MethodGet, "/auth/me", "", testUserID)
	if !strings.Contains(rec.Body.String(), `"themePreference":"system"`) {
		t.Fatalf("a blank stored theme must read back as system. Body: %s", rec.Body.String())
	}
}
