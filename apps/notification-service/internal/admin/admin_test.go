package admin_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/sirupsen/logrus"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/jtumidanski/myfleet/apps/notification-service/internal/admin"
)

func newNotificationDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.Exec("ATTACH DATABASE ':memory:' AS notification").Error; err != nil {
		t.Fatalf("attach notification schema: %v", err)
	}
	ddl := []string{
		`CREATE TABLE notification.notifications (
			id TEXT PRIMARY KEY, user_id TEXT, type TEXT, title TEXT, body TEXT,
			dedupe_key TEXT, vehicle_id TEXT, fleet_id TEXT, read_at DATETIME,
			created_at DATETIME, deleted_at DATETIME, purge_operation_id TEXT)`,
		`CREATE TABLE notification.notification_preferences (
			id TEXT PRIMARY KEY, user_id TEXT, type TEXT, in_app_enabled BOOLEAN,
			deleted_at DATETIME, purge_operation_id TEXT)`,
		`CREATE TABLE notification.processed_events (event_id TEXT PRIMARY KEY, processed_at DATETIME)`,
	}
	for _, stmt := range ddl {
		if err := db.Exec(stmt).Error; err != nil {
			t.Fatalf("ddl: %v", err)
		}
	}
	// user-1 is in BOTH fleets. That is the whole point of these fixtures: R4's
	// trap is a purge that keys on user_id and takes both streams.
	seed := []string{
		`INSERT INTO notification.notifications (id, user_id, type, title, dedupe_key, fleet_id)
		 VALUES ('n1', 'user-1', 'schedule.overdue', 'A', 'dk-1', 'fleet-1')`,
		`INSERT INTO notification.notifications (id, user_id, type, title, dedupe_key, fleet_id)
		 VALUES ('n2', 'user-1', 'schedule.overdue', 'B', 'dk-2', 'fleet-2')`,
		`INSERT INTO notification.notifications (id, user_id, type, title, dedupe_key, fleet_id)
		 VALUES ('n3', 'user-1', 'account.notice', 'C', 'dk-3', '')`,
		`INSERT INTO notification.notification_preferences (id, user_id, type, in_app_enabled)
		 VALUES ('p1', 'user-1', 'schedule.overdue', 1)`,
		`INSERT INTO notification.processed_events (event_id, processed_at) VALUES ('evt-1', CURRENT_TIMESTAMP)`,
	}
	for _, stmt := range seed {
		if err := db.Exec(stmt).Error; err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	return db
}

func newNotificationRouter(t *testing.T, db *gorm.DB) chi.Router {
	t.Helper()
	r := chi.NewRouter()
	admin.InitializeInternalRoutes(logrus.New(), db)(r)
	return r
}

func post(t *testing.T, r chi.Router, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(rec, req)
	return rec
}

