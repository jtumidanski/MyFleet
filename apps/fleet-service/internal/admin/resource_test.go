package admin_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/sirupsen/logrus"

	"github.com/jtumidanski/myfleet/apps/fleet-service/internal/admin"
	"github.com/jtumidanski/myfleet/apps/fleet-service/internal/admin/admintest"
	"github.com/jtumidanski/myfleet/packages/shared-go/auth"
)

type route struct{ method, path string }

// Every endpoint the /admin tree exposes. The table is exhaustive on purpose —
// a new endpoint added without the guard is exactly the failure this catches.
var adminRoutes = []route{
	{http.MethodGet, "/admin/stats"},
	{http.MethodGet, "/admin/fleets"},
	{http.MethodGet, "/admin/fleets/fleet-1"},
	{http.MethodGet, "/admin/users"},
	{http.MethodGet, "/admin/purge-operations"},
	{http.MethodGet, "/admin/purge-operations/op-1"},
	{http.MethodPost, "/admin/purge-operations"},
	{http.MethodDelete, "/admin/purge-operations/op-1"},
	{http.MethodPost, "/admin/purge-operations/op-1/retry"},
	{http.MethodGet, "/admin/audit-events"},
}

// readRoutes are the endpoints that must answer for an admin with no fleet at
// all, without needing a seeded target.
var readRoutes = []route{
	{http.MethodGet, "/admin/stats"},
	{http.MethodGet, "/admin/fleets"},
	{http.MethodGet, "/admin/users"},
	{http.MethodGet, "/admin/purge-operations"},
	{http.MethodGet, "/admin/audit-events"},
}

// serveAs builds the admin router and injects an identity directly, bypassing
// the JWT middleware — the guard under test is authz.RequirePlatformAdmin, not
// token parsing.
func serveAs(t *testing.T, id auth.Identity, rt route) *httptest.ResponseRecorder {
	t.Helper()
	db := admintest.NewDB(t)
	admintest.SeedFleet(t, db, "fleet-1")
	proc := newBrowseProcessor(t, db)

	r := chi.NewRouter()
	admin.InitializeRoutes(discardLogger(), proc)(r)

	body := ""
	if rt.method == http.MethodPost {
		body = `{"data":{"type":"purge-operations","attributes":{"scope":"fleet","target_id":"fleet-1","confirmation":"x"}}}`
	}
	req := httptest.NewRequest(rt.method, rt.path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/vnd.api+json")
	req = req.WithContext(auth.WithIdentity(req.Context(), id))

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

// PRD §10: every /admin endpoint is 403 for a non-admin. A fleet owner is a
// deliberately awkward case — owner of a fleet is not owner of the platform.
func TestAdminRoutes_rejectNonAdmins(t *testing.T) {
	for _, rt := range adminRoutes {
		t.Run(rt.method+" "+rt.path, func(t *testing.T) {
			rec := serveAs(t, auth.Identity{UserID: "u1", Role: "owner", ActiveFleetID: "f1"}, rt)
			if rec.Code != http.StatusForbidden {
				t.Errorf("status = %d, want 403 for a non-admin", rec.Code)
			}
		})
	}
}

// An anonymous caller carries no identity at all, and must be refused by the
// same guard rather than reaching a handler.
func TestAdminRoutes_rejectAnonymousCallers(t *testing.T) {
	for _, rt := range adminRoutes {
		t.Run(rt.method+" "+rt.path, func(t *testing.T) {
			rec := serveAs(t, auth.Identity{}, rt)
			if rec.Code != http.StatusForbidden {
				t.Errorf("status = %d, want 403 with no identity", rec.Code)
			}
		})
	}
}

// FR-ADMIN-AUTH-9: no admin endpoint may require an active fleet. An
// administrator standing in the wreckage of the system purge they just ran must
// still reach every one of them.
func TestAdminRoutes_doNotRequireAnActiveFleet(t *testing.T) {
	for _, rt := range readRoutes {
		t.Run(rt.method+" "+rt.path, func(t *testing.T) {
			rec := serveAs(t, auth.Identity{UserID: "u1", PlatformAdmin: true}, rt)
			if rec.Code == http.StatusForbidden {
				t.Errorf("%s %s returned 403 for a fleetless admin", rt.method, rt.path)
			}
			if rec.Code != http.StatusOK {
				t.Errorf("%s %s = %d, want 200: %s", rt.method, rt.path, rec.Code, rec.Body.String())
			}
		})
	}
}

// A fleetless admin can still inspect any fleet — that is the cross-fleet API
// working (FR-ADMIN-FLEET-1).
func TestAdminRoutes_fleetlessAdminCanInspectAnyFleet(t *testing.T) {
	rec := serveAs(t, auth.Identity{UserID: "u1", PlatformAdmin: true},
		route{http.MethodGet, "/admin/fleets/fleet-1"})
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
}

// An unknown purge operation is a 404, not a 500: the sentinel is mapped.
func TestAdminRoutes_unknownOperationIs404(t *testing.T) {
	rec := serveAs(t, auth.Identity{UserID: "u1", PlatformAdmin: true},
		route{http.MethodGet, "/admin/purge-operations/op-1"})
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404: %s", rec.Code, rec.Body.String())
	}
}

// discardLogger keeps the route tests quiet; the handlers log deliberately and
// none of it is what is under test here.
func discardLogger() logrus.FieldLogger {
	l := logrus.New()
	l.SetOutput(io.Discard)
	return l
}
