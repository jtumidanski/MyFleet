package mileage

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/sirupsen/logrus"
	"gorm.io/gorm"

	"github.com/jtumidanski/myfleet/packages/shared-go/auth"
	"github.com/jtumidanski/myfleet/packages/shared-go/server"

	"github.com/jtumidanski/myfleet/apps/fleet-service/internal/vehicle"
)

// stubVehicles satisfies VehicleAccessor. The handler only needs the fleet of
// the vehicle for authorization; odometer state is read from the database.
type stubVehicles struct{ fleetByVehicle map[string]string }

func (s stubVehicles) GetByID(id string) (vehicle.Model, error) {
	fleetID, ok := s.fleetByVehicle[id]
	if !ok {
		return vehicle.Model{}, server.ErrNotFound
	}
	return vehicle.NewBuilder().SetFleetID(fleetID).SetMake("Subaru").SetModel("Outback").SetYear(2019).Build()
}

func newMileageRouter(t *testing.T) (chi.Router, *gorm.DB) {
	t.Helper()
	db := newTestDB(t)
	log := logrus.New()
	log.SetOutput(io.Discard)

	r := chi.NewRouter()
	r.Group(InitializeRoutes(log, db, stubVehicles{map[string]string{"v1": "f1"}}, NewAdministrator(db)))
	return r, db
}

func serveAs(r chi.Router, method, path, body string, id auth.Identity) *httptest.ResponseRecorder {
	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, path, reader)
	req = req.WithContext(auth.WithIdentity(req.Context(), id))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

func ownerOf(fleetID string) auth.Identity {
	return auth.Identity{UserID: "u1", Email: "u1@example.com", ActiveFleetID: fleetID, Role: "owner"}
}

func postedMileage(t *testing.T, rec *httptest.ResponseRecorder) (flagged bool, source string) {
	t.Helper()
	var doc struct {
		Data struct {
			Attributes struct {
				Flagged bool   `json:"flagged"`
				Source  string `json:"source"`
			} `json:"attributes"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &doc); err != nil {
		t.Fatalf("decode body %q: %v", rec.Body.String(), err)
	}
	return doc.Data.Attributes.Flagged, doc.Data.Attributes.Source
}

func postMileage(r chi.Router, miles string) *httptest.ResponseRecorder {
	return serveAs(r, http.MethodPost, "/vehicles/v1/mileage",
		`{"data":{"type":"mileageRecords","attributes":{"mileage":`+miles+`}}}`, ownerOf("f1"))
}

func TestPostMileage_advancesOdometer(t *testing.T) {
	r, db := newMileageRouter(t)
	seedVehicle(t, db, "v1", 1000)

	rec := postMileage(r, "1500")

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body.String())
	}
	if flagged, _ := postedMileage(t, rec); flagged {
		t.Error("flagged = true, want false")
	}
	if got := currentMileage(t, db, "v1"); got != 1500 {
		t.Errorf("currentMileage = %d, want 1500", got)
	}
}

func TestPostMileage_lowerReadingFlaggedAndKept(t *testing.T) {
	r, db := newMileageRouter(t)
	seedVehicle(t, db, "v1", 2000)

	rec := postMileage(r, "1500")

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body.String())
	}
	flagged, source := postedMileage(t, rec)
	if !flagged {
		t.Error("flagged = false, want true")
	}
	if source != SourceManual {
		t.Errorf("source = %q, want %q", source, SourceManual)
	}
	if got := currentMileage(t, db, "v1"); got != 2000 {
		t.Errorf("currentMileage = %d, want 2000", got)
	}
	if n := liveRows(t, db, "v1"); n != 1 {
		t.Errorf("live rows = %d, want 1", n)
	}
}
