package admin_test

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/sirupsen/logrus"
	"gorm.io/gorm"

	"github.com/jtumidanski/myfleet/apps/media-service/internal/admin"
	"github.com/jtumidanski/myfleet/apps/media-service/internal/mediaobject"
	"github.com/jtumidanski/myfleet/packages/shared-go/server"
)

// reassignServer wires the internal routes over a seeded database, matching how
// admin_test.go stands up the purge routes.
func reassignServer(t *testing.T, db *gorm.DB) *httptest.Server {
	t.Helper()
	r := chi.NewRouter()
	log := logrus.New()
	log.SetOutput(io.Discard)
	admin.InitializeInternalRoutes(log, db, &recordingRemover{})(r)
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)
	return srv
}

func postReassign(t *testing.T, srv *httptest.Server, body string) (int, map[string]int) {
	t.Helper()
	res, err := http.Post(srv.URL+"/internal/admin/reassign-fleet",
		"application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer func() { _ = res.Body.Close() }()
	var decoded struct {
		Affected map[string]int `json:"affected"`
	}
	_ = json.NewDecoder(res.Body).Decode(&decoded)
	return res.StatusCode, decoded.Affected
}

func fleetOf(t *testing.T, db *gorm.DB, id string) string {
	t.Helper()
	var out []string
	if err := db.Raw(`SELECT fleet_id FROM media.media_objects WHERE id = ?`, id).Scan(&out).Error; err != nil {
		t.Fatalf("read fleet_id: %v", err)
	}
	if len(out) == 0 {
		t.Fatalf("no media object %s", id)
	}
	return out[0]
}

func TestReassign_movesTheNamedObjects(t *testing.T) {
	db := newMediaDB(t)
	srv := reassignServer(t, db)

	status, affected := postReassign(t, srv,
		`{"media_ids":["mo-1"],"source_fleet_id":"fleet-1","destination_fleet_id":"fleet-9"}`)
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	if affected["media_objects"] != 1 {
		t.Errorf("affected = %v, want media_objects 1", affected)
	}
	if got := fleetOf(t, db, "mo-1"); got != "fleet-9" {
		t.Errorf("mo-1 fleet_id = %q, want fleet-9", got)
	}
	// An object that was not named must not move.
	if got := fleetOf(t, db, "mo-2"); got != "fleet-2" {
		t.Errorf("mo-2 fleet_id = %q, want the untouched fleet-2", got)
	}
}

// FR-XFER-MEDIA-4. The count is READ BACK, not taken from RowsAffected, so a
// replay reports the same number instead of zero — which is what makes the
// compensating reverse call safe to attempt.
func TestReassign_replayIsANoOpWithTheSameCount(t *testing.T) {
	db := newMediaDB(t)
	srv := reassignServer(t, db)
	body := `{"media_ids":["mo-1"],"source_fleet_id":"fleet-1","destination_fleet_id":"fleet-9"}`

	_, first := postReassign(t, srv, body)
	_, second := postReassign(t, srv, body)
	if first["media_objects"] != second["media_objects"] {
		t.Errorf("replay reported %v, first call reported %v", second, first)
	}
	if second["media_objects"] != 1 {
		t.Errorf("replay affected = %v, want media_objects 1", second)
	}
}

// Unknown ids are ignored rather than an error, matching the tolerance the
// purge path already shows.
func TestReassign_ignoresUnknownIDs(t *testing.T) {
	db := newMediaDB(t)
	srv := reassignServer(t, db)

	status, affected := postReassign(t, srv,
		`{"media_ids":["mo-1","does-not-exist"],"source_fleet_id":"fleet-1","destination_fleet_id":"fleet-9"}`)
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	if affected["media_objects"] != 1 {
		t.Errorf("affected = %v, want media_objects 1", affected)
	}
}

// A pending-purge media object is not re-homed: it is on its way out, and
// moving it would drag it into a fleet that never had it.
func TestReassign_skipsSoftDeletedObjects(t *testing.T) {
	db := newMediaDB(t)
	if err := db.Exec(`UPDATE media.media_objects SET deleted_at = CURRENT_TIMESTAMP WHERE id = 'mo-1'`).
		Error; err != nil {
		t.Fatalf("soft delete: %v", err)
	}
	srv := reassignServer(t, db)

	_, affected := postReassign(t, srv, `{"media_ids":["mo-1"],"source_fleet_id":"fleet-1","destination_fleet_id":"fleet-9"}`)
	if affected["media_objects"] != 0 {
		t.Errorf("affected = %v, want media_objects 0", affected)
	}
	if got := fleetOf(t, db, "mo-1"); got != "fleet-1" {
		t.Errorf("mo-1 fleet_id = %q, want the untouched fleet-1", got)
	}
}

// The ownership predicate. fleet-service cannot vouch for the media ids it
// sends: POST /vehicles/{id}/media takes an arbitrary mediaId from a fleet
// member with no cross-service ownership check, so a member of fleet-1 who
// learns fleet-2's media UUID can attach it to their own vehicle and have it
// swept into an admin transfer. Without `AND fleet_id = ?` that transfer would
// hand fleet-9's members read access to fleet-2's object.
func TestReassign_refusesAnObjectOwnedByAThirdFleet(t *testing.T) {
	db := newMediaDB(t)
	srv := reassignServer(t, db)
	// mo-2 belongs to fleet-2; the transfer is fleet-1 -> fleet-9 and names it
	// anyway, exactly as a poisoned vehicle_media row would.
	status, affected := postReassign(t, srv,
		`{"media_ids":["mo-1","mo-2"],"source_fleet_id":"fleet-1","destination_fleet_id":"fleet-9"}`)
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	if got := fleetOf(t, db, "mo-2"); got != "fleet-2" {
		t.Errorf("mo-2 fleet_id = %q — a third fleet's object was moved into fleet-9", got)
	}
	if got := fleetOf(t, db, "mo-1"); got != "fleet-9" {
		t.Errorf("mo-1 fleet_id = %q, want fleet-9 — the legitimate object must still move", got)
	}
	if affected["media_objects"] != 1 {
		t.Errorf("affected = %v, want media_objects 1 (only the object fleet-1 owns)", affected)
	}
}

func TestReassign_rejectsEmptyInput(t *testing.T) {
	db := newMediaDB(t)
	srv := reassignServer(t, db)

	for _, body := range []string{
		`{"media_ids":[],"source_fleet_id":"fleet-1","destination_fleet_id":"fleet-9"}`,
		`{"media_ids":["mo-1"],"source_fleet_id":"fleet-1","destination_fleet_id":""}`,
		// A missing source fleet id is 422, not a silent no-op: without the
		// ownership predicate this endpoint would move anyone's media.
		`{"media_ids":["mo-1"],"destination_fleet_id":"fleet-9"}`,
	} {
		if status, _ := postReassign(t, srv, body); status != http.StatusUnprocessableEntity {
			t.Errorf("body %s: status = %d, want 422", body, status)
		}
	}
}

// The route must live on the /internal/* surface and nowhere else. That surface
// is the ONLY thing keeping an unauthenticated fleet-rewriting endpoint off the
// public internet: the gateway's priority-200 internal-deny rule and
// media-stripprefix both key on the /internal prefix, so a route registered
// outside it would be reachable from the outside world.
func TestReassign_isRegisteredOnlyUnderInternal(t *testing.T) {
	r := chi.NewRouter()
	admin.InitializeInternalRoutes(logrus.New(), newMediaDB(t), &recordingRemover{})(r)

	var seen []string
	err := chi.Walk(r, func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		seen = append(seen, method+" "+route)
		return nil
	})
	if err != nil {
		t.Fatalf("walk: %v", err)
	}
	if len(seen) == 0 {
		t.Fatal("walked no routes — this test would pass vacuously")
	}
	found := false
	for _, rt := range seen {
		if !strings.HasPrefix(strings.SplitN(rt, " ", 2)[1], "/internal/") {
			t.Errorf("route %q is registered outside the /internal/ surface", rt)
		}
		if rt == "POST /internal/admin/reassign-fleet" {
			found = true
		}
	}
	if !found {
		t.Errorf("POST /internal/admin/reassign-fleet is not registered; routes = %v", seen)
	}
}

