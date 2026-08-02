# Media `card` Image Variant — Design

Task: task-013-media-card-image-variant
PRD: `docs/tasks/task-013-media-card-image-variant/prd.md` (v1, approved)
Status: Draft
Created: 2026-08-02

---

## 1. Shape of the change

The PRD resolves the product questions (768px, lazy generation, `card → thumbnail`
downgrade only). What is left is structural, and it decomposes into five pieces
that can be built and tested independently:

| # | Piece | Package | Depends on |
|---|---|---|---|
| A | The `card` variant constant, parser value, and third worker spec entry | `mediavariant`, `mediaobject`, `processing` | — |
| B | Additive single-variant write (`Upsert`) + uniqueness on `(media_object_id, variant)` | `mediavariant` | — |
| C | Permanent-failure ledger | `variantfailures` (new) | — |
| D | Lazy card generator: single-flight, global cap, failure recording | `processing` | B, C |
| E | Downgrade + scheduling in the content endpoint | `mediaobject` | D (through a port) |
| F | Frontend: union widening, variant switch, doc comments | `apps/web` | A |

A is the smallest useful increment and closes half the acceptance criteria on its
own (new uploads become sharp immediately). B–E are what make pre-existing photos
catch up. F is three lines plus comments. Nothing in D or E is required for A to
ship correctly, which is the natural review boundary if the work is split.

### Dependency direction

`mediaobject` must not import `processing`, and does not today. The existing
`VariantLookup` port (`processor.go:66`) is the established shape: the interface
is declared in the consumer, the implementation lives in the composition root
(`cmd/main.go:178`), and the two sibling domain packages stay independent. The
lazy generator crosses the same way.

```
                       cmd/main.go  (composition root)
                        │        │
        variantLookup ──┘        └── cardGenerator
              │                              │
              ▼                              ▼
   mediavariant.Provider          processing.CardGenerator
                                        │      │       │
                                        │      │       └── variantfailures.Store
                                        │      └────────── mediavariant.Administrator
                                        └───────────────── storage (ObjectStore)

   mediaobject ── declares VariantLookup, CardGenerator ── imports neither impl
```

---

## 2. A — The variant itself

Mechanical, and the only part that touches the hot upload path.

- `mediavariant.VariantCard Variant = "card"` (model.go:8-11), with the existing
  `// max edge NNN` comment convention.
- `processing.cardMaxEdge = 768` (worker.go:29-32).
- `mediaobject.ContentCard ContentVariant = "card"`; `ParseContentVariant` gains a
  `case string(ContentCard)` arm. The `default: return "", server.ErrBadRequest`
  is untouched, so a typo is still 400.
- `worker.handle`'s spec slice gains `{mediavariant.VariantCard, cardMaxEdge}`
  between thumbnail and display (ascending max edge), and `make([]mediavariant.Model, 0, 2)`
  becomes `0, 3`.

Everything else falls out of code that already exists: `ResizeDims` handles 768
with no new branch, `variantEncoding`/`encode` pick PNG-or-JPEG-85 from the
original's content type, and `storage.ObjectKey(fleetID, id, "card"+ext)` cannot
collide because the variant name is the key's discriminator. Persistence stays a
single `ReplaceForMediaObject` call, so redelivery idempotency is unchanged —
three rows instead of two, by the same mechanism.

`ClassDocument` objects never reach this loop (`Confirm` routes them through
`MarkReadyDirect` without publishing `media.uploaded`), so FR-2.4 needs no code.

**Cost check.** The worker decodes the original once (`worker.go:160`) and shares
`src` across every spec entry, so adding `card` adds one scale + encode + PUT per
image, not a second decode. That is the NFR-2 claim, and it is structural rather
than incidental.

---

## 3. B — Additive write and where the race is enforced

`ReplaceForMediaObject` is unusable for lazy generation: it deletes every row for
the media object before inserting (`administrator.go:19-32`), so calling it with
one card model would destroy that object's `thumbnail` and `display`. This is
FR-3.4, and it is the single most damaging way to get this task wrong — a
regression test guards it (§9).

