package user

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/sirupsen/logrus"

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
func serveMe(t *testing.T, userID string) *httptest.ResponseRecorder {
	t.Helper()
	db := newTestDB(t)
	seedUser(t, db)

	log := logrus.New()
	log.SetOutput(io.Discard)

	r := chi.NewRouter()
	r.Group(InitializeRoutes(log, db))

	req := httptest.NewRequest(http.MethodGet, "/auth/me", nil)
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
