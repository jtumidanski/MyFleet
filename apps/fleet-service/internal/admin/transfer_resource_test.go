package admin_test

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/sirupsen/logrus"

	"github.com/jtumidanski/myfleet/apps/fleet-service/internal/admin"
	"github.com/jtumidanski/myfleet/packages/shared-go/auth"
)

// transferRouter mounts the admin tree with the given identity injected.
// resource_test.go's own `serveAs` builds a router from a bare Processor; this
// needs the harness's fully-wired one, hence the second helper. Read `serveAs`
// first and keep the two consistent.
func transferRouter(t *testing.T, h transferHarness, id auth.Identity) http.Handler {
	t.Helper()
	log := logrus.New()
	log.SetOutput(io.Discard)
	r := chi.NewRouter()
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			next.ServeHTTP(w, req.WithContext(auth.WithIdentity(req.Context(), id)))
		})
	})
	admin.InitializeRoutes(log, h.proc)(r)
	return r
}

var (
	adminIdentity = auth.Identity{UserID: "admin-1", Email: "admin@example.com", PlatformAdmin: true}
	plainIdentity = auth.Identity{UserID: "user-1", Email: "user@example.com"}
)

// errorDetail pulls the first error's `detail` out of the envelope. Every 4xx
// and 503 the transfer contract defines must carry one: FR-XFER-UI-7 shows it
// to the operator verbatim, and a status code on its own tells them nothing
// they can act on.
func errorDetail(t *testing.T, body []byte) string {
	t.Helper()
	var env struct {
		Errors []struct {
			Detail string `json:"detail"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(body, &env); err != nil {
		t.Fatalf("decode error envelope %s: %v", body, err)
	}
	if len(env.Errors) == 0 {
		t.Fatalf("no errors in the envelope: %s", body)
	}
	return env.Errors[0].Detail
}

// FR-XFER-ELIG-1. Both routes, both verbs.
//
// The preview is the one that matters most: the processor's
// PreviewVehicleTransfer runs no authorization of its own, so an ungated route
// would hand any signed-in caller another household's vehicle label and both
// fleet names.
func TestTransferRoutes_forbidNonPlatformAdmins(t *testing.T) {
	h := newTransferHarness(t)
	r := transferRouter(t, h, plainIdentity)

	for _, tc := range []struct{ method, path, body string }{
		{http.MethodGet, "/admin/vehicles/" + h.src.VehicleID + "/transfer-preview?destination_fleet_id=fleet-b", ""},
		{http.MethodGet, "/admin/vehicles/" + h.src.VehicleID + "/transfer-preview", ""},
		{
			http.MethodPost, "/admin/vehicles/" + h.src.VehicleID + "/transfer",
			`{"data":{"type":"vehicle-transfers","attributes":{"destination_fleet_id":"fleet-b","confirmation":"x"}}}`,
		},
	} {
		req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusForbidden {
			t.Errorf("%s %s: status = %d, want 403", tc.method, tc.path, w.Code)
			continue
		}
		if d := errorDetail(t, w.Body.Bytes()); !strings.Contains(d, "platform administrator") {
			t.Errorf("%s %s: detail = %q, want it to name the missing privilege", tc.method, tc.path, d)
		}
		// The refusal must not have leaked the thing it refused.
		if strings.Contains(w.Body.String(), seededLabel) {
			t.Errorf("%s %s: body leaked the vehicle label: %s", tc.method, tc.path, w.Body.String())
		}
	}
	if len(h.media.calls) != 0 {
		t.Error("media was called for a non-admin request")
	}
}

// An anonymous caller carries no identity at all and must be refused by the
// same gate rather than reaching either handler.
func TestTransferRoutes_forbidAnonymousCallers(t *testing.T) {
	h := newTransferHarness(t)
	r := transferRouter(t, h, auth.Identity{})

	req := httptest.NewRequest(http.MethodGet,
		"/admin/vehicles/"+h.src.VehicleID+"/transfer-preview?destination_fleet_id=fleet-b", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", w.Code)
	}
	if strings.Contains(w.Body.String(), seededLabel) {
		t.Errorf("body leaked the vehicle label: %s", w.Body.String())
	}
}

func TestTransferPreviewRoute_returnsTheDocumentedShape(t *testing.T) {
	h := newTransferHarness(t)
	r := transferRouter(t, h, adminIdentity)

	req := httptest.NewRequest(http.MethodGet,
		"/admin/vehicles/"+h.src.VehicleID+"/transfer-preview?destination_fleet_id=fleet-b", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}

	var doc struct {
		Data struct {
			Type       string `json:"type"`
			ID         string `json:"id"`
			Attributes struct {
				VehicleLabel         string         `json:"vehicle_label"`
				SourceFleetID        string         `json:"source_fleet_id"`
				SourceFleetName      string         `json:"source_fleet_name"`
				DestinationFleetID   string         `json:"destination_fleet_id"`
				DestinationFleetName string         `json:"destination_fleet_name"`
				Counts               map[string]int `json:"counts"`
				CategoriesToCreate   []struct {
					Name string `json:"name"`
					Kind string `json:"kind"`
				} `json:"categories_to_create"`
				Warnings []string `json:"warnings"`
			} `json:"attributes"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &doc); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if doc.Data.Type != "vehicle-transfer-previews" {
		t.Errorf("type = %q", doc.Data.Type)
	}
	if doc.Data.ID != h.src.VehicleID {
		t.Errorf("id = %q, want the vehicle id", doc.Data.ID)
	}
	if doc.Data.Attributes.VehicleLabel != seededLabel {
		t.Errorf("vehicle_label = %q", doc.Data.Attributes.VehicleLabel)
	}
	if doc.Data.Attributes.SourceFleetName == "" || doc.Data.Attributes.DestinationFleetName == "" {
		t.Error("fleet names are missing; the dialog names the destination in its toast")
	}
	if doc.Data.Attributes.SourceFleetID != "fleet-a" || doc.Data.Attributes.DestinationFleetID != "fleet-b" {
		t.Errorf("fleets = %s -> %s", doc.Data.Attributes.SourceFleetID, doc.Data.Attributes.DestinationFleetID)
	}
	if doc.Data.Attributes.Counts["fuel_logs"] != 1 {
		t.Errorf("counts = %v", doc.Data.Attributes.Counts)
	}
	if doc.Data.Attributes.Warnings == nil {
		t.Error("warnings serialised as null; it must be []")
	}
	if doc.Data.Attributes.CategoriesToCreate == nil {
		t.Error("categories_to_create serialised as null; it must be []")
	}
}

func TestTransferPreviewRoute_unknownVehicleIs404(t *testing.T) {
	h := newTransferHarness(t)
	r := transferRouter(t, h, adminIdentity)

	req := httptest.NewRequest(http.MethodGet, "/admin/vehicles/nope/transfer-preview", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", w.Code)
	}
	if d := errorDetail(t, w.Body.Bytes()); !strings.Contains(d, "vehicle not found") {
		t.Errorf("detail = %q, want the server's own sentence", d)
	}
}

func TestTransferPreviewRoute_unknownDestinationIs404(t *testing.T) {
	h := newTransferHarness(t)
	r := transferRouter(t, h, adminIdentity)

	req := httptest.NewRequest(http.MethodGet,
		"/admin/vehicles/"+h.src.VehicleID+"/transfer-preview?destination_fleet_id=fleet-nope", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404: %s", w.Code, w.Body.String())
	}
	if d := errorDetail(t, w.Body.Bytes()); !strings.Contains(d, "destination fleet not found") {
		t.Errorf("detail = %q", d)
	}
}

// A destination id that could not name any fleet is the caller's mistake, and
// the operator needs to be told that rather than "not found" — which invites
// them to go hunting for a fleet that never could have existed.
func TestTransferRoutes_malformedDestinationIs422(t *testing.T) {
	h := newTransferHarness(t)
	r := transferRouter(t, h, adminIdentity)

	for _, tc := range []struct {
		name, dest string
	}{
		{"whitespace only", "   "},
		{"embedded whitespace", "fleet b"},
		{"newline injected", "fleet-b\n"},
		{"absurdly long", strings.Repeat("f", 200)},
	} {
		t.Run("preview/"+tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet,
				"/admin/vehicles/"+h.src.VehicleID+"/transfer-preview?destination_fleet_id="+
					url.QueryEscape(tc.dest), nil)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			if w.Code != http.StatusUnprocessableEntity {
				t.Fatalf("status = %d, want 422: %s", w.Code, w.Body.String())
			}
			if d := errorDetail(t, w.Body.Bytes()); !strings.Contains(d, "destination_fleet_id") {
				t.Errorf("detail = %q, want it to name the offending field", d)
			}
		})

		t.Run("transfer/"+tc.name, func(t *testing.T) {
			body, err := json.Marshal(map[string]any{"data": map[string]any{
				"type": "vehicle-transfers",
				"attributes": map[string]any{
					"destination_fleet_id": tc.dest,
					"confirmation":         seededLabel,
				},
			}})
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			req := httptest.NewRequest(http.MethodPost,
				"/admin/vehicles/"+h.src.VehicleID+"/transfer", strings.NewReader(string(body)))
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			if w.Code != http.StatusUnprocessableEntity {
				t.Fatalf("status = %d, want 422: %s", w.Code, w.Body.String())
			}
			if d := errorDetail(t, w.Body.Bytes()); !strings.Contains(d, "destination_fleet_id") {
				t.Errorf("detail = %q, want it to name the offending field", d)
			}
			if got := scanOne[string](t, h.db, `SELECT fleet_id FROM fleet.vehicles WHERE id = ?`,
				h.src.VehicleID); got != "fleet-a" {
				t.Errorf("the vehicle moved to %q on a malformed request", got)
			}
		})
	}
	if len(h.media.calls) != 0 {
		t.Errorf("media was called for a malformed destination: %v", h.media.calls)
	}
}

