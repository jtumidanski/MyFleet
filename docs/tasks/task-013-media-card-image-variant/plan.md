# Media `card` Image Variant — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a third derived image variant, `card` (768px max edge), generate it for new uploads and lazily for pre-existing media, and point the vehicles-list hero at it — so the list photo stops looking soft without regressing per-page byte cost.

**Architecture:** `mediavariant` gains a `card` constant and an additive `Upsert` guarded by a unique `(media_object_id, variant)` index. A new `variantfailures` ledger package records generations that can never succeed. `processing` gains a `CardGenerator` that single-flights per media object behind a global semaphore and reuses the worker's decode/resize/encode/upload code, which is refactored from `*Worker` methods into package-level functions. `mediaobject.Processor` declares a `CardGenerator` port (implemented in the composition root, as `VariantLookup` already is), extracts its variant-serving half into `openVariant`, and adds exactly one narrow downgrade: a `card` that cannot be served falls back to `thumbnail` and nothing else. The frontend widens its `MediaVariant` union and switches the one component that renders a card hero.

**Tech Stack:** Go 1.x + GORM (Postgres in production, SQLite in tests), logrus, `golang.org/x/image/draw`; React + TypeScript + Vitest + TanStack React Query; kustomize for `deploy/k8s`.

## Global Constraints

- `card` max edge is **768**. `thumbnail` (320) and `display` (1280) are unchanged, as are their call sites.
- The downgrade is **`card → thumbnail` only**. A missing `display` still 404s; a missing `thumbnail` still 404s. There is no general ladder, and nothing **larger** than what was asked for is ever served.
- `?variant=original` and the no-parameter form must stay byte-identical on the wire, `Content-Length` included. `TestContent_originalIsUnchanged` and `MediaService.test.ts` pin this and must not be edited.
- A variant response never carries `Content-Length` — `media_variants` records no byte count.
- Lazy generation must **never** call `mediavariant.Administrator.ReplaceForMediaObject`: it deletes every row for the media object first, which would destroy that object's `thumbnail` and `display`.
- Any `clause.OnConflict` on `media_variants` must use an explicit `DoUpdates` column list that **excludes `created_at`**, and must not use `UpdateAll: true`. `entityguard` only recognises `.Save(` call sites, so it will not catch this.
- Background generation must not use the HTTP request's context, and must be scheduled only **after** `GetByID` has authorized the caller.
- Config key: `MEDIA_LAZY_VARIANT_CONCURRENCY`, default `4`. `0` disables lazy generation entirely; negatives clamp to `0`.
- Ledger reasons are short constants (`"undecodable"`, `"original-missing"`), never raw error text.
- Log levels: `Info` on a completed generation; `Warn` on permanent failure, transient failure, and store drift; `Debug` on every skip and on the downgrade itself.
- Verification command for the whole branch is `make ci`. Node may need `export NVM_DIR="$HOME/.nvm" && . "$NVM_DIR/nvm.sh" && nvm use 22` first.
- All work happens in the worktree `/home/tumidanski/source/MyFleet/.worktrees/task-013-media-card-image-variant` on branch `task-013-media-card-image-variant`.

---

## File Structure

**`apps/media-service`**

| File | Responsibility |
|---|---|
| `internal/mediavariant/model.go` | Add `VariantCard` |
| `internal/mediavariant/entity.go` | Composite unique index on `(media_object_id, variant)` |
| `internal/mediavariant/administrator.go` | Add `Upsert` — additive single-variant write |
| `internal/mediavariant/administrator_test.go` (new) | `Upsert` insert/update, `created_at` preservation, other variants untouched |
| `internal/mediavariant/provider_test.go` | Test DDL gains the `UNIQUE` constraint |
| `internal/mediaobject/contentvariant.go` | Add `ContentCard`; accept `"card"` |
| `internal/mediaobject/processor.go` | `CardSource`/`CardGenerator` port, `ProcessorOption`, `openVariant`, downgrade, `scheduleCard` |
| `internal/mediaobject/resource.go` | `InitializeRoutes` forwards `...ProcessorOption` |
| `internal/processing/worker.go` | `cardMaxEdge`; third spec entry; `Source`; `decodeOriginal`/`buildVariant` as package functions |
| `internal/processing/card.go` (new) | `CardGenerator`: single-flight, global cap, failure recording |
| `internal/variantfailures/variantfailures.go` (new) | Permanent-failure ledger (`Entity`, `Store`, `Record`, `Recorded`, `Migration`) |
| `cmd/main.go` | Ledger migration, concurrency config, generator construction, `mediaobject.CardGenerator` adapter |

**`apps/web`**

| File | Responsibility |
|---|---|
| `src/types/models/media.ts` | `MediaVariant` gains `'card'` |
| `src/components/features/vehicles/VehiclePhotoThumbnail.tsx` | Request `'card'` |
| `src/services/api/MediaService.ts` | Route + no-fallback doc comments record the `card → thumbnail` exception |
| `src/lib/hooks/api/media.ts` | Key-factory comment refreshed |

**`deploy/k8s`** — `base/media-service/configmap.yaml` gains `MEDIA_LAZY_VARIANT_CONCURRENCY`.

**`docs/tasks/task-013-media-card-image-variant`** — `deploy-preflight.md` (duplicate-row check) and `manual-verification.md`.

---

## Task 1: The `card` variant itself

New uploads produce a `card` rendition, and the content endpoint accepts `?variant=card`. Nothing lazy yet.

**Files:**
- Modify: `apps/media-service/internal/mediavariant/model.go:8-11`
- Modify: `apps/media-service/internal/mediaobject/contentvariant.go:10-36`
- Modify: `apps/media-service/internal/processing/worker.go:29-32,160-181`
- Test: `apps/media-service/internal/mediaobject/contentvariant_test.go`
- Test: `apps/media-service/internal/processing/worker_test.go`

**Interfaces:**
- Consumes: nothing.
- Produces: `mediavariant.VariantCard Variant = "card"`; `mediaobject.ContentCard ContentVariant = "card"`; unexported `processing.cardMaxEdge = 768`. `fakeVariantAdmin` in `worker_test.go` gains fields `replaceCalls int` and `replaced []mediavariant.Model`.

- [ ] **Step 1: Extend the `ParseContentVariant` test**

In `apps/media-service/internal/mediaobject/contentvariant_test.go`, add `"card": ContentCard,` to the `valid` map and `"Card"`, `"cards"` to the rejected slice. The map and slice become:

```go
	valid := map[string]ContentVariant{
		"":          ContentOriginal, // parameter absent, or ?variant= with no value
		"original":  ContentOriginal,
		"thumbnail": ContentThumbnail,
		"display":   ContentDisplay,
		"card":      ContentCard,
	}
```

```go
	for _, raw := range []string{"Thumbnail", "THUMBNAIL", "bogus", "small", " thumbnail", "Card", "cards"} {
```

- [ ] **Step 2: Run it to verify it fails**

Run: `go test ./apps/media-service/internal/mediaobject/ -run TestParseContentVariant`
Expected: FAIL — `undefined: ContentCard`.

- [ ] **Step 3: Add `ContentCard` and its parse arm**

In `apps/media-service/internal/mediaobject/contentvariant.go`, add the constant and the case:

```go
const (
	ContentOriginal  ContentVariant = "original"
	ContentThumbnail ContentVariant = "thumbnail"
	ContentCard      ContentVariant = "card"
	ContentDisplay   ContentVariant = "display"
)
```

```go
	case string(ContentCard):
		return ContentCard, nil
```

Place the new `case` between the `ContentThumbnail` and `ContentDisplay` arms. Leave the `default: return "", server.ErrBadRequest` untouched.

- [ ] **Step 4: Run it to verify it passes**

Run: `go test ./apps/media-service/internal/mediaobject/ -run TestParseContentVariant`
Expected: PASS.

- [ ] **Step 5: Write the failing `ResizeDims` tests at 768**

Append to `apps/media-service/internal/processing/worker_test.go`:

```go
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
```

- [ ] **Step 6: Run it to verify it fails**

Run: `go test ./apps/media-service/internal/processing/ -run TestResizeDims_cardMaxEdge`
Expected: FAIL — `undefined: cardMaxEdge`.

- [ ] **Step 7: Add `cardMaxEdge`**

In `apps/media-service/internal/processing/worker.go`, replace the const block at lines 29-32:

```go
const (
	thumbnailMaxEdge = 320
	// 768 covers the vehicles-list hero at 1x out to roughly a 2600px viewport
	// and at 2x out to roughly 1450px, while staying ~2.8x cheaper in pixels
	// than display. See the task-013 PRD §8.1 for the arithmetic.
	cardMaxEdge    = 768
	displayMaxEdge = 1280
)
```

- [ ] **Step 8: Run it to verify it passes**

Run: `go test ./apps/media-service/internal/processing/ -run TestResizeDims`
Expected: PASS (all `TestResizeDims*` tests).

- [ ] **Step 9: Widen `fakeVariantAdmin` to record what it was given**

The existing fake records only a `called` bool, which cannot express "exactly three variants, one of each kind". In `apps/media-service/internal/processing/worker_test.go`, replace the `fakeVariantAdmin` block (currently at lines 62-68):

```go
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
```

- [ ] **Step 10: Add an image-bytes test helper**

Append to `apps/media-service/internal/processing/worker_test.go`:

```go
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
```

Add `"image"` and `"image/png"` to the test file's import block.

- [ ] **Step 11: Write the failing three-variant test**

Append to `apps/media-service/internal/processing/worker_test.go`:

```go
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
```

- [ ] **Step 12: Run them to verify they fail**

Run: `go test ./apps/media-service/internal/processing/ -run 'TestHandle_generatesThumbnailCardAndDisplay|TestHandle_pngOriginalProducesPngCard'`
Expected: FAIL — `undefined: mediavariant.VariantCard`.

- [ ] **Step 13: Add `VariantCard`**

In `apps/media-service/internal/mediavariant/model.go`, replace the const block:

```go
const (
	VariantThumbnail Variant = "thumbnail" // max edge 320
	VariantCard      Variant = "card"      // max edge 768
	VariantDisplay   Variant = "display"   // max edge 1280
)
```

- [ ] **Step 14: Add the third worker spec entry**

In `apps/media-service/internal/processing/worker.go`, replace lines 168-175:

```go
	built := make([]mediavariant.Model, 0, 3)
	for _, spec := range []struct {
		kind    mediavariant.Variant
		maxEdge int
	}{
		{mediavariant.VariantThumbnail, thumbnailMaxEdge},
		{mediavariant.VariantCard, cardMaxEdge},
		{mediavariant.VariantDisplay, displayMaxEdge},
	} {
```

Also update the package doc comment on line 2 from `(thumbnail, display)` to `(thumbnail, card, display)`.

- [ ] **Step 15: Run them to verify they pass**

Run: `go test ./apps/media-service/internal/processing/ ./apps/media-service/internal/mediaobject/ ./apps/media-service/internal/mediavariant/`
Expected: PASS (three packages, all tests).

- [ ] **Step 16: Commit**

```bash
git add apps/media-service/internal/mediavariant/model.go \
        apps/media-service/internal/mediaobject/contentvariant.go \
        apps/media-service/internal/mediaobject/contentvariant_test.go \
        apps/media-service/internal/processing/worker.go \
        apps/media-service/internal/processing/worker_test.go
git commit -m "feat(media-service): add the card image variant at a 768px max edge"
```

---

## Task 2: Additive `Upsert` and a unique `(media_object_id, variant)`