### Decision B1: enforce uniqueness in the database

`mediavariant.Entity` gains a composite unique index:

```go
MediaObjectID string `gorm:"type:uuid;not null;index;uniqueIndex:ux_media_variants_object_variant"`
Variant       string `gorm:"not null;uniqueIndex:ux_media_variants_object_variant"`
```

and `Administrator` gains:

```go
// Upsert writes a single variant additively, leaving every other variant of the
// same media object untouched. It keys on the unique (media_object_id, variant)
// index, so two processes racing to generate the same variant leave exactly one
// row rather than two.
Upsert(m Model) error
```

implemented with `clause.OnConflict{Columns: {media_object_id, variant},
DoUpdates: clause.AssignmentColumns{object_key, width, height, content_type}}`
over `.Create(&e)`.

This is the exact pattern `notification-service/internal/preferences` already
uses (composite `uniqueIndex:ux_pref_user_type` + `OnConflict` upsert,
`administrator.go:29`), so it is house style rather than a new idea.

Three details that are easy to get wrong:

1. **`created_at` must not appear in `DoUpdates`.** GORM would otherwise rewrite
   it on every conflict. This is the same column-wipe class task-006 fixed, in a
   new disguise; `entityguard` will not catch it because the guard recognises
   `.Save(` call sites only (`entityguard.go:19-23`) and explicitly names
   `OnConflict` as its blind spot.
2. **`UpdateAll: true` is forbidden here** for the same reason, and for the same
   documented reason in the guard.
3. **The existing `index` tag on `MediaObjectID` stays.** The composite index has
   `media_object_id` as its leading column and would serve the same lookups, but
   AutoMigrate never drops indexes, so removing the tag leaves an orphan in
   deployed databases while changing nothing. Keeping it is the smaller diff.

**Why the database and not just the in-process single-flight (D3).** `deploy/k8s`
runs media-service at `replicas: 1`, but a rolling update transiently runs the
old and new pods together, and each has its own single-flight map. The window is
small and the consequence (a duplicate row that `First()` then picks
arbitrarily) is mild — which is exactly why it would never be noticed and never
be fixed. A unique index makes it structurally impossible and documents an
invariant the `ReplaceForMediaObject` path already satisfies.

**Alternative rejected — deterministic row ID (UUIDv5 of `media_object_id` +
variant) with `ON CONFLICT (id)`.** No schema change, and the existing primary
key does the work. Rejected because it only helps rows written by the new path:
`ReplaceForMediaObject` inserts with `uuid.NewString()`, so a worker-written
`card` row and a lazily-written one would have different IDs and would not
conflict — the duplicate this is meant to prevent survives. Making it work means
changing ID generation for all three variants, a larger and less obvious change
than an index.

### Risk B2: AutoMigrate creating a unique index over dirty data

`Migration` is `db.AutoMigrate(&Entity{})` (entity.go:23), run inside
`database.Connect`, and a failure there is `log.Fatal` in `main`. If any deployed
database already holds two rows with the same `(media_object_id, variant)`,
`CREATE UNIQUE INDEX` fails and **media-service will not boot**.

Duplicates should not exist — `ReplaceForMediaObject` is transactional
delete-then-insert — but two concurrent redeliveries under READ COMMITTED can
each delete a snapshot that does not include the other's inserts. Unlikely, not
impossible.

Failing loudly at boot is the correct behaviour for a data-integrity change; it
must simply not be a surprise. The plan carries an explicit pre-flight step:

```sql
SELECT media_object_id, variant, count(*)
FROM media.media_variants
GROUP BY 1, 2 HAVING count(*) > 1;
```

run against each target database before the deploy, with a de-dupe (keep newest
`created_at`) if it returns rows.

### Risk B3: the SQLite test DDL is hand-maintained

`mediavariant/provider_test.go:26` creates `media.media_variants` with raw SQL
because "GORM AutoMigrate mishandles schema-qualified table names on SQLite when
the entity carries index tags". So the GORM tag and the test DDL are two
independent statements of the same schema.