// An absent destination is NOT malformed on the preview: it is the "pick a
// destination" state of the dialog, and the source-side counts are still
// computable without one.
func TestTransferPreviewRoute_absentDestinationIsAllowed(t *testing.T) {
	h := newTransferHarness(t)
	r := transferRouter(t, h, adminIdentity)

	req := httptest.NewRequest(http.MethodGet,
		"/admin/vehicles/"+h.src.VehicleID+"/transfer-preview", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"destination_fleet_id":""`) {
		t.Errorf("body = %s, want an empty destination", w.Body.String())
	}
}

// The POST, however, cannot proceed without one — and that is a 422 with its
// own sentence, not the malformed one.
func TestTransferRoute_absentDestinationIs422(t *testing.T) {
	h := newTransferHarness(t)
	r := transferRouter(t, h, adminIdentity)

	body := `{"data":{"type":"vehicle-transfers","attributes":{` +
		`"destination_fleet_id":"","confirmation":"` + seededLabel + `"}}}`
	req := httptest.NewRequest(http.MethodPost,
		"/admin/vehicles/"+h.src.VehicleID+"/transfer", strings.NewReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422: %s", w.Code, w.Body.String())
	}
	if d := errorDetail(t, w.Body.Bytes()); !strings.Contains(d, "required") {
		t.Errorf("detail = %q, want the 'required' sentence", d)
	}
}

func TestTransferRoute_happyPathReturnsTheAppliedCounts(t *testing.T) {
	h := newTransferHarness(t)
	r := transferRouter(t, h, adminIdentity)

	body := `{"data":{"type":"vehicle-transfers","attributes":{` +
		`"destination_fleet_id":"fleet-b","confirmation":"` + seededLabel + `"}}}`
	req := httptest.NewRequest(http.MethodPost,
		"/admin/vehicles/"+h.src.VehicleID+"/transfer", strings.NewReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}

	var doc struct {
		Data struct {
			Type       string `json:"type"`
			ID         string `json:"id"`
			Attributes struct {
				VehicleID          string         `json:"vehicle_id"`
				SourceFleetID      string         `json:"source_fleet_id"`
				DestinationFleetID string         `json:"destination_fleet_id"`
				TransferredAt      time.Time      `json:"transferred_at"`
				AffectedCounts     map[string]int `json:"affected_counts"`
			} `json:"attributes"`
		} `json:"data"`
		Meta struct {
			CountSemantics map[string]string `json:"count_semantics"`
		} `json:"meta"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &doc); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if doc.Data.Type != "vehicle-transfers" || doc.Data.ID != h.src.VehicleID {
		t.Errorf("type/id = %s/%s", doc.Data.Type, doc.Data.ID)
	}
	if doc.Data.Attributes.DestinationFleetID != "fleet-b" {
		t.Errorf("destination_fleet_id = %q", doc.Data.Attributes.DestinationFleetID)
	}
	if doc.Data.Attributes.SourceFleetID != "fleet-a" {
		t.Errorf("source_fleet_id = %q", doc.Data.Attributes.SourceFleetID)
	}
	if doc.Data.Attributes.VehicleID != h.src.VehicleID {
		t.Errorf("vehicle_id = %q", doc.Data.Attributes.VehicleID)
	}
	if doc.Data.Attributes.AffectedCounts["media_objects"] != 1 {
		t.Errorf("affected_counts = %v", doc.Data.Attributes.AffectedCounts)
	}
	if doc.Data.Attributes.TransferredAt.IsZero() {
		t.Error("transferred_at is zero")
	}

	// The two downstream keys are the reassign endpoints' READ-BACK of the
	// destination fleet, not "rows this transfer moved" (design D7,
	// mergeDownstreamCount). The Go doc comment saying so does not travel into
	// the JSON an operator reads, so the response says it too.
	for _, key := range []string{"media_objects", "notifications"} {
		got := doc.Meta.CountSemantics[key]
		if !strings.Contains(got, "now in the destination fleet") {
			t.Errorf("meta.count_semantics[%q] = %q, want it to say the count is the destination's end state", key, got)
		}
		if !strings.Contains(got, "not the number moved by this transfer") {
			t.Errorf("meta.count_semantics[%q] = %q, want it to deny the 'moved' reading", key, got)
		}
	}
	// The local keys mean what they look like, so annotating them would be
	// noise — and would dilute the two annotations that matter.
	if _, ok := doc.Meta.CountSemantics["fuel_logs"]; ok {
		t.Error("a fleet-service-local count was annotated; only the downstream read-backs need it")
	}
}

