package processing

import (
	"bytes"
	"context"
	"errors"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/jtumidanski/myfleet/apps/media-service/internal/mediavariant"
	"github.com/jtumidanski/myfleet/apps/media-service/internal/storage"
	"github.com/jtumidanski/myfleet/apps/media-service/internal/variantfailures"
)

// --- fakes ---

// blockingStore serves fixed bytes, counts GetObject calls, and can hold every
// call until a gate is closed — which is how the single-flight and cap tests
// keep generations alive long enough to observe them.
type blockingStore struct {
	data []byte
	err  error
	gate chan struct{}

	mu       sync.Mutex
	getCalls int
	putKeys  []string
}

func (b *blockingStore) GetObject(_ context.Context, _ string) (io.ReadCloser, error) {
	b.mu.Lock()
	b.getCalls++
	b.mu.Unlock()
	if b.gate != nil {
		<-b.gate
	}
	if b.err != nil {
		return nil, b.err
	}
	return io.NopCloser(bytes.NewReader(b.data)), nil
}

func (b *blockingStore) PutObject(_ context.Context, key string, _ io.Reader, _ int64, _ string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.putKeys = append(b.putKeys, key)
	return nil
}

func (b *blockingStore) calls() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.getCalls
}

// panicStore's GetObject succeeds but returns a reader that panics on Read —
// standing in for a decoder that panics deep inside image.Decode on malformed
// bytes, which is real: it runs on arbitrary stored bytes it does not control.
type panicStore struct{}

func (panicStore) GetObject(_ context.Context, _ string) (io.ReadCloser, error) {
	return panicReader{}, nil
}

func (panicStore) PutObject(_ context.Context, _ string, _ io.Reader, _ int64, _ string) error {
	return nil
}

type panicReader struct{}

func (panicReader) Read(_ []byte) (int, error) { panic("simulated decoder panic on corrupt bytes") }
func (panicReader) Close() error               { return nil }

// newCardTestDB gives the generator a real mediavariant table and a real
// failure ledger. The variants table is created with raw SQL because GORM
// AutoMigrate mishandles schema-qualified names on SQLite for an entity with
// index tags — the same reason mediavariant's own tests do it.
func newCardTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	// The ":memory:" DSN carries no cache=shared, so every physical connection
	// is an independent empty database. The pool must be capped to the single
	// connection the ATTACH and CREATE TABLE below run on, or a background
	// goroutine's write can land on a second, schema-less connection and
	// silently disappear. CardGenerator is the first thing in this package to
	// drive concurrent goroutines against one gorm.DB, which is why this
	// surfaces here first.
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("get sql.DB: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)
	if err := db.Exec("ATTACH DATABASE ':memory:' AS media").Error; err != nil {
		t.Fatalf("attach media schema: %v", err)
	}
	if err := db.Exec(`CREATE TABLE media.media_variants (
		id              TEXT PRIMARY KEY,
		media_object_id TEXT NOT NULL,
		variant         TEXT NOT NULL,
		object_key      TEXT NOT NULL,
		width           INTEGER,
		height          INTEGER,
		content_type    TEXT,
		created_at      DATETIME,
		UNIQUE (media_object_id, variant)
	)`).Error; err != nil {
		t.Fatalf("create media_variants: %v", err)
	}
	if err := variantfailures.Migration(db); err != nil {
		t.Fatalf("migrate variantfailures: %v", err)
	}
	return db
}

func cardSource(id string) Source {
	return Source{
		MediaObjectID: id,
		FleetID:       "fleet-a",
		ObjectKey:     "fleet-a/" + id + "/original.jpg",
		ContentType:   "image/jpeg",
	}
}

// waitFor polls until cond holds or the deadline passes. Generation is
// asynchronous by design, so tests observe its effects rather than its return.
func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

// --- tests ---

