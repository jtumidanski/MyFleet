package user

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

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

const patchBody = `{"data":{"type":"users","attributes":{"themePreference":%q}}}`

// The value must survive a round trip through storage, not merely be echoed
// back out of the request that set it.
func TestPatchMe_persistsAndEchoesTheNewPreference(t *testing.T) {
	r, _ := newAuthRouter(t)

	rec := serve(r, http.MethodPatch, "/auth/me", fmt.Sprintf(patchBody, ThemeDark), testUserID)
	if rec.Code != http.StatusOK {
		t.Fatalf("PATCH /auth/me = %d, want 200. Body: %s", rec.Code, strings.TrimSpace(rec.Body.String()))
	}
	if !strings.Contains(rec.Body.String(), `"themePreference":"dark"`) {
		t.Fatalf("PATCH /auth/me did not echo the new value. Body: %s", rec.Body.String())
	}

	got := serve(r, http.MethodGet, "/auth/me", "", testUserID)
	if !strings.Contains(got.Body.String(), `"themePreference":"dark"`) {
		t.Fatalf("the PATCH did not reach storage — a later GET returned: %s", got.Body.String())
	}
}

// PRD §5.2: the field is required. PATCH-as-partial-update is not supported for
// a single-field resource, so an absent or empty value is a client error rather
// than a no-op that silently reports success.
func TestPatchMe_rejectsInvalidValues(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{"empty string", fmt.Sprintf(patchBody, "")},
		{"unknown value", fmt.Sprintf(patchBody, "purple")},
		{"absent attribute", `{"data":{"type":"users","attributes":{}}}`},
		{"malformed json", `{"data":`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, _ := newAuthRouter(t)

			// 422, not the PRD's 400: shared-go has no 400 sentinel, and
			// RegisterInputHandler already renders a malformed body as 422
			// (design §3.1).
			rec := serve(r, http.MethodPatch, "/auth/me", tt.body, testUserID)
			if rec.Code != http.StatusUnprocessableEntity {
				t.Fatalf("PATCH %s = %d, want 422. Body: %s", tt.name, rec.Code, strings.TrimSpace(rec.Body.String()))
			}

			// A rejected write must leave the stored value alone.
			got := serve(r, http.MethodGet, "/auth/me", "", testUserID)
			if !strings.Contains(got.Body.String(), `"themePreference":"system"`) {
				t.Fatalf("a rejected PATCH modified the stored value: %s", got.Body.String())
			}
		})
	}
}

// The title names the field and the accepted values without echoing the
// caller's raw input back (PRD §5.2, FR-SEC-2).
func TestPatchMe_validationTitleNamesTheFieldNotTheInput(t *testing.T) {
	r, _ := newAuthRouter(t)

	rec := serve(r, http.MethodPatch, "/auth/me", fmt.Sprintf(patchBody, "<script>purple"), testUserID)
	body := rec.Body.String()
	if !strings.Contains(body, "themePreference") || !strings.Contains(body, "light, dark, system") {
		t.Fatalf("the validation error must name the field and the allow-list: %s", body)
	}
	if strings.Contains(body, "purple") || strings.Contains(body, "<script>") {
		t.Fatalf("the validation error echoed the caller's raw input back: %s", body)
	}
}

// Regression for the pre-PR review finding: Administrator.Update issues a full
// gorm.Save on an Entity whose CreatedAt field ToEntity never populates. Before
// entity.go tagged CreatedAt `<-:create`, that full-column UPDATE clobbered
// created_at with the zero value (0001-01-01) on every PATCH /auth/me — and,
// pre-existing, on every returning-user login via ProvisionFromGoogle.
func TestPatchMe_doesNotZeroCreatedAt(t *testing.T) {
	r, db := newAuthRouter(t)

	want := time.Date(2020, 6, 15, 12, 0, 0, 0, time.UTC)
	if err := db.Exec("UPDATE auth.users SET created_at = ? WHERE id = ?", want, testUserID).Error; err != nil {
		t.Fatalf("seed created_at: %v", err)
	}

	rec := serve(r, http.MethodPatch, "/auth/me", fmt.Sprintf(patchBody, ThemeDark), testUserID)
	if rec.Code != http.StatusOK {
		t.Fatalf("PATCH /auth/me = %d, want 200. Body: %s", rec.Code, strings.TrimSpace(rec.Body.String()))
	}

	var e Entity
	if err := db.First(&e, "id = ?", testUserID).Error; err != nil {
		t.Fatalf("read back user: %v", err)
	}
	if !e.CreatedAt.Equal(want) {
		t.Fatalf("PATCH /auth/me changed created_at: got %v, want %v — Administrator.Update must not touch created_at",
			e.CreatedAt, want)
	}
}

func TestPatchMe_unknownUserIsNotFound(t *testing.T) {
	r, _ := newAuthRouter(t)

	rec := serve(r, http.MethodPatch, "/auth/me", fmt.Sprintf(patchBody, ThemeDark), testGoogleSub)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("PATCH /auth/me for an unknown user = %d, want 404. Body: %s",
			rec.Code, strings.TrimSpace(rec.Body.String()))
	}
}
