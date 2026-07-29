package processing

import (
	"context"
	"errors"
	"io"
	"testing"

	"github.com/sirupsen/logrus"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/jtumidanski/myfleet/apps/media-service/internal/mediaobject"
	"github.com/jtumidanski/myfleet/apps/media-service/internal/mediavariant"
	"github.com/jtumidanski/myfleet/apps/media-service/internal/processedevents"
	"github.com/jtumidanski/myfleet/packages/shared-go/events"
)

// --- fakes ---

// fakeObjectStore implements ObjectStore; GetObject always fails with errGet.
type fakeObjectStore struct{ errGet error }

func (f *fakeObjectStore) GetObject(_ context.Context, _ string) (io.ReadCloser, error) {
	return nil, f.errGet
}

func (f *fakeObjectStore) PutObject(_ context.Context, _ string, _ io.Reader, _ int64, _ string) error {
	return nil
}

// fakeProvider implements mediaobject.Provider; returns the configured model.
type fakeProvider struct{ m mediaobject.Model }

func (f *fakeProvider) GetByID(_ string) (mediaobject.Model, error)                 { return f.m, nil }
func (f *fakeProvider) GetByIDIncludingDeleted(_ string) (mediaobject.Model, error) { return f.m, nil }

// fakeObjectAdmin implements mediaobject.Administrator; records Update calls.
type fakeObjectAdmin struct{ updated []mediaobject.Model }

func (f *fakeObjectAdmin) Insert(m mediaobject.Model) (mediaobject.Model, error) { return m, nil }
func (f *fakeObjectAdmin) Update(m mediaobject.Model) (mediaobject.Model, error) {
	f.updated = append(f.updated, m)
	return m, nil
}

func (f *fakeObjectAdmin) UpdateInTx(m mediaobject.Model, hook func(tx *gorm.DB) error) (mediaobject.Model, error) {
	f.updated = append(f.updated, m)
	return m, nil
}

func (f *fakeObjectAdmin) SoftDelete(_ string) (mediaobject.Model, error) {
	return mediaobject.Model{}, nil
}

// fakeVariantAdmin implements mediavariant.Administrator; records calls.
type fakeVariantAdmin struct{ called bool }

func (f *fakeVariantAdmin) ReplaceForMediaObject(_ string, _ []mediavariant.Model) error {
	f.called = true
	return nil
}

// newWorkerTestDB creates an in-memory SQLite DB with the processedevents table.
func newWorkerTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.Exec("ATTACH DATABASE ':memory:' AS media").Error; err != nil {
		t.Fatalf("attach media schema: %v", err)
	}
	if err := processedevents.Migration(db); err != nil {
		t.Fatalf("migrate processedevents: %v", err)
	}
	return db
}

// buildProcessingObj returns a minimal mediaobject.Model in the processing state.
func buildProcessingObj(t *testing.T) mediaobject.Model {
	t.Helper()
	m, err := mediaobject.NewBuilder().
		SetID("media-1").
		SetFleetID("fleet-1").
		SetUploadedByUserID("user-1").
		SetBucket("bucket").
		SetObjectKey("fleet-1/media-1/original.jpg").
		SetContentType("image/jpeg").
		SetOriginalFilename("original.jpg").
		SetStatus(mediaobject.StatusProcessing).
		Build()
	if err != nil {
		t.Fatalf("build media object: %v", err)
	}
	return m
}

// TestHandle_failedVariantGeneration_doesNotMarkProcessed asserts that when
// variant generation fails (object store error), the event is NOT written to the
// processedevents ledger, so it will be redelivered and retried.
func TestHandle_failedVariantGeneration_doesNotMarkProcessed(t *testing.T) {
	db := newWorkerTestDB(t)
	store := processedevents.New(logrus.New(), db)

	obj := buildProcessingObj(t)
	objStore := &fakeObjectStore{errGet: errors.New("storage unavailable")}
	objProvider := &fakeProvider{m: obj}
	objAdmin := &fakeObjectAdmin{}
	varAdmin := &fakeVariantAdmin{}

	worker := NewWorker(logrus.New(), objStore, objProvider, objAdmin, varAdmin, store)

	env := events.Envelope{
		EventID: "evt-fail-1",
		Type:    mediaobject.EventTypeMediaUploaded,
		Data:    map[string]any{"media_id": "media-1"},
	}

	err := worker.handle(context.Background(), env)
	if err == nil {
		t.Fatal("expected an error when object store fails, got nil")
	}

	// The event must NOT be recorded in the ledger.
	recorded, checkErr := store.Exists("evt-fail-1")
	if checkErr != nil {
		t.Fatalf("Exists: %v", checkErr)
	}
	if recorded {
		t.Fatal("event must NOT be marked processed when variant generation fails")
	}
}