The new `UNIQUE (media_object_id, variant)` must be added to that DDL, or every
`Upsert` test fails with SQLite's *"ON CONFLICT clause does not match any PRIMARY
KEY or UNIQUE constraint"* — and, worse, a test suite that omitted the constraint
while production had it would be green on a code path production rejects. SQLite
and Postgres agree on `ON CONFLICT (cols) DO UPDATE` syntax, so with the
constraint present the same GORM clause exercises both.

---

## 4. C — The permanent-failure ledger

FR-6 needs a record of "the `card` variant for this media object can never be
produced" that survives a restart and is scoped to (media object, variant).

### Decision C1: a small dedicated ledger package

New package `apps/media-service/internal/variantfailures`, modelled directly on
`processedevents` — which is the same kind of thing: a ledger, not a domain
aggregate, so it gets an `Entity` and a `Store` rather than the immutable-Model +
builder treatment.

```go
// Entity maps to media.media_variant_failures.
type Entity struct {
    MediaObjectID string    `gorm:"type:uuid;primaryKey"`
    Variant       string    `gorm:"primaryKey"`
    Reason        string
    FailedAt      time.Time
}

type Store struct{ log logrus.FieldLogger; db *gorm.DB }

// Recorded reports whether generation of this variant is known to be impossible.
func (s *Store) Recorded(mediaObjectID, variant string) (bool, error)

// Record notes a permanent failure. First failure wins: re-recording is a no-op
// (ON CONFLICT DO NOTHING), so the original reason is never overwritten by a
// later, less informative one.
func (s *Store) Record(mediaObjectID, variant, reason string) error
```

The composite primary key is the uniqueness guarantee — no surrogate ID, no
extra index. `Migration` joins the list in `cmd/main.go:36-41`.

`Reason` is a short constant string (`"undecodable"`, `"original-missing"`), not
the raw error text: it is a diagnostic aid, and error strings can carry object
keys and filenames.

**Alternatives rejected:**

- *A nullable column on `media_objects`.* Two problems. It puts variant-scoped
  state on the object aggregate, which FR-6.4 explicitly forbids conflating with
  `status`; and `Administrator.Update` persists through `db.Save` of a full
  entity built from the immutable `Model`, so a generator writing the flag
  out-of-band would be silently erased by any concurrent `Update` that saved a
  Model read before the flag was set. That is precisely the column-wipe defect
  task-006 existed to eliminate, and `entityguard` would flag the unassigned
  field — correctly.
- *A sentinel row in `media_variants` with an empty object key.* Least invasive
  to write, worst to read: `ListByMediaObject` hands every row to any caller, and
  a row that looks servable but is not is a trap for code that does not know the
  convention. The PRD reaches the same conclusion in §9.5.
- *An in-memory set with no persistence.* Cheapest, and adequate for the actual
  frequency of these failures — but §6.3 requires surviving a restart, and a
  process restart is exactly when a cold grid re-requests everything.

### Note C2: how rare this is

For lazy generation to fail permanently, a `ready` image must have an original
that is now missing or undecodable — yet the worker decoded that same original
successfully to produce `thumbnail` and `display`. This ledger will be empty in
normal operation. It is designed for cheapness at rest (one indexed read, on a
table that is almost always empty), not for throughput.

---

## 5. D — The lazy generator

New file `apps/media-service/internal/processing/card.go`.

```go
// CardGenerator produces the card variant for media objects that predate it,
// on demand and in the background. Card only, by construction: a missing
// thumbnail or display still 404s and schedules nothing (FR-3.5).
type CardGenerator struct {
    log      logrus.FieldLogger
    store    ObjectStore
    variants mediavariant.Administrator
    failures *variantfailures.Store

    base    context.Context // process lifetime — never a request context
    sem     chan struct{}   // global concurrency cap (FR-4.2)
    mu      sync.Mutex
    inFlight map[string]struct{} // media object IDs currently generating
}

// Source is everything needed to derive a variant from an original.
type Source struct {
    MediaObjectID string
    FleetID       string
    ObjectKey     string
    ContentType   string
}

func (g *CardGenerator) Generate(src Source)
```

