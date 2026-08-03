package admin_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/sirupsen/logrus"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/jtumidanski/myfleet/apps/media-service/internal/admin"
)

// recordingRemover captures the MinIO keys a reap asks to delete, and can be
// told to fail for one of them.
type recordingRemover struct {
	removed []string
	fail    map[string]bool
}

func (r *recordingRemover) RemoveObject(_ context.Context, key string) error {
	if r.fail[key] {
		return context.DeadlineExceeded
	}
	r.removed = append(r.removed, key)
	return nil
}

func newMediaDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.Exec("ATTACH DATABASE ':memory:' AS media").Error; err != nil {
		t.Fatalf("attach media schema: %v", err)
	}
	ddl := []string{
		`CREATE TABLE media.media_objects (
			id TEXT PRIMARY KEY, fleet_id TEXT, uploaded_by_user_id TEXT, bucket TEXT,
			object_key TEXT, content_type TEXT, size INTEGER, original_filename TEXT,
			status TEXT, created_at DATETIME, deleted_at DATETIME, purge_after DATETIME,
			purge_operation_id TEXT)`,
		`CREATE TABLE media.media_variants (
			id TEXT PRIMARY KEY, media_object_id TEXT, variant TEXT, object_key TEXT,
			width INTEGER, height INTEGER, content_type TEXT, created_at DATETIME,
			deleted_at DATETIME, purge_operation_id TEXT)`,
		`CREATE TABLE media.media_variant_failures (
			media_object_id TEXT, variant TEXT, reason TEXT, failed_at DATETIME,
			deleted_at DATETIME, purge_operation_id TEXT,
			PRIMARY KEY (media_object_id, variant))`,
		`CREATE TABLE media.processed_events (event_id TEXT PRIMARY KEY, processed_at DATETIME)`,
	}
	for _, stmt := range ddl {
		if err := db.Exec(stmt).Error; err != nil {
			t.Fatalf("ddl: %v", err)
		}
	}
	seed := []string{
		`INSERT INTO media.media_objects (id, fleet_id, bucket, object_key, status)
		 VALUES ('mo-1', 'fleet-1', 'media', 'k/mo-1', 'ready')`,
		`INSERT INTO media.media_objects (id, fleet_id, bucket, object_key, status)
		 VALUES ('mo-2', 'fleet-2', 'media', 'k/mo-2', 'ready')`,
		`INSERT INTO media.media_variants (id, media_object_id, variant, object_key)
		 VALUES ('mv-1', 'mo-1', 'thumb', 'k/mo-1-thumb')`,
		`INSERT INTO media.media_variants (id, media_object_id, variant, object_key)
		 VALUES ('mv-2', 'mo-2', 'thumb', 'k/mo-2-thumb')`,
		`INSERT INTO media.media_variant_failures (media_object_id, variant, reason, failed_at)
		 VALUES ('mo-1', 'card', 'undecodable', CURRENT_TIMESTAMP)`,
		`INSERT INTO media.media_variant_failures (media_object_id, variant, reason, failed_at)
		 VALUES ('mo-2', 'card', 'undecodable', CURRENT_TIMESTAMP)`,
		`INSERT INTO media.processed_events (event_id, processed_at)
		 VALUES ('evt-1', CURRENT_TIMESTAMP)`,
	}
	for _, stmt := range seed {
		if err := db.Exec(stmt).Error; err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	return db
}