// ★ The regression guard for the single most damaging way to get this wrong:
// reaching for ReplaceForMediaObject, which deletes every row for the media
// object first and would destroy the thumbnail and display the object already
// has (FR-3.4, NFR-17).
func TestCardGenerator_doesNotDestroyExistingVariants(t *testing.T) {
	db := newCardTestDB(t)
	admin := mediavariant.NewAdministrator(db)

	seeded := make([]mediavariant.Model, 0, 2)
	for _, spec := range []struct {
		v   mediavariant.Variant
		key string
	}{
		{mediavariant.VariantThumbnail, "fleet-a/m1/thumbnail.jpg"},
		{mediavariant.VariantDisplay, "fleet-a/m1/display.jpg"},
	} {
		m, err := mediavariant.NewBuilder().
			SetMediaObjectID("m1").
			SetVariant(spec.v).
			SetObjectKey(spec.key).
			SetContentType("image/jpeg").
			Build()
		if err != nil {
			t.Fatalf("build %s: %v", spec.v, err)
		}
		seeded = append(seeded, m)
	}
	if err := admin.ReplaceForMediaObject("m1", seeded); err != nil {
		t.Fatalf("seed: %v", err)
	}

	store := &blockingStore{data: pngBytes(t, 2000, 1000)}
	g := NewCardGenerator(context.Background(), logrus.New(), store, admin,
		variantfailures.New(logrus.New(), db), 4)

	g.Generate(cardSource("m1"))

	provider := mediavariant.NewProvider(db)
	waitFor(t, "the card row to be written", func() bool {
		rows, err := provider.ListByMediaObject("m1")
		return err == nil && len(rows) == 3
	})

	rows, err := provider.ListByMediaObject("m1")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	kinds := map[mediavariant.Variant]bool{}
	for _, r := range rows {
		kinds[r.Variant()] = true
	}
	for _, want := range []mediavariant.Variant{
		mediavariant.VariantThumbnail, mediavariant.VariantCard, mediavariant.VariantDisplay,
	} {
		if !kinds[want] {
			t.Fatalf("after lazy generation the %s row is gone; present: %v", want, kinds)
		}
	}
}

// The generated card must be indistinguishable from one the upload worker would
// have produced: same max edge, same key scheme, same encoding.
func TestCardGenerator_producesTheSameCardTheWorkerWould(t *testing.T) {
	db := newCardTestDB(t)
	store := &blockingStore{data: pngBytes(t, 2000, 1000)}
	g := NewCardGenerator(context.Background(), logrus.New(), store,
		mediavariant.NewAdministrator(db), variantfailures.New(logrus.New(), db), 4)

	g.Generate(cardSource("m1"))

	provider := mediavariant.NewProvider(db)
	waitFor(t, "the card row to be written", func() bool {
		_, err := provider.GetByMediaObjectAndVariant(context.Background(), "m1", mediavariant.VariantCard)
		return err == nil
	})

	got, err := provider.GetByMediaObjectAndVariant(context.Background(), "m1", mediavariant.VariantCard)
	if err != nil {
		t.Fatalf("read card: %v", err)
	}
	if got.Width() != 768 || got.Height() != 384 {
		t.Fatalf("card dims = (%d,%d), want (768,384)", got.Width(), got.Height())
	}
	if got.ObjectKey() != "fleet-a/m1/card.jpg" {
		t.Fatalf("card ObjectKey = %q, want fleet-a/m1/card.jpg", got.ObjectKey())
	}
	if got.ContentType() != "image/jpeg" {
		t.Fatalf("card ContentType = %q, want image/jpeg", got.ContentType())
	}

	store.mu.Lock()
	putKeys := append([]string(nil), store.putKeys...)
	store.mu.Unlock()
	if len(putKeys) != 1 || putKeys[0] != got.ObjectKey() {
		t.Fatalf("putKeys = %v, want exactly one PUT at the row's own object key %q", putKeys, got.ObjectKey())
	}
}

// ★ Single-flight (FR-4.1, NFR-16): a cold grid can ask for the same missing
// card many times before the first generation finishes. Exactly one decode and
// one row must result.
func TestCardGenerator_singleFlightsPerMediaObject(t *testing.T) {
	db := newCardTestDB(t)
	gate := make(chan struct{})
	store := &blockingStore{data: pngBytes(t, 2000, 1000), gate: gate}
	g := NewCardGenerator(context.Background(), logrus.New(), store,
		mediavariant.NewAdministrator(db), variantfailures.New(logrus.New(), db), 4)

	for i := 0; i < 8; i++ {
		g.Generate(cardSource("m1"))
	}
	// Let the one admitted generation reach GetObject before releasing it, so
	// the other seven are provably rejected while it is still in flight.
	waitFor(t, "the first generation to reach the store", func() bool { return store.calls() == 1 })
	close(gate)

	provider := mediavariant.NewProvider(db)
	waitFor(t, "the card row to be written", func() bool {
		_, err := provider.GetByMediaObjectAndVariant(context.Background(), "m1", mediavariant.VariantCard)
		return err == nil
	})

	if store.calls() != 1 {
		t.Fatalf("GetObject ran %d times for one media object, want exactly 1", store.calls())
	}
	rows, err := provider.ListByMediaObject("m1")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("got %d card rows, want exactly 1", len(rows))
	}
}