### D1: reuse, not duplication

`decodeOriginal` and `generateVariant` are currently methods on `*Worker`
(worker.go:216, 235) and both close over `w.store` only — plus, for
`generateVariant`, four accessors of a `mediaobject.Model`. They become
package-level functions taking `ObjectStore` and a `Source`:

```go
func decodeOriginal(ctx context.Context, store ObjectStore, key string) (image.Image, error)
func buildVariant(ctx context.Context, store ObjectStore, src Source, img image.Image,
                  kind mediavariant.Variant, maxEdge int) (mediavariant.Model, error)
```

`Worker` calls them with a `Source` built from its `mediaobject.Model`; the
generator calls them with the `Source` handed across the port. This keeps the
`ErrPermanent` wrapping, the never-upscale rule, the encoding rules and the key
scheme in exactly one place, which is what makes "the lazy path produces the same
bytes as the upload path" true by construction rather than by review.

The refactor is mechanical and behaviour-preserving; the existing worker tests
are the regression net for it.

### D2: admission, then work

`Generate` is non-blocking and returns before any I/O:

```
Generate(src):
  1. reserve in-flight slot for src.MediaObjectID   — taken? → Debug "already in flight", return
  2. try-acquire semaphore (select/default)          — full? → release (1), Debug "cap saturated, dropped", return
  3. go func():
        defer release semaphore; defer release in-flight slot
        ctx, cancel := context.WithTimeout(g.base, generateTimeout); defer cancel()
        if failures.Recorded(id, "card")             → Debug "permanent failure recorded", return
        img := decodeOriginal(ctx, store, src.ObjectKey)
            ErrPermanent → failures.Record(...); Warn; return
            other        → Warn (transient, not recorded); return
        v := buildVariant(ctx, store, src, img, VariantCard, cardMaxEdge)
            → error is transient (store/encode); Warn; return
        variants.Upsert(v)   → error → Warn; return
        Info "card variant generated"
```

Both reservations are taken synchronously in `Generate` and released by paired
`defer`s in the one goroutine that owns them, so there is no path where a slot
leaks. Taking the semaphore *before* spawning is also what makes FR-4.2 directly
testable: the number of live goroutines can never exceed the cap, rather than
merely converging on it.

- **FR-4.1 (single-flight)** — the `inFlight` map, keyed by media object ID.
  Card is the only variant that enters this path, so the key needs no variant
  component; a comment says so.
- **FR-4.3 (drop, never queue)** — the non-blocking `select`/`default`. A drop
  costs the caller nothing: it has already been served its thumbnail, and the
  next request reschedules.
- **FR-4.4 (not the request context)** — `base` is captured at construction from
  `main`'s `context.Background()`. A client disconnecting mid-download cancels
  nothing.
- **FR-4.5 (bounded lifetime)** — `generateTimeout = 60s`, ample for decoding,
  scaling and uploading a 25 MiB original (the `MEDIA_MAX_UPLOAD_BYTES` ceiling)
  and short enough that a wedged object-store call cannot hold a cap slot
  indefinitely. Worth naming plainly: media-service has no graceful-shutdown
  path today — `server.Run` is `http.ListenAndServe` and SIGTERM kills the
  process — so in-flight generations die with the pod. That is harmless here:
  nothing is half-written (the `Upsert` is the only write and it is the last
  step), and the next request for that object reschedules.

### D3: the failure check runs inside the goroutine, not in `Generate`

FR-6.3 says a recorded permanent failure "schedules nothing". Placing the
`Recorded` lookup in `Generate` would honour that literally, at the cost of a
synchronous database round trip on the *common* path — every downgraded response
during the entire lazy-fill period — to guard against a case that should never
occur (§C2).

The check therefore sits at the top of the goroutine, before any object-store or
decode work. The observable contract the PRD's own acceptance criterion states —
*"a second request does not re-attempt decoding"* — holds exactly. What a
repeat request costs is one goroutine and one indexed read on an empty table,
bounded by single-flight and the cap. An in-process negative cache in front of
the ledger would remove even that; it is deliberately not built, because the
ledger is expected to stay empty.