Lazy generation needs a write that adds one variant without touching the others. `ReplaceForMediaObject` cannot do it — it deletes first. A database-level unique index makes the racing case structurally impossible rather than merely unlikely.

**Files:**
- Modify: `apps/media-service/internal/mediavariant/entity.go:10-19`
- Modify: `apps/media-service/internal/mediavariant/administrator.go`
- Modify: `apps/media-service/internal/mediavariant/provider_test.go:27-38`
- Modify: `apps/media-service/internal/processing/worker_test.go` (interface-widening stub)
- Create: `apps/media-service/internal/mediavariant/administrator_test.go`
- Create: `docs/tasks/task-013-media-card-image-variant/deploy-preflight.md`

**Interfaces:**
- Consumes: `mediavariant.VariantCard` (Task 1).
- Produces: `mediavariant.Administrator.Upsert(m Model) error` — additive single-variant write, idempotent on `(media_object_id, variant)`, never rewrites `created_at`. `fakeVariantAdmin` gains `upsertCalls int` and `upserted []mediavariant.Model`.

- [ ] **Step 1: Add the `UNIQUE` constraint to the SQLite test DDL**

The GORM tag and this raw DDL are two independent statements of the same schema. A suite missing the constraint would be green on a code path production rejects. In `apps/media-service/internal/mediavariant/provider_test.go`, replace the `CREATE TABLE` statement:

```go
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
```

Extend the helper's doc comment with a sentence naming why the constraint is duplicated here:

```go
// The UNIQUE (media_object_id, variant) constraint mirrors the composite
// uniqueIndex tag on Entity. It is restated here because AutoMigrate does not
// run in these tests, and without it SQLite rejects Upsert's ON CONFLICT clause
// outright — so a suite that omitted it would pass against a schema production
// does not have.
```

- [ ] **Step 2: Write the failing `Upsert` tests**

Create `apps/media-service/internal/mediavariant/administrator_test.go`:

```go
package mediavariant

import (
	"context"
	"testing"
	"time"
)

// TestUpsert_insertsThenUpdatesWithoutDuplicating is the contract lazy variant
// generation leans on: calling it twice for the same (media object, variant)
// leaves exactly one row, updated in place. Two rows would make the content
// endpoint's First() pick arbitrarily between them.
func TestUpsert_insertsThenUpdatesWithoutDuplicating(t *testing.T) {
	db := newVariantTestDB(t)
	admin := NewAdministrator(db)

	first, err := NewBuilder().
		SetMediaObjectID("m1").
		SetVariant(VariantCard).
		SetObjectKey("fleet-a/m1/card.jpg").
		SetWidth(768).
		SetHeight(384).
		SetContentType("image/jpeg").
		Build()
	if err != nil {
		t.Fatalf("build first: %v", err)
	}
	if err := admin.Upsert(first); err != nil {
		t.Fatalf("first Upsert: %v", err)
	}

	second, err := NewBuilder().
		SetMediaObjectID("m1").
		SetVariant(VariantCard).
		SetObjectKey("fleet-a/m1/card-regenerated.jpg").
		SetWidth(768).
		SetHeight(400).
		SetContentType("image/png").
		Build()
	if err != nil {
		t.Fatalf("build second: %v", err)
	}
	if err := admin.Upsert(second); err != nil {
		t.Fatalf("second Upsert: %v", err)
	}

	rows, err := NewProvider(db).ListByMediaObject("m1")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("after two Upserts got %d rows, want exactly 1", len(rows))
	}
	got := rows[0]
	if got.ObjectKey() != "fleet-a/m1/card-regenerated.jpg" {
		t.Fatalf("ObjectKey = %q, want the second write's key", got.ObjectKey())
	}
	if got.Height() != 400 {
		t.Fatalf("Height = %d, want 400", got.Height())
	}
	if got.ContentType() != "image/png" {
		t.Fatalf("ContentType = %q, want image/png", got.ContentType())
	}
}

// TestUpsert_preservesCreatedAt guards the column-wipe class of defect task-006
// existed to eliminate, in its OnConflict disguise: listing created_at in
// DoUpdates (or using UpdateAll) would silently rewrite the row's age on every
// regeneration. entityguard cannot see this — it recognises .Save( call sites
// only — so this test is the only thing standing behind it.
func TestUpsert_preservesCreatedAt(t *testing.T) {
	db := newVariantTestDB(t)
	admin := NewAdministrator(db)

	first, err := NewBuilder().
		SetMediaObjectID("m1").
		SetVariant(VariantCard).
		SetObjectKey("fleet-a/m1/card.jpg").
		SetContentType("image/jpeg").
		Build()
	if err != nil {
		t.Fatalf("build first: %v", err)
	}
	if err := admin.Upsert(first); err != nil {
		t.Fatalf("first Upsert: %v", err)
	}
	stored, err := NewProvider(db).GetByMediaObjectAndVariant(context.Background(), "m1", VariantCard)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	originalCreatedAt := stored.CreatedAt()

	// Make the second model's own createdAt provably later, so "unchanged"
	// cannot pass by coincidence of clock resolution.
	time.Sleep(10 * time.Millisecond)
	second, err := NewBuilder().
		SetMediaObjectID("m1").
		SetVariant(VariantCard).
		SetObjectKey("fleet-a/m1/card-regenerated.jpg").
		SetContentType("image/jpeg").
		Build()
	if err != nil {
		t.Fatalf("build second: %v", err)
	}
	if second.CreatedAt().Equal(originalCreatedAt) {
		t.Fatal("test is not meaningful: the second model's createdAt matches the first")
	}
	if err := admin.Upsert(second); err != nil {
		t.Fatalf("second Upsert: %v", err)
	}

	after, err := NewProvider(db).GetByMediaObjectAndVariant(context.Background(), "m1", VariantCard)
	if err != nil {
		t.Fatalf("read back after update: %v", err)
	}
	if !after.CreatedAt().Equal(originalCreatedAt) {
		t.Fatalf("created_at moved from %v to %v; the upsert must not rewrite it",
			originalCreatedAt, after.CreatedAt())
	}
}

// TestUpsert_leavesOtherVariantsUntouched is the regression guard for the single
// most damaging way to get lazy generation wrong: reaching for
// ReplaceForMediaObject, which deletes every row for the media object first and
// would destroy the thumbnail and display this object already has.
func TestUpsert_leavesOtherVariantsUntouched(t *testing.T) {
	db := newVariantTestDB(t)
	admin := NewAdministrator(db)

	seeded := make([]Model, 0, 2)
	for _, spec := range []struct {
		v   Variant
		key string
	}{
		{VariantThumbnail, "fleet-a/m1/thumbnail.jpg"},
		{VariantDisplay, "fleet-a/m1/display.jpg"},
	} {
		m, err := NewBuilder().
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

	card, err := NewBuilder().
		SetMediaObjectID("m1").
		SetVariant(VariantCard).
		SetObjectKey("fleet-a/m1/card.jpg").
		SetContentType("image/jpeg").
		Build()
	if err != nil {
		t.Fatalf("build card: %v", err)
	}
	if err := admin.Upsert(card); err != nil {
		t.Fatalf("Upsert card: %v", err)
	}

	rows, err := NewProvider(db).ListByMediaObject("m1")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	kinds := map[Variant]bool{}
	for _, r := range rows {
		kinds[r.Variant()] = true
	}
	for _, want := range []Variant{VariantThumbnail, VariantCard, VariantDisplay} {
		if !kinds[want] {
			t.Fatalf("after Upsert the %s row is gone; rows present: %v", want, kinds)
		}
	}
	if len(rows) != 3 {
		t.Fatalf("got %d rows, want 3", len(rows))
	}
}

// A different variant of the same object is a different row, not a conflict.
func TestUpsert_scopesConflictToTheVariant(t *testing.T) {
	db := newVariantTestDB(t)
	admin := NewAdministrator(db)

	for _, v := range []Variant{VariantThumbnail, VariantCard} {
		m, err := NewBuilder().
			SetMediaObjectID("m1").
			SetVariant(v).
			SetObjectKey("fleet-a/m1/" + string(v) + ".jpg").
			SetContentType("image/jpeg").
			Build()
		if err != nil {
			t.Fatalf("build %s: %v", v, err)
		}
		if err := admin.Upsert(m); err != nil {
			t.Fatalf("Upsert %s: %v", v, err)
		}
	}

	rows, err := NewProvider(db).ListByMediaObject("m1")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("got %d rows, want 2 — thumbnail and card must not collide", len(rows))
	}
}
```

- [ ] **Step 3: Run them to verify they fail**

Run: `go test ./apps/media-service/internal/mediavariant/`
Expected: FAIL — `admin.Upsert undefined (type Administrator has no field or method Upsert)`.

- [ ] **Step 4: Add the composite unique index tag**

In `apps/media-service/internal/mediavariant/entity.go`, replace the `Entity` struct:

```go
// Entity maps to media.media_variants (PRD §6). (media_object_id, variant) is
// unique: a media object has at most one rendition of each kind. The constraint
// is what makes Upsert's additive write safe against two processes racing to
// generate the same variant — which a rolling update can transiently produce,
// since each pod has its own in-process single-flight map.
//
// The plain index on MediaObjectID is kept even though the composite index
// leads with the same column: AutoMigrate never drops indexes, so removing the
// tag would leave an orphan in deployed databases while changing nothing.
type Entity struct {
	ID            string `gorm:"type:uuid;primaryKey"`
	MediaObjectID string `gorm:"type:uuid;not null;index;uniqueIndex:ux_media_variants_object_variant"`
	Variant       string `gorm:"not null;uniqueIndex:ux_media_variants_object_variant"`
	ObjectKey     string `gorm:"not null"`
	Width         int
	Height        int
	ContentType   string
	CreatedAt     time.Time
}
```

- [ ] **Step 5: Add `Upsert` to the interface and its implementation**

In `apps/media-service/internal/mediavariant/administrator.go`, replace the whole file:

```go
package mediavariant

import (
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Administrator is the write interface for media-variant data access.
type Administrator interface {
	// ReplaceForMediaObject deletes any existing variants for the media object
	// and inserts the given set, in one transaction. This keeps variant
	// generation idempotent: a re-delivered media.uploaded event regenerates
	// and overwrites the variants rather than duplicating them.
	ReplaceForMediaObject(mediaObjectID string, variants []Model) error

	// Upsert writes a single variant additively, leaving every other variant of
	// the same media object untouched. It keys on the unique
	// (media_object_id, variant) index, so two processes racing to generate the
	// same variant leave exactly one row rather than two.
	//
	// This is the ONLY write lazy generation may use. ReplaceForMediaObject
	// deletes every row for the media object before inserting, so calling it
	// with a single card model would destroy that object's thumbnail and
	// display.
	Upsert(m Model) error
}

type dbAdministrator struct{ db *gorm.DB }

// NewAdministrator returns an Administrator backed by the given database.
func NewAdministrator(db *gorm.DB) Administrator { return &dbAdministrator{db: db} }

func (a *dbAdministrator) ReplaceForMediaObject(mediaObjectID string, variants []Model) error {
	return a.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("media_object_id = ?", mediaObjectID).Delete(&Entity{}).Error; err != nil {
			return err
		}
		for _, m := range variants {
			e := m.ToEntity()
			if err := tx.Create(&e).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func (a *dbAdministrator) Upsert(m Model) error {
	e := m.ToEntity()
	// DoUpdates names its columns explicitly. created_at is deliberately absent
	// and UpdateAll is deliberately not used: either would rewrite the row's age
	// on every regeneration, which is the column-wipe defect task-006 existed to
	// eliminate. entityguard will not catch it here — it recognises .Save( call
	// sites only — so the explicit list is the whole guard, alongside
	// TestUpsert_preservesCreatedAt.
	return a.db.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "media_object_id"}, {Name: "variant"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"object_key", "width", "height", "content_type",
		}),
	}).Create(&e).Error
}
```

