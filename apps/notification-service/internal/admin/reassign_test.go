package admin_test

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

	"github.com/jtumidanski/myfleet/apps/notification-service/internal/admin"
)

// seedVehicleNotifications adds rows carrying a vehicle_id; newNotificationDB's
// own fixtures deliberately have none, because a purge keys on fleet_id.
func seedVehicleNotifications(t *testing.T, db *gorm.DB) {
	t.Helper()
	stmts := []string{
		`INSERT INTO notification.notifications
		 (id, user_id, type, title, dedupe_key, vehicle_id, fleet_id)
		 VALUES ('nv1', 'user-1', 'schedule.overdue', 'D', 'dk-4', 'veh-1', 'fleet-1')`,
		`INSERT INTO notification.notifications
		 (id, user_id, type, title, dedupe_key, vehicle_id, fleet_id)
		 VALUES ('nv2', 'user-2', 'schedule.overdue', 'E', 'dk-5', 'veh-1', 'fleet-1')`,
		`INSERT INTO notification.notifications
		 (id, user_id, type, title, dedupe_key, vehicle_id, fleet_id)
		 VALUES ('nv3', 'user-1', 'schedule.overdue', 'F', 'dk-6', 'veh-2', 'fleet-1')`,
	}
	for _, s := range stmts {
		if err := db.Exec(s).Error; err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
}

func reassignSrv(t *testing.T, db *gorm.DB) *httptest.Server {
	t.Helper()
	r := chi.NewRouter()
	log := logrus.New()
	log.SetOutput(io.Discard)
	admin.InitializeInternalRoutes(log, db)(r)
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)
	return srv
}

// postReassign, not post: admin_test.go already declares a post helper with a
// different signature, and this package would not compile with two of them.
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

func fleetOfNotification(t *testing.T, db *gorm.DB, id string) string {
	t.Helper()
	var out []string
	if err := db.Raw(`SELECT fleet_id FROM notification.notifications WHERE id = ?`, id).
		Scan(&out).Error; err != nil {
		t.Fatalf("read fleet_id: %v", err)
	}
	if len(out) == 0 {
		t.Fatalf("no notification %s", id)
	}
	return out[0]
}

func TestReassign_repointsNotificationsForTheVehicle(t *testing.T) {
	db := newNotificationDB(t)
	seedVehicleNotifications(t, db)
	srv := reassignSrv(t, db)

	status, affected := postReassign(t, srv, `{"vehicle_ids":["veh-1"],"destination_fleet_id":"fleet-9"}`)
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	if affected["notifications"] != 2 {
		t.Errorf("affected = %v, want notifications 2", affected)
	}
	for _, id := range []string{"nv1", "nv2"} {
		if got := fleetOfNotification(t, db, id); got != "fleet-9" {
			t.Errorf("%s fleet_id = %q, want fleet-9", id, got)
		}
	}
	// A different vehicle's notification stays put.
	if got := fleetOfNotification(t, db, "nv3"); got != "fleet-1" {
		t.Errorf("nv3 fleet_id = %q, want the untouched fleet-1", got)
	}
	// So does a notification with no vehicle at all.
	if got := fleetOfNotification(t, db, "n1"); got != "fleet-1" {
		t.Errorf("n1 fleet_id = %q, want the untouched fleet-1", got)
	}
}

// The count is READ BACK, not taken from RowsAffected, so a replay reports the
// same nonzero number rather than the zero rows a second UPDATE touches. That
// is what lets fleet-service tell "already done" from "did nothing".
func TestReassign_replayIsANoOpWithTheSameCount(t *testing.T) {
	db := newNotificationDB(t)
	seedVehicleNotifications(t, db)
	srv := reassignSrv(t, db)
	body := `{"vehicle_ids":["veh-1"],"destination_fleet_id":"fleet-9"}`

	_, first := postReassign(t, srv, body)
	_, second := postReassign(t, srv, body)
	if first["notifications"] != second["notifications"] || second["notifications"] != 2 {
		t.Errorf("first %v, replay %v; both must report notifications 2", first, second)
	}
}

func TestReassign_ignoresUnknownVehicleIDs(t *testing.T) {
	db := newNotificationDB(t)
	seedVehicleNotifications(t, db)
	srv := reassignSrv(t, db)

	status, affected := postReassign(t, srv,
		`{"vehicle_ids":["veh-1","nope"],"destination_fleet_id":"fleet-9"}`)
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	if affected["notifications"] != 2 {
		t.Errorf("affected = %v, want notifications 2", affected)
	}
}

// A notification already stamped for purge is on its way out and must not be
// dragged into a fleet that never had it.
func TestReassign_skipsSoftDeletedNotifications(t *testing.T) {
	db := newNotificationDB(t)
	seedVehicleNotifications(t, db)
	if err := db.Exec(`UPDATE notification.notifications
	     SET deleted_at = CURRENT_TIMESTAMP WHERE id = 'nv1'`).Error; err != nil {
		t.Fatalf("soft delete: %v", err)
	}
	srv := reassignSrv(t, db)

	_, affected := postReassign(t, srv, `{"vehicle_ids":["veh-1"],"destination_fleet_id":"fleet-9"}`)
	if affected["notifications"] != 1 {
		t.Errorf("affected = %v, want notifications 1 — only the live row", affected)
	}
	if got := fleetOfNotification(t, db, "nv1"); got != "fleet-1" {
		t.Errorf("nv1 fleet_id = %q, want the untouched fleet-1", got)
	}
	if got := fleetOfNotification(t, db, "nv2"); got != "fleet-9" {
		t.Errorf("nv2 fleet_id = %q, want fleet-9", got)
	}
}

func TestReassign_rejectsEmptyInput(t *testing.T) {
	db := newNotificationDB(t)
	srv := reassignSrv(t, db)

	for _, body := range []string{
		`{"vehicle_ids":[],"destination_fleet_id":"fleet-9"}`,
		`{"vehicle_ids":["veh-1"],"destination_fleet_id":""}`,
	} {
		if status, _ := postReassign(t, srv, body); status != http.StatusUnprocessableEntity {
			t.Errorf("body %s: status = %d, want 422", body, status)
		}
	}
}

// The route must live on the /internal/* surface and nowhere else. This service
// is the one that is not safe by accident: notifications-stripprefix strips the
// FULL /api/notifications prefix, so the gateway's priority-200 internal-deny
// rule matching ^/+api/+notifications[^/]*/*internal is the ONLY thing keeping
// an unauthenticated fleet-rewriting endpoint off the public internet.
func TestReassign_isRegisteredOnlyUnderInternal(t *testing.T) {
	r := chi.NewRouter()
	log := logrus.New()
	log.SetOutput(io.Discard)
	admin.InitializeInternalRoutes(log, newNotificationDB(t))(r)

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