This is a conscious, narrow reading of FR-6.3's wording and is called out here so
review does not have to rediscover it.

### D4: concurrency configuration

`MEDIA_LAZY_VARIANT_CONCURRENCY`, read via `config.GetInt` in `cmd/main.go`
alongside `MEDIA_WORKERS`, default **4** (PRD FR-4.2).

**`0` disables lazy generation entirely** and negatives clamp to `0`. This
deviates from the `MEDIA_WORKERS` precedent, which clamps up to 1, and it is
deliberate: this feature schedules work in response to unauthenticated-by-volume
request traffic, and an operator who sees it misbehave needs an off switch that
is not a rollback. Disabled is a coherent state, not a broken one — the downgrade
path (§6) keeps serving thumbnails, which is exactly today's behaviour.

The key is also added to `deploy/k8s/base/media-service/configmap.yaml` with its
default value. PRD §7 lists `deploy/k8s` as unaffected on the grounds that an
in-code default suffices; that is true, but `MEDIA_WORKERS` and
`MEDIA_MAX_UPLOAD_BYTES` both have in-code defaults and both appear in the
ConfigMap, and the ConfigMap is where an operator looks for the knob. This is an
additive, value-preserving deviation, flagged rather than silent.

---

## 6. E — Downgrade and scheduling in `Processor.Content`

### E1: the port

Declared in `mediaobject`, beside `VariantLookup`, for the same stated reason:

```go
// CardSource is what the generator needs in order to derive a card variant.
// It crosses the port as plain data so the implementer needs none of
// mediaobject's types — the same shape as VariantRef above.
type CardSource struct {
    MediaObjectID, FleetID, ObjectKey, ContentType string
}

// CardGenerator schedules background generation of a missing card variant.
// Generate MUST return without blocking: it is called while serving an HTTP
// response, and the whole point of the lazy path is that the request does not
// wait for a decode (NFR-4). It has no error return because the caller has
// nothing to do with one — the response has already been decided.
type CardGenerator interface {
    Generate(src CardSource)
}
```

A named struct rather than four positional `string` parameters: four
same-typed arguments in a row is a swap waiting to happen, and a silently
transposed `fleetID`/`objectKey` would write a variant under the wrong key.

The adapter lives in `cmd/main.go` next to `variantLookup`, converting
`mediaobject.CardSource` → `processing.Source`. Six lines, and it is what keeps
`mediaobject` free of any import of `processing`.

### E2: wiring — a functional option, not a seventh parameter

`NewProcessor` gains the generator as an **option**, not a positional parameter:

```go
func NewProcessor(log, p, a, st, variants, allow, opts ...ProcessorOption) *Processor
func WithCardGenerator(g CardGenerator) ProcessorOption
```

`InitializeRoutes` takes and forwards the same variadic. When no option is
supplied the field is set to an unexported `nopCardGenerator{}`, so `Content`
never needs a nil check and "lazy generation is off" is expressed as a no-op
implementation rather than a branch.

Two reasons over the positional parameter that `variants VariantLookup` got:

1. The dependency is genuinely optional. `MEDIA_LAZY_VARIANT_CONCURRENCY=0`
   wires no generator, and that is a supported deployment, not a degraded one.
2. Sixteen existing `NewProcessor` call sites (fifteen of them in tests) would
   otherwise change without any behavioural reason to, burying the three lines
   that actually matter in mechanical churn.

`database.Connect(log, database.SetMigrations(...))` establishes the functional
option pattern in this codebase, so this is not a new idiom either.

`InitializeRoutes` staying at six required parameters plus options is a
deliberate non-refactor: bundling them into a config struct is a reasonable
cleanup and is out of scope here.

### E3: restructuring `Content`

Today `Content` inlines the whole variant path, which is why the downgrade cannot
be expressed without duplicating it. Extract the variant half as-is:

