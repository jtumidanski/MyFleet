package processing

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/png"
	"io"
	"testing"

	"github.com/sirupsen/logrus"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/jtumidanski/myfleet/apps/media-service/internal/mediaobject"
	"github.com/jtumidanski/myfleet/apps/media-service/internal/mediavariant"
	"github.com/jtumidanski/myfleet/apps/media-service/internal/processedevents"
	"github.com/jtumidanski/myfleet/apps/media-service/internal/storage"
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

func (f *fakeProvider) ListActiveByFleetAndIDs(_ string, _ []string) ([]mediaobject.Model, error) {
	return nil, nil
}

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

// fakeVariantAdmin implements mediavariant.Administrator; records calls and the
// models it was handed, so a test can assert exactly which variants were built.
type fakeVariantAdmin struct {
	called       bool
	replaceCalls int
	replaced     []mediavariant.Model
}

func (f *fakeVariantAdmin) ReplaceForMediaObject(_ string, variants []mediavariant.Model) error {
	f.called = true
	f.replaceCalls++
	f.replaced = variants
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

// The card variant's max edge, exercised on the four shapes that matter: a
// landscape original, a portrait one, one already exactly at the edge, and one
// smaller than the edge — which must NOT be upscaled, because inventing pixels
// costs bytes and buys nothing (NFR-12).
func TestResizeDims_cardMaxEdge(t *testing.T) {
	if w, h := ResizeDims(4000, 3000, cardMaxEdge); w != 768 || h != 576 {
		t.Fatalf("landscape card dims: want (768,576), got (%d,%d)", w, h)
	}
	if w, h := ResizeDims(3000, 4000, cardMaxEdge); w != 576 || h != 768 {
		t.Fatalf("portrait card dims: want (576,768), got (%d,%d)", w, h)
	}
	if w, h := ResizeDims(768, 768, cardMaxEdge); w != 768 || h != 768 {
		t.Fatalf("square at edge: want (768,768), got (%d,%d)", w, h)
	}
	if w, h := ResizeDims(600, 400, cardMaxEdge); w != 600 || h != 400 {
		t.Fatalf("must not upscale below the card edge: want (600,400), got (%d,%d)", w, h)
	}
}

// bytesStore returns fixed bytes from GetObject so the decode path can be
// exercised with content that is not a valid image.
type bytesStore struct{ data []byte }

func (b *bytesStore) GetObject(_ context.Context, _ string) (io.ReadCloser, error) {
	return io.NopCloser(bytes.NewReader(b.data)), nil
}

func (b *bytesStore) PutObject(_ context.Context, _ string, _ io.Reader, _ int64, _ string) error {
	return nil
}

// A corrupt or mislabelled file will never become decodable, so retrying it
// forever blocks the partition for every other object behind it. It must reach
// a terminal state AND have its event recorded (PRD FR-MEDIA-5, design D13).
func TestHandle_undecodableBytesMarkFailedAndProcessed(t *testing.T) {
	db := newWorkerTestDB(t)
	dedupe := processedevents.New(logrus.New(), db)

	obj := buildProcessingObj(t)
	objStore := &bytesStore{data: []byte("this is definitely not a jpeg")}
	objAdmin := &fakeObjectAdmin{}

	worker := NewWorker(logrus.New(), objStore, &fakeProvider{m: obj}, objAdmin, &fakeVariantAdmin{}, dedupe)

	env := events.Envelope{
		EventID: "evt-undecodable",
		Type:    mediaobject.EventTypeMediaUploaded,
		Data:    map[string]any{"media_id": "media-1"},
	}

	if err := worker.handle(context.Background(), env); err != nil {
		t.Fatalf("handle returned %v; a permanent failure must be committed, not retried", err)
	}

	if len(objAdmin.updated) != 1 || objAdmin.updated[0].Status() != mediaobject.StatusFailed {
		t.Fatalf("object was not moved to failed: %+v", objAdmin.updated)
	}

	recorded, err := dedupe.Exists("evt-undecodable")
	if err != nil {
		t.Fatalf("Exists: %v", err)
	}
	if !recorded {
		t.Fatal("event was not marked processed; it would redeliver forever and block the partition")
	}
}

// An original whose bytes were never stored is permanent too: the PUT that
// would have created them already failed and will not retry itself.
func TestHandle_missingOriginalMarksFailedAndProcessed(t *testing.T) {
	db := newWorkerTestDB(t)
	dedupe := processedevents.New(logrus.New(), db)

	obj := buildProcessingObj(t)
	objStore := &fakeObjectStore{errGet: storage.ErrObjectNotFound}
	objAdmin := &fakeObjectAdmin{}

	worker := NewWorker(logrus.New(), objStore, &fakeProvider{m: obj}, objAdmin, &fakeVariantAdmin{}, dedupe)

	env := events.Envelope{
		EventID: "evt-missing-original",
		Type:    mediaobject.EventTypeMediaUploaded,
		Data:    map[string]any{"media_id": "media-1"},
	}

	if err := worker.handle(context.Background(), env); err != nil {
		t.Fatalf("handle returned %v; a missing original is permanent", err)
	}
	if len(objAdmin.updated) != 1 || objAdmin.updated[0].Status() != mediaobject.StatusFailed {
		t.Fatalf("object was not moved to failed: %+v", objAdmin.updated)
	}
	recorded, err := dedupe.Exists("evt-missing-original")
	if err != nil {
		t.Fatalf("Exists: %v", err)
	}
	if !recorded {
		t.Fatal("event was not marked processed")
	}
}

// pngBytes encodes a blank PNG of the requested size. The worker decodes
// whatever bytes the store returns, so a real encoded image is the only way to
// exercise the resize path end to end.
func pngBytes(t *testing.T, w, h int) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, w, h))); err != nil {
		t.Fatalf("encode png: %v", err)
	}
	return buf.Bytes()
}