// risks.md R4, dissolved structurally: a fleet purge keys on fleet_id and NEVER
// on user_id, so a user in two fleets keeps the other fleet's stream.
func TestPurge_fleetScope_neverKeysOnUser(t *testing.T) {
	db := newNotificationDB(t)
	r := newNotificationRouter(t, db)

	rec := post(t, r, "/internal/admin/purge",
		`{"operation_id":"op-1","scope":"fleet","fleet_ids":["fleet-1"]}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Affected map[string]int `json:"affected"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Affected["notifications"] != 1 {
		t.Errorf("affected notifications = %d, want exactly 1", body.Affected["notifications"])
	}

	var otherFleet, accountLevel int64
	db.Raw(`SELECT count(*) FROM notification.notifications
	        WHERE id = 'n2' AND deleted_at IS NULL`).Scan(&otherFleet)
	db.Raw(`SELECT count(*) FROM notification.notifications
	        WHERE id = 'n3' AND deleted_at IS NULL`).Scan(&accountLevel)
	if otherFleet != 1 {
		t.Error("purging one fleet took the same user's OTHER fleet stream — this is R4")
	}
	if accountLevel != 1 {
		t.Error("purging a fleet took an account-level notification (empty fleet_id)")
	}
}

// design OQ-2: preferences have no fleet linkage, so they are out of fleet scope
// entirely — there is no correct predicate for them at that root.
func TestPurge_fleetScope_leavesPreferencesAlone(t *testing.T) {
	db := newNotificationDB(t)
	r := newNotificationRouter(t, db)
	post(t, r, "/internal/admin/purge", `{"operation_id":"op-1","scope":"fleet","fleet_ids":["fleet-1"]}`)

	var live int64
	db.Raw(`SELECT count(*) FROM notification.notification_preferences WHERE deleted_at IS NULL`).Scan(&live)
	if live != 1 {
		t.Errorf("a fleet purge must not touch preferences, %d of 1 live", live)
	}
}

// A system purge DOES take preferences, and takes account-level notifications
// (empty fleet_id) with them.
func TestPurge_systemScope_takesEverythingIncludingAccountLevel(t *testing.T) {
	db := newNotificationDB(t)
	r := newNotificationRouter(t, db)
	post(t, r, "/internal/admin/purge", `{"operation_id":"op-1","scope":"system"}`)

	for _, table := range []string{
		"notification.notifications", "notification.notification_preferences",
	} {
		var live int64
		db.Raw("SELECT count(*) FROM " + table + " WHERE deleted_at IS NULL").Scan(&live)
		if live != 0 {
			t.Errorf("%s has %d live rows after a system purge", table, live)
		}
	}
}

func TestRestoreAndReap(t *testing.T) {
	db := newNotificationDB(t)
	r := newNotificationRouter(t, db)
	post(t, r, "/internal/admin/purge", `{"operation_id":"op-1","scope":"fleet","fleet_ids":["fleet-1"]}`)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodDelete, "/internal/admin/purge/op-1", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("restore status = %d: %s", rec.Code, rec.Body.String())
	}
	var live int64
	db.Raw(`SELECT count(*) FROM notification.notifications WHERE deleted_at IS NULL`).Scan(&live)
	if live != 3 {
		t.Errorf("restore returned %d of 3 notifications", live)
	}

	post(t, r, "/internal/admin/purge", `{"operation_id":"op-1","scope":"fleet","fleet_ids":["fleet-1"]}`)
	if rec := post(t, r, "/internal/admin/reap/op-1", ""); rec.Code != http.StatusOK {
		t.Fatalf("reap status = %d: %s", rec.Code, rec.Body.String())
	}
	var rows int64
	db.Raw(`SELECT count(*) FROM notification.notifications WHERE id = 'n1'`).Scan(&rows)
	if rows != 0 {
		t.Errorf("reap left the stamped notification behind")
	}
	// Idempotent.
	if rec := post(t, r, "/internal/admin/reap/op-1", ""); rec.Code != http.StatusOK {
		t.Errorf("a second reap must succeed, got %d", rec.Code)
	}
}

// design §3.3: truncating the idempotency ledger lets a Kafka replay regenerate
// notifications for data that was just purged — a system purge that undoes
// itself on the next consumer restart.
func TestSystemPurge_leavesProcessedEventsAlone(t *testing.T) {
	db := newNotificationDB(t)
	r := newNotificationRouter(t, db)
	post(t, r, "/internal/admin/purge", `{"operation_id":"op-1","scope":"system"}`)
	post(t, r, "/internal/admin/reap/op-1", "")

	var rows int64
	db.Raw(`SELECT count(*) FROM notification.processed_events`).Scan(&rows)
	if rows != 1 {
		t.Errorf("processed_events must survive a system purge, got %d rows", rows)
	}
}

func TestStats_countsLiveNotifications(t *testing.T) {
	db := newNotificationDB(t)
	r := newNotificationRouter(t, db)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/internal/admin/stats", nil))
	var body struct {
		Notifications int `json:"notifications"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Notifications != 3 {
		t.Errorf("notifications = %d, want 3", body.Notifications)
	}
}