- [ ] **Step 6: Add the `Upsert` stub to `fakeVariantAdmin`**

Widening the interface breaks the fake in the sibling package. In `apps/media-service/internal/processing/worker_test.go`, extend the struct and add the method:

```go
// fakeVariantAdmin implements mediavariant.Administrator; records calls and the
// models it was handed, so a test can assert exactly which variants were built.
type fakeVariantAdmin struct {
	called       bool
	replaceCalls int
	replaced     []mediavariant.Model
	upsertCalls  int
	upserted     []mediavariant.Model
}

func (f *fakeVariantAdmin) ReplaceForMediaObject(_ string, variants []mediavariant.Model) error {
	f.called = true
	f.replaceCalls++
	f.replaced = variants
	return nil
}

func (f *fakeVariantAdmin) Upsert(m mediavariant.Model) error {
	f.upsertCalls++
	f.upserted = append(f.upserted, m)
	return nil
}
```

- [ ] **Step 7: Run the tests to verify they pass**

Run: `go test ./apps/media-service/...`
Expected: PASS.

- [ ] **Step 8: Write the deploy pre-flight note**

`Migration` is `AutoMigrate` run inside `database.Connect`, and a failure there is `log.Fatal` in `main`. If any deployed database already holds duplicate `(media_object_id, variant)` rows, `CREATE UNIQUE INDEX` fails and media-service will not boot. Failing loudly is correct; being surprised by it is not.

Create `docs/tasks/task-013-media-card-image-variant/deploy-preflight.md`:

```markdown
# task-013 deploy pre-flight — unique index on `media.media_variants`

This task adds `uniqueIndex:ux_media_variants_object_variant` over
`(media_object_id, variant)`. GORM `AutoMigrate` runs inside
`database.Connect`, and a migration failure is `log.Fatal` in `main` — so if a
target database already holds duplicate rows, **media-service will not boot**.

Duplicates should not exist: `ReplaceForMediaObject` is a transactional
delete-then-insert. But two concurrent redeliveries under READ COMMITTED can
each delete a snapshot that does not include the other's inserts. Unlikely, not
impossible — so check before deploying.

## 1. Check each target database

```sql
SELECT media_object_id, variant, count(*)
FROM media.media_variants
GROUP BY 1, 2 HAVING count(*) > 1;
```

Zero rows → nothing to do; deploy.

## 2. If it returns rows, de-dupe (keep the newest `created_at`)

```sql
DELETE FROM media.media_variants v
USING media.media_variants keep
WHERE v.media_object_id = keep.media_object_id
  AND v.variant        = keep.variant
  AND (v.created_at, v.id) < (keep.created_at, keep.id);
```

Re-run the check in step 1 — it must return zero rows — then deploy.

The newest row is the right one to keep: it was written by the most recent
successful generation, so its `object_key` is the one whose bytes are certain to
be in storage.
```

- [ ] **Step 9: Commit**

```bash
git add apps/media-service/internal/mediavariant/entity.go \
        apps/media-service/internal/mediavariant/administrator.go \
        apps/media-service/internal/mediavariant/administrator_test.go \
        apps/media-service/internal/mediavariant/provider_test.go \
        apps/media-service/internal/processing/worker_test.go \
        docs/tasks/task-013-media-card-image-variant/deploy-preflight.md
git commit -m "feat(media-service): add an additive variant upsert keyed on a unique (media_object_id, variant)"
```

---

## Task 3: The permanent-failure ledger

A record of "the `card` variant for this media object can never be produced" that survives a restart and is scoped to (media object, variant). Modelled on `processedevents` — a ledger, not a domain aggregate, so it gets an `Entity` and a `Store` rather than the immutable-Model + builder treatment.

**Files:**
- Create: `apps/media-service/internal/variantfailures/variantfailures.go`
- Create: `apps/media-service/internal/variantfailures/variantfailures_test.go`
- Modify: `apps/media-service/cmd/main.go:36-41`

**Interfaces:**
- Consumes: nothing.
- Produces: `variantfailures.New(log logrus.FieldLogger, db *gorm.DB) *Store`; `(*Store).Recorded(mediaObjectID, variant string) (bool, error)`; `(*Store).Record(mediaObjectID, variant, reason string) error`; `variantfailures.Migration(db *gorm.DB) error`; constants `ReasonUndecodable = "undecodable"` and `ReasonOriginalMissing = "original-missing"`.

- [ ] **Step 1: Write the failing ledger tests**

Create `apps/media-service/internal/variantfailures/variantfailures_test.go`:

```go
package variantfailures

import (
	"testing"

	"github.com/sirupsen/logrus"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func newTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	// The entity's TableName is schema-qualified for Postgres; SQLite has no
	// schemas, so attach an in-memory database aliased "media" to make the
	// qualified name resolve — the same approach processedevents takes.
	if err := db.Exec("ATTACH DATABASE ':memory:' AS media").Error; err != nil {
		t.Fatalf("attach media schema: %v", err)
	}
	if err := Migration(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func TestRecordThenRecorded(t *testing.T) {
	s := New(logrus.New(), newTestDB(t))

	recorded, err := s.Recorded("m1", "card")
	if err != nil {
		t.Fatalf("Recorded before any write: %v", err)
	}
	if recorded {
		t.Fatal("Recorded must be false for a media object with no failure")
	}

	if err := s.Record("m1", "card", ReasonUndecodable); err != nil {
		t.Fatalf("Record: %v", err)
	}

	recorded, err = s.Recorded("m1", "card")
	if err != nil {
		t.Fatalf("Recorded after write: %v", err)
	}
	if !recorded {
		t.Fatal("Recorded must be true after Record")
	}
}

// The failure is scoped to (media object, variant): it must not suppress
// generation of any other variant, and it must not leak to another object.
func TestRecorded_isScopedToTheObjectAndVariant(t *testing.T) {
	s := New(logrus.New(), newTestDB(t))
	if err := s.Record("m1", "card", ReasonUndecodable); err != nil {
		t.Fatalf("Record: %v", err)
	}

	for _, c := range []struct{ id, variant string }{
		{"m1", "display"},
		{"m1", "thumbnail"},
		{"m2", "card"},
	} {
		recorded, err := s.Recorded(c.id, c.variant)
		if err != nil {
			t.Fatalf("Recorded(%s,%s): %v", c.id, c.variant, err)
		}
		if recorded {
			t.Fatalf("Recorded(%s,%s) = true; the record must not leak beyond (m1,card)", c.id, c.variant)
		}
	}
}

// First failure wins. Re-recording is a no-op so the original, most informative
// reason is never overwritten by a later one — and so a repeated attempt cannot
// turn into a write amplification loop.
func TestRecord_firstReasonWins(t *testing.T) {
	db := newTestDB(t)
	s := New(logrus.New(), db)

	if err := s.Record("m1", "card", ReasonUndecodable); err != nil {
		t.Fatalf("first Record: %v", err)
	}
	if err := s.Record("m1", "card", ReasonOriginalMissing); err != nil {
		t.Fatalf("second Record must not error: %v", err)
	}

	var rows []Entity
	if err := db.Find(&rows).Error; err != nil {
		t.Fatalf("read rows: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("got %d rows, want exactly 1", len(rows))
	}
	if rows[0].Reason != ReasonUndecodable {
		t.Fatalf("Reason = %q, want the first reason %q", rows[0].Reason, ReasonUndecodable)
	}
}
```

- [ ] **Step 2: Run them to verify they fail**

Run: `go test ./apps/media-service/internal/variantfailures/`
Expected: FAIL — the package does not exist yet (`no Go files in .../variantfailures`).

- [ ] **Step 3: Write the ledger**

Create `apps/media-service/internal/variantfailures/variantfailures.go`:

```go
// Package variantfailures is a ledger of derived-variant generations that can
// never succeed. Lazy card generation consults it before doing any work, so an
// original that does not decode is attempted once rather than on every request
// for the rest of the object's life (task-013 PRD §4.6).
//
// It is a ledger, not a domain aggregate — the same kind of thing as
// processedevents — so it gets an Entity and a Store rather than the
// immutable-Model-plus-builder treatment the domain packages use.
//
// In normal operation this table is empty: for lazy generation to fail
// permanently, a ready image must have an original that is now missing or
// undecodable, yet the processing worker decoded that same original
// successfully to produce its thumbnail and display. It is designed to be cheap
// at rest, not to carry throughput.
package variantfailures

import (
	"time"

	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Recorded reasons. Short constants rather than raw error text: this is a
// diagnostic aid, and error strings can carry object keys and filenames.
const (
	ReasonUndecodable     = "undecodable"
	ReasonOriginalMissing = "original-missing"
)

// Entity maps to media.media_variant_failures. The composite primary key is the
// uniqueness guarantee — no surrogate ID and no extra index needed.
type Entity struct {
	MediaObjectID string `gorm:"type:uuid;primaryKey"`
	Variant       string `gorm:"primaryKey"`
	Reason        string
	FailedAt      time.Time
}

func (Entity) TableName() string { return "media.media_variant_failures" }

// Migration auto-migrates the variant-failure ledger table.
func Migration(db *gorm.DB) error { return db.AutoMigrate(&Entity{}) }

// Store records and queries permanently-failed variant generations.
type Store struct {
	log logrus.FieldLogger
	db  *gorm.DB
}

// New returns a Store backed by the given database.
func New(log logrus.FieldLogger, db *gorm.DB) *Store { return &Store{log: log, db: db} }

// Recorded reports whether generation of this variant for this media object is
// known to be impossible.
func (s *Store) Recorded(mediaObjectID, variant string) (bool, error) {
	var count int64
	err := s.db.Model(&Entity{}).
		Where("media_object_id = ? AND variant = ?", mediaObjectID, variant).
		Count(&count).Error
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// Record notes a permanent failure. First failure wins: re-recording is a no-op
// (ON CONFLICT DO NOTHING), so the original reason is never overwritten by a
// later, less informative one.
//
// Only permanent failures belong here. A transport error from the object store
// or a database error is transient and must NOT be recorded — the next request
// is allowed to try again.
func (s *Store) Record(mediaObjectID, variant, reason string) error {
	return s.db.Clauses(clause.OnConflict{DoNothing: true}).
		Create(&Entity{
			MediaObjectID: mediaObjectID,
			Variant:       variant,
			Reason:        reason,
			FailedAt:      time.Now().UTC(),
		}).Error
}
```

- [ ] **Step 4: Run them to verify they pass**

Run: `go test ./apps/media-service/internal/variantfailures/`
Expected: PASS.

- [ ] **Step 5: Wire the migration into the composition root**

In `apps/media-service/cmd/main.go`, add the import and the migration entry:

```go
	"github.com/jtumidanski/myfleet/apps/media-service/internal/variantfailures"
```

```go
	db, err := database.Connect(log, database.SetMigrations(
		mediaobject.Migration,
		mediavariant.Migration,
		processedevents.Migration,
		variantfailures.Migration,
		events.MigrateOutbox,
	))
```

- [ ] **Step 6: Verify the service still builds and the guard is clean**

Run: `go build ./apps/media-service/... && go test ./apps/media-service/cmd/`
Expected: build succeeds; `TestNoLossySaveRoundTrips` passes (the new package has no `db.Save` path, so it has nothing to report).

