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
		purge_after         DATETIME,
		purge_operation_id  TEXT
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

	// getBodies serves per-key bytes, so a test can hold a variant and its
	// original at once; keys absent from the map fall back to getBody.
	getBodies map[string][]byte
	// missing marks keys that answer storage.ErrObjectNotFound, which is how
	// DB/store drift (a variant row whose object is gone) is simulated.
	missing map[string]bool
	// getCalls records every key requested, in order, so a test can assert that
	// a cross-fleet read touched storage zero times.
	getCalls []string
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
	f.getCalls = append(f.getCalls, key)
	if f.missing[key] {
		return nil, storage.ErrObjectNotFound
	}
	if f.getErr != nil {
		return nil, f.getErr
	}
	f.getKey = key
	if b, ok := f.getBodies[key]; ok {
		return io.NopCloser(bytes.NewReader(b)), nil
	}
	return io.NopCloser(bytes.NewReader(f.getBody)), nil
}

// fakeVariants stands in for the mediavariant-backed adapter. refs is keyed by
// variant name ("thumbnail", "display"); an absent key is the normal miss.
type fakeVariants struct {
	refs map[string]VariantRef
	err  error
	// calls records the variant name of each lookup; ctxs records the context
	// each one was handed, so a test can prove the request context is threaded
	// through the port rather than dropped on the floor.
	calls []string
	ctxs  []context.Context
}

func (f *fakeVariants) Lookup(ctx context.Context, mediaObjectID, variant string) (VariantRef, bool, error) {
	f.calls = append(f.calls, variant)
	f.ctxs = append(f.ctxs, ctx)
	if f.err != nil {
		return VariantRef{}, false, f.err
	}
	ref, ok := f.refs[variant]
	return ref, ok, nil
}