// The access DECISION follows the reassign, not merely the column.
//
// The refusal is ErrNotFound, not ErrForbidden: AuthorizeAccess answers 404
// deliberately, so cross-fleet EXISTENCE is never leaked. The PRD says 403 in
// FR-XFER-MEDIA-1; the code says 404 and the code is right (design §2.3.1).
func TestReassign_flipsTheAccessDecision(t *testing.T) {
	db := newMediaDB(t)
	// The seed leaves the NOT NULL-in-production columns unset, which the
	// entity's non-pointer fields cannot scan. Fill them so the real provider
	// can read the row.
	if err := db.Exec(`UPDATE media.media_objects
	     SET uploaded_by_user_id = 'user-1', content_type = 'image/jpeg', size = 1,
	         original_filename = 'a.jpg', created_at = CURRENT_TIMESTAMP`).Error; err != nil {
		t.Fatalf("fill columns: %v", err)
	}
	srv := reassignServer(t, db)
	provider := mediaobject.NewProvider(db)

	before, err := provider.GetByID("mo-1")
	if err != nil {
		t.Fatalf("load before: %v", err)
	}
	if aerr := mediaobject.AuthorizeAccess(before, "fleet-1"); aerr != nil {
		t.Fatalf("the source fleet could not read its own object before the move: %v", aerr)
	}

	if status, _ := postReassign(t, srv,
		`{"media_ids":["mo-1"],"source_fleet_id":"fleet-1","destination_fleet_id":"fleet-9"}`); status != http.StatusOK {
		t.Fatalf("reassign status = %d", status)
	}

	after, err := provider.GetByID("mo-1")
	if err != nil {
		t.Fatalf("load after: %v", err)
	}
	if aerr := mediaobject.AuthorizeAccess(after, "fleet-9"); aerr != nil {
		t.Errorf("the destination fleet cannot read the object: %v", aerr)
	}
	if aerr := mediaobject.AuthorizeAccess(after, "fleet-1"); !errors.Is(aerr, server.ErrNotFound) {
		t.Errorf("the source fleet still has access: err = %v, want ErrNotFound", aerr)
	}
}