- [ ] **Step 7: Commit**

```bash
git add apps/media-service/internal/variantfailures/ apps/media-service/cmd/main.go
git commit -m "feat(media-service): add a permanent variant-failure ledger"
```

---

## Task 4: Extract decode and build into package-level functions

Behaviour-preserving refactor. `decodeOriginal` and `generateVariant` are methods on `*Worker` that close over `w.store` plus four accessors of a `mediaobject.Model`. The lazy generator needs the same code and has no `Model`, so both become package-level functions taking an `ObjectStore` and a `Source`. This is what makes "the lazy path produces the same bytes as the upload path" true by construction rather than by review. The existing worker tests are the regression net.

**Files:**
- Modify: `apps/media-service/internal/processing/worker.go:160-181,211-261`

**Interfaces:**
- Consumes: `processing.cardMaxEdge` (Task 1).
- Produces:
  - `processing.Source struct { MediaObjectID, FleetID, ObjectKey, ContentType string }`
  - `decodeOriginal(ctx context.Context, store ObjectStore, key string) (image.Image, error)`
  - `buildVariant(ctx context.Context, store ObjectStore, s Source, img image.Image, kind mediavariant.Variant, maxEdge int) (mediavariant.Model, error)`

- [ ] **Step 1: Add the `Source` type**

In `apps/media-service/internal/processing/worker.go`, insert after the `ObjectStore` interface (currently ending at line 74):

```go
// Source is everything needed to derive a variant from an original: which media
// object it belongs to, where its bytes live, and what they are.
//
// It exists so decodeOriginal and buildVariant need no mediaobject.Model. The
// worker builds one from the Model it already loaded; the lazy card generator
// receives one across a port from the composition root. Both therefore run the
// exact same decode, resize, encode, key-naming and ErrPermanent-wrapping code,
// which is what makes the lazy path's bytes identical to the upload path's by
// construction rather than by review.
type Source struct {
	MediaObjectID string
	FleetID       string
	ObjectKey     string
	ContentType   string
}
```

- [ ] **Step 2: Convert `decodeOriginal` to a package-level function**

Replace the method at lines 216-230:

```go
func decodeOriginal(ctx context.Context, store ObjectStore, key string) (image.Image, error) {
	rc, err := store.GetObject(ctx, key)
	if err != nil {
		if errors.Is(err, storage.ErrObjectNotFound) {
			return nil, fmt.Errorf("%w: original bytes were never stored: %w", ErrPermanent, err)
		}
		return nil, err
	}
	defer func() { _ = rc.Close() }()
	img, _, err := image.Decode(rc)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrPermanent, err)
	}
	return img, nil
}
```

Leave the doc comment above it exactly as it is (lines 211-215) — the behaviour it describes is unchanged.

- [ ] **Step 3: Convert `generateVariant` to `buildVariant`**

Replace the method at lines 232-261 with:

```go
// buildVariant scales img to the variant's max edge (never upscaling), encodes
// it, uploads it to MinIO under a variant-suffixed key, and returns the variant
// model. The key's discriminator is the variant name, so thumbnail, card and
// display cannot collide.
func buildVariant(ctx context.Context, store ObjectStore, s Source, img image.Image, kind mediavariant.Variant, maxEdge int) (mediavariant.Model, error) {
	b := img.Bounds()
	tw, th := ResizeDims(b.Dx(), b.Dy(), maxEdge)

	dst := image.NewRGBA(image.Rect(0, 0, tw, th))
	xdraw.CatmullRom.Scale(dst, dst.Bounds(), img, b, xdraw.Over, nil)

	contentType, ext := variantEncoding(s.ContentType)
	var buf bytes.Buffer
	if err := encode(&buf, dst, contentType); err != nil {
		return mediavariant.Model{}, err
	}

	key := storage.ObjectKey(s.FleetID, s.MediaObjectID, string(kind)+ext)
	if err := store.PutObject(ctx, key, bytes.NewReader(buf.Bytes()), int64(buf.Len()), contentType); err != nil {
		return mediavariant.Model{}, err
	}

	return mediavariant.NewBuilder().
		SetMediaObjectID(s.MediaObjectID).
		SetVariant(kind).
		SetObjectKey(key).
		SetWidth(tw).
		SetHeight(th).
		SetContentType(contentType).
		Build()
}
```

- [ ] **Step 4: Update `handle` to build a `Source` and call the functions**

In `handle`, replace the step-4 block (currently lines 159-181):

```go
	// Step 4 — generate and persist variants, then transition to ready. The
	// original is decoded ONCE and the decoded image is shared across every
	// spec entry, so adding a variant costs one scale + encode + PUT, not a
	// second decode.
	s := Source{
		MediaObjectID: obj.ID(),
		FleetID:       obj.FleetID(),
		ObjectKey:     obj.ObjectKey(),
		ContentType:   obj.ContentType(),
	}
	img, err := decodeOriginal(ctx, w.store, s.ObjectKey)
	if err != nil {
		if errors.Is(err, ErrPermanent) {
			return w.failPermanently(e, obj, err)
		}
		return fmt.Errorf("decode original %s: %w", s.ObjectKey, err)
	}

	built := make([]mediavariant.Model, 0, 3)
	for _, spec := range []struct {
		kind    mediavariant.Variant
		maxEdge int
	}{
		{mediavariant.VariantThumbnail, thumbnailMaxEdge},
		{mediavariant.VariantCard, cardMaxEdge},
		{mediavariant.VariantDisplay, displayMaxEdge},
	} {
		v, err := buildVariant(ctx, w.store, s, img, spec.kind, spec.maxEdge)
		if err != nil {
			return fmt.Errorf("generate %s variant: %w", spec.kind, err)
		}
		built = append(built, v)
	}
```

- [ ] **Step 5: Run the full processing suite to verify nothing changed**

Run: `go test ./apps/media-service/internal/processing/ -v`
Expected: PASS — every pre-existing test, including `TestHandle_undecodableBytesMarkFailedAndProcessed` and `TestHandle_missingOriginalMarksFailedAndProcessed`, which are what prove the `ErrPermanent` wrapping survived the move.

- [ ] **Step 6: Commit**

```bash
git add apps/media-service/internal/processing/worker.go
git commit -m "refactor(media-service): make variant decode and build reusable outside the worker"
```

---

## Task 5: The lazy `CardGenerator`

Non-blocking, single-flighted per media object, bounded by a global cap, and detached from the request context.

**Files:**
- Create: `apps/media-service/internal/processing/card.go`
- Create: `apps/media-service/internal/processing/card_test.go`

**Interfaces:**
- Consumes: `processing.Source`, `decodeOriginal`, `buildVariant`, `cardMaxEdge` (Tasks 1 & 4); `mediavariant.Administrator.Upsert` and `mediavariant.VariantCard` (Tasks 1 & 2); `variantfailures.Store` with `Record`/`Recorded` and its reason constants (Task 3).
- Produces: `processing.NewCardGenerator(base context.Context, log logrus.FieldLogger, store ObjectStore, variants mediavariant.Administrator, failures *variantfailures.Store, concurrency int) *CardGenerator` and `(*CardGenerator).Generate(src Source)`.

- [ ] **Step 1: Write the failing generator tests**

Create `apps/media-service/internal/processing/card_test.go`:

```go
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
// cancel the generation it caused.
func TestCardGenerator_ignoresACancelledCallerContext(t *testing.T) {
	db := newCardTestDB(t)
	store := &blockingStore{data: pngBytes(t, 2000, 1000)}
	g := NewCardGenerator(context.Background(), logrus.New(), store,
		mediavariant.NewAdministrator(db), variantfailures.New(logrus.New(), db), 4)

	// Stand in for the request context: cancelled the instant the response is
	// written. Generate takes no context at all, which is the point.
	reqCtx, cancel := context.WithCancel(context.Background())
	cancel()
	_ = reqCtx

	g.Generate(cardSource("m1"))

	provider := mediavariant.NewProvider(db)
	waitFor(t, "the card row to be written despite the cancelled request", func() bool {
		_, err := provider.GetByMediaObjectAndVariant(context.Background(), "m1", mediavariant.VariantCard)
		return err == nil
	})
}
```

- [ ] **Step 2: Run them to verify they fail**

Run: `go test ./apps/media-service/internal/processing/ -run TestCardGenerator`
Expected: FAIL — `undefined: NewCardGenerator`.

- [ ] **Step 3: Write the generator**

Create `apps/media-service/internal/processing/card.go`:

```go
package processing

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/jtumidanski/myfleet/apps/media-service/internal/mediavariant"
	"github.com/jtumidanski/myfleet/apps/media-service/internal/storage"
	"github.com/jtumidanski/myfleet/apps/media-service/internal/variantfailures"
)

// generateTimeout bounds one lazy generation. It is ample for decoding, scaling
// and uploading an original at the MEDIA_MAX_UPLOAD_BYTES ceiling, and short
// enough that a wedged object-store call cannot hold a concurrency slot
// indefinitely.
//
// media-service has no graceful-shutdown path — server.Run is
// http.ListenAndServe and SIGTERM kills the process — so in-flight generations
// die with the pod. That is harmless: the Upsert is the only write and it is the
// last step, so nothing is ever half-written, and the next request reschedules.
const generateTimeout = 60 * time.Second

// CardGenerator produces the card variant for media objects that predate it, on
// demand and in the background.
//
// Card only, by construction: a missing thumbnail or display still 404s and
// schedules nothing. The bytes it produces are identical to the upload worker's
// because it calls the same decodeOriginal and buildVariant.
type CardGenerator struct {
	log      logrus.FieldLogger
	store    ObjectStore
	variants mediavariant.Administrator
	failures *variantfailures.Store

	// base is the process lifetime, captured at construction — never a request
	// context. A client that disconnects mid-download must not cancel the work
	// its request triggered (FR-4.4).
	base context.Context
	// sem is the global concurrency cap: a cold twelve-card grid must not spawn
	// twelve simultaneous full-size image decodes. It is acquired with a
	// non-blocking send, so a saturated cap DROPS the request rather than
	// queueing it — including when capacity is 0, where an unbuffered channel
	// with no receiver rejects every send and lazy generation is off entirely.
	sem chan struct{}

	mu sync.Mutex
	// inFlight is keyed by media object ID with no variant component, because
	// card is the only variant that ever enters this path.
	inFlight map[string]struct{}
}

// NewCardGenerator builds a generator bounded by concurrency simultaneous
// generations. base must outlive any request — pass the process context.
//
// concurrency 0 disables lazy generation entirely, and negatives clamp to 0.
// This deviates from the MEDIA_WORKERS precedent, which clamps up to 1, and it
// is deliberate: this feature schedules work in response to request traffic, so
// an operator who sees it misbehave needs an off switch that is not a rollback.
// Disabled is a coherent state — the content endpoint keeps serving the
// thumbnail downgrade, which is exactly the pre-task behaviour.
func NewCardGenerator(
	base context.Context,
	log logrus.FieldLogger,
	store ObjectStore,
	variants mediavariant.Administrator,
	failures *variantfailures.Store,
	concurrency int,
) *CardGenerator {
	if concurrency < 0 {
		concurrency = 0
	}
	return &CardGenerator{
		log:      log,
		store:    store,
		variants: variants,
		failures: failures,
		base:     base,
		sem:      make(chan struct{}, concurrency),
		inFlight: make(map[string]struct{}),
	}
}

// Generate schedules card generation for src and returns immediately, before any
// I/O. It is called while serving an HTTP response, so it must never block: the
// whole point of the lazy path is that the request does not wait for a decode.
//
// Both admission checks are taken synchronously here and released by paired
// defers in the one goroutine that owns them, so no slot can leak. Taking the
// semaphore BEFORE spawning is what makes the cap an actual bound on live
// goroutines rather than something they merely converge on.
func (g *CardGenerator) Generate(src Source) {
	l := g.log.WithField("media_id", src.MediaObjectID)

	if !g.reserve(src.MediaObjectID) {
		l.Debug("card generation already in flight for this media object; skipping")
		return
	}
	select {
	case g.sem <- struct{}{}:
	default:
		g.release(src.MediaObjectID)
		l.Debug("lazy variant concurrency cap saturated; dropping card generation")
		return
	}

	go func() {
		defer func() { <-g.sem }()
		defer g.release(src.MediaObjectID)

		ctx, cancel := context.WithTimeout(g.base, generateTimeout)
		defer cancel()

		// The ledger check sits here rather than in Generate deliberately.
		// Doing it synchronously would put a database round trip on the COMMON
		// path — every downgraded response for the whole lazy-fill period — to
		// guard a case that should never occur. The observable contract holds
		// exactly: a second request performs no decode. What it costs is one
		// goroutine and one indexed read on an almost-always-empty table.
		recorded, err := g.failures.Recorded(src.MediaObjectID, string(mediavariant.VariantCard))
		if err != nil {
			l.WithError(err).Warn("reading the variant-failure ledger failed; skipping card generation")
			return
		}
		if recorded {
			l.Debug("card generation recorded as permanently impossible; skipping")
			return
		}

		img, err := decodeOriginal(ctx, g.store, src.ObjectKey)
		if err != nil {
			if errors.Is(err, ErrPermanent) {
				reason := variantfailures.ReasonUndecodable
				if errors.Is(err, storage.ErrObjectNotFound) {
					reason = variantfailures.ReasonOriginalMissing
				}
				if rerr := g.failures.Record(src.MediaObjectID, string(mediavariant.VariantCard), reason); rerr != nil {
					l.WithError(rerr).Warn("recording a permanent card-generation failure failed")
				}
				l.WithError(err).WithField("reason", reason).
					Warn("card generation failed permanently; it will not be retried")
				return
			}
			// Transient — the store was briefly unreachable, say. Recording it
			// would permanently condemn a perfectly good image.
			l.WithError(err).Warn("card generation failed transiently; a later request may retry")
			return
		}

		v, err := buildVariant(ctx, g.store, src, img, mediavariant.VariantCard, cardMaxEdge)
		if err != nil {
			l.WithError(err).Warn("building the card variant failed; a later request may retry")
			return
		}
		// Upsert, never ReplaceForMediaObject: the latter deletes every row for
		// the media object first and would destroy its thumbnail and display.
		if err := g.variants.Upsert(v); err != nil {
			l.WithError(err).Warn("persisting the card variant failed; a later request may retry")
			return
		}
		l.WithField("object_key", v.ObjectKey()).Info("card variant generated")
	}()
}

// reserve takes the single-flight slot for a media object, reporting false when
// another generation for the same object already holds it (FR-4.1).
func (g *CardGenerator) reserve(mediaObjectID string) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	if _, taken := g.inFlight[mediaObjectID]; taken {
		return false
	}
	g.inFlight[mediaObjectID] = struct{}{}
	return true
}

func (g *CardGenerator) release(mediaObjectID string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	delete(g.inFlight, mediaObjectID)
}

// inFlightFor reports whether a generation for this media object is running. It
// exists so tests can wait for an asynchronous generation to finish instead of
// sleeping a guessed interval.
func (g *CardGenerator) inFlightFor(mediaObjectID string) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	_, taken := g.inFlight[mediaObjectID]
	return taken
}
```

- [ ] **Step 4: Run them to verify they pass, under the race detector**

Run: `go test ./apps/media-service/internal/processing/ -race`
Expected: PASS with no race reports. The concurrency here is the feature, so `-race` is not optional for this task.

- [ ] **Step 5: Commit**

```bash
git add apps/media-service/internal/processing/card.go apps/media-service/internal/processing/card_test.go
git commit -m "feat(media-service): add a single-flighted, capped lazy card-variant generator"
```

---

## Task 6: The downgrade and scheduling in `Processor.Content`

The endpoint half: extract the variant path into `openVariant`, add exactly one downgrade, and schedule generation after authorization.

**Files:**
- Modify: `apps/media-service/internal/mediaobject/processor.go:142-157,309-391`
- Modify: `apps/media-service/internal/mediaobject/resource.go:45-46`
- Test: `apps/media-service/internal/mediaobject/processor_test.go`

**Interfaces:**
- Consumes: `mediaobject.ContentCard` (Task 1).
- Produces:
  - `mediaobject.CardSource struct { MediaObjectID, FleetID, ObjectKey, ContentType string }`
  - `mediaobject.CardGenerator interface { Generate(src CardSource) }`
  - `mediaobject.ProcessorOption func(*Processor)` and `mediaobject.WithCardGenerator(g CardGenerator) ProcessorOption`
  - `NewProcessor(...)` and `InitializeRoutes(...)` both gain a trailing `opts ...ProcessorOption`; all sixteen existing call sites keep compiling unchanged.

- [ ] **Step 1: Write the failing downgrade and scheduling tests**

Append to `apps/media-service/internal/mediaobject/processor_test.go`:

```go
// fakeCardGenerator records what was scheduled, so a test can assert both that
// eligible objects schedule and that ineligible ones do not.
type fakeCardGenerator struct{ scheduled []CardSource }

func (f *fakeCardGenerator) Generate(src CardSource) { f.scheduled = append(f.scheduled, src) }

// ★ The downgrade matrix (NFR-15). The display case is the one that matters
// most: it proves the rule did NOT generalise into a next-smaller-available
// ladder, which would let a detail view asking for display silently receive a
// 768px image with no way to detect it.
func TestContent_cardDowngradesToThumbnailOnly(t *testing.T) {
	t.Run("card missing, thumbnail present, serves the thumbnail bytes", func(t *testing.T) {
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

		info, rc, err := pr.Content(context.Background(), obj.ID(), "fleet-a", ContentCard)
		if err != nil {
			t.Fatalf("Content(card) = %v, want the thumbnail downgrade", err)
		}
		if got := readAllAndClose(t, rc); got != "thumb-bytes" {
			t.Fatalf("body = %q, want thumb-bytes", got)
		}
		// The response must describe the bytes actually sent, resolved through
		// the allowlist exactly as the normal variant path does.
		if info.ContentType != "image/jpeg" {
			t.Fatalf("ContentType = %q, want the thumbnail row's image/jpeg", info.ContentType)
		}
		if info.Size != 0 {
			t.Fatalf("Size = %d, want 0 — no variant response carries Content-Length", info.Size)
		}
	})

	t.Run("card missing and thumbnail missing is 404", func(t *testing.T) {
		db := newConfirmTestDB(t)
		store := &fakeStore{bucket: "myfleet-media", getBody: []byte("original-bytes")}
		variants := &fakeVariants{} // no rows at all
		pr := NewProcessor(logrus.New(), NewProvider(db), NewAdministrator(db), store, variants, testAllowlist(t))
		obj := seedReadyObject(t, pr, "fleet-a", []byte("original-bytes"))

		_, rc, err := pr.Content(context.Background(), obj.ID(), "fleet-a", ContentCard)
		if !errors.Is(err, server.ErrNotFound) {
			t.Fatalf("Content(card) = %v, want ErrNotFound", err)
		}
		if rc != nil {
			_ = rc.Close()
			t.Fatal("Content returned a body alongside the 404")
		}
	})

	t.Run("display missing still 404s even with a thumbnail present", func(t *testing.T) {
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

		_, rc, err := pr.Content(context.Background(), obj.ID(), "fleet-a", ContentDisplay)
		if !errors.Is(err, server.ErrNotFound) {
			t.Fatalf("Content(display) = %v, want ErrNotFound — the downgrade must not generalise", err)
		}
		if rc != nil {
			_ = rc.Close()
			t.Fatal("a missing display served bytes; only card downgrades")
		}
	})

	t.Run("card row present but object missing downgrades AND reschedules", func(t *testing.T) {
		// Store/DB drift. Regenerating repairs it: Upsert rewrites the row and
		// PutObject restores the bytes. Scheduling only on a MISSING row would
		// leave a permanently broken card row that downgrades on every request
		// forever, with no path back.
		db := newConfirmTestDB(t)
		store := &fakeStore{
			bucket:    "myfleet-media",
			getBody:   []byte("original-bytes"),
			getBodies: map[string][]byte{"fleet-a/thumb.jpg": []byte("thumb-bytes")},
			missing:   map[string]bool{"fleet-a/card.jpg": true},
		}
		variants := &fakeVariants{refs: map[string]VariantRef{
			"card":      {ObjectKey: "fleet-a/card.jpg", ContentType: "image/jpeg"},
			"thumbnail": {ObjectKey: "fleet-a/thumb.jpg", ContentType: "image/jpeg"},
		}}
		cards := &fakeCardGenerator{}
		pr := NewProcessor(logrus.New(), NewProvider(db), NewAdministrator(db), store, variants,
			testAllowlist(t), WithCardGenerator(cards))
		obj := seedReadyObject(t, pr, "fleet-a", []byte("original-bytes"))
		ready, err := pr.a.Update(obj.WithStatus(StatusReady))
		if err != nil {
			t.Fatalf("mark ready: %v", err)
		}

		_, rc, err := pr.Content(context.Background(), ready.ID(), "fleet-a", ContentCard)
		if err != nil {
			t.Fatalf("Content(card) with store drift = %v, want the thumbnail downgrade", err)
		}
		if got := readAllAndClose(t, rc); got != "thumb-bytes" {
			t.Fatalf("body = %q, want thumb-bytes", got)
		}
		if len(cards.scheduled) != 1 {
			t.Fatalf("scheduled %d generations for a card row whose object is gone, want 1 — "+
				"regeneration is what repairs the drift", len(cards.scheduled))
		}
	})
}

// A present card is served directly — the downgrade is a fallback, not a
// redirect, and it must not fire when there is nothing to fall back from.
func TestContent_cardPresentServesTheCardBytes(t *testing.T) {
	db := newConfirmTestDB(t)
	store := &fakeStore{
		bucket:    "myfleet-media",
		getBody:   []byte("original-bytes"),
		getBodies: map[string][]byte{"fleet-a/card.jpg": []byte("card-bytes")},
	}
	variants := &fakeVariants{refs: map[string]VariantRef{
		"card": {ObjectKey: "fleet-a/card.jpg", ContentType: "image/jpeg"},
	}}
	cards := &fakeCardGenerator{}
	pr := NewProcessor(logrus.New(), NewProvider(db), NewAdministrator(db), store, variants,
		testAllowlist(t), WithCardGenerator(cards))
	obj := seedReadyObject(t, pr, "fleet-a", []byte("original-bytes"))

	_, rc, err := pr.Content(context.Background(), obj.ID(), "fleet-a", ContentCard)
	if err != nil {
		t.Fatalf("Content(card): %v", err)
	}
	if got := readAllAndClose(t, rc); got != "card-bytes" {
		t.Fatalf("body = %q, want card-bytes", got)
	}
	if len(cards.scheduled) != 0 {
		t.Fatalf("scheduled %d generations for a card that already exists, want 0", len(cards.scheduled))
	}
}

// A database error is a fault, not a miss: it must still 500, with no downgrade
// attempted. A 404 here would hide a real problem, and GetByID just read the
// same database successfully.
func TestContent_cardLookupErrorIs500WithNoDowngrade(t *testing.T) {
	db := newConfirmTestDB(t)
	store := &fakeStore{bucket: "myfleet-media", getBody: []byte("original-bytes")}
	boom := errors.New("database is on fire")
	variants := &fakeVariants{err: boom}
	cards := &fakeCardGenerator{}
	pr := NewProcessor(logrus.New(), NewProvider(db), NewAdministrator(db), store, variants,
		testAllowlist(t), WithCardGenerator(cards))
	obj := seedReadyObject(t, pr, "fleet-a", []byte("original-bytes"))

	_, _, err := pr.Content(context.Background(), obj.ID(), "fleet-a", ContentCard)
	if !errors.Is(err, boom) {
		t.Fatalf("Content(card) = %v, want the underlying lookup error", err)
	}
	if len(variants.calls) != 1 {
		t.Fatalf("lookup ran %v; a fault must not trigger a second, downgrading lookup", variants.calls)
	}
	if len(cards.scheduled) != 0 {
		t.Fatalf("scheduled %d generations on a database fault, want 0", len(cards.scheduled))
	}
}

// Scheduling eligibility (FR-3.2, FR-3.3, NFR-18).
func TestContent_schedulesCardGenerationOnlyWhenEligible(t *testing.T) {
	t.Run("a ready image schedules, with the source the generator needs", func(t *testing.T) {
		db := newConfirmTestDB(t)
		store := &fakeStore{
			bucket:    "myfleet-media",
			getBody:   []byte("original-bytes"),
			getBodies: map[string][]byte{"fleet-a/thumb.jpg": []byte("thumb-bytes")},
		}
		variants := &fakeVariants{refs: map[string]VariantRef{
			"thumbnail": {ObjectKey: "fleet-a/thumb.jpg", ContentType: "image/jpeg"},
		}}
		cards := &fakeCardGenerator{}
		pr := NewProcessor(logrus.New(), NewProvider(db), NewAdministrator(db), store, variants,
			testAllowlist(t), WithCardGenerator(cards))
		obj := seedReadyObject(t, pr, "fleet-a", []byte("original-bytes"))
		ready, err := pr.a.Update(obj.WithStatus(StatusReady))
		if err != nil {
			t.Fatalf("mark ready: %v", err)
		}

		_, rc, err := pr.Content(context.Background(), ready.ID(), "fleet-a", ContentCard)
		if err != nil {
			t.Fatalf("Content(card): %v", err)
		}
		_ = rc.Close()

		if len(cards.scheduled) != 1 {
			t.Fatalf("scheduled %d generations, want 1", len(cards.scheduled))
		}
		got := cards.scheduled[0]
		if got.MediaObjectID != ready.ID() || got.FleetID != "fleet-a" ||
			got.ObjectKey != ready.ObjectKey() || got.ContentType != "image/png" {
			t.Fatalf("CardSource = %+v, want the object's own id/fleet/key/type", got)
		}
	})

	t.Run("a non-ready object schedules nothing", func(t *testing.T) {
		db := newConfirmTestDB(t)
		store := &fakeStore{
			bucket:    "myfleet-media",
			getBody:   []byte("original-bytes"),
			getBodies: map[string][]byte{"fleet-a/thumb.jpg": []byte("thumb-bytes")},
		}
		variants := &fakeVariants{refs: map[string]VariantRef{
			"thumbnail": {ObjectKey: "fleet-a/thumb.jpg", ContentType: "image/jpeg"},
		}}
		cards := &fakeCardGenerator{}
		pr := NewProcessor(logrus.New(), NewProvider(db), NewAdministrator(db), store, variants,
			testAllowlist(t), WithCardGenerator(cards))
		// seedReadyObject leaves the object in the uploaded state; that is
		// exactly the ineligible case.
		obj := seedReadyObject(t, pr, "fleet-a", []byte("original-bytes"))

		_, rc, err := pr.Content(context.Background(), obj.ID(), "fleet-a", ContentCard)
		if err != nil {
			t.Fatalf("Content(card): %v", err)
		}
		_ = rc.Close()

		if len(cards.scheduled) != 0 {
			t.Fatalf("scheduled %d generations for a non-ready object, want 0", len(cards.scheduled))
		}
	})

	t.Run("a document schedules nothing", func(t *testing.T) {
		db := newConfirmTestDB(t)
		store := &fakeStore{bucket: "myfleet-media", getBody: []byte("pdf-bytes")}
		variants := &fakeVariants{}
		cards := &fakeCardGenerator{}
		pr := NewProcessor(logrus.New(), NewProvider(db), NewAdministrator(db), store, variants,
			testAllowlist(t), WithCardGenerator(cards))

		created, err := pr.InitUpload("fleet-a", "u1", "application/pdf", "manual.pdf")
		if err != nil {
			t.Fatalf("init upload: %v", err)
		}
		stored, err := pr.StoreContent(context.Background(), created.ID(), "fleet-a",
			bytes.NewReader([]byte("pdf-bytes")), int64(len("pdf-bytes")))
		if err != nil {
			t.Fatalf("store content: %v", err)
		}
		if _, err := pr.a.Update(stored.WithStatus(StatusReady)); err != nil {
			t.Fatalf("mark ready: %v", err)
		}

		_, _, err = pr.Content(context.Background(), created.ID(), "fleet-a", ContentCard)
		if !errors.Is(err, server.ErrNotFound) {
			t.Fatalf("Content(card) on a document = %v, want ErrNotFound", err)
		}
		if len(cards.scheduled) != 0 {
			t.Fatalf("scheduled %d generations for a document, want 0", len(cards.scheduled))
		}
	})

	t.Run("a cross-fleet caller schedules nothing and never reaches the lookup", func(t *testing.T) {
		db := newConfirmTestDB(t)
		store := &fakeStore{bucket: "myfleet-media", getBody: []byte("original-bytes")}
		variants := &fakeVariants{}
		cards := &fakeCardGenerator{}
		pr := NewProcessor(logrus.New(), NewProvider(db), NewAdministrator(db), store, variants,
			testAllowlist(t), WithCardGenerator(cards))
		obj := seedReadyObject(t, pr, "fleet-a", []byte("original-bytes"))

		_, _, err := pr.Content(context.Background(), obj.ID(), "fleet-b", ContentCard)
		if !errors.Is(err, server.ErrNotFound) {
			t.Fatalf("cross-fleet Content(card) = %v, want ErrNotFound — never 403", err)
		}
		if len(variants.calls) != 0 {
			t.Fatalf("variant lookup ran %v for a cross-fleet caller; authorization must come first", variants.calls)
		}
		if len(cards.scheduled) != 0 {
			t.Fatalf("a caller who cannot read the object scheduled %d generations, want 0", len(cards.scheduled))
		}
	})
}

// With no generator wired — MEDIA_LAZY_VARIANT_CONCURRENCY=0, a supported
// deployment — the downgrade still works and Content does not panic on a nil
// dependency.
func TestContent_downgradesWithNoGeneratorWired(t *testing.T) {
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

	_, rc, err := pr.Content(context.Background(), obj.ID(), "fleet-a", ContentCard)
	if err != nil {
		t.Fatalf("Content(card) with no generator = %v, want the thumbnail downgrade", err)
	}
	if got := readAllAndClose(t, rc); got != "thumb-bytes" {
		t.Fatalf("body = %q, want thumb-bytes", got)
	}
}
```

- [ ] **Step 2: Run them to verify they fail**

Run: `go test ./apps/media-service/internal/mediaobject/ -run 'TestContent_card|TestContent_schedules|TestContent_downgrades'`
Expected: FAIL — `undefined: WithCardGenerator`.

- [ ] **Step 3: Declare the port and the option**

In `apps/media-service/internal/mediaobject/processor.go`, insert after the `VariantLookup` interface (currently ending at line 68):

```go
// CardSource is what the generator needs in order to derive a card variant. It
// crosses the port as plain data so the implementer needs none of mediaobject's
// types — the same shape as VariantRef above.
//
// A named struct rather than four positional strings: four same-typed arguments
// in a row is a swap waiting to happen, and a transposed fleetID/objectKey would
// write a variant under the wrong key.
type CardSource struct {
	MediaObjectID string
	FleetID       string
	ObjectKey     string
	ContentType   string
}

// CardGenerator schedules background generation of a missing card variant.
//
// Generate MUST return without blocking: it is called while serving an HTTP
// response, and the whole point of the lazy path is that the request does not
// wait for a decode. It has no error return because the caller has nothing to do
// with one — the response has already been decided.
//
// Declared here and implemented in the composition root, the same shape as
// VariantLookup, so mediaobject never imports the processing package.
type CardGenerator interface {
	Generate(src CardSource)
}

// nopCardGenerator is the default when no generator is wired
// (MEDIA_LAZY_VARIANT_CONCURRENCY=0). Expressing "lazy generation is off" as a
// no-op implementation rather than a nil check is what keeps Content free of a
// branch that exists only for configuration.
type nopCardGenerator struct{}

func (nopCardGenerator) Generate(CardSource) {}
```

- [ ] **Step 4: Add the option to `Processor` and `NewProcessor`**

Replace the `Processor` struct and constructor (lines 142-157):

```go
// Processor contains media-object business logic, injected with Provider,
// Administrator, and an ObjectStore (MinIO). Event publication is handled by the
// transactional-outbox relay (design A8); the processor never calls Publish
// directly.
type Processor struct {
	log      logrus.FieldLogger
	p        Provider
	a        Administrator
	storage  ObjectStore
	variants VariantLookup
	allow    Allowlist
	cards    CardGenerator
}

// ProcessorOption configures an optional Processor dependency.
type ProcessorOption func(*Processor)

// WithCardGenerator enables lazy generation of missing card variants. It is an
// option rather than a parameter because the dependency is genuinely optional —
// MEDIA_LAZY_VARIANT_CONCURRENCY=0 wires no generator, and that is a supported
// deployment, not a degraded one.
func WithCardGenerator(g CardGenerator) ProcessorOption {
	return func(pr *Processor) { pr.cards = g }
}

func NewProcessor(log logrus.FieldLogger, p Provider, a Administrator, st ObjectStore, variants VariantLookup, allow Allowlist, opts ...ProcessorOption) *Processor {
	pr := &Processor{
		log: log, p: p, a: a, storage: st, variants: variants, allow: allow,
		cards: nopCardGenerator{},
	}
	for _, opt := range opts {
		opt(pr)
	}
	return pr
}
```

- [ ] **Step 5: Extract `openVariant` and restructure `Content`**

Replace `Content` and its doc comment (lines 309-391) with:

```go
// Content authorizes by fleet and opens the requested rendition's bytes for
// streaming to the client. The caller owns closing the returned ReadCloser.
// Bytes are proxied rather than presigned so MinIO stays unreachable from the
// browser.
//
// The media object is resolved and fleet-scoped FIRST, before any variant
// lookup, object-store read, or scheduling decision, so a variant is never
// reachable by a caller who could not read the original (FR-7.5) and a caller
// who cannot read a media object can cause no work to be scheduled for it. A
// cross-fleet caller exits here with 404 — never 403, which would restore the
// existence oracle AuthorizeAccess exists to prevent.
//
// A derived variant that cannot be served is a 404, with exactly one exception:
// a missing card is served as a thumbnail. It does NOT fall back to the
// original. Falling back to the original looks harmless per request and is
// ruinous per page: a twelve-card grid asking for thumbnails would quietly pull
// twelve full-size originals, up to 25 MiB each, which is precisely the cost the
// derived variants exist to avoid. The card exception carries none of that: a
// 320px thumbnail standing in for a 768px card is SMALLER than what was asked
// for, never larger, and it exists because card variants are filled in lazily
// for media uploaded before the variant existed. It is scoped to
// card → thumbnail and generalises no further — a missing display still 404s,
// so a detail view can never silently receive a smaller rendition than it asked
// for.
//
// ?variant=original and a request with no parameter are untouched by any of
// this: they serve the original with its Content-Length exactly as they always
// have. That is the backwards-compatibility contract every pre-existing caller
// depends on.
func (pr *Processor) Content(ctx context.Context, id, identityFleetID string, want ContentVariant) (ContentInfo, io.ReadCloser, error) {
	m, err := pr.GetByID(id, identityFleetID)
	if err != nil {
		return ContentInfo{}, nil, err
	}
	if want == ContentOriginal {
		return pr.openOriginal(ctx, m)
	}

	info, rc, err := pr.openVariant(ctx, m, want)
	if err == nil {
		return info, rc, nil
	}
	// Everything that is not a missing card leaves here: display and thumbnail
	// 404s, and every 500. Only server.ErrNotFound downgrades, so a database or
	// store fault still surfaces as a fault rather than being hidden behind a
	// thumbnail.
	if want != ContentCard || !errors.Is(err, server.ErrNotFound) {
		return ContentInfo{}, nil, err
	}

	pr.scheduleCard(m)
	// Expected and common during the lazy-fill period, so Debug: logging it
	// loudly would be noise.
	pr.log.WithField("media_id", m.ID()).
		Debug("serving the thumbnail in place of an unavailable card variant")
	// 200 with the thumbnail's own bytes, type and disposition — or its own 404
	// if there is no thumbnail either. No third attempt.
	return pr.openVariant(ctx, m, ContentThumbnail)
}

// openVariant opens one stored rendition. It returns server.ErrNotFound for both
// ways a variant can be unservable — no row at all, and a row whose object is
// missing from the store — because the caller's response is the same in both
// cases; only the log level differs, since the second is drift someone should
// see.
func (pr *Processor) openVariant(ctx context.Context, m Model, want ContentVariant) (ContentInfo, io.ReadCloser, error) {
	ref, found, err := pr.variants.Lookup(ctx, m.ID(), string(want))
	if err != nil {
		// A miss is found=false, so an error here means the database is broken
		// — and GetByID just read the same database successfully. A 500 is the
		// honest answer; a 404 would hide a real fault.
		return ContentInfo{}, nil, err
	}
	if !found {
		// Expected whenever processing has not run yet, or the media is not a
		// processable image; debug, not warn.
		pr.log.WithField("media_id", m.ID()).WithField("variant", string(want)).
			Debug("no stored variant for the requested rendition")
		return ContentInfo{}, nil, server.ErrNotFound
	}
	rc, err := pr.storage.GetObject(ctx, ref.ObjectKey)
	switch {
	case err == nil:
		ct := ref.ContentType
		if ct == "" {
			// Should never happen — variants are re-encoded and always record a
			// type — but an empty header is worse than a slightly wrong one.
			ct = m.ContentType()
		}
		// A variant is re-encoded by the worker rather than supplied by a
		// client, but it is still resolved through the allowlist so every
		// response — original or variant — is described by the same trusted
		// vocabulary, and a type nobody recognises degrades to octet-stream +
		// attachment instead of being served as-is.
		resolved, class := pr.allow.Resolve(ct)
		// Size stays 0: media_variants records width/height/content_type but no
		// byte count, so Content-Length is omitted (FR-7.8).
		return ContentInfo{
			ContentType: resolved,
			Disposition: ContentDisposition(class, m.OriginalFilename(), m.ID()),
		}, rc, nil
	case errors.Is(err, storage.ErrObjectNotFound):
		// DB/store drift, unlike the miss above — someone should see it, so this
		// stays a Warn even though the response is the same 404.
		pr.log.WithField("media_id", m.ID()).WithField("variant", string(want)).
			WithField("object_key", ref.ObjectKey).
			Warn("variant row has no object in storage")
		return ContentInfo{}, nil, server.ErrNotFound
	default:
		return ContentInfo{}, nil, err
	}
}

// scheduleCard asks the generator to fill in a missing card variant, if this
// object is one a card can be derived from.
//
// Eligibility lives here rather than in the generator because the processor
// holds the Model and the Allowlist and would otherwise be handing both across
// the port. The generator owns single-flight, the concurrency cap, the failure
// ledger and the work itself, because those are all its own state. Neither needs
// to know the other's rules.
//
// It is called only after GetByID has authorized the caller, and only for
// ContentCard — a missing thumbnail or display schedules nothing.
func (pr *Processor) scheduleCard(m Model) {
	if m.Status() != StatusReady {
		pr.log.WithField("media_id", m.ID()).WithField("status", string(m.Status())).
			Debug("card generation not scheduled: media object is not ready")
		return
	}
	// ClassUnknown fails this check too, so a pre-allowlist row with an
	// unrecognised type is never handed to image.Decode — the same guard Confirm
	// applies before publishing media.uploaded.
	if pr.allow.Classify(m.ContentType()) != ClassImage {
		pr.log.WithField("media_id", m.ID()).
			Debug("card generation not scheduled: media object is not a renderable image")
		return
	}
	pr.cards.Generate(CardSource{
		MediaObjectID: m.ID(),
		FleetID:       m.FleetID(),
		ObjectKey:     m.ObjectKey(),
		ContentType:   m.ContentType(),
	})
}
```

- [ ] **Step 6: Forward options through `InitializeRoutes`**

In `apps/media-service/internal/mediaobject/resource.go`, change lines 45-46:

```go
func InitializeRoutes(log logrus.FieldLogger, db *gorm.DB, st ObjectStore, variants VariantLookup, maxUploadBytes int64, allow Allowlist, opts ...ProcessorOption) func(chi.Router) {
	proc := NewProcessor(log, NewProvider(db), NewAdministrator(db), st, variants, allow, opts...)
```

Keeping the six required parameters and adding options is deliberate: bundling them into a config struct is a reasonable cleanup and is out of scope here.

- [ ] **Step 7: Run the whole media-service suite**

Run: `go test ./apps/media-service/...`
Expected: PASS. In particular `TestContent_originalIsUnchanged`, `TestContent_variantMissingIs404AndServesNoOriginal` and `TestContent_crossFleetNeverTouchesLookupOrStore` must pass **unedited** — they are the contract this task must not disturb.

- [ ] **Step 8: Commit**

```bash
git add apps/media-service/internal/mediaobject/processor.go \
        apps/media-service/internal/mediaobject/processor_test.go \
        apps/media-service/internal/mediaobject/resource.go
git commit -m "feat(media-service): serve a thumbnail for a missing card and schedule its generation"
```

---

## Task 7: Wire the generator in the composition root

**Files:**
- Modify: `apps/media-service/cmd/main.go:86-104,129-140,170-189`
- Modify: `deploy/k8s/base/media-service/configmap.yaml`

**Interfaces:**
- Consumes: `processing.NewCardGenerator` (Task 5); `mediaobject.CardSource`, `mediaobject.WithCardGenerator` (Task 6); `variantfailures.New` (Task 3).
- Produces: an unexported `cardGenerator` adapter in `package main` translating `mediaobject.CardSource` → `processing.Source`.

- [ ] **Step 1: Read the concurrency knob and build the generator**

In `apps/media-service/cmd/main.go`, after the `maxUploadBytes` line (currently line 92), add:

```go
	// Lazy card-variant generation (task-013). 0 disables it entirely and
	// negatives clamp to 0 inside NewCardGenerator — this feature schedules work
	// in response to request traffic, so an operator who sees it misbehave needs
	// an off switch that is not a rollback. With it off, a missing card is still
	// served as a thumbnail; that is simply the pre-task behaviour.
	cardGen := processing.NewCardGenerator(
		ctx,
		log,
		store,
		mediavariant.NewAdministrator(db),
		variantfailures.New(log, db),
		config.GetInt("MEDIA_LAZY_VARIANT_CONCURRENCY", 4),
	)
```

`ctx` is `main`'s `context.Background()` (line 34) — process lifetime, never a request context.

- [ ] **Step 2: Pass the option into the route initializer**

Replace line 138:

```go
					mediaobject.InitializeRoutes(log, db, store,
						variantLookup{p: mediavariant.NewProvider(db)},
						maxUploadBytes, allow,
						mediaobject.WithCardGenerator(cardGenerator{g: cardGen}),
					)(pr)
```

- [ ] **Step 3: Add the adapter**

After the `variantLookup` adapter (currently ending at line 189), add:

```go
// cardGenerator adapts processing.CardGenerator to mediaobject.CardGenerator.
//
// It lives here, in the composition root — the one place that already imports
// both packages — so that mediaobject never imports processing and the two
// sibling packages stay independent, exactly as variantLookup does above.
type cardGenerator struct{ g *processing.CardGenerator }

func (c cardGenerator) Generate(src mediaobject.CardSource) {
	c.g.Generate(processing.Source{
		MediaObjectID: src.MediaObjectID,
		FleetID:       src.FleetID,
		ObjectKey:     src.ObjectKey,
		ContentType:   src.ContentType,
	})
}
```

- [ ] **Step 4: Verify it builds and the guard is clean**

Run: `go build ./apps/media-service/... && go vet ./apps/media-service/... && go test ./apps/media-service/...`
Expected: all pass.

- [ ] **Step 5: Add the ConfigMap key**

The in-code default suffices, but `MEDIA_WORKERS` and `MEDIA_MAX_UPLOAD_BYTES` both have in-code defaults and both appear here, and the ConfigMap is where an operator looks for the knob. In `deploy/k8s/base/media-service/configmap.yaml`, add after `MEDIA_MAX_UPLOAD_BYTES`:

```yaml
  # How many lazy card-variant generations may run at once, across all media
  # objects (task-013). Bounds the cost of a cold vehicles grid: without it,
  # twelve cards could spawn twelve simultaneous full-size image decodes.
  # "0" turns lazy generation off entirely — missing cards are then served as
  # thumbnails, which is the pre-task behaviour and a supported state.
  MEDIA_LAZY_VARIANT_CONCURRENCY: "4"
```

- [ ] **Step 6: Render both overlays**

Run:

```bash
kustomize build deploy/k8s/overlays/local > /dev/null
kustomize build deploy/k8s/overlays/main | grep -c PersistentVolumeClaim
kustomize build deploy/k8s/overlays/main | grep MEDIA_LAZY_VARIANT_CONCURRENCY
```

Expected: `local` renders with no error; the PVC count is `0`; the new key appears in the `main` render.

- [ ] **Step 7: Commit**

```bash
git add apps/media-service/cmd/main.go deploy/k8s/base/media-service/configmap.yaml
git commit -m "feat(media-service): wire lazy card generation and its concurrency knob"
```

---

## Task 8: Frontend — request the `card` variant

Three functional lines and the comments that keep the union and its documentation from drifting apart.

**Files:**
- Modify: `apps/web/src/types/models/media.ts:12-18`
- Modify: `apps/web/src/components/features/vehicles/VehiclePhotoThumbnail.tsx:62-65,81`
- Modify: `apps/web/src/components/features/vehicles/VehiclePhotoThumbnail.test.tsx:48-58`
- Modify: `apps/web/src/services/api/MediaService.ts:17,60-70`
- Modify: `apps/web/src/services/api/MediaService.test.ts`
- Modify: `apps/web/src/lib/hooks/api/media.ts:12-17`

