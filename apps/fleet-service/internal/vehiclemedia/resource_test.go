package vehiclemedia

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/sirupsen/logrus"
	"gorm.io/gorm"

	"github.com/jtumidanski/myfleet/packages/shared-go/auth"
	"github.com/jtumidanski/myfleet/packages/shared-go/server"

	"github.com/jtumidanski/myfleet/apps/fleet-service/internal/vehicle"
)

// stubVehicles satisfies VehicleGetter. The route resolves a vehicle only to
// learn its fleet for the authorization check, so the fleet id is all that
// matters here.
type stubVehicles struct {
	fleetByVehicle map[string]string
}

func (s stubVehicles) GetByID(id string) (vehicle.Model, error) {
	fleetID, ok := s.fleetByVehicle[id]
	if !ok {
		return vehicle.Model{}, server.ErrNotFound
	}
	m, err := vehicle.NewBuilder().SetFleetID(fleetID).SetMake("Subaru").SetModel("Outback").SetYear(2019).Build()
	if err != nil {
		return vehicle.Model{}, err
	}
	return m, nil
}

// newMediaRouter builds the real chi router over a seeded in-memory database
// and returns both, so a test can drive a request and then read the rows back
// rather than trusting the response.
func newMediaRouter(t *testing.T, fleetByVehicle map[string]string) (chi.Router, *gorm.DB) {
	t.Helper()
	db := newTestDB(t)
	log := logrus.New()
	log.SetOutput(io.Discard)

	r := chi.NewRouter()
	r.Group(InitializeRoutes(log, db, stubVehicles{fleetByVehicle: fleetByVehicle}))
	return r, db
}

// serveAs drives one request with a validated Identity on context, standing in
// for the JWT middleware the real router mounts upstream.
func serveAs(r chi.Router, method, path string, id auth.Identity) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, nil)
	req = req.WithContext(auth.WithIdentity(req.Context(), id))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

func identity(role, fleetID string) auth.Identity {
	return auth.Identity{UserID: "u1", Email: "u1@example.com", ActiveFleetID: fleetID, Role: role}
}

func deletePhoto(r chi.Router, vehicleID, mediaID string, id auth.Identity) *httptest.ResponseRecorder {
	return serveAs(r, http.MethodDelete, "/vehicles/"+vehicleID+"/media/"+mediaID, id)
}

// The route existing at all is the point: before this change the UI's only
// delete call went to media-service, and fleet-service had no way to drop the
// reference the gallery lists.
func TestDeleteMedia_removesTheReferenceAndReturns204(t *testing.T) {
	r, db := newMediaRouter(t, map[string]string{"v1": "f1"})
	seedVehicle(t, db, "v1", "m1", "m2")

	rec := deletePhoto(r, "v1", "m1", identity("owner", "f1"))

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", rec.Code)
	}
	if got := liveMediaIDs(t, db, "v1"); len(got) != 1 || got[0] != "m2" {
		t.Errorf("live media = %v, want [m2]", got)
	}
}

func TestDeleteMedia_viewerIsForbidden(t *testing.T) {
	r, db := newMediaRouter(t, map[string]string{"v1": "f1"})
	seedVehicle(t, db, "v1", "m1")

	rec := deletePhoto(r, "v1", "m1", identity("viewer", "f1"))

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
	// Authorization must run BEFORE the mutation, not merely change the status.
	if got := liveMediaIDs(t, db, "v1"); len(got) != 1 {
		t.Errorf("live media = %v, want the row untouched by a forbidden call", got)
	}
}

// Cross-fleet access answers 404 rather than 403 so existence is not leaked.
func TestDeleteMedia_otherFleetIsNotFound(t *testing.T) {
	r, db := newMediaRouter(t, map[string]string{"v1": "f1"})
	seedVehicle(t, db, "v1", "m1")

	rec := deletePhoto(r, "v1", "m1", identity("owner", "f2"))

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
	if got := liveMediaIDs(t, db, "v1"); len(got) != 1 {
		t.Errorf("live media = %v, want the row untouched by a cross-fleet call", got)
	}
}

// The media id is scoped to the vehicle in the path; a reference belonging to
// another vehicle in the SAME fleet must still be unreachable.
func TestDeleteMedia_cannotReachAnotherVehiclesReference(t *testing.T) {
	r, db := newMediaRouter(t, map[string]string{"v1": "f1", "v2": "f1"})
	seedVehicle(t, db, "v1", "m1")
	seedVehicle(t, db, "v2", "m2")

	rec := deletePhoto(r, "v1", "m2", identity("owner", "f1"))

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
	if got := liveMediaIDs(t, db, "v2"); len(got) != 1 {
		t.Errorf("v2 media = %v, want it untouched", got)
	}
}

func TestDeleteMedia_unknownMediaIsNotFound(t *testing.T) {
	r, db := newMediaRouter(t, map[string]string{"v1": "f1"})
	seedVehicle(t, db, "v1", "m1")

	if rec := deletePhoto(r, "v1", "nope", identity("owner", "f1")); rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}