// TestHandle_generatesThumbnailCardAndDisplay pins the whole derived set: one
// decode of the original produces exactly three renditions, each scaled to its
// own max edge, persisted in ONE ReplaceForMediaObject call — which is what
// keeps a redelivered event idempotent (NFR-13). A second delivery of the same
// event does no work at all, because the ledger short-circuits it.
func TestHandle_generatesThumbnailCardAndDisplay(t *testing.T) {
	db := newWorkerTestDB(t)
	dedupe := processedevents.New(logrus.New(), db)

	obj := buildProcessingObj(t)
	objStore := &bytesStore{data: pngBytes(t, 2000, 1000)}
	varAdmin := &fakeVariantAdmin{}

	worker := NewWorker(logrus.New(), objStore, &fakeProvider{m: obj}, &fakeObjectAdmin{}, varAdmin, dedupe)

	env := events.Envelope{
		EventID: "evt-three-variants",
		Type:    mediaobject.EventTypeMediaUploaded,
		Data:    map[string]any{"media_id": "media-1"},
	}
	if err := worker.handle(context.Background(), env); err != nil {
		t.Fatalf("handle: %v", err)
	}

	if len(varAdmin.replaced) != 3 {
		t.Fatalf("persisted %d variants, want exactly 3", len(varAdmin.replaced))
	}
	got := map[mediavariant.Variant][2]int{}
	for _, v := range varAdmin.replaced {
		got[v.Variant()] = [2]int{v.Width(), v.Height()}
	}
	want := map[mediavariant.Variant][2]int{
		mediavariant.VariantThumbnail: {320, 160},
		mediavariant.VariantCard:      {768, 384},
		mediavariant.VariantDisplay:   {1280, 640},
	}
	for kind, dims := range want {
		if got[kind] != dims {
			t.Fatalf("%s dims = %v, want %v", kind, got[kind], dims)
		}
	}

	// Redelivery: the object is not re-fetched as processing, and no second
	// write happens. One ReplaceForMediaObject call, total.
	if err := worker.handle(context.Background(), env); err != nil {
		t.Fatalf("handle on redelivery: %v", err)
	}
	if varAdmin.replaceCalls != 1 {
		t.Fatalf("ReplaceForMediaObject ran %d times across two deliveries, want 1", varAdmin.replaceCalls)
	}
}

// A PNG original keeps its encoding through every variant, card included:
// re-encoding a PNG as JPEG would introduce artefacts on exactly the flat-colour
// images PNG is chosen for.
func TestHandle_pngOriginalProducesPngCard(t *testing.T) {
	db := newWorkerTestDB(t)
	dedupe := processedevents.New(logrus.New(), db)

	obj, err := mediaobject.NewBuilder().
		SetID("media-png").
		SetFleetID("fleet-1").
		SetUploadedByUserID("user-1").
		SetBucket("bucket").
		SetObjectKey("fleet-1/media-png/original.png").
		SetContentType("image/png").
		SetOriginalFilename("original.png").
		SetStatus(mediaobject.StatusProcessing).
		Build()
	if err != nil {
		t.Fatalf("build media object: %v", err)
	}

	varAdmin := &fakeVariantAdmin{}
	worker := NewWorker(logrus.New(), &bytesStore{data: pngBytes(t, 1000, 1000)},
		&fakeProvider{m: obj}, &fakeObjectAdmin{}, varAdmin, dedupe)

	env := events.Envelope{
		EventID: "evt-png",
		Type:    mediaobject.EventTypeMediaUploaded,
		Data:    map[string]any{"media_id": "media-png"},
	}
	if err := worker.handle(context.Background(), env); err != nil {
		t.Fatalf("handle: %v", err)
	}

	var card mediavariant.Model
	for _, v := range varAdmin.replaced {
		if v.Variant() == mediavariant.VariantCard {
			card = v
		}
	}
	if card.ContentType() != "image/png" {
		t.Fatalf("card ContentType = %q, want image/png", card.ContentType())
	}
	if card.ObjectKey() != "fleet-1/media-png/card.png" {
		t.Fatalf("card ObjectKey = %q, want fleet-1/media-png/card.png", card.ObjectKey())
	}
}
