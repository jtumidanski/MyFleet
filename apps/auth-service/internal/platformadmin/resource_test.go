package platformadmin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

func newRouter(t *testing.T) (chi.Router, *gorm.DB) {
	t.Helper()
	db := newSeedDB(t)
	r := chi.NewRouter()
	InitializeInternalRoutes(logrus.New(), db)(r)
	return r, db
}

func TestInternalStats_countsUsers(t *testing.T) {
	r, db := newRouter(t)
	for _, id := range []string{"u1", "u2", "u3"} {
		if err := db.Exec(`INSERT INTO auth.users (id, google_sub, email) VALUES (?, ?, ?)`,
			id, "sub-"+id, id+"@example.com").Error; err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/internal/admin/stats", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Users int `json:"users"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Users != 3 {
		t.Errorf("users = %d, want 3", body.Users)
	}
}

func TestInternalUsers_resolvesRequestedIDsOnly(t *testing.T) {
	r, db := newRouter(t)
	for _, id := range []string{"u1", "u2"} {
		if err := db.Exec(`INSERT INTO auth.users (id, google_sub, email, display_name)
		                   VALUES (?, ?, ?, ?)`, id, "sub-"+id, id+"@example.com", "Name "+id).Error; err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/internal/admin/users?ids=u1,missing", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var body struct {
		Users []struct {
			ID          string `json:"id"`
			Email       string `json:"email"`
			DisplayName string `json:"display_name"`
		} `json:"users"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Users) != 1 || body.Users[0].ID != "u1" || body.Users[0].Email != "u1@example.com" {
		t.Errorf("want only u1 resolved, got %+v", body.Users)
	}
}

// The paginated mode backs /admin/users (FR-ADMIN-FLEET-6). Total is the whole
// directory, not the page, so the console can render a page count.
func TestInternalUsers_paginatedMode(t *testing.T) {
	r, db := newRouter(t)
	for _, id := range []string{"u1", "u2", "u3"} {
		if err := db.Exec(`INSERT INTO auth.users (id, google_sub, email) VALUES (?, ?, ?)`,
			id, "sub-"+id, id+"@example.com").Error; err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet,
		"/internal/admin/users?page[number]=1&page[size]=2", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Users []struct {
			ID string `json:"id"`
		} `json:"users"`
		Total int `json:"total"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Users) != 2 {
		t.Errorf("page size 2 returned %d users", len(body.Users))
	}
	if body.Total != 3 {
		t.Errorf("total = %d, want the whole directory (3), not the page", body.Total)
	}
}

// The endpoint is unauthenticated, so its input must be bounded — the same
// ceiling media-service already applies to /internal/media.
func TestInternalUsers_rejectsAnOversizedIDList(t *testing.T) {
	r, _ := newRouter(t)
	ids := make([]string, MaxInternalLookupIDs+1)
	for i := range ids {
		ids[i] = "u"
	}
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet,
		"/internal/admin/users?ids="+strings.Join(ids, ","), nil))
	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want 422", rec.Code)
	}
}

// FR-ADMIN-AUTH-7: this is the stale-claim re-verification fleet-service calls
// before an irreversible purge.
func TestInternalPlatformAdmins_reflectsTheTable(t *testing.T) {
	r, db := newRouter(t)
	if err := NewAdministrator(db).Grant("u1", BootstrapGrantedBy); err != nil {
		t.Fatalf("grant: %v", err)
	}

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/internal/admin/platform-admins/u1", nil))
	if rec.Code != http.StatusOK {
		t.Errorf("a granted admin must be 200, got %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/internal/admin/platform-admins/u2", nil))
	if rec.Code != http.StatusNotFound {
		t.Errorf("a non-admin must be 404, got %d", rec.Code)
	}
}