func newMediaRouter(t *testing.T, db *gorm.DB, store admin.ObjectRemover) chi.Router {
	t.Helper()
	r := chi.NewRouter()
	admin.InitializeInternalRoutes(logrus.New(), db, store)(r)
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

// design OQ-1: a fleet-scoped media purge is WHERE fleet_id = ?. No id-set
// passing, and no way to reach another tenant's media.
func TestPurge_fleetScope_takesOnlyThatFleet(t *testing.T) {
	db := newMediaDB(t)
	r := newMediaRouter(t, db, &recordingRemover{})

	rec := post(t, r, "/internal/admin/purge",
		`{"operation_id":"op-1","scope":"fleet","fleet_ids":["fleet-1"]}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Affected map[string]int `json:"affected"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Affected["media_objects"] != 1 || body.Affected["media_variants"] != 1 {
		t.Errorf("affected = %+v, want one object and one variant", body.Affected)
	}

	var liveOther int64
	db.Raw(`SELECT count(*) FROM media.media_objects
	        WHERE fleet_id = 'fleet-2' AND deleted_at IS NULL`).Scan(&liveOther)
	if liveOther != 1 {
		t.Errorf("a fleet purge reached another tenant's media: %d of 1 live", liveOther)
	}
}

// FR-ADMIN-PURGE-10: a replay is a no-op that returns the SAME counts.
func TestPurge_isIdempotent(t *testing.T) {
	db := newMediaDB(t)
	r := newMediaRouter(t, db, &recordingRemover{})
	const body = `{"operation_id":"op-1","scope":"fleet","fleet_ids":["fleet-1"]}`

	decode := func(rec *httptest.ResponseRecorder) map[string]int {
		t.Helper()
		var out struct {
			Affected map[string]int `json:"affected"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatalf("decode: %v", err)
		}
		return out.Affected
	}
	first := decode(post(t, r, "/internal/admin/purge", body))
	second := decode(post(t, r, "/internal/admin/purge", body))
	for k, want := range first {
		if second[k] != want {
			t.Errorf("%s: replay returned %d, first call %d", k, second[k], want)
		}
	}
}

// The record-scope path: fleet-service names the media objects belonging to one
// vehicle, and only those (plus their variants) are taken.
func TestPurge_mediaIDsScope(t *testing.T) {
	db := newMediaDB(t)
	r := newMediaRouter(t, db, &recordingRemover{})

	rec := post(t, r, "/internal/admin/purge",
		`{"operation_id":"op-1","scope":"media_ids","media_ids":["mo-1"]}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	var live int64
	db.Raw(`SELECT count(*) FROM media.media_variants WHERE deleted_at IS NULL`).Scan(&live)
	if live != 1 {
		t.Errorf("want mo-2's variant still live, got %d live variants", live)
	}
}

func TestRestore_returnsEverythingTheOperationTook(t *testing.T) {
	db := newMediaDB(t)
	r := newMediaRouter(t, db, &recordingRemover{})
	post(t, r, "/internal/admin/purge", `{"operation_id":"op-1","scope":"system"}`)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodDelete, "/internal/admin/purge/op-1", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	var live int64
	db.Raw(`SELECT count(*) FROM media.media_objects WHERE deleted_at IS NULL`).Scan(&live)
	if live != 2 {
		t.Errorf("restore returned %d of 2 media objects", live)
	}
}

// FR-ADMIN-RESTORE-5: reap removes the MinIO objects too — the media object's
// key AND every variant's key.
func TestReap_removesRowsAndObjects(t *testing.T) {
	db := newMediaDB(t)
	store := &recordingRemover{}
	r := newMediaRouter(t, db, store)
	post(t, r, "/internal/admin/purge", `{"operation_id":"op-1","scope":"fleet","fleet_ids":["fleet-1"]}`)

	rec := post(t, r, "/internal/admin/reap/op-1", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	want := map[string]bool{"k/mo-1": true, "k/mo-1-thumb": true}
	for _, key := range store.removed {
		delete(want, key)
	}
	if len(want) != 0 {
		t.Errorf("reap did not remove these MinIO objects: %v (removed %v)", want, store.removed)
	}
	var rows int64
	db.Raw(`SELECT count(*) FROM media.media_objects WHERE purge_operation_id = 'op-1'`).Scan(&rows)
	if rows != 0 {
		t.Errorf("reap left %d stamped rows behind", rows)
	}
}

// A MinIO object that cannot be removed must leave its ROW in place, so the
// next tick retries. Deleting the row would strand the object forever with
// nothing left pointing at it.
func TestReap_keepsRowsWhoseObjectCouldNotBeRemoved(t *testing.T) {
	db := newMediaDB(t)
	store := &recordingRemover{fail: map[string]bool{"k/mo-1": true}}
	r := newMediaRouter(t, db, store)
	post(t, r, "/internal/admin/purge", `{"operation_id":"op-1","scope":"fleet","fleet_ids":["fleet-1"]}`)

	rec := post(t, r, "/internal/admin/reap/op-1", "")
	if rec.Code == http.StatusOK {
		t.Errorf("a failed object removal must not report success, got %d", rec.Code)
	}
	var rows int64
	db.Raw(`SELECT count(*) FROM media.media_objects WHERE id = 'mo-1'`).Scan(&rows)
	if rows != 1 {
		t.Errorf("the row whose object survived must be kept for the next tick, got %d rows", rows)
	}
}

// design §3.3: deleting the idempotency ledger would let a Kafka replay
// regenerate variants for media that was just purged.
func TestSystemPurge_leavesProcessedEventsAlone(t *testing.T) {
	db := newMediaDB(t)
	r := newMediaRouter(t, db, &recordingRemover{})
	post(t, r, "/internal/admin/purge", `{"operation_id":"op-1","scope":"system"}`)
	post(t, r, "/internal/admin/reap/op-1", "")

	var rows int64
	db.Raw(`SELECT count(*) FROM media.processed_events`).Scan(&rows)
	if rows != 1 {
		t.Errorf("processed_events must survive a system purge, got %d rows", rows)
	}
}

func TestStats_countsLiveMediaObjects(t *testing.T) {
	db := newMediaDB(t)
	r := newMediaRouter(t, db, &recordingRemover{})
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/internal/admin/stats", nil))
	var body struct {
		MediaObjects int `json:"media_objects"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.MediaObjects != 2 {
		t.Errorf("media_objects = %d, want 2", body.MediaObjects)
	}
}

// A row spared because its bytes could not be removed must remain REACHABLE by
// the next attempt.
//
// The first implementation spared it by NULLing purge_operation_id — and then
// keyed the retry off that same column, so the row became unreachable by
// restore, by reap and by the purge_after sweep: soft-deleted, orphaned, and
// its bytes stranded in the bucket forever. Sparing a row and forgetting which
// operation owns it are not the same thing.
func TestReap_sparedRowKeepsItsOperationIDSoTheNextTickCanRetry(t *testing.T) {
	db := newMediaDB(t)
	store := &recordingRemover{fail: map[string]bool{"k/mo-1": true}}
	r := newMediaRouter(t, db, store)
	post(t, r, "/internal/admin/purge", `{"operation_id":"op-1","scope":"fleet","fleet_ids":["fleet-1"]}`)

	if rec := post(t, r, "/internal/admin/reap/op-1", ""); rec.Code == http.StatusOK {
		t.Fatalf("a failed object removal must not report success, got %d", rec.Code)
	}

	var stamped int64
	db.Raw(`SELECT count(*) FROM media.media_objects
	        WHERE id = 'mo-1' AND purge_operation_id = 'op-1'`).Scan(&stamped)
	if stamped != 1 {
		t.Fatal("the spared row lost its purge_operation_id and is now unreachable by any path")
	}

	// The bucket recovers; the next tick must finish the job.
	store.fail = nil
	if rec := post(t, r, "/internal/admin/reap/op-1", ""); rec.Code != http.StatusOK {
		t.Fatalf("the retry tick must succeed once the object can be removed, got %d", rec.Code)
	}
	var removed bool
	for _, k := range store.removed {
		if k == "k/mo-1" {
			removed = true
		}
	}
	if !removed {
		t.Error("the previously-failed object was never removed on retry")
	}
	var rows int64
	db.Raw(`SELECT count(*) FROM media.media_objects WHERE id = 'mo-1'`).Scan(&rows)
	if rows != 0 {
		t.Error("the row survived a successful retry")
	}
}

// FR-ADMIN-1/2. The ledger joins the manifest so an admin reap does not leave
// its rows orphaned — the same defect the hourly sweep has, on a different
// route. The full stamp -> restore -> reap round-trip, because each phase keys
// on a different column and any one of them can be the thing that is missing.
func TestPurge_reachesTheVariantFailureLedger(t *testing.T) {
	db := newMediaDB(t)
	r := newMediaRouter(t, db, &recordingRemover{})
	const scope = `{"operation_id":"op-1","scope":"fleet","fleet_ids":["fleet-1"]}`

	rec := post(t, r, "/internal/admin/purge", scope)
	if rec.Code != http.StatusOK {
		t.Fatalf("stamp status = %d: %s", rec.Code, rec.Body.String())
	}
	var stamped struct {
		Affected map[string]int `json:"affected"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &stamped); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if stamped.Affected["media_variant_failures"] != 1 {
		t.Errorf("affected = %+v, want one media_variant_failures row", stamped.Affected)
	}

	// Restore must bring it back, or a cancelled purge leaves the ledger
	// soft-deleted forever and lazy generation silently re-enables.
	restore := httptest.NewRecorder()
	r.ServeHTTP(restore, httptest.NewRequest(http.MethodDelete, "/internal/admin/purge/op-1", nil))
	if restore.Code != http.StatusOK {
		t.Fatalf("restore status = %d: %s", restore.Code, restore.Body.String())
	}
	var live int64
	db.Raw(`SELECT count(*) FROM media.media_variant_failures
	        WHERE media_object_id = 'mo-1' AND deleted_at IS NULL`).Scan(&live)
	if live != 1 {
		t.Errorf("restore left %d of 1 ledger rows live", live)
	}

	// And a reap must actually remove it.
	post(t, r, "/internal/admin/purge", scope)
	if rec := post(t, r, "/internal/admin/reap/op-1", ""); rec.Code != http.StatusOK {
		t.Fatalf("reap status = %d: %s", rec.Code, rec.Body.String())
	}
	var left int64
	db.Raw(`SELECT count(*) FROM media.media_variant_failures WHERE media_object_id = 'mo-1'`).Scan(&left)
	if left != 0 {
		t.Errorf("reap left %d ledger rows behind for the purged media object", left)
	}
	// The other tenant's ledger row is untouched.
	db.Raw(`SELECT count(*) FROM media.media_variant_failures WHERE media_object_id = 'mo-2'`).Scan(&left)
	if left != 1 {
		t.Errorf("the purge reached another tenant's ledger rows: %d of 1 left", left)
	}
}

// A-1. A media object spared because its bytes could not be removed keeps its
// row and its variant rows for the retry — its LEDGER rows must be spared with
// them, or the operation is left half-applied and the next tick reaps a media
// object whose failure ledger has already been thrown away.
func TestReap_sparedObjectKeepsItsFailureLedgerRows(t *testing.T) {
	db := newMediaDB(t)
	store := &recordingRemover{fail: map[string]bool{"k/mo-1": true}}
	r := newMediaRouter(t, db, store)
	post(t, r, "/internal/admin/purge", `{"operation_id":"op-1","scope":"fleet","fleet_ids":["fleet-1"]}`)

	if rec := post(t, r, "/internal/admin/reap/op-1", ""); rec.Code == http.StatusOK {
		t.Fatalf("a failed object removal must not report success, got %d", rec.Code)
	}

	var spared int64
	db.Raw(`SELECT count(*) FROM media.media_variant_failures
	        WHERE media_object_id = 'mo-1' AND purge_operation_id = 'op-1'`).Scan(&spared)
	if spared != 1 {
		t.Fatalf("the spared media object's ledger rows were reaped anyway: %d of 1 left", spared)
	}
}