```go
// openVariant opens one stored rendition, returning server.ErrNotFound for both
// ways it can be unservable — no row, and a row whose object is missing — since
// the caller's response is the same in both cases (FR-5.4).
func (pr *Processor) openVariant(ctx, m Model, want ContentVariant) (ContentInfo, io.ReadCloser, error)
```

with the existing lookup-error → 500, miss → Debug + 404, store-drift → Warn +
404 and allowlist/disposition resolution moved wholesale. `Content` then reads:

```go
m, err := pr.GetByID(id, identityFleetID)          // authz first, unchanged
if err != nil { return … }
if want == ContentOriginal { return pr.openOriginal(ctx, m) }

info, rc, err := pr.openVariant(ctx, m, want)
if err == nil { return info, rc, nil }
if want != ContentCard || !errors.Is(err, server.ErrNotFound) {
    return ContentInfo{}, nil, err                  // display/thumbnail 404 and every 500
}
pr.scheduleCard(m)                                  // FR-3.1, after authz (FR-3.2)
pr.log…Debug("serving thumbnail in place of an unavailable card variant")  // NFR-10
return pr.openVariant(ctx, m, ContentThumbnail)     // 200, or 404 if absent (FR-5.5)
```

Properties this shape gives for free, each of which is a PRD requirement:

- **FR-5.2 / no ladder.** The downgrade is guarded by `want != ContentCard`. A
  missing `display` returns before reaching it; a missing `thumbnail` is the
  second `openVariant` call and returns its own 404 with no third attempt.
- **FR-5.3 / never larger.** The only fallback target is a literal
  `ContentThumbnail`, so `processor.go:320-329`'s prohibition on serving the
  multi-megabyte original is untouched — and the doc comment there is amended to
  say exactly that rather than being left contradicting the code.
- **Database and store errors still 500.** Only `server.ErrNotFound` triggers the
  downgrade; the "the database is broken and a 404 would hide it" reasoning in
  the existing comment survives verbatim.
- **FR-5.6 / honest headers.** The second `openVariant` resolves `ContentType`
  and disposition from the thumbnail row through the allowlist, exactly as the
  normal variant path does, so the response describes the bytes actually sent.
- **FR-3.2 / authz first.** `GetByID` has already returned. A cross-fleet caller
  exited with 404 several lines earlier and can schedule nothing.

### E4: eligibility lives in the processor

```go
func (pr *Processor) scheduleCard(m Model) {
    if m.Status() != StatusReady { …Debug("not ready"); return }        // FR-3.3
    if pr.allow.Classify(m.ContentType()) != ClassImage { …Debug; return } // FR-3.3
    pr.cards.Generate(CardSource{…})
}
```

The split is: the processor owns eligibility, because it holds the `Model` and
the `Allowlist` and would otherwise be handing them across the port; the
generator owns single-flight, the cap, the failure ledger and the work itself,
because those are all its own state. Neither needs to know the other's rules.

`ClassUnknown` fails the `ClassImage` check, so a pre-allowlist row with an
unrecognised type is never handed to `image.Decode` — the same guard `Confirm`
applies at `processor.go:257`.

### E5: scheduling when the row exists but the object does not

FR-5.4 puts both unservable cases on the same downgrade path. It leaves open
whether the *scheduling* also applies to the second one; this design says yes,
and the code above does it without a special case.

An existing `card` row whose object is gone is store/DB drift. Regenerating
repairs it: `Upsert` rewrites the row (`DoUpdates` on `object_key`, dimensions
and content type) and `PutObject` restores the bytes under the same key. The
alternative — schedule only on a missing row — leaves a permanently broken card
row that downgrades on every request forever, with no path back. The `Warn` at
`processor.go:381` still fires, so the drift stays visible; it simply also gets
fixed. All of it is bounded by the same single-flight, cap and ledger.

### E6: browser HTTP cache, in addition to the React Query window

