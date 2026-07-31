package mediaobject

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	dtoevents "github.com/jtumidanski/myfleet/packages/dto-go/events"
	sharedevents "github.com/jtumidanski/myfleet/packages/shared-go/events"
	"github.com/jtumidanski/myfleet/packages/shared-go/server"

	"github.com/jtumidanski/myfleet/apps/media-service/internal/storage"
)

func TestAuthorizeAccess_404CrossFleet(t *testing.T) {
	obj, err := NewBuilder().
		SetFleetID("fleet-a").
		SetUploadedByUserID("u1").
		SetBucket("media").
		SetObjectKey("fleet-a/o1/file.jpg").
		SetContentType("image/jpeg").
		SetOriginalFilename("file.jpg").
		Build()
	if err != nil {
		t.Fatalf("build: %v", err)
	}

	// Same fleet → allowed.
	if err := AuthorizeAccess(obj, "fleet-a"); err != nil {
		t.Fatalf("same-fleet access must succeed, got %v", err)
	}

	// Different fleet → 404 (never leak existence).
	if err := AuthorizeAccess(obj, "fleet-b"); !errors.Is(err, server.ErrNotFound) {
		t.Fatalf("cross-fleet access must be 404, got %v", err)
	}
}

func TestMarkReady_requiresProcessing(t *testing.T) {
	base := NewBuilder().
		SetFleetID("f1").
		SetUploadedByUserID("u1").
		SetBucket("media").
		SetObjectKey("f1/o1/file.jpg").
		SetContentType("image/jpeg").
		SetOriginalFilename("file.jpg")

	// ready is only valid FROM processing.
	processing, _ := base.Build()
	processing = processing.WithStatus(StatusProcessing)
	got, err := MarkReady(processing)
	if err != nil {
		t.Fatalf("processing→ready must be allowed, got %v", err)
	}
	if got.Status() != StatusReady {
		t.Fatalf("expected status ready, got %q", got.Status())
	}

	// uploaded → ready is invalid (must go through processing first).
	uploaded, _ := base.Build() // defaults to uploaded
	if _, err := MarkReady(uploaded); !errors.Is(err, server.ErrConflict) {
		t.Fatalf("uploaded→ready must be a conflict, got %v", err)
	}
}

func TestMarkProcessing_requiresUploaded(t *testing.T) {
	base := NewBuilder().
		SetFleetID("f1").
		SetUploadedByUserID("u1").
		SetBucket("media").
		SetObjectKey("f1/o1/file.jpg").
		SetContentType("image/jpeg").
		SetOriginalFilename("file.jpg")

	uploaded, _ := base.Build()
	got, err := MarkProcessing(uploaded)
	if err != nil {
		t.Fatalf("uploaded→processing must be allowed, got %v", err)
	}
	if got.Status() != StatusProcessing {
		t.Fatalf("expected status processing, got %q", got.Status())
	}

	// processing → processing is invalid.
	already, _ := base.Build()
	already = already.WithStatus(StatusProcessing)
	if _, err := MarkProcessing(already); !errors.Is(err, server.ErrConflict) {
		t.Fatalf("processing→processing must be a conflict, got %v", err)
	}
}

// newConfirmTestDB opens an in-memory SQLite database with the media schema
// attached, creates media_objects and the outbox table, and returns the handle.
//
// GORM AutoMigrate with schema-qualified table names (media.media_objects) fails
// for SQLite when the entity has index tags because SQLite's index-existence
// check uses "main.<table>" even after ATTACH. We create the table with raw SQL
// instead to keep the test portable.
func newConfirmTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	// Attach a second in-memory database as "media" so the schema-qualified table
	// name "media.media_objects" resolves correctly.
	if err := db.Exec("ATTACH DATABASE ':memory:' AS media").Error; err != nil {
		t.Fatalf("attach media schema: %v", err)
	}
	// Create the media_objects table directly (bypassing AutoMigrate which
	// mishandles schema-qualified index creation in SQLite).
	if err := db.Exec(`CREATE TABLE media.media_objects (
		id                  TEXT PRIMARY KEY,
		fleet_id            TEXT NOT NULL,
		uploaded_by_user_id TEXT NOT NULL,
		bucket              TEXT NOT NULL,
		object_key          TEXT NOT NULL,
		content_type        TEXT,
		size                INTEGER,
		original_filename   TEXT,
		status              TEXT NOT NULL,
		created_at          DATETIME,
		deleted_at          DATETIME,
		purge_after         DATETIME
	)`).Error; err != nil {
		t.Fatalf("create media_objects: %v", err)
	}
	if err := sharedevents.MigrateOutbox(db); err != nil {
		t.Fatalf("migrate outbox: %v", err)
	}
	return db
}