// TestHandle_alreadyReady_marksProcessedAndSkipsVariants asserts that when the
// media object is already ready (worker completed work but crashed before writing
// the ledger), the event is recorded and no variant generation is attempted.
func TestHandle_alreadyReady_marksProcessedAndSkipsVariants(t *testing.T) {
	db := newWorkerTestDB(t)
	store := processedevents.New(logrus.New(), db)

	readyObj, err := mediaobject.NewBuilder().
		SetID("media-2").
		SetFleetID("fleet-1").
		SetUploadedByUserID("user-1").
		SetBucket("bucket").
		SetObjectKey("fleet-1/media-2/original.jpg").
		SetContentType("image/jpeg").
		SetOriginalFilename("original.jpg").
		SetStatus(mediaobject.StatusReady).
		Build()
	if err != nil {
		t.Fatalf("build media object: %v", err)
	}

	objStore := &fakeObjectStore{}
	objProvider := &fakeProvider{m: readyObj}
	objAdmin := &fakeObjectAdmin{}
	varAdmin := &fakeVariantAdmin{}

	worker := NewWorker(logrus.New(), objStore, objProvider, objAdmin, varAdmin, store)

	env := events.Envelope{
		EventID: "evt-ready-1",
		Type:    mediaobject.EventTypeMediaUploaded,
		Data:    map[string]any{"media_id": "media-2"},
	}

	if err := worker.handle(context.Background(), env); err != nil {
		t.Fatalf("handle returned error: %v", err)
	}

	// Event must be recorded (so future redeliveries are skipped cheaply).
	recorded, checkErr := store.Exists("evt-ready-1")
	if checkErr != nil {
		t.Fatalf("Exists: %v", checkErr)
	}
	if !recorded {
		t.Fatal("event must be marked processed when object is already ready")
	}

	// No variant generation should have been attempted.
	if varAdmin.called {
		t.Fatal("ReplaceForMediaObject must not be called when object is already ready")
	}
}

// TestHandle_alreadyProcessed_skipsWithoutWork asserts that a re-delivered event
// (already in the ledger) is skipped immediately without touching variants or the
// media object.
func TestHandle_alreadyProcessed_skipsWithoutWork(t *testing.T) {
	db := newWorkerTestDB(t)
	store := processedevents.New(logrus.New(), db)

	// Pre-record the event.
	if _, err := store.MarkProcessed("evt-dup-1"); err != nil {
		t.Fatalf("MarkProcessed: %v", err)
	}

	obj := buildProcessingObj(t)
	objStore := &fakeObjectStore{}
	objProvider := &fakeProvider{m: obj}
	objAdmin := &fakeObjectAdmin{}
	varAdmin := &fakeVariantAdmin{}

	worker := NewWorker(logrus.New(), objStore, objProvider, objAdmin, varAdmin, store)

	env := events.Envelope{
		EventID: "evt-dup-1",
		Type:    mediaobject.EventTypeMediaUploaded,
		Data:    map[string]any{"media_id": "media-1"},
	}

	if err := worker.handle(context.Background(), env); err != nil {
		t.Fatalf("handle returned error on duplicate event: %v", err)
	}

	// No variant work should have been attempted.
	if varAdmin.called {
		t.Fatal("ReplaceForMediaObject must not be called for a duplicate event")
	}
	if len(objAdmin.updated) != 0 {
		t.Fatal("media object must not be updated for a duplicate event")
	}
}

func TestResizeDims_preservesAspect(t *testing.T) {
	// Landscape thumbnail: longest edge (4000) scales to 320, height scales
	// proportionally (3000 * 320/4000 = 240).
	w, h := ResizeDims(4000, 3000, 320)
	if w != 320 || h != 240 {
		t.Fatalf("thumbnail dims: want (320,240), got (%d,%d)", w, h)
	}
}

func TestResizeDims_portrait(t *testing.T) {
	// Portrait: longest edge is height (4000) → height becomes 1280, width
	// scales (3000 * 1280/4000 = 960).
	w, h := ResizeDims(3000, 4000, 1280)
	if w != 960 || h != 1280 {
		t.Fatalf("display dims: want (960,1280), got (%d,%d)", w, h)
	}
}

func TestResizeDims_neverUpscales(t *testing.T) {
	// Both dims already <= maxEdge → return original (no upscaling).
	w, h := ResizeDims(100, 80, 320)
	if w != 100 || h != 80 {
		t.Fatalf("must not upscale: want (100,80), got (%d,%d)", w, h)
	}
}

func TestResizeDims_squareExactEdge(t *testing.T) {
	// Square exactly at maxEdge → unchanged.
	w, h := ResizeDims(320, 320, 320)
	if w != 320 || h != 320 {
		t.Fatalf("square at edge: want (320,320), got (%d,%d)", w, h)
	}
}