func TestStoreContent_streamsToObjectStoreAndRecordsSize(t *testing.T) {
	db := newConfirmTestDB(t)
	store := &fakeStore{bucket: "myfleet-media"}
	pr := NewProcessor(logrus.New(), NewProvider(db), NewAdministrator(db), store, &fakeVariants{}, testAllowlist(t))

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
	pr := NewProcessor(logrus.New(), NewProvider(db), NewAdministrator(db), store, &fakeVariants{}, testAllowlist(t))

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
	pr := NewProcessor(logrus.New(), NewProvider(db), NewAdministrator(db), store, &fakeVariants{}, testAllowlist(t))

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
	pr := NewProcessor(logrus.New(), NewProvider(db), NewAdministrator(db), store, &fakeVariants{}, testAllowlist(t))

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
	pr := NewProcessor(logrus.New(), NewProvider(db), NewAdministrator(db), store, &fakeVariants{}, testAllowlist(t))

	created, err := pr.InitUpload("fleet-a", "u1", "image/jpeg", "photo.jpg")
	if err != nil {
		t.Fatalf("init upload: %v", err)
	}

	info, rc, err := pr.Content(context.Background(), created.ID(), "fleet-a", ContentOriginal)
	if err != nil {
		t.Fatalf("content: %v", err)
	}
	defer func() { _ = rc.Close() }()

	if store.getKey != created.ObjectKey() {
		t.Fatalf("read key %q, want %q", store.getKey, created.ObjectKey())
	}
	if info.ContentType != "image/jpeg" {
		t.Fatalf("content type = %q, want image/jpeg", info.ContentType)
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
	pr := NewProcessor(logrus.New(), NewProvider(db), NewAdministrator(db), store, &fakeVariants{}, testAllowlist(t))

	created, err := pr.InitUpload("fleet-a", "u1", "image/jpeg", "photo.jpg")
	if err != nil {
		t.Fatalf("init upload: %v", err)
	}

	if _, _, err := pr.Content(context.Background(), created.ID(), "fleet-b", ContentOriginal); !errors.Is(err, server.ErrNotFound) {
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
	pr := NewProcessor(logrus.New(), NewProvider(db), NewAdministrator(db), store, &fakeVariants{}, testAllowlist(t))

	created, err := pr.InitUpload("fleet-a", "u1", "image/jpeg", "photo.jpg")
	if err != nil {
		t.Fatalf("init upload: %v", err)
	}

	_, rc, err := pr.Content(context.Background(), created.ID(), "fleet-a", ContentOriginal)
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
	pr := NewProcessor(logrus.New(), NewProvider(db), NewAdministrator(db), store, &fakeVariants{}, testAllowlist(t))

	created, err := pr.InitUpload("fleet-a", "u1", "image/jpeg", "photo.jpg")
	if err != nil {
		t.Fatalf("init upload: %v", err)
	}

	if _, _, err := pr.Content(context.Background(), created.ID(), "fleet-a", ContentOriginal); !errors.Is(err, boom) {
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

// seedReadyObject creates a media object and records its byte count, returning
// the model — the state a completed upload leaves behind.
func seedReadyObject(t *testing.T, pr *Processor, fleetID string, payload []byte) Model {
	t.Helper()
	created, err := pr.InitUpload(fleetID, "u1", "image/png", "photo.png")
	if err != nil {
		t.Fatalf("init upload: %v", err)
	}
	stored, err := pr.StoreContent(context.Background(), created.ID(), fleetID, bytes.NewReader(payload), int64(len(payload)))
	if err != nil {
		t.Fatalf("store content: %v", err)
	}
	return stored
}

func readAllAndClose(t *testing.T, rc io.ReadCloser) string {
	t.Helper()
	defer func() { _ = rc.Close() }()
	b, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("read content: %v", err)
	}
	return string(b)
}

// TestContent_originalIsUnchanged pins the pre-existing contract: asking for the
// original serves the media object's own key, content type, and recorded size.
func TestContent_originalIsUnchanged(t *testing.T) {
	db := newConfirmTestDB(t)
	store := &fakeStore{bucket: "myfleet-media", getBody: []byte("original-bytes")}
	variants := &fakeVariants{}
	pr := NewProcessor(logrus.New(), NewProvider(db), NewAdministrator(db), store, variants, testAllowlist(t))

	obj := seedReadyObject(t, pr, "fleet-a", []byte("original-bytes"))

	info, rc, err := pr.Content(context.Background(), obj.ID(), "fleet-a", ContentOriginal)
	if err != nil {
		t.Fatalf("Content: %v", err)
	}
	if got := readAllAndClose(t, rc); got != "original-bytes" {
		t.Fatalf("body = %q, want original-bytes", got)
	}
	if info.ContentType != "image/png" {
		t.Fatalf("ContentType = %q, want image/png", info.ContentType)
	}
	if info.Size != int64(len("original-bytes")) {
		t.Fatalf("Size = %d, want %d", info.Size, len("original-bytes"))
	}
	if len(variants.calls) != 0 {
		t.Fatalf("variant lookup ran %v times for an original request; it must not", variants.calls)
	}
}

// TestContent_variantFoundServesVariantBytes is the whole point of the feature:
// the variant's own key and content type, and NO size — media_variants records
// width/height/content_type but no byte count, so Content-Length must be omitted.
func TestContent_variantFoundServesVariantBytes(t *testing.T) {
	db := newConfirmTestDB(t)
	store := &fakeStore{
		bucket:    "myfleet-media",
		getBody:   []byte("original-bytes"),
		getBodies: map[string][]byte{"fleet-a/thumb.jpg": []byte("thumb-bytes")},
	}
	variants := &fakeVariants{refs: map[string]VariantRef{
		"thumbnail": {ObjectKey: "fleet-a/thumb.jpg", ContentType: "image/jpeg"},
	}}
	pr := NewProcessor(logrus.New(), NewProvider(db), NewAdministrator(db), store, variants, testAllowlist(t))

	obj := seedReadyObject(t, pr, "fleet-a", []byte("original-bytes"))

	type ctxKey struct{}
	ctx := context.WithValue(context.Background(), ctxKey{}, "carried")

	info, rc, err := pr.Content(ctx, obj.ID(), "fleet-a", ContentThumbnail)
	if err != nil {
		t.Fatalf("Content: %v", err)
	}
	if got := readAllAndClose(t, rc); got != "thumb-bytes" {
		t.Fatalf("body = %q, want thumb-bytes", got)
	}
	if info.ContentType != "image/jpeg" {
		t.Fatalf("ContentType = %q, want the variant's own image/jpeg", info.ContentType)
	}
	if info.Size != 0 {
		t.Fatalf("Size = %d, want 0 — the original's size must never describe a variant", info.Size)
	}
	// The lookup runs as part of serving a request, so it must receive that
	// request's context — not context.Background() manufactured inside the
	// provider, which would survive a client disconnect.
	if len(variants.ctxs) != 1 || variants.ctxs[0].Value(ctxKey{}) != "carried" {
		t.Fatalf("Lookup received %d contexts, first value %v; want Content's own ctx threaded through",
			len(variants.ctxs), func() any {
				if len(variants.ctxs) == 0 {
					return nil
				}
				return variants.ctxs[0].Value(ctxKey{})
			}())
	}
}

// TestContent_variantMissingIs404AndServesNoOriginal is the reversal of the
// original fallback design. A missing variant row is the normal state for media
// whose processing has not finished and for anything that is not a processable
// image — but answering it with the original means a twelve-card grid asking for
// thumbnails silently pulls twelve full-size uploads, up to 25 MiB each. The
// caller asked for a small rendition; 404 is the honest answer.
func TestContent_variantMissingIs404AndServesNoOriginal(t *testing.T) {
	db := newConfirmTestDB(t)
	store := &fakeStore{bucket: "myfleet-media", getBody: []byte("original-bytes")}
	variants := &fakeVariants{} // no rows at all
	pr := NewProcessor(logrus.New(), NewProvider(db), NewAdministrator(db), store, variants, testAllowlist(t))

	obj := seedReadyObject(t, pr, "fleet-a", []byte("original-bytes"))
	callsBefore := len(store.getCalls)

	info, rc, err := pr.Content(context.Background(), obj.ID(), "fleet-a", ContentThumbnail)
	if !errors.Is(err, server.ErrNotFound) {
		t.Fatalf("Content = %v, want ErrNotFound (404) for a variant that does not exist", err)
	}
	if rc != nil {
		_ = rc.Close()
		t.Fatal("Content returned a body alongside the 404; nothing may be served")
	}
	if info != (ContentInfo{}) {
		t.Fatalf("ContentInfo = %+v, want the zero value on a 404", info)
	}
	// The point of the change: the original's bytes were never opened.
	if len(store.getCalls) != callsBefore {
		t.Fatalf("a missing variant read the object store: %v — the original must NOT be served",
			store.getCalls[callsBefore:])
	}
}

// TestContent_variantObjectMissingIs404AndServesNoOriginal covers DB/store
// drift: the variant row exists but its object is gone from MinIO. The response
// is the same 404 as a missing row, because the size consequence of falling back
// to the original is identical. The Warn log is what distinguishes the two for
// an operator — drift is a real fault, a missing row usually is not.
func TestContent_variantObjectMissingIs404AndServesNoOriginal(t *testing.T) {
	db := newConfirmTestDB(t)
	store := &fakeStore{
		bucket:  "myfleet-media",
		getBody: []byte("original-bytes"),
		missing: map[string]bool{"fleet-a/gone.jpg": true},
	}
	variants := &fakeVariants{refs: map[string]VariantRef{
		"thumbnail": {ObjectKey: "fleet-a/gone.jpg", ContentType: "image/jpeg"},
	}}
	log := logrus.New()
	log.SetLevel(logrus.WarnLevel)
	var logged logrusTestHook
	log.AddHook(&logged)
	pr := NewProcessor(log, NewProvider(db), NewAdministrator(db), store, variants, testAllowlist(t))

	obj := seedReadyObject(t, pr, "fleet-a", []byte("original-bytes"))
	callsBefore := len(store.getCalls)

	_, rc, err := pr.Content(context.Background(), obj.ID(), "fleet-a", ContentThumbnail)
	if !errors.Is(err, server.ErrNotFound) {
		t.Fatalf("Content = %v, want ErrNotFound (404) when the variant's object is gone", err)
	}
	if rc != nil {
		_ = rc.Close()
		t.Fatal("Content returned a body alongside the 404; nothing may be served")
	}
	// Exactly one store read — the missing variant key. Falling through to the
	// original would show up here as a second call.
	if got := store.getCalls[callsBefore:]; len(got) != 1 || got[0] != "fleet-a/gone.jpg" {
		t.Fatalf("store calls = %v; want exactly the variant key and never the original", got)
	}
	if !logged.hasWarn() {
		t.Fatal("DB/store drift was not logged at Warn; the 404 alone leaves an operator blind to it")
	}
}

// logrusTestHook captures emitted entries so a test can assert an operator-facing
// log was actually written, not merely that the code path was taken.
type logrusTestHook struct{ entries []*logrus.Entry }

func (h *logrusTestHook) Levels() []logrus.Level { return logrus.AllLevels }

func (h *logrusTestHook) Fire(e *logrus.Entry) error {
	h.entries = append(h.entries, e)
	return nil
}

func (h *logrusTestHook) hasWarn() bool { return h.has(logrus.WarnLevel) }

func (h *logrusTestHook) hasError() bool { return h.has(logrus.ErrorLevel) }

func (h *logrusTestHook) has(level logrus.Level) bool {
	for _, e := range h.entries {
		if e.Level == level {
			return true
		}
	}
	return false
}

// TestContent_crossFleetNeverTouchesLookupOrStore is FR-7.5: the media object is
// resolved and fleet-scoped BEFORE any variant lookup or object-store read, so a
// variant can never be reachable by a caller who could not read the original.
func TestContent_crossFleetNeverTouchesLookupOrStore(t *testing.T) {
	db := newConfirmTestDB(t)
	store := &fakeStore{bucket: "myfleet-media", getBody: []byte("original-bytes")}
	variants := &fakeVariants{refs: map[string]VariantRef{
		"thumbnail": {ObjectKey: "fleet-b/thumb.jpg", ContentType: "image/jpeg"},
	}}
	pr := NewProcessor(logrus.New(), NewProvider(db), NewAdministrator(db), store, variants, testAllowlist(t))

	obj := seedReadyObject(t, pr, "fleet-b", []byte("original-bytes"))

	_, _, err := pr.Content(context.Background(), obj.ID(), "fleet-a", ContentThumbnail)
	if !errors.Is(err, server.ErrNotFound) {
		t.Fatalf("Content across fleets = %v, want ErrNotFound (404, never 403 — 403 would leak existence)", err)
	}
	if len(variants.calls) != 0 {
		t.Fatalf("variant lookup ran %v for a cross-fleet read", variants.calls)
	}
	if len(store.getCalls) != 0 {
		t.Fatalf("cross-fleet read touched storage: %v", store.getCalls)
	}
}

// TestContent_lookupErrorIsReturned: a miss is found=false, so an actual error
// means the database is broken. Masking it behind the original would hide a real
// fault.
func TestContent_lookupErrorIsReturned(t *testing.T) {
	db := newConfirmTestDB(t)
	store := &fakeStore{bucket: "myfleet-media", getBody: []byte("original-bytes")}
	boom := errors.New("simulated variant query failure")
	variants := &fakeVariants{err: boom}
	pr := NewProcessor(logrus.New(), NewProvider(db), NewAdministrator(db), store, variants, testAllowlist(t))

	obj := seedReadyObject(t, pr, "fleet-a", []byte("original-bytes"))

	if _, _, err := pr.Content(context.Background(), obj.ID(), "fleet-a", ContentThumbnail); !errors.Is(err, boom) {
		t.Fatalf("Content = %v, want the lookup error propagated", err)
	}
}
