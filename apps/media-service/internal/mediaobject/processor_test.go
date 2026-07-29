package mediaobject

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	dtoevents "github.com/jtumidanski/myfleet/packages/dto-go/events"
	sharedevents "github.com/jtumidanski/myfleet/packages/shared-go/events"
	"github.com/jtumidanski/myfleet/packages/shared-go/server"
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