// TestConfirm_enqueuesOutboxAtomically verifies that Administrator.UpdateInTx
// writes both the status transition (uploaded→processing) and the media.uploaded
// outbox row in a single transaction (design A8). After a successful Confirm:
//   - exactly one unsent outbox row of type "media.uploaded" exists
//   - the row's decoded data contains the correct media_id
//   - the media object row is in the "processing" state
func TestConfirm_enqueuesOutboxAtomically(t *testing.T) {
	db := newConfirmTestDB(t)

	// Persist a media object in the uploaded state.
	mediaID := uuid.NewString()
	obj, err := NewBuilder().
		SetID(mediaID).
		SetFleetID("fleet-1").
		SetUploadedByUserID("user-1").
		SetBucket("media-bucket").
		SetObjectKey("fleet-1/" + mediaID + "/photo.jpg").
		SetContentType("image/jpeg").
		SetOriginalFilename("photo.jpg").
		SetStatus(StatusUploaded).
		Build()
	if err != nil {
		t.Fatalf("build object: %v", err)
	}
	admin := NewAdministrator(db)
	obj, err = admin.Insert(obj)
	if err != nil {
		t.Fatalf("insert object: %v", err)
	}

	// Build the envelope (mirrors what Processor.Confirm does).
	processing := obj.WithStatus(StatusProcessing)
	env := sharedevents.Envelope{
		EventID:     uuid.NewString(),
		Type:        EventTypeMediaUploaded,
		Version:     1,
		OccurredAt:  time.Now().UTC(),
		FleetID:     processing.FleetID(),
		ActorUserID: processing.UploadedByUserID(),
		Data: mustData(dtoevents.MediaUploadedData{
			MediaID:     processing.ID(),
			ContentType: processing.ContentType(),
		}),
	}

	// Run the transactional update + enqueue.
	updated, err := admin.UpdateInTx(processing, func(tx *gorm.DB) error {
		return sharedevents.Enqueue(tx, env)
	})
	if err != nil {
		t.Fatalf("UpdateInTx: %v", err)
	}

	// 1. The returned model must be in processing state.
	if updated.Status() != StatusProcessing {
		t.Fatalf("model status: want processing, got %q", updated.Status())
	}

	// 2. The DB row must also reflect processing state.
	provider := NewProvider(db)
	persisted, err := provider.GetByID(mediaID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if persisted.Status() != StatusProcessing {
		t.Fatalf("persisted status: want processing, got %q", persisted.Status())
	}

	// 3. Exactly one unsent outbox row of type "media.uploaded" must exist.
	var rows []sharedevents.OutboxRow
	if err := db.Where("sent_at IS NULL").Find(&rows).Error; err != nil {
		t.Fatalf("read outbox: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("want exactly 1 outbox row, got %d", len(rows))
	}
	row := rows[0]
	if row.Type != EventTypeMediaUploaded {
		t.Fatalf("outbox row type: want %q, got %q", EventTypeMediaUploaded, row.Type)
	}
	if row.SentAt != nil {
		t.Fatalf("sent_at must be NULL on enqueue, got %v", row.SentAt)
	}

	// 4. The row's payload must decode to an envelope whose Data contains the
	//    correct media_id (design A8).
	var decoded sharedevents.Envelope
	if err := json.Unmarshal(row.Payload, &decoded); err != nil {
		t.Fatalf("unmarshal envelope: %v", err)
	}
	dataBytes, _ := json.Marshal(decoded.Data)
	var data dtoevents.MediaUploadedData
	if err := json.Unmarshal(dataBytes, &data); err != nil {
		t.Fatalf("unmarshal data: %v", err)
	}
	if data.MediaID != mediaID {
		t.Fatalf("data.MediaID: want %q, got %q", mediaID, data.MediaID)
	}
	if data.ContentType != "image/jpeg" {
		t.Fatalf("data.ContentType: want image/jpeg, got %q", data.ContentType)
	}

	// 5. The fleet_id on the envelope must match.
	if decoded.FleetID != "fleet-1" {
		t.Fatalf("envelope FleetID: want fleet-1, got %q", decoded.FleetID)
	}
}

// TestConfirm_outboxRollsBackOnEnqueueError verifies that if Enqueue returns an
// error the status update is also rolled back — the object stays in uploaded.
func TestConfirm_outboxRollsBackOnEnqueueError(t *testing.T) {
	db := newConfirmTestDB(t)

	mediaID := uuid.NewString()
	obj, err := NewBuilder().
		SetID(mediaID).
		SetFleetID("fleet-2").
		SetUploadedByUserID("user-2").
		SetBucket("media-bucket").
		SetObjectKey("fleet-2/" + mediaID + "/file.png").
		SetContentType("image/png").
		SetOriginalFilename("file.png").
		SetStatus(StatusUploaded).
		Build()
	if err != nil {
		t.Fatalf("build object: %v", err)
	}
	admin := NewAdministrator(db)
	obj, err = admin.Insert(obj)
	if err != nil {
		t.Fatalf("insert object: %v", err)
	}

	processing := obj.WithStatus(StatusProcessing)
	injectErr := errors.New("simulated enqueue failure")

	// UpdateInTx must propagate the hook error and roll back.
	_, txErr := admin.UpdateInTx(processing, func(_ *gorm.DB) error {
		return injectErr
	})
	if !errors.Is(txErr, injectErr) {
		t.Fatalf("UpdateInTx must return hook error, got: %v", txErr)
	}

	// The row must remain in uploaded state (rollback).
	provider := NewProvider(db)
	persisted, err := provider.GetByID(mediaID)
	if err != nil {
		t.Fatalf("GetByID after rollback: %v", err)
	}
	if persisted.Status() != StatusUploaded {
		t.Fatalf("after rollback status must be uploaded, got %q", persisted.Status())
	}

	// No outbox rows must exist.
	var rows []sharedevents.OutboxRow
	if err := db.Find(&rows).Error; err != nil {
		t.Fatalf("read outbox: %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("after rollback want 0 outbox rows, got %d", len(rows))
	}
}

// fakeStore records what was written so the proxy path can be asserted without
// a live MinIO.
type fakeStore struct {
	bucket   string
	putCalls int
	putKey   string
	putBody  []byte
	putSize  int64
	putCT    string
	putErr   error

	getKey  string
	getBody []byte
	getErr  error
}

func (f *fakeStore) Bucket() string { return f.bucket }

func (f *fakeStore) PutObject(ctx context.Context, key string, r io.Reader, size int64, contentType string) error {
	f.putCalls++
	if f.putErr != nil {
		return f.putErr
	}
	b, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	f.putKey, f.putBody, f.putSize, f.putCT = key, b, size, contentType
	return nil
}

func (f *fakeStore) GetObject(ctx context.Context, key string) (io.ReadCloser, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	f.getKey = key
	return io.NopCloser(bytes.NewReader(f.getBody)), nil
}

func TestStoreContent_streamsToObjectStoreAndRecordsSize(t *testing.T) {
	db := newConfirmTestDB(t)
	store := &fakeStore{bucket: "myfleet-media"}
	pr := NewProcessor(logrus.New(), NewProvider(db), NewAdministrator(db), store)

	created, err := pr.InitUpload("fleet-a", "u1", "image/jpeg", "photo.jpg")
	if err != nil {
		t.Fatalf("init upload: %v", err)
	}

	body := []byte("jpeg-bytes")
	updated, err := pr.StoreContent(context.Background(), created.ID(), "fleet-a", bytes.NewReader(body), int64(len(body)))
	if err != nil {
		t.Fatalf("store content: %v", err)
	}

	if store.putKey != created.ObjectKey() {
		t.Fatalf("wrote to key %q, want %q", store.putKey, created.ObjectKey())
	}
	if string(store.putBody) != string(body) {
		t.Fatalf("wrote body %q, want %q", store.putBody, body)
	}
	// The content type must come from the row created at init, not the request.
	if store.putCT != "image/jpeg" {
		t.Fatalf("wrote content-type %q, want image/jpeg", store.putCT)
	}
	if updated.Size() != int64(len(body)) {
		t.Fatalf("size = %d, want %d", updated.Size(), len(body))
	}
	if updated.Status() != StatusUploaded {
		t.Fatalf("status = %q, want uploaded (confirm does the transition)", updated.Status())
	}
}

func TestStoreContent_crossFleetIs404(t *testing.T) {
	db := newConfirmTestDB(t)
	store := &fakeStore{bucket: "myfleet-media"}
	pr := NewProcessor(logrus.New(), NewProvider(db), NewAdministrator(db), store)

	created, err := pr.InitUpload("fleet-a", "u1", "image/jpeg", "photo.jpg")
	if err != nil {
		t.Fatalf("init upload: %v", err)
	}

	_, err = pr.StoreContent(context.Background(), created.ID(), "fleet-b", bytes.NewReader([]byte("x")), 1)
	if !errors.Is(err, server.ErrNotFound) {
		t.Fatalf("cross-fleet write must be 404, got %v", err)
	}
	if store.putKey != "" {
		t.Fatalf("cross-fleet write must not touch storage, wrote key %q", store.putKey)
	}
}

// TestStoreContent_storeFailurePropagatesAndLeavesSizeUntouched verifies that
// when the object store fails mid-write, StoreContent returns that error and
// never persists a size — a failed upload must leave the row consistent
// (still zero, not partially/incorrectly updated).
func TestStoreContent_storeFailurePropagatesAndLeavesSizeUntouched(t *testing.T) {
	db := newConfirmTestDB(t)
	injectErr := errors.New("simulated minio put failure")
	store := &fakeStore{bucket: "myfleet-media", putErr: injectErr}
	pr := NewProcessor(logrus.New(), NewProvider(db), NewAdministrator(db), store)

	created, err := pr.InitUpload("fleet-a", "u1", "image/jpeg", "photo.jpg")
	if err != nil {
		t.Fatalf("init upload: %v", err)
	}

	_, err = pr.StoreContent(context.Background(), created.ID(), "fleet-a", bytes.NewReader([]byte("jpeg-bytes")), 10)
	if !errors.Is(err, injectErr) {
		t.Fatalf("store content: want %v, got %v", injectErr, err)
	}

	provider := NewProvider(db)
	persisted, err := provider.GetByID(created.ID())
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if persisted.Size() != 0 {
		t.Fatalf("size after failed store: want 0 (untouched), got %d", persisted.Size())
	}
}

// TestStoreContent_unknownLengthRecordsActualByteCount verifies that when the
// caller passes size=-1 (mirroring resource.go's handling of chunked or
// over-advertised request bodies), PutObject still receives -1 unchanged (the
// SDK needs it to stream an unknown-length body) but the size recorded on the
// row is the real number of bytes read, not the -1 sentinel.
func TestStoreContent_unknownLengthRecordsActualByteCount(t *testing.T) {
	db := newConfirmTestDB(t)
	store := &fakeStore{bucket: "myfleet-media"}
	pr := NewProcessor(logrus.New(), NewProvider(db), NewAdministrator(db), store)

	created, err := pr.InitUpload("fleet-a", "u1", "image/jpeg", "photo.jpg")
	if err != nil {
		t.Fatalf("init upload: %v", err)
	}

	body := []byte("a body whose length the caller chose not to advertise")
	updated, err := pr.StoreContent(context.Background(), created.ID(), "fleet-a", bytes.NewReader(body), -1)
	if err != nil {
		t.Fatalf("store content: %v", err)
	}

	if store.putSize != -1 {
		t.Fatalf("PutObject must still receive the unknown-length sentinel, got %d", store.putSize)
	}
	if updated.Size() != int64(len(body)) {
		t.Fatalf("recorded size = %d, want actual byte count %d", updated.Size(), len(body))
	}

	provider := NewProvider(db)
	persisted, err := provider.GetByID(created.ID())
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if persisted.Size() != int64(len(body)) {
		t.Fatalf("persisted size = %d, want %d", persisted.Size(), len(body))
	}
}

func TestContent_returnsBytesAndModelForOwnFleet(t *testing.T) {
	db := newConfirmTestDB(t)
	store := &fakeStore{bucket: "myfleet-media", getBody: []byte("jpeg-bytes")}
	pr := NewProcessor(logrus.New(), NewProvider(db), NewAdministrator(db), store)

	created, err := pr.InitUpload("fleet-a", "u1", "image/jpeg", "photo.jpg")
	if err != nil {
		t.Fatalf("init upload: %v", err)
	}

	m, rc, err := pr.Content(context.Background(), created.ID(), "fleet-a")
	if err != nil {
		t.Fatalf("content: %v", err)
	}
	defer func() { _ = rc.Close() }()

	if store.getKey != created.ObjectKey() {
		t.Fatalf("read key %q, want %q", store.getKey, created.ObjectKey())
	}
	if m.ContentType() != "image/jpeg" {
		t.Fatalf("content type = %q, want image/jpeg", m.ContentType())
	}
	got, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(got) != "jpeg-bytes" {
		t.Fatalf("body = %q, want jpeg-bytes", got)
	}
}

func TestContent_crossFleetIs404(t *testing.T) {
	db := newConfirmTestDB(t)
	store := &fakeStore{bucket: "myfleet-media", getBody: []byte("jpeg-bytes")}
	pr := NewProcessor(logrus.New(), NewProvider(db), NewAdministrator(db), store)

	created, err := pr.InitUpload("fleet-a", "u1", "image/jpeg", "photo.jpg")
	if err != nil {
		t.Fatalf("init upload: %v", err)
	}

	if _, _, err := pr.Content(context.Background(), created.ID(), "fleet-b"); !errors.Is(err, server.ErrNotFound) {
		t.Fatalf("cross-fleet read must be 404, got %v", err)
	}
	if store.getKey != "" {
		t.Fatalf("cross-fleet read must not touch storage, read key %q", store.getKey)
	}
}

// TestContent_missingObjectIs404 verifies the store's "the bytes are not there"
// signal becomes a 404 rather than being handed to the caller as a readable
// stream. This is the state POST /media leaves behind before any content is
// PUT, and the state a failed PUT leaves behind (see
// TestStoreContent_storeFailurePropagatesAndLeavesSizeUntouched).
func TestContent_missingObjectIs404(t *testing.T) {
	db := newConfirmTestDB(t)
	store := &fakeStore{bucket: "myfleet-media", getErr: storage.ErrObjectNotFound}
	pr := NewProcessor(logrus.New(), NewProvider(db), NewAdministrator(db), store)

	created, err := pr.InitUpload("fleet-a", "u1", "image/jpeg", "photo.jpg")
	if err != nil {
		t.Fatalf("init upload: %v", err)
	}

	_, rc, err := pr.Content(context.Background(), created.ID(), "fleet-a")
	if !errors.Is(err, server.ErrNotFound) {
		if rc != nil {
			_ = rc.Close()
		}
		t.Fatalf("content for an object with no stored bytes = %v, want ErrNotFound (404)", err)
	}
	if rc != nil {
		t.Fatal("no reader may be returned alongside the error")
	}
}

// TestContent_otherStorageFailuresAreNot404 keeps the mapping narrow: only a
// genuinely absent object becomes 404. Anything else must stay a 500 so a
// broken MinIO is not reported to clients as "your media does not exist".
func TestContent_otherStorageFailuresAreNot404(t *testing.T) {
	db := newConfirmTestDB(t)
	boom := errors.New("simulated minio outage")
	store := &fakeStore{bucket: "myfleet-media", getErr: boom}
	pr := NewProcessor(logrus.New(), NewProvider(db), NewAdministrator(db), store)

	created, err := pr.InitUpload("fleet-a", "u1", "image/jpeg", "photo.jpg")
	if err != nil {
		t.Fatalf("init upload: %v", err)
	}

	if _, _, err := pr.Content(context.Background(), created.ID(), "fleet-a"); !errors.Is(err, boom) {
		t.Fatalf("content = %v, want the underlying storage error passed through", err)
	}
}

// MarkReadyDirect is the documents-only shortcut: uploaded → ready with no
// worker in between (design D12). Any other source state is a conflict.
func TestMarkReadyDirect_requiresUploaded(t *testing.T) {
	uploaded := Model{status: StatusUploaded}
	got, err := MarkReadyDirect(uploaded)
	if err != nil {
		t.Fatalf("MarkReadyDirect(uploaded): %v", err)
	}
	if got.Status() != StatusReady {
		t.Fatalf("status = %q, want ready", got.Status())
	}

	for _, s := range []Status{StatusProcessing, StatusReady, StatusFailed} {
		if _, err := MarkReadyDirect(Model{status: s}); !errors.Is(err, server.ErrConflict) {
			t.Fatalf("MarkReadyDirect(%q) err = %v, want ErrConflict", s, err)
		}
	}
}

// MarkFailed is the only terminal transition out of the pipeline that is not
// ready. It accepts uploaded or processing (design D13).
func TestMarkFailed_acceptsUploadedAndProcessing(t *testing.T) {
	for _, s := range []Status{StatusUploaded, StatusProcessing} {
		got, err := MarkFailed(Model{status: s})
		if err != nil {
			t.Fatalf("MarkFailed(%q): %v", s, err)
		}
		if got.Status() != StatusFailed {
			t.Fatalf("MarkFailed(%q) status = %q, want failed", s, got.Status())
		}
	}
	for _, s := range []Status{StatusReady, StatusFailed} {
		if _, err := MarkFailed(Model{status: s}); !errors.Is(err, server.ErrConflict) {
			t.Fatalf("MarkFailed(%q) err = %v, want ErrConflict", s, err)
		}
	}
}