**Interfaces:**
- Consumes: the backend's accepted `?variant=card` (Task 1).
- Produces: `MediaVariant = 'original' | 'thumbnail' | 'display' | 'card'`.

If `npm` is missing: `export NVM_DIR="$HOME/.nvm" && . "$NVM_DIR/nvm.sh" && nvm use 22`.

- [ ] **Step 1: Retarget the component test and add the service test**

In `apps/web/src/components/features/vehicles/VehiclePhotoThumbnail.test.tsx`, replace the variant test (lines 48-58):

```tsx
  it('requests the card variant, not the original', async () => {
    // The whole point of the backend half of this task: a card must cost
    // kilobytes, not the full-size upload — and at a resolution that matches the
    // 16:9 hero it is rendered into, which `thumbnail` (320px) did not.
    vi.mocked(mediaService.getContentBlob).mockResolvedValue(new Blob(['x']));

    renderWithProviders(<VehiclePhotoThumbnail mediaId="m1" vehicleLabel="2019 Honda Civic" />);

    await waitFor(() => {
      expect(mediaService.getContentBlob).toHaveBeenCalledWith('m1', 'card');
    });
  });
```

In `apps/web/src/services/api/MediaService.test.ts`, add alongside the existing variant tests:

```ts
  it("appends ?variant=card for the list card's request", async () => {
    await mediaService.getContentBlob('m1', 'card');
    expect(apiClient.requestBlob).toHaveBeenCalledWith('/api/media/m1/content?variant=card');
  });
```

Leave the existing `thumbnail` and `display` cases in place — the backend still serves both, and `MediaThumbnail` still uses the default.

- [ ] **Step 2: Run them to verify they fail**

Run: `npm --prefix apps/web run test -- --run VehiclePhotoThumbnail MediaService`
Expected: FAIL — the component test reports `'m1', 'thumbnail'` where `'card'` was expected; the service test fails to typecheck `'card'` against `MediaVariant`.

- [ ] **Step 3: Widen the union**

In `apps/web/src/types/models/media.ts`, replace lines 12-18:

```ts
/**
 * Renditions served by GET /api/media/{id}/content?variant=…
 * (apps/media-service/internal/mediaobject/contentvariant.go). Omitting the
 * parameter means 'original'.
 *
 * Sizes are 320 (thumbnail), 768 (card) and 1280 (display) on the longest edge.
 * The vehicles list asks for 'card': its hero is a full-width 16:9 box, which a
 * 320px thumbnail visibly softens, while the full-size upload would cost
 * megabytes per card.
 */
export type MediaVariant = 'original' | 'thumbnail' | 'display' | 'card';
```

- [ ] **Step 4: Switch the component**

In `apps/web/src/components/features/vehicles/VehiclePhotoThumbnail.tsx`, change line 81:

```tsx
  const { url, isLoading, isError } = useMediaContentUrl(mediaId, 'card');
```

and amend the doc comment (lines 62-65):

```tsx
 * Bytes come through the authenticated API as an object URL — a bare <img src>
 * cannot be used because the API requires an Authorization header that the
 * browser will not send for image subresource requests. The card variant
 * (768px longest edge) is requested: it matches the rendered hero at 1x and 2x
 * without costing the full-size upload.
```

- [ ] **Step 5: Update the service documentation**

In `apps/web/src/services/api/MediaService.ts`, line 17:

```ts
 *   GET    /api/media/{id}/content  — stream the bytes (proxied from MinIO);
 *                                     optional ?variant=thumbnail|card|display
```

and the `getContentBlob` doc comment, replacing its last paragraph:

```ts
   * A derived variant that the service cannot produce answers 404; it does NOT
   * fall back to the original
   * (apps/media-service/internal/mediaobject/processor.go).
   *
   * One exception, and only one: a missing `card` is answered with the
   * `thumbnail` bytes rather than a 404, because card variants are filled in
   * lazily for media uploaded before the variant existed. Nothing LARGER than
   * the requested rendition is ever substituted, and the rule does not
   * generalise — a missing `display` still 404s. The response carries no signal
   * that a downgrade happened, so the caller cannot distinguish it; the visible
   * consequence is a slightly soft image until the background generation lands.
```

- [ ] **Step 6: Refresh the key-factory comment**

In `apps/web/src/lib/hooks/api/media.ts`, replace lines 12-17:

```ts
// Hierarchical query-key factory.
// all                       -> ['media']
// detail('m1')              -> ['media', 'detail', 'm1']
// content('m1')             -> ['media', 'content', 'm1', 'original']
// content('m1','card')      -> ['media', 'content', 'm1', 'card']
// vehicleMedia(vehicleId)   -> ['media', 'vehicle', vehicleId]
```

- [ ] **Step 7: Run the frontend suite**

Run: `npm --prefix apps/web run test -- --run`
Expected: PASS, including the untouched `MediaThumbnail` tests — that component renders a gallery tile, not a card hero, and keeps its existing variant.

- [ ] **Step 8: Typecheck and build**

Run: `make fe-build`
Expected: success.

- [ ] **Step 9: Commit**

```bash
git add apps/web/src/types/models/media.ts \
        apps/web/src/components/features/vehicles/VehiclePhotoThumbnail.tsx \
        apps/web/src/components/features/vehicles/VehiclePhotoThumbnail.test.tsx \
        apps/web/src/services/api/MediaService.ts \
        apps/web/src/services/api/MediaService.test.ts \
        apps/web/src/lib/hooks/api/media.ts
git commit -m "feat(web): request the card variant for the vehicles-list hero"
```

---

## Task 9: Full verification and manual-verification notes

**Files:**
- Create: `docs/tasks/task-013-media-card-image-variant/manual-verification.md`

- [ ] **Step 1: Run the full CI target**

Run:

```bash
export NVM_DIR="$HOME/.nvm" && . "$NVM_DIR/nvm.sh" && nvm use 22
make ci
```

Expected: `lint-check`, `vet`, `test`, `build`, `fe-test`, `fe-build` all pass. If `lint-check` reports anything, run `make lint` and re-run `make ci`.

- [ ] **Step 2: Re-run the concurrency-sensitive package under the race detector**

Run: `go test ./apps/media-service/internal/processing/ -race -count=2`
Expected: PASS both runs, no race reports. `-count=2` re-runs without the test cache, which is what catches an ordering-dependent single-flight test.

- [ ] **Step 3: Render both deployment overlays**

Run:

```bash
kustomize build deploy/k8s/overlays/local > /dev/null
kustomize build deploy/k8s/overlays/main > /dev/null
```

Expected: both render without error. Against a reachable cluster, also run both server dry-runs:

```bash
kustomize build deploy/k8s/overlays/main  | kubectl apply --dry-run=server -f -
kustomize build deploy/k8s/overlays/local | kubectl apply --dry-run=server -f -
```

- [ ] **Step 4: Write the manual-verification notes**

Create `docs/tasks/task-013-media-card-image-variant/manual-verification.md`:

```markdown
# task-013 manual verification

Two acceptance criteria are measurements, not assertions, and are not covered by
the automated suite.

## Before deploying

Run the duplicate-row pre-flight in `deploy-preflight.md` against each target
database. A duplicate `(media_object_id, variant)` pair makes the new unique
index fail to create, and media-service will not boot.

## 1. A pre-existing photo becomes sharp on the vehicles list

This is the defect task-007 deferred ("Deferred, note only" in its
`manual-verification.md`).

1. Pick a vehicle whose primary photo was uploaded before this task — it has
   `thumbnail` and `display` rows but no `card` row.
2. Open the vehicles list on a high-DPI display, in a window wide enough for
   `lg:grid-cols-3` (≥1024px), ideally at 1440px where the softness was first
   reported.
3. **First load:** the hero renders from the downgraded thumbnail. Expected —
   this is the current behaviour, not a regression.
4. Confirm a `card` row appeared:
   `SELECT variant, width, height FROM media.media_variants WHERE media_object_id = '<id>';`
   Expect three rows, with `card` at 768 on its longest edge.
5. Confirm the object's `thumbnail` and `display` rows are still there. If they
   are gone, lazy generation used `ReplaceForMediaObject` — a blocking defect.
6. Hard-reload (bypassing both the React Query 5-minute stale window and the
   `Cache-Control: private, max-age=300` browser cache). The hero should now be
   visibly sharper.

## 2. `card` is materially cheaper than `display` (NFR-1)

In DevTools → Network, request the same photo twice and compare transferred
sizes:

- `GET /api/media/<id>/content?variant=card`
- `GET /api/media/<id>/content?variant=display`

`card` must be substantially smaller. 768 vs 1280 on the longest edge is roughly
2.8x fewer pixels, so expect a large fraction of that in bytes. If `card` is
close to `display`, the variant is not earning its keep and the sizing decision
needs revisiting.

Record both numbers here when measured.

## 3. Nothing else changed

- The vehicle detail gallery (`MediaThumbnail`) still sends **no** `?variant=`
  parameter and renders as before.
- `GET /api/media/<id>/content?variant=display` for an object with no display
  row still returns 404 — the downgrade did not generalise.
- `GET /api/media/<id>/content?variant=bogus` still returns 400.
```

- [ ] **Step 5: Confirm the worktree and branch are still correct**

Run:

```bash
git rev-parse --show-toplevel
git branch --show-current
git status --short
```

Expected: the toplevel ends with `/.worktrees/task-013-media-card-image-variant`, the branch is `task-013-media-card-image-variant`, and nothing is unexpectedly modified.

- [ ] **Step 6: Commit**

```bash
git add docs/tasks/task-013-media-card-image-variant/manual-verification.md
git commit -m "docs(task-013): record manual verification steps"
```

- [ ] **Step 7: Run the code review before opening a PR**

Per CLAUDE.md, the code-review step is not optional. Invoke `superpowers:requesting-code-review` (or `/audit-plan`); it dispatches `plan-adherence-reviewer`, `backend-guidelines-reviewer` and `frontend-guidelines-reviewer`, which write to `docs/tasks/task-013-media-card-image-variant/audit.md`.

---

## Requirements that need no code

Two PRD requirements are satisfied by existing behaviour. They have no task, deliberately — a reviewer should not read their absence as an omission.

- **FR-2.4 / "an uploaded PDF still produces zero variant rows."** `Confirm` routes anything that is not `ClassImage` through `MarkReadyDirect` without publishing `media.uploaded` (`processor.go:257-263`), so a document never reaches the worker's variant loop at all. `card` adds no exception to that. The complementary lazy path is covered by Task 6's "a document schedules nothing" sub-test.
- **§6.1 / "no schema change for the `variant` column."** `media_variants.variant` is a plain string with no database enum or check constraint (`entity.go:15`), so `'card'` rows insert with no migration. The only schema change in this plan is the unique index added in Task 2.

## Out of scope

Restated from PRD §2 and design §11, so a reviewer does not have to rediscover it:

- No change to the 320 / 1280 max edges or their call sites.
- No batch or sweep backfill, no event replay, no operator-run migration script.
- No `srcset` / `sizes` / per-breakpoint variant selection.
- No variant byte sizes, and therefore no `Content-Length` on a variant response.
- No generalisation of lazy generation or the downgrade beyond `card`.
- No re-encoding of variants that already exist.
- No change to `apps/fleet-service`.
- PRD §9.6 (a shorter React Query `staleTime` for `card`) and §9.7 (whether
  `VehicleDetailPage` wants `card`) stay open and unaddressed.