// FR-XFER-UI-7 depends on the detail reaching the client verbatim rather than
// being replaced by the redacted errInternal.
func TestTransferRoute_surfacesTheServersDetail(t *testing.T) {
	h := newTransferHarness(t)
	r := transferRouter(t, h, adminIdentity)

	body := `{"data":{"type":"vehicle-transfers","attributes":{` +
		`"destination_fleet_id":"fleet-a","confirmation":"` + seededLabel + `"}}}`
	req := httptest.NewRequest(http.MethodPost,
		"/admin/vehicles/"+h.src.VehicleID+"/transfer", strings.NewReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422", w.Code)
	}
	if !strings.Contains(w.Body.String(), "vehicle already belongs to that fleet") {
		t.Errorf("body = %s, want the server's own detail", w.Body.String())
	}
}

// A 409 confirmation mismatch must be a 409 through the whole stack, and must
// still have written nothing.
func TestTransferRoute_confirmationMismatchIs409(t *testing.T) {
	h := newTransferHarness(t)
	r := transferRouter(t, h, adminIdentity)

	body := `{"data":{"type":"vehicle-transfers","attributes":{` +
		`"destination_fleet_id":"fleet-b","confirmation":"wrong"}}}`
	req := httptest.NewRequest(http.MethodPost,
		"/admin/vehicles/"+h.src.VehicleID+"/transfer", strings.NewReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409", w.Code)
	}
	if d := errorDetail(t, w.Body.Bytes()); !strings.Contains(d, "confirmation does not match") {
		t.Errorf("detail = %q", d)
	}
	if n := scanOne[int](t, h.db, `SELECT count(*) FROM fleet.admin_audit_events`); n != 0 {
		t.Errorf("audit rows = %d, want 0", n)
	}
}

