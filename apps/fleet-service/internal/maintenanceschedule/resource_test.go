package maintenanceschedule

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/sirupsen/logrus"
	"gorm.io/gorm"

	"github.com/jtumidanski/myfleet/packages/shared-go/auth"

	"github.com/jtumidanski/myfleet/apps/fleet-service/internal/maintenancerecord"
	"github.com/jtumidanski/myfleet/apps/fleet-service/internal/vehicle"
)

// stubVehicleAccessor satisfies VehicleAccessor. The create route resolves the
// vehicle for authz-fleet-scoping AND to read the current mileage baseline
// that the handler feeds into SetCurrentMileage.
type stubVehicleAccessor struct {
	fleetID        string
	currentMileage int
}

func (s stubVehicleAccessor) GetByID(id string) (vehicle.Model, error) {
	return vehicle.NewBuilder().SetFleetID(s.fleetID).SetMake("Subaru").SetModel("Outback").
		SetYear(2019).SetCurrentMileage(s.currentMileage).Build()
}

// newScheduleRouter builds the real chi router over a seeded in-memory
// database, wiring the actual InitializeRoutes handler (including the create
// closure under test) rather than calling any builder directly.
func newScheduleRouter(t *testing.T, fleetID string, currentMileage int) (chi.Router, *gorm.DB) {
	t.Helper()
	db := newCompletionDB(t)
	log := logrus.New()
	log.SetOutput(io.Discard)

	deps := NewCompletionDeps(db, maintenancerecord.NewAdministrator(db), NewAdministrator(db))
	r := chi.NewRouter()
	r.Group(InitializeRoutes(log, db, stubVehicleAccessor{fleetID: fleetID, currentMileage: currentMileage}, deps))
	return r, db
}

func scheduleIdentity(role, fleetID string) auth.Identity {
	return auth.Identity{UserID: "u1", Email: "u1@example.com", ActiveFleetID: fleetID, Role: role}
}

