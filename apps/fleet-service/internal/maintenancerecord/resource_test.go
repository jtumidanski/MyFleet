package maintenancerecord

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

	"github.com/jtumidanski/myfleet/apps/fleet-service/internal/maintenancecategory"
	"github.com/jtumidanski/myfleet/apps/fleet-service/internal/vehicle"
)

// stubVehicles satisfies VehicleAccessor. The handler resolves a vehicle only
// to learn its fleet for authorization, so the fleet id is all that matters.
type stubVehicles struct{ fleetByVehicle map[string]string }

func (s stubVehicles) GetByID(id string) (vehicle.Model, error) {
	fleetID, ok := s.fleetByVehicle[id]
	if !ok {
		return vehicle.Model{}, server.ErrNotFound
	}
	return vehicle.NewBuilder().SetFleetID(fleetID).SetMake("Subaru").SetModel("Outback").SetYear(2019).Build()
}

// stubCategories satisfies CategoryAccessor; only the list endpoint's kind
// filter consults it, which these tests do not exercise.
type stubCategories struct{}

func (stubCategories) IDsByKind(maintenancecategory.Kind, string) ([]string, error) { return nil, nil }

func newRecordRouter(t *testing.T) (chi.Router, *gorm.DB) {
	t.Helper()
	db := newTestDB(t)
	log := logrus.New()
	log.SetOutput(io.Discard)

	r := chi.NewRouter()
	// nil DocumentValidator is explicitly legal (see DocumentValidator's doc
	// comment); these tests attach no documents.
	r.Group(InitializeRoutes(log, db, stubVehicles{map[string]string{"v1": "f1"}}, stubCategories{}, nil))
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

func identity(role, fleetID string) auth.Identity {
	return auth.Identity{UserID: "u1", Email: "u1@example.com", ActiveFleetID: fleetID, Role: role}
}

func patchRecord(r chi.Router, id, body string, ident auth.Identity) *httptest.ResponseRecorder {
	return serveAs(r, http.MethodPatch, "/maintenance-records/"+id,
		`{"data":{"type":"maintenanceRecords","attributes":`+body+`}}`, ident)
}

// This is the defect the branch fixes, asserted at the layer it lived in: the
// PATCH attrs struct carried no categoryId, so the edit was accepted and
// discarded. Deleting the CategoryID field or its apply-block fails this.
func TestPatch_persistsAnEditedCategory(t *testing.T) {
	r, db := newRecordRouter(t)
	admin := NewAdministrator(db)
	provider := NewProvider(db)
	recID := insertRecord(t, admin, "v1", "cat-original", 5, 0)

	rec := patchRecord(r, recID, `{"categoryId":"cat-reassigned"}`, identity("owner", "f1"))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	// Read the row back rather than trusting the response body: the response
	// used to echo an in-memory model and agreed with the caller even when
	// nothing was written.
	stored, err := provider.GetByID(recID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if stored.CategoryID() != "cat-reassigned" {
		t.Errorf("stored CategoryID() = %q, want %q", stored.CategoryID(), "cat-reassigned")
	}
}

// Omitting categoryId must leave it alone — the handler takes *string precisely
// so absent and empty are distinguishable.
func TestPatch_omittedCategoryIsUnchanged(t *testing.T) {
	r, db := newRecordRouter(t)
	recID := insertRecord(t, NewAdministrator(db), "v1", "cat-original", 5, 0)

	if rec := patchRecord(r, recID, `{"vendor":"Corner Garage"}`, identity("owner", "f1")); rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	stored, err := NewProvider(db).GetByID(recID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if stored.CategoryID() != "cat-original" {
		t.Errorf("CategoryID() = %q, want it unchanged", stored.CategoryID())
	}
	if stored.Vendor() != "Corner Garage" {
		t.Errorf("Vendor() = %q", stored.Vendor())
	}
}

// categoryID is an invariant; clearing it is a validation error, not a write.
func TestPatch_clearedCategoryIsRejected(t *testing.T) {
	r, db := newRecordRouter(t)
	recID := insertRecord(t, NewAdministrator(db), "v1", "cat-original", 5, 0)

	rec := patchRecord(r, recID, `{"categoryId":""}`, identity("owner", "f1"))

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422", rec.Code)
	}
	stored, err := NewProvider(db).GetByID(recID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if stored.CategoryID() != "cat-original" {
		t.Errorf("CategoryID() = %q, want the rejected write not to have landed", stored.CategoryID())
	}
}

// The PATCH response is what the client caches; it must reflect stored state.
// It previously carried a zero createdAt because the echoed model was built
// from an entity that never carried one.
func TestPatch_responseCarriesStoredState(t *testing.T) {
	r, db := newRecordRouter(t)
	recID := insertRecord(t, NewAdministrator(db), "v1", "cat-original", 5, 0)

	rec := patchRecord(r, recID, `{"categoryId":"cat-reassigned"}`, identity("owner", "f1"))

	var env struct {
		Data struct {
			Attributes struct {
				CategoryID string `json:"categoryId"`
				CreatedAt  string `json:"createdAt"`
			} `json:"attributes"`
		} `json:"data"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&env); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if env.Data.Attributes.CategoryID != "cat-reassigned" {
		t.Errorf("response categoryId = %q", env.Data.Attributes.CategoryID)
	}
	if strings.HasPrefix(env.Data.Attributes.CreatedAt, "0001-01-01") {
		t.Errorf("response createdAt = %q, want the stored timestamp", env.Data.Attributes.CreatedAt)
	}
}

func TestPatch_viewerIsForbidden(t *testing.T) {
	r, db := newRecordRouter(t)
	recID := insertRecord(t, NewAdministrator(db), "v1", "cat-original", 5, 0)

	if rec := patchRecord(r, recID, `{"categoryId":"cat-x"}`, identity("viewer", "f1")); rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
	stored, _ := NewProvider(db).GetByID(recID)
	if stored.CategoryID() != "cat-original" {
		t.Errorf("a forbidden call mutated the row")
	}
}

func TestPatch_otherFleetIsNotFound(t *testing.T) {
	r, db := newRecordRouter(t)
	recID := insertRecord(t, NewAdministrator(db), "v1", "cat-original", 5, 0)

	if rec := patchRecord(r, recID, `{"categoryId":"cat-x"}`, identity("owner", "f2")); rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}