// A downstream refusal is a 503 that still carries its sentence: it is an
// incident, so it stays logged, but the operator is told the transfer was
// rolled back rather than left guessing.
func TestTransferRoute_downstreamRefusalIs503(t *testing.T) {
	h := newTransferHarness(t)
	h.notif.err = errors.New("notification-service is down")
	r := transferRouter(t, h, adminIdentity)

	body := `{"data":{"type":"vehicle-transfers","attributes":{` +
		`"destination_fleet_id":"fleet-b","confirmation":"` + seededLabel + `"}}}`
	req := httptest.NewRequest(http.MethodPost,
		"/admin/vehicles/"+h.src.VehicleID+"/transfer", strings.NewReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503: %s", w.Code, w.Body.String())
	}
	if d := errorDetail(t, w.Body.Bytes()); !strings.Contains(d, "rolled back") {
		t.Errorf("detail = %q, want it to say the transfer was rolled back", d)
	}
	if got := scanOne[string](t, h.db, `SELECT fleet_id FROM fleet.vehicles WHERE id = ?`,
		h.src.VehicleID); got != "fleet-a" {
		t.Errorf("fleet_id = %q, want the rollback to have left it on fleet-a", got)
	}
}

// A platform-admin grant revoked between the JWT being minted and the write
// being attempted is a 403 from the processor's re-verification, not from the
// route guard — and it must survive the handler's error mapping intact.
func TestTransferRoute_revokedAdminIs403(t *testing.T) {
	h := newTransferHarness(t)
	log := logrus.New()
	log.SetOutput(io.Discard)
	h.proc = admin.NewProcessor(log, admin.Deps{
		DB: h.db, Provider: admin.NewProvider(h.db), Administrator: admin.NewAdministrator(h.db),
		Auth: fakeAuth{ok: false}, MediaReassign: h.media, NotificationReassign: notifAdapter{inner: h.notif},
	}, admin.NewTargetResolver(h.db))
	r := transferRouter(t, h, adminIdentity)

	body := `{"data":{"type":"vehicle-transfers","attributes":{` +
		`"destination_fleet_id":"fleet-b","confirmation":"` + seededLabel + `"}}}`
	req := httptest.NewRequest(http.MethodPost,
		"/admin/vehicles/"+h.src.VehicleID+"/transfer", strings.NewReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403: %s", w.Code, w.Body.String())
	}
	if d := errorDetail(t, w.Body.Bytes()); !strings.Contains(d, "no longer a platform administrator") {
		t.Errorf("detail = %q", d)
	}
}