// postSchedule drives a real POST /vehicles/{id}/maintenance-schedules request
// through the router, standing in for the JWT middleware upstream with an
// Identity placed directly on the request context.
func postSchedule(r chi.Router, vehicleID string, attrs map[string]any, id auth.Identity) *httptest.ResponseRecorder {
	body := map[string]any{"data": map[string]any{"attributes": attrs}}
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/vehicles/"+vehicleID+"/maintenance-schedules", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(auth.WithIdentity(req.Context(), id))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

// TestCreate_handlerWiresDuePointThrough exercises the actual HTTP create
// route (not the builder directly). It pins the wiring in resource.go that
// threads attrs.DueDate/attrs.DueMileage into SetDuePoint and v.CurrentMileage
// into SetCurrentMileage before Build/Insert: if any of those three setter
// calls is dropped from the create handler, this test fails even though
// Builder/validate themselves are unchanged.
func TestCreate_handlerWiresDuePointThrough(t *testing.T) {
	cases := []struct {
		name           string
		recurrenceType string
		oneTime        bool
		intervalMonths int
		intervalMiles  int
	}{
		{name: "one-time", recurrenceType: "hybrid", oneTime: true},
		{name: "recurring", recurrenceType: "hybrid", oneTime: false, intervalMonths: 12, intervalMiles: 5000},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r, db := newScheduleRouter(t, "f1", 59000)
			vehicleID := seedVehicle(t, db, 59000)

			rec := postSchedule(r, vehicleID, map[string]any{
				"categoryId":     "c1",
				"recurrenceType": tc.recurrenceType,
				"oneTime":        tc.oneTime,
				"intervalMonths": tc.intervalMonths,
				"intervalMiles":  tc.intervalMiles,
				"dueDate":        "2026-11-30T00:00:00Z",
				"dueMileage":     60000,
			}, scheduleIdentity("member", "f1"))

			if rec.Code != http.StatusCreated {
				t.Fatalf("status = %d, body = %s, want 201", rec.Code, rec.Body.String())
			}

			var doc struct {
				Data struct {
					ID         string `json:"id"`
					Attributes struct {
						OneTime    bool   `json:"oneTime"`
						DueDate    string `json:"dueDate"`
						DueMileage int    `json:"dueMileage"`
					} `json:"attributes"`
				} `json:"data"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &doc); err != nil {
				t.Fatalf("decode response: %v", err)
			}
			if doc.Data.Attributes.OneTime != tc.oneTime {
				t.Errorf("response oneTime = %v want %v", doc.Data.Attributes.OneTime, tc.oneTime)
			}
			if doc.Data.Attributes.DueDate != "2026-11-30T00:00:00Z" {
				t.Errorf("response dueDate = %q want the posted due point", doc.Data.Attributes.DueDate)
			}
			if doc.Data.Attributes.DueMileage != 60000 {
				t.Errorf("response dueMileage = %d want 60000", doc.Data.Attributes.DueMileage)
			}

			// Re-read from the DB, not just the response: the due point must
			// have actually been persisted by way of the handler's Build/Insert.
			persisted, err := NewProvider(db).GetByID(doc.Data.ID)
			if err != nil {
				t.Fatalf("reload persisted schedule: %v", err)
			}
			if persisted.OneTime() != tc.oneTime {
				t.Errorf("persisted OneTime = %v want %v", persisted.OneTime(), tc.oneTime)
			}
			if persisted.DueMileage() != 60000 {
				t.Errorf("persisted DueMileage = %d want 60000", persisted.DueMileage())
			}
			if persisted.DueDate().IsZero() {
				t.Error("persisted DueDate is zero, want the posted due point")
			}
		})
	}
}

// TestCreate_rejectsMissingDuePointForNeverCompletedSchedule drives the same
// real HTTP route without a due point. validate rejects a never-completed
// schedule with no due point (the regression Task 4 introduced and Task 8
// fixed); this proves the create route actually surfaces that rejection to
// the caller as a 422, not just that Builder.Build does internally.
func TestCreate_rejectsMissingDuePointForNeverCompletedSchedule(t *testing.T) {
	cases := []struct {
		name           string
		recurrenceType string
		oneTime        bool
		intervalMonths int
		intervalMiles  int
	}{
		{name: "one-time without a due point", recurrenceType: "hybrid", oneTime: true},
		{name: "recurring without a due point", recurrenceType: "hybrid", oneTime: false, intervalMonths: 12, intervalMiles: 5000},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r, db := newScheduleRouter(t, "f1", 59000)
			vehicleID := seedVehicle(t, db, 59000)

			rec := postSchedule(r, vehicleID, map[string]any{
				"categoryId":     "c1",
				"recurrenceType": tc.recurrenceType,
				"oneTime":        tc.oneTime,
				"intervalMonths": tc.intervalMonths,
				"intervalMiles":  tc.intervalMiles,
			}, scheduleIdentity("member", "f1"))

			if rec.Code != http.StatusUnprocessableEntity {
				t.Fatalf("status = %d, body = %s, want 422", rec.Code, rec.Body.String())
			}

			if n := countRows(t, db, "fleet.maintenance_schedules", "vehicle_id = ?", vehicleID); n != 0 {
				t.Errorf("a rejected create wrote %d schedule rows", n)
			}
		})
	}
}

// TestCreate_viewerIsForbidden confirms the real authz guard runs on the
// create route, not just that the handler would build a valid model.
func TestCreate_viewerIsForbidden(t *testing.T) {
	r, db := newScheduleRouter(t, "f1", 59000)
	vehicleID := seedVehicle(t, db, 59000)

	rec := postSchedule(r, vehicleID, map[string]any{
		"categoryId":     "c1",
		"recurrenceType": "hybrid",
		"oneTime":        true,
		"dueDate":        "2026-11-30T00:00:00Z",
		"dueMileage":     60000,
	}, scheduleIdentity("viewer", "f1"))

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, body = %s, want 403", rec.Code, rec.Body.String())
	}
	if n := countRows(t, db, "fleet.maintenance_schedules", "vehicle_id = ?", vehicleID); n != 0 {
		t.Errorf("a forbidden create wrote %d schedule rows", n)
	}
}
