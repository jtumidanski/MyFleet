package maintenancerecord

import (
	"context"
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

func newRecordRouterWithDocs(t *testing.T, docs DocumentValidator) (chi.Router, *gorm.DB) {
	t.Helper()
	db := newTestDB(t)
	log := logrus.New()
	log.SetOutput(io.Discard)

	r := chi.NewRouter()
	r.Group(InitializeRoutes(log, db, stubVehicles{map[string]string{"v1": "f1"}}, stubCategories{}, docs))
	return r, db
}

func newRecordRouter(t *testing.T) (chi.Router, *gorm.DB) {
	t.Helper()
	// nil DocumentValidator is explicitly legal (see DocumentValidator's doc
	// comment); these tests attach no documents.
	return newRecordRouterWithDocs(t, nil)
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

// stubDocs satisfies DocumentValidator. err is what ValidateOwnership returns;
// calls records every id set it was asked about, so a test can prove the check
// ran BEFORE anything was written rather than after.
type stubDocs struct {
	err   error
	calls [][]string
}

func (s *stubDocs) ValidateOwnership(_ context.Context, _ string, mediaIDs []string) error {
	s.calls = append(s.calls, mediaIDs)
	return s.err
}

func attachDoc(r chi.Router, id, mediaID string, ident auth.Identity) *httptest.ResponseRecorder {
	return serveAs(r, http.MethodPost, "/maintenance-records/"+id+"/document-media",
		`{"data":{"type":"mediaRefs","attributes":{"mediaId":"`+mediaID+`"}}}`, ident)
}

func detachDoc(r chi.Router, id, mediaID string, ident auth.Identity) *httptest.ResponseRecorder {
	return serveAs(r, http.MethodDelete,
		"/maintenance-records/"+id+"/document-media/"+mediaID, "", ident)
}

// documentIDs decodes documentMediaIds out of a maintenanceRecords response.
func documentIDs(t *testing.T, rec *httptest.ResponseRecorder) []string {
	t.Helper()
	var env struct {
		Data struct {
			Attributes struct {
				DocumentMediaIDs []string `json:"documentMediaIds"`
			} `json:"attributes"`
		} `json:"data"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&env); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return env.Data.Attributes.DocumentMediaIDs
}

// This is the defect the branch fixes at the layer it lived in: the route did
// not exist, so the frontend's append call 404'd and left a confirmed media
// object referenced by nothing.
func TestAttachDocument_returns201WithTheUpdatedRecord(t *testing.T) {
	docs := &stubDocs{}
	r, db := newRecordRouterWithDocs(t, docs)
	recID := insertRecord(t, NewAdministrator(db), "v1", "cat-1", 5, 0)

	rec := attachDoc(r, recID, "media-1", identity("owner", "f1"))

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201", rec.Code)
	}
	if got := documentIDs(t, rec); len(got) != 1 || got[0] != "media-1" {
		t.Errorf("documentMediaIds = %v, want [media-1]", got)
	}
	// Read the row back rather than trusting the body.
	stored, err := NewProvider(db).GetByID(recID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if len(stored.DocumentMediaIDs()) != 1 {
		t.Errorf("stored documents = %v, want one", stored.DocumentMediaIDs())
	}
}

// FR-ATT-6: ownership must be proven BEFORE any write, so a rejection leaves
// nothing to roll back and cross-fleet media can never be grafted on.
func TestAttachDocument_rejectsMediaThatFailsOwnershipAndWritesNothing(t *testing.T) {
	docs := &stubDocs{err: server.ErrValidation}
	r, db := newRecordRouterWithDocs(t, docs)
	recID := insertRecord(t, NewAdministrator(db), "v1", "cat-1", 5, 0)

	rec := attachDoc(r, recID, "someone-elses-media", identity("owner", "f1"))

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422", rec.Code)
	}
	if len(docs.calls) != 1 || len(docs.calls[0]) != 1 || docs.calls[0][0] != "someone-elses-media" {
		t.Errorf("ValidateOwnership calls = %v, want one call with the single id", docs.calls)
	}
	stored, err := NewProvider(db).GetByID(recID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if len(stored.DocumentMediaIDs()) != 0 {
		t.Errorf("a rejected attach wrote %v", stored.DocumentMediaIDs())
	}
}

// FR-ATT-3: a blank id must never reach media-service as `?ids=`.
func TestAttachDocument_emptyMediaIDIs422AndMakesNoOwnershipCall(t *testing.T) {
	docs := &stubDocs{}
	r, db := newRecordRouterWithDocs(t, docs)
	recID := insertRecord(t, NewAdministrator(db), "v1", "cat-1", 5, 0)

	rec := attachDoc(r, recID, "", identity("owner", "f1"))

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422", rec.Code)
	}
	if len(docs.calls) != 0 {
		t.Errorf("ValidateOwnership was called with %v; a blank id must not reach media-service", docs.calls)
	}
}

// FR-ATT-8: the drawer's sequential loop retries after a partial failure.
func TestAttachDocument_isIdempotent(t *testing.T) {
	r, db := newRecordRouterWithDocs(t, &stubDocs{})
	recID := insertRecord(t, NewAdministrator(db), "v1", "cat-1", 5, 0)
	ident := identity("owner", "f1")

	if rec := attachDoc(r, recID, "media-1", ident); rec.Code != http.StatusCreated {
		t.Fatalf("first attach status = %d, want 201", rec.Code)
	}
	rec := attachDoc(r, recID, "media-1", ident)
	if rec.Code != http.StatusCreated {
		t.Fatalf("second attach status = %d, want 201", rec.Code)
	}
	if got := documentIDs(t, rec); len(got) != 1 {
		t.Errorf("documentMediaIds = %v, want the id exactly once", got)
	}
}

// FR-ATT-7 at the HTTP layer: the cap answers 422, not 500.
func TestAttachDocument_atTheCapIs422(t *testing.T) {
	r, db := newRecordRouterWithDocs(t, &stubDocs{})
	recID := insertRecord(t, NewAdministrator(db), "v1", "cat-1", 5, MaxDocuments)

	if rec := attachDoc(r, recID, "one-too-many", identity("owner", "f1")); rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422", rec.Code)
	}
}

func TestAttachDocument_viewerIsForbidden(t *testing.T) {
	docs := &stubDocs{}
	r, db := newRecordRouterWithDocs(t, docs)
	recID := insertRecord(t, NewAdministrator(db), "v1", "cat-1", 5, 0)

	if rec := attachDoc(r, recID, "media-1", identity("viewer", "f1")); rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
	if len(docs.calls) != 0 {
		t.Error("a forbidden call reached media-service")
	}
	stored, _ := NewProvider(db).GetByID(recID)
	if len(stored.DocumentMediaIDs()) != 0 {
		t.Error("a forbidden call mutated the row")
	}
}

// Cross-fleet is 404, NOT 403: authz.RequireSameFleet returns ErrNotFound so
// cross-fleet existence is never leaked. Same as TestPatch_otherFleetIsNotFound.
func TestAttachDocument_otherFleetIsNotFound(t *testing.T) {
	r, db := newRecordRouterWithDocs(t, &stubDocs{})
	recID := insertRecord(t, NewAdministrator(db), "v1", "cat-1", 5, 0)

	if rec := attachDoc(r, recID, "media-1", identity("owner", "f2")); rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestAttachDocumentRoute_softDeletedRecordIsNotFound(t *testing.T) {
	r, db := newRecordRouterWithDocs(t, &stubDocs{})
	a := NewAdministrator(db)
	recID := insertRecord(t, a, "v1", "cat-1", 5, 0)
	if err := a.SoftDelete(recID); err != nil {
		t.Fatalf("soft delete: %v", err)
	}

	if rec := attachDoc(r, recID, "media-1", identity("owner", "f1")); rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

// FR-DET-1: 204, and the id is gone from the next GET.
func TestDetachDocument_returns204AndTheIDIsGoneFromTheNextGet(t *testing.T) {
	r, db := newRecordRouterWithDocs(t, &stubDocs{})
	recID := insertRecord(t, NewAdministrator(db), "v1", "cat-1", 5, 0)
	ident := identity("owner", "f1")
	if rec := attachDoc(r, recID, "media-1", ident); rec.Code != http.StatusCreated {
		t.Fatalf("attach status = %d", rec.Code)
	}

	if rec := detachDoc(r, recID, "media-1", ident); rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", rec.Code)
	}

	got := serveAs(r, http.MethodGet, "/maintenance-records/"+recID, "", ident)
	if got.Code != http.StatusOK {
		t.Fatalf("get status = %d", got.Code)
	}
	if ids := documentIDs(t, got); len(ids) != 0 {
		t.Errorf("documentMediaIds = %v, want empty", ids)
	}
}

// The list path groups documents separately from the detail path, so it needs
// its own assertion — task-025's attachment-count column reads from it.
func TestDetachDocument_theIDIsGoneFromTheListPathToo(t *testing.T) {
	r, db := newRecordRouterWithDocs(t, &stubDocs{})
	recID := insertRecord(t, NewAdministrator(db), "v1", "cat-1", 5, 0)
	ident := identity("owner", "f1")
	if rec := attachDoc(r, recID, "media-1", ident); rec.Code != http.StatusCreated {
		t.Fatalf("attach status = %d", rec.Code)
	}
	if rec := detachDoc(r, recID, "media-1", ident); rec.Code != http.StatusNoContent {
		t.Fatalf("detach status = %d", rec.Code)
	}

	rec := serveAs(r, http.MethodGet, "/vehicles/v1/maintenance-records", "", ident)
	if rec.Code != http.StatusOK {
		t.Fatalf("list status = %d", rec.Code)
	}
	var env struct {
		Data []struct {
			Attributes struct {
				DocumentMediaIDs []string `json:"documentMediaIds"`
			} `json:"attributes"`
		} `json:"data"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&env); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(env.Data) != 1 {
		t.Fatalf("list returned %d records, want 1", len(env.Data))
	}
	if ids := env.Data[0].Attributes.DocumentMediaIDs; len(ids) != 0 {
		t.Errorf("list documentMediaIds = %v, want empty", ids)
	}
}

// FR-DET-4: never attached and already detached are indistinguishable.
func TestDetachDocument_unattachedIsNotFound(t *testing.T) {
	r, db := newRecordRouterWithDocs(t, &stubDocs{})
	recID := insertRecord(t, NewAdministrator(db), "v1", "cat-1", 5, 0)
	ident := identity("owner", "f1")

	if rec := detachDoc(r, recID, "never-attached", ident); rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}

	if rec := attachDoc(r, recID, "media-1", ident); rec.Code != http.StatusCreated {
		t.Fatalf("attach status = %d", rec.Code)
	}
	if rec := detachDoc(r, recID, "media-1", ident); rec.Code != http.StatusNoContent {
		t.Fatalf("first detach status = %d", rec.Code)
	}
	if rec := detachDoc(r, recID, "media-1", ident); rec.Code != http.StatusNotFound {
		t.Fatalf("second detach status = %d, want 404", rec.Code)
	}
}

func TestDetachDocument_viewerIsForbidden(t *testing.T) {
	r, db := newRecordRouterWithDocs(t, &stubDocs{})
	recID := insertRecord(t, NewAdministrator(db), "v1", "cat-1", 5, 0)
	if rec := attachDoc(r, recID, "media-1", identity("owner", "f1")); rec.Code != http.StatusCreated {
		t.Fatalf("attach status = %d", rec.Code)
	}

	if rec := detachDoc(r, recID, "media-1", identity("viewer", "f1")); rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
	stored, _ := NewProvider(db).GetByID(recID)
	if len(stored.DocumentMediaIDs()) != 1 {
		t.Error("a forbidden call detached the document")
	}
}

func TestDetachDocument_otherFleetIsNotFound(t *testing.T) {
	r, db := newRecordRouterWithDocs(t, &stubDocs{})
	recID := insertRecord(t, NewAdministrator(db), "v1", "cat-1", 5, 0)
	if rec := attachDoc(r, recID, "media-1", identity("owner", "f1")); rec.Code != http.StatusCreated {
		t.Fatalf("attach status = %d", rec.Code)
	}

	if rec := detachDoc(r, recID, "media-1", identity("owner", "f2")); rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

// Maintenance and modification are the same entity distinguished by the
// category's kind, and the routes must not care which it is. The handler never
// consults the category, so this pins that it stays that way.
func TestAttachAndDetach_behaveIdenticallyForAModificationKindRecord(t *testing.T) {
	r, db := newRecordRouterWithDocs(t, &stubDocs{})
	recID := insertRecord(t, NewAdministrator(db), "v1", "mod-category", 5, 0)
	ident := identity("owner", "f1")

	if rec := attachDoc(r, recID, "media-1", ident); rec.Code != http.StatusCreated {
		t.Fatalf("attach status = %d, want 201", rec.Code)
	}
	if rec := detachDoc(r, recID, "media-1", ident); rec.Code != http.StatusNoContent {
		t.Fatalf("detach status = %d, want 204", rec.Code)
	}
}

// PRD D2: PATCH is a pure field-patch. It accepts no documentMediaIds and must
// leave document rows entirely alone. This is the invariant that decision
// exists to protect, so it gets its own test.
func TestPatch_leavesDocumentRowsUntouched(t *testing.T) {
	r, db := newRecordRouterWithDocs(t, &stubDocs{})
	recID := insertRecord(t, NewAdministrator(db), "v1", "cat-1", 5, 0)
	ident := identity("owner", "f1")
	if rec := attachDoc(r, recID, "media-1", ident); rec.Code != http.StatusCreated {
		t.Fatalf("attach status = %d", rec.Code)
	}

	rec := patchRecord(r, recID, `{"vendor":"Corner Garage","documentMediaIds":[]}`, ident)
	if rec.Code != http.StatusOK {
		t.Fatalf("patch status = %d, want 200", rec.Code)
	}

	stored, err := NewProvider(db).GetByID(recID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got := stored.DocumentMediaIDs(); len(got) != 1 || got[0] != "media-1" {
		t.Errorf("documents = %v, want [media-1] — PATCH must not touch them", got)
	}
	if stored.Vendor() != "Corner Garage" {
		t.Errorf("Vendor() = %q; the patch itself should still have applied", stored.Vendor())
	}
}