// After a generation completes, the object is no longer in flight — a later
// request for a different variant of the same object (or a repair after drift)
// must be admitted rather than blocked forever by a leaked slot.
func TestCardGenerator_releasesTheInFlightSlot(t *testing.T) {
	db := newCardTestDB(t)
	store := &blockingStore{data: pngBytes(t, 2000, 1000)}
	g := NewCardGenerator(context.Background(), logrus.New(), store,
		mediavariant.NewAdministrator(db), variantfailures.New(logrus.New(), db), 4)

	g.Generate(cardSource("m1"))
	waitFor(t, "the first generation to finish", func() bool { return store.calls() == 1 })
	waitFor(t, "the in-flight slot to be released", func() bool { return !g.inFlightFor("m1") })

	g.Generate(cardSource("m1"))
	waitFor(t, "the second generation to run", func() bool { return store.calls() == 2 })
}

// The cap bounds concurrent work across DIFFERENT media objects (FR-4.2), and
// excess requests are dropped rather than queued (FR-4.3): a dropped request
// costs the caller nothing, because it has already been served its thumbnail.
func TestCardGenerator_capDropsRatherThanQueues(t *testing.T) {
	db := newCardTestDB(t)
	gate := make(chan struct{})
	store := &blockingStore{data: pngBytes(t, 2000, 1000), gate: gate}
	g := NewCardGenerator(context.Background(), logrus.New(), store,
		mediavariant.NewAdministrator(db), variantfailures.New(logrus.New(), db), 1)

	g.Generate(cardSource("m1"))
	waitFor(t, "the first generation to occupy the cap", func() bool { return store.calls() == 1 })

	// A different media object, so single-flight is not what rejects it.
	g.Generate(cardSource("m2"))
	if store.calls() != 1 {
		t.Fatalf("GetObject ran %d times; the second request must be dropped, not queued", store.calls())
	}

	close(gate)
	provider := mediavariant.NewProvider(db)
	waitFor(t, "the first card row", func() bool {
		_, err := provider.GetByMediaObjectAndVariant(context.Background(), "m1", mediavariant.VariantCard)
		return err == nil
	})

	// The row becomes visible at Upsert, but the cap token is only surrendered
	// by the goroutine's deferred <-g.sem, which runs after Upsert, after the
	// Info log, and after the other defers. Without waiting for that release
	// too, a Generate landing in that window is dropped with no reschedule of
	// its own, and the assertion below hangs for the full waitFor deadline.
	waitFor(t, "the cap slot to be released", func() bool { return len(g.sem) == 0 })

	// Dropped, not lost: the next request reschedules.
	g.Generate(cardSource("m2"))
	waitFor(t, "the rescheduled second card row", func() bool {
		_, err := provider.GetByMediaObjectAndVariant(context.Background(), "m2", mediavariant.VariantCard)
		return err == nil
	})
}

// concurrency 0 is a supported deployment, not a degraded one: it is the off
// switch an operator needs when the feature misbehaves, without a rollback.
func TestCardGenerator_zeroConcurrencyDisablesGeneration(t *testing.T) {
	db := newCardTestDB(t)
	store := &blockingStore{data: pngBytes(t, 2000, 1000)}
	g := NewCardGenerator(context.Background(), logrus.New(), store,
		mediavariant.NewAdministrator(db), variantfailures.New(logrus.New(), db), 0)

	g.Generate(cardSource("m1"))
	time.Sleep(50 * time.Millisecond)

	if store.calls() != 0 {
		t.Fatalf("GetObject ran %d times with concurrency 0; nothing may be scheduled", store.calls())
	}
}

// A negative value is an operator typo, not a request for unbounded work.
func TestCardGenerator_negativeConcurrencyClampsToDisabled(t *testing.T) {
	db := newCardTestDB(t)
	store := &blockingStore{data: pngBytes(t, 2000, 1000)}
	g := NewCardGenerator(context.Background(), logrus.New(), store,
		mediavariant.NewAdministrator(db), variantfailures.New(logrus.New(), db), -3)

	g.Generate(cardSource("m1"))
	time.Sleep(50 * time.Millisecond)

	if store.calls() != 0 {
		t.Fatalf("GetObject ran %d times with negative concurrency; it must clamp to disabled", store.calls())
	}
}