`resource.go:208` sets `Cache-Control: private, max-age=300` on every content
response, downgraded ones included. §8.5 of the PRD analyses the React Query
`staleTime` (5 min) but not this. They are the same magnitude and compose to the
same accepted outcome — a pre-existing photo stays soft for the first visit and
sharpens on a later one (NFR-11) — but it means the effective window is bounded
by both caches, not just the one the PRD names. No change: adding a
`Cache-Control` special case for downgraded responses would need the
response-signalling that FR-5.7 rejected.

---

## 7. F — Frontend

Three functional lines and three comments.

- `MediaVariant` (`types/models/media.ts:18`) becomes
  `'original' | 'thumbnail' | 'display' | 'card'`. Nothing switches exhaustively
  on this union, so widening it is safe; it flows into `mediaKeys.content` and
  `getContentBlob` as a parameter type only.
- `VehiclePhotoThumbnail.tsx:81` requests `'card'`.
- `VehiclePhotoThumbnail.test.tsx:48` ("requests the thumbnail variant, not the
  original") is retargeted to `'card'` — the assertion that matters is *not the
  original*, and it keeps that force.
- `MediaService.ts:67-68`'s "does NOT fall back to the original" comment is
  amended with the one exception, naming it as `card → thumbnail` and only that.
  The route comment at `MediaService.ts:17` gains `card` in its variant list.
- `media.ts:15`'s "the list view asks for 'thumbnail'" comment is corrected;
  leaving it is how the union and its documentation drift apart.

`mediaKeys.content` already carries the variant (`media.ts:26-28`), so `card` and
`thumbnail` bytes cannot be served in place of one another and the object-URL
lifecycle needs no thought. `MediaThumbnail` (the gallery tile) is not touched —
it is not a card hero. The component's four render states, box sizing and
no-toast-on-failure property are all untouched by a variant-string change.

---

## 8. Failure modes, end to end

| Situation | Response | Background |
|---|---|---|
| `card` row + object present | 200 card bytes | none |
| No `card` row, ready image, thumbnail present | 200 thumbnail bytes | generation scheduled |
| No `card` row, ready image, no thumbnail | 404 | generation scheduled |
| `card` row present, object missing | 200 thumbnail bytes, `Warn` logged | generation scheduled (repairs drift) |
| No `card` row, document / non-`ready` | 200 thumbnail if one exists, else 404 | none — ineligible |
| Permanent failure recorded | 200 thumbnail bytes | goroutine returns before decode |
| Cap saturated | 200 thumbnail bytes | dropped; next request reschedules |
| Variant lookup raises a DB error | 500 | none — no downgrade on a fault |
| Cross-fleet caller | 404 | none — `GetByID` returned first |
| `?variant=display` missing | 404 | none — rule did not generalise |
| `?variant=bogus` | 400 | none |
| `?variant=original` / no parameter | unchanged, `Content-Length` included | none |

Log levels (NFR-9/10): `Info` on a completed generation; `Warn` on a permanent
failure, a transient failure, and pre-existing store drift; `Debug` on every skip
(already in flight, cap saturated, ineligible, failure recorded) and on the
downgrade itself. During the lazy-fill period the downgrade is the common case,
which is why it is not a `Warn`.

---

## 9. Testing

Unit-first, following each package's existing harness. The three tests that would
catch the expensive mistakes are marked ★.

**`processing`**
- `ResizeDims` at 768: landscape, portrait, exact edge, and never-upscale for an
  original already ≤ 768 (NFR-12).
- The worker persists exactly three variants for an image, one of each kind, and
  a redelivered event still yields three (NFR-13).
- ★ `CardGenerator` does not destroy existing variants: seed thumbnail + display,
  generate a card, assert all three rows survive (NFR-17, FR-3.4). This is the
  regression guard for the `ReplaceForMediaObject` trap.
- ★ Single-flight: block the fake store's `GetObject` on a channel, call
  `Generate` N times for one ID, release, assert exactly one decode and one row
  (NFR-16, FR-4.1).
- Cap: with the semaphore sized 1 and two different media objects, the second
  `Generate` is dropped rather than queued (FR-4.3), and concurrent in-flight
  work never exceeds the cap (FR-4.2).
- An undecodable original records a permanent failure; a second `Generate`
  performs no `GetObject` (FR-6.1, FR-6.3). A transient store error records
  nothing and the next call retries (FR-6.2).
- `Generate` does not use the caller's context: pass a cancelled context to the
  request path and assert the generation still completes (FR-4.4).

**`mediaobject`**
- `ParseContentVariant("card")` → `ContentCard`; `"Card"`, `"cards"`, `"bogus"`
  → 400 (NFR-14).
- ★ Downgrade matrix (NFR-15): card missing + thumbnail present → 200 thumbnail
  bytes; card missing + thumbnail missing → 404; **display missing → still 404**;
  card row present + object missing + thumbnail present → 200 thumbnail bytes.
  The display case is what proves the rule did not generalise.
- Scheduling: a fake `CardGenerator` records calls. Scheduled for a ready image;
  not scheduled for a document, a non-`ready` object, or a cross-fleet request
  (FR-3.2, FR-3.3, NFR-18). The cross-fleet case reuses the existing
  `TestContent_crossFleetNeverTouchesLookupOrStore` shape.
- A variant-lookup error still returns 500 with no downgrade attempted.
- `?variant=original` remains byte-identical including `Content-Length`
  (`TestContent_originalIsUnchanged` already pins this and must not change).

**`mediavariant`**
- `Upsert` inserts once, then updates rather than duplicating on a second call
  with the same `(media_object_id, variant)`; `created_at` is unchanged across
  the update (the `DoUpdates` column-list guard from §3).
- `Upsert` leaves other variants of the same object untouched.
- Test DDL updated with `UNIQUE (media_object_id, variant)` (§B3).

**`variantfailures`** — `Record` then `Recorded` round-trips; a second `Record`
does not overwrite the first `Reason`; `Recorded` is false for an unrelated
variant of the same object (FR-6.4).

**`apps/web`** — `VehiclePhotoThumbnail` requests `card` (NFR-19);
`MediaService` appends `?variant=card`; `MediaThumbnail`'s variant is unchanged.

**Interface widening fallout** — `fakeVariantAdmin` in `worker_test.go:63`
implements `mediavariant.Administrator` and needs an `Upsert` stub.

**Not unit-tested, deferred to manual verification** (PRD §10): that a card
renders visibly sharper than a thumbnail at `lg:grid-cols-3` on a high-DPI
display, and that the card variant's transferred size is materially smaller than
the same photo's `display` (NFR-1). Both are measurements, not assertions.

---

## 10. Risks

| Risk | Severity | Mitigation |
|---|---|---|
| Unique index creation fails on dirty data → media-service will not boot | High | Pre-flight duplicate query per environment before deploy (§B2) |
| SQLite test DDL missing the unique constraint → every `Upsert` test fails, or passes against a schema production does not have | Medium | Update the DDL in the same commit as the tag (§B3) |
| `created_at` or `UpdateAll: true` in the upsert reintroduces the task-006 column wipe, invisible to `entityguard` | Medium | Explicit `DoUpdates` column list + a `created_at`-preservation test (§3) |
| `ReplaceForMediaObject` used for the lazy write, destroying thumbnail and display | High | ★ regression test asserting all three rows survive (§9) |
| Downgrade generalises by accident during review or later edits | Medium | The `want != ContentCard` guard plus the display-still-404 test (§E3, §9) |
| In-flight generations lost on pod restart | Low | Accepted — nothing is half-written and the next request reschedules (§D2) |
| Storage grows by one rendition per image | Low | Accepted, NFR-3 |

## 11. Out of scope

Unchanged from PRD §2, restated for the plan's benefit: no change to the 320/1280
max edges or their call sites; no batch backfill; no `srcset`; no variant byte
sizes or `Content-Length` for variants; no generalisation of lazy generation or
downgrade beyond `card`; no re-encoding of existing variants; no change to
`apps/fleet-service`. Open questions §9.6 (a shorter `staleTime` for `card`) and
§9.7 (whether `VehicleDetailPage` wants `card`) stay open and unaddressed here.