// An undecodable original is permanent: record it once and never decode it
// again (FR-6.1, FR-6.3).
func TestCardGenerator_undecodableOriginalIsRecordedAndNotRetried(t *testing.T) {
	db := newCardTestDB(t)
	failures := variantfailures.New(logrus.New(), db)
	store := &blockingStore{data: []byte("this is definitely not a jpeg")}
	g := NewCardGenerator(context.Background(), logrus.New(), store,
		mediavariant.NewAdministrator(db), failures, 4)

	g.Generate(cardSource("m1"))
	waitFor(t, "the permanent failure to be recorded", func() bool {
		recorded, err := failures.Recorded("m1", string(mediavariant.VariantCard))
		return err == nil && recorded
	})
	waitFor(t, "the first attempt to finish", func() bool { return !g.inFlightFor("m1") })

	callsAfterFirst := store.calls()
	g.Generate(cardSource("m1"))
	waitFor(t, "the second attempt to finish", func() bool { return !g.inFlightFor("m1") })

	if store.calls() != callsAfterFirst {
		t.Fatalf("GetObject ran again (%d → %d); a recorded permanent failure must not re-attempt decoding",
			callsAfterFirst, store.calls())
	}
}

// A missing original is permanent too, and is recorded with its own reason so
// the ledger stays diagnostic.
func TestCardGenerator_missingOriginalRecordsItsOwnReason(t *testing.T) {
	db := newCardTestDB(t)
	failures := variantfailures.New(logrus.New(), db)
	store := &blockingStore{err: storage.ErrObjectNotFound}
	g := NewCardGenerator(context.Background(), logrus.New(), store,
		mediavariant.NewAdministrator(db), failures, 4)

	g.Generate(cardSource("m1"))
	waitFor(t, "the permanent failure to be recorded", func() bool {
		recorded, err := failures.Recorded("m1", string(mediavariant.VariantCard))
		return err == nil && recorded
	})

	var rows []variantfailures.Entity
	if err := db.Find(&rows).Error; err != nil {
		t.Fatalf("read ledger: %v", err)
	}
	if len(rows) != 1 || rows[0].Reason != variantfailures.ReasonOriginalMissing {
		t.Fatalf("ledger rows = %+v, want one row with reason %q", rows, variantfailures.ReasonOriginalMissing)
	}
}

// A transient failure records nothing: the object store being briefly
// unreachable must not permanently condemn a perfectly good image (FR-6.2).
func TestCardGenerator_transientFailureIsNotRecorded(t *testing.T) {
	db := newCardTestDB(t)
	failures := variantfailures.New(logrus.New(), db)
	store := &blockingStore{err: errors.New("storage unavailable")}
	g := NewCardGenerator(context.Background(), logrus.New(), store,
		mediavariant.NewAdministrator(db), failures, 4)

	g.Generate(cardSource("m1"))
	waitFor(t, "the attempt to finish", func() bool { return !g.inFlightFor("m1") })

	recorded, err := failures.Recorded("m1", string(mediavariant.VariantCard))
	if err != nil {
		t.Fatalf("Recorded: %v", err)
	}
	if recorded {
		t.Fatal("a transient store error must not be recorded as a permanent failure")
	}
}

// FR-4.4: the work is detached from the request that triggered it. A client
// disconnecting immediately after receiving its downgraded thumbnail must not
// cancel the generation it caused. The real guarantee here is enforced by
// Generate's signature carrying no context parameter at all — the compiler
// checks it, since there is nothing a caller could pass in even if it wanted
// to. This test is the end-to-end confirmation that generation still runs to
// completion and the card row lands.
func TestCardGenerator_ignoresACancelledCallerContext(t *testing.T) {
	db := newCardTestDB(t)
	store := &blockingStore{data: pngBytes(t, 2000, 1000)}
	g := NewCardGenerator(context.Background(), logrus.New(), store,
		mediavariant.NewAdministrator(db), variantfailures.New(logrus.New(), db), 4)

	g.Generate(cardSource("m1"))

	provider := mediavariant.NewProvider(db)
	waitFor(t, "the card row to be written despite the cancelled request", func() bool {
		_, err := provider.GetByMediaObjectAndVariant(context.Background(), "m1", mediavariant.VariantCard)
		return err == nil
	})
}

// A panic inside the goroutine — a decoder choking on malformed bytes deep
// inside image.Decode, say — must not crash the process, and must not leak
// the semaphore token or the in-flight key. On the lazy path the trigger is
// read traffic, so an unrecovered panic here would not be a one-off crash: it
// would re-fire on every render of a grid containing the bad object.
func TestCardGenerator_recoversFromDecodePanic(t *testing.T) {
	db := newCardTestDB(t)
	g := NewCardGenerator(context.Background(), logrus.New(), panicStore{},
		mediavariant.NewAdministrator(db), variantfailures.New(logrus.New(), db), 4)

	g.Generate(cardSource("m1"))

	waitFor(t, "the panicking attempt to finish", func() bool { return !g.inFlightFor("m1") })
	waitFor(t, "the cap slot to be released", func() bool { return len(g.sem) == 0 })
}
