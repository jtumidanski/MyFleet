# Media `card` Image Variant — Product Requirements Document

Version: v1
Status: Draft
Created: 2026-08-02

---

## 1. Overview

The vehicles list renders each vehicle's primary photo as a full-width 16:9 hero
(`VehicleCard.tsx:102` — `boxClassName="aspect-[16/9] w-full rounded-none"`), but
`VehiclePhotoThumbnail` still requests the `thumbnail` variant
(`VehiclePhotoThumbnail.tsx:81`). That variant has a 320px max edge
(`processing/worker.go:30`), chosen in task-005 for an 80x80 square. Stretched
across a hero box that is 240–540 CSS px wide, it is visibly soft.

task-007 deliberately did not fix this. Its design D5 shipped `thumbnail`
unchanged and recorded that serving `display` (1280px max edge) for N cards is
"precisely the cost task-005 §4.7 existed to avoid", concluding that if the result
reads soft "that is a variant-sizing task in `media-service`, not a change here".
`manual-verification.md` carried the same item forward under "Deferred, note
only". This task is that deferred work.

This adds a third derived variant, `card`, at a 768px max edge; generates it for
new uploads alongside `thumbnail` and `display`; generates it on demand for media
objects that predate it; and points the list card at it. Because generation for
existing photos is lazy and asynchronous, the content endpoint gains one narrow,
explicitly-scoped exception to its no-fallback rule: a missing `card` may be
served as `thumbnail`, never as anything larger.

## 2. Goals

Primary goals:

- Serve the vehicles-list hero at a resolution appropriate to its rendered size,
  eliminating the visible softness deferred by task-007.
- Do so without regressing the per-page byte cost that task-005 §4.7 and task-007
  D5 both protected — `card` must stay substantially cheaper than `display`.
- Bring media objects uploaded before this task up to the new variant without a
  migration window, an operator-run script, or a redeploy-time batch.
- Keep every existing caller of `GET /api/media/{id}/content` byte-for-byte
  unchanged, including `?variant=original`, `?variant=thumbnail`,
  `?variant=display`, and the no-parameter form.

Non-goals:

- Changing the `thumbnail` (320) or `display` (1280) max edges. Both keep their
  current sizes and their current call sites.
- A batch or sweep-based backfill job. Considered and rejected in favour of lazy
  generation (§9.1).
- Responsive `srcset`/`sizes` or per-breakpoint variant selection on the card.
  One variant serves every breakpoint (§9.3).
- Recording variant byte sizes so `Content-Length` can be sent for a variant
  response. Variants continue to omit it, exactly as today (`processor.go:372`).
- Generalising lazy generation or downgrade to `thumbnail` or `display`. Both
  mechanisms apply to `card` and only to `card` (§9.2).
- Re-encoding, re-compressing, or otherwise altering variants that already exist.
- Any change to `apps/fleet-service`.

## 3. User Stories

- As a fleet member browsing the vehicles list, I want each vehicle's photo to
  look sharp in the card hero so the list does not look low-quality.
- As a fleet member on a high-DPI laptop, I want the hero to stay sharp at the
  three-column breakpoint, which is where the softness was first observed.
- As a fleet member with photos uploaded before this change, I want them to
  become sharp on their own, without re-uploading anything.
- As a fleet member on a slow connection, I want the list to stay as cheap to
  load as it is today, not to start pulling full-size images.
- As an operator, I want the upgrade to require no migration step, no manual
  backfill command, and no downtime.

## 4. Functional Requirements

### 4.1 The `card` variant

- **FR-1.1** A new derived variant named `card` is added with a max edge of
  **768px**. Aspect ratio is preserved and the image is never upscaled, via the
  existing `processing.ResizeDims` (`worker.go:45`).
- **FR-1.2** `mediavariant.Variant` gains `VariantCard Variant = "card"`
  (`mediavariant/model.go:8-11`).
- **FR-1.3** `processing` gains `cardMaxEdge = 768` alongside `thumbnailMaxEdge`
  and `displayMaxEdge` (`worker.go:29-32`).
- **FR-1.4** `mediaobject.ContentVariant` gains `ContentCard ContentVariant =
  "card"`, and `ParseContentVariant` accepts the exact lowercase string `"card"`
  (`contentvariant.go:10-36`). Any other unrecognised value continues to return
  `server.ErrBadRequest` (400) — the no-silent-fallback rule for typos is
  unchanged.
- **FR-1.5** The `card` variant is encoded by the same rules as the others:
  PNG originals stay PNG, everything else is JPEG at quality 85
  (`worker.go:266-278`).
- **FR-1.6** Its object key follows the existing scheme,
  `storage.ObjectKey(fleetID, mediaObjectID, "card"+ext)` (`worker.go:248`), so
  it cannot collide with `thumbnail` or `display`.

### 4.2 Generation for new uploads

- **FR-2.1** The processing worker generates `card` in the same pass as
  `thumbnail` and `display` — a third entry in the spec slice at
  `worker.go:169-175`, with the `built` slice capacity updated from 2 to 3.
- **FR-2.2** All three variants are persisted in the single existing
  `ReplaceForMediaObject` call (`worker.go:184`), so a re-delivered
  `media.uploaded` event remains idempotent and produces exactly three rows.
- **FR-2.3** A failure generating `card` fails the whole event, exactly as a
  `thumbnail` or `display` failure does today. Partial variant sets are not
  persisted.
- **FR-2.4** Non-image media (`ClassDocument`) continues to produce no variants
  at all. `card` introduces no exception.

### 4.3 Lazy generation for pre-existing media

- **FR-3.1** When `GET /api/media/{id}/content?variant=card` finds no `card` row,
  and the media object is a **ready image**, the service schedules generation of
  the `card` variant in the background and returns immediately without waiting
  for it.
- **FR-3.2** Generation is scheduled only *after* the request has passed the
  existing fleet authorization (`Processor.Content` calls `GetByID` first,
  `processor.go:336`). A caller who cannot read the media object must not be able
  to cause any work to be scheduled for it.
- **FR-3.3** Lazy generation is gated on the media object's status being `ready`
  and its content type resolving to `ClassImage` via the allowlist. Documents,
  `uploaded`, `processing`, and `failed` objects never enter the lazy path.
- **FR-3.4** Lazy generation **must not** use
  `mediavariant.Administrator.ReplaceForMediaObject`. That method deletes every
  existing row for the media object before inserting (`administrator.go:18-30`),
  which would destroy the object's `thumbnail` and `display`. An additive
  insert-or-update of a single variant is required.
- **FR-3.5** Lazy generation applies to `card` only. A missing `thumbnail` or
  `display` continues to 404 with no generation attempt.
- **FR-3.6** Once the `card` row exists, subsequent requests serve it normally and
  no further generation is scheduled.

### 4.4 Concurrency control

- **FR-4.1** Generation is **single-flighted per media object**: concurrent
  requests for the same missing `card` variant result in exactly one decode,
  resize, upload, and insert. The others schedule nothing.
- **FR-4.2** A **global concurrency cap** bounds how many lazy generations run at
  once across all media objects, so a cold twelve-card grid cannot spawn twelve
  simultaneous full-size image decodes. Default **4**, configurable via a new
  config key (default supplied in code, following the `MEDIA_WORKERS` /
  `config.GetInt` precedent at `cmd/main.go:86`).
- **FR-4.3** When the cap is saturated, additional generation requests are
  **dropped, not queued unboundedly**. A dropped request costs nothing: the
  caller has already been served its thumbnail downgrade, and the next request
  for that media object will schedule again.
- **FR-4.4** Scheduled generation must not be tied to the HTTP request's context.
  A client that disconnects immediately after receiving its downgrade must not
  cancel the background work it triggered.
- **FR-4.5** In-flight generations must be bounded by the process lifetime and
  not block a clean shutdown indefinitely.

### 4.5 Downgrade on a missing `card`

- **FR-5.1** When `?variant=card` cannot be served and a `thumbnail` row exists
  for the same media object, the service responds **200 with the thumbnail
  bytes** rather than 404.
- **FR-5.2** This downgrade is scoped to `card → thumbnail` and nothing else. A
  missing `display` still 404s. A missing `thumbnail` still 404s. There is no
  general ladder.
- **FR-5.3** The downgrade never serves anything **larger** than what was asked
  for. This is what keeps `processor.go:320-329`'s rationale intact: that comment
  forbids falling back to the multi-megabyte original, and a 320px thumbnail
  standing in for a 768px card carries none of that cost.
- **FR-5.4** Both ways `card` can be unservable — no row, and a row whose object
  is missing from storage (`processor.go:378`) — take the same downgrade path.
- **FR-5.5** If `card` is unservable **and** no `thumbnail` row exists, the
  response is 404, unchanged from today.
- **FR-5.6** The response describes the bytes actually sent: `Content-Type` and
  disposition come from the thumbnail row, resolved through the allowlist exactly
  as the normal variant path does (`processor.go:371`).
- **FR-5.7** No response header signals that a downgrade occurred. Accepted
  deliberately: the client cannot distinguish, but the failure mode is a slightly
  soft image — which is precisely today's behaviour — rather than a broken or
  oversized one. Revisit only if a caller needs to act on it.

### 4.6 Permanent-failure memory

- **FR-6.1** When lazy generation fails **permanently** — the original bytes do
  not decode, or are absent from storage — the service records that fact so
  subsequent requests do not re-attempt it. This reuses the distinction
  `processing.ErrPermanent` already draws (`worker.go:34-40`).
- **FR-6.2** A **transient** failure (object-store transport error, database
  error) is not recorded. The next request may schedule generation again.
- **FR-6.3** A media object with a recorded permanent `card` failure serves the
  `thumbnail` downgrade (FR-5.1) and schedules nothing.
- **FR-6.4** The recorded failure is scoped to the media object and the `card`
  variant. It must not suppress generation for any other variant, and must not
  affect the object's own `status`.
- **FR-6.5** Deleting and re-uploading a photo produces a new media object with no
  inherited failure record.

### 4.7 Frontend

- **FR-7.1** `MediaVariant` in `apps/web/src/types/models/media.ts:18` gains
  `'card'`, keeping the union a mirror of `contentvariant.go`.
- **FR-7.2** `VehiclePhotoThumbnail` requests `'card'` instead of `'thumbnail'`
  (`VehiclePhotoThumbnail.tsx:81`).
- **FR-7.3** No change to the component's four render states, its box sizing, its
  placeholder behaviour, or its no-toast-on-failure property.
- **FR-7.4** No change to `useMediaContentUrl`, `mediaKeys`, or the object-URL
  lifecycle. The variant is already part of the query key
  (`media.ts:26-28`), so `card` and `thumbnail` bytes cannot be served in place
  of one another.
- **FR-7.5** Other `thumbnail` callers are unaffected. `MediaThumbnail`
  (`components/features/vehicles/media/MediaThumbnail.tsx`) keeps its current
  variant — it renders a gallery tile, not a card hero.
- **FR-7.6** The `MediaService.getContentBlob` doc comment
  (`MediaService.ts:67-68`), which currently states that a derived variant that
  cannot be produced answers 404 and does not fall back, is updated to record the
  `card → thumbnail` exception.

## 5. API Surface

No new endpoints. One existing endpoint gains an accepted parameter value and one
narrow response-behaviour change.

### `GET /api/media/{id}/content?variant=card`

| Condition | Status | Body |
|---|---|---|
| `card` row exists, object present in storage | 200 | `card` bytes |
| `card` missing, `thumbnail` exists | 200 | `thumbnail` bytes (FR-5.1); generation scheduled if eligible (FR-3.1) |
| `card` row exists but object missing from storage, `thumbnail` exists | 200 | `thumbnail` bytes |
| `card` missing and `thumbnail` missing | 404 | JSON:API error |
| Caller's fleet does not own the media object | 404 | JSON:API error — never 403 (`processor.go:316-318`) |
| Database error on variant lookup | 500 | JSON:API error (`processor.go:344-347`) |

Unchanged: `?variant=original`, absent parameter, `?variant=thumbnail`,
`?variant=display`, and the 400 for any unrecognised value.

`Content-Length` continues to be omitted on every variant response, downgraded or
not, because `media_variants` records no byte count (`processor.go:372`).

## 6. Data Model

### 6.1 `media.media_variants`

No schema change. The `variant` column is a plain string (`entity.go:15`) with no
database-level enum or check constraint, so `'card'` rows insert without a
migration. GORM `AutoMigrate` (`entity.go:23`) is a no-op for this change.

### 6.2 Additive variant write

`mediavariant.Administrator` gains an additive single-variant write to satisfy
FR-3.4. It must be idempotent under concurrent callers — two racing lazy
generations for the same media object must leave exactly one `card` row, not two
and not a constraint violation. A uniqueness guarantee on
`(media_object_id, variant)` is the natural enforcement point; whether that is
added as a database constraint or handled purely in the write is a design
decision (§9.4).

### 6.3 Permanent-failure record

FR-6.1 needs somewhere to record "the `card` variant for this media object can
never be produced". The storage shape is left to the design phase (§9.5). It must
be scoped to (media object, variant), must survive a process restart, and must
not overload the media object's `status`, which is a lifecycle field with an
existing transition guard.

## 7. Service Impact

### `apps/media-service`

| Area | Change |
|---|---|
| `internal/mediavariant/model.go` | Add `VariantCard` |
| `internal/mediavariant/administrator.go` | Add additive single-variant write (FR-3.4) |
| `internal/mediaobject/contentvariant.go` | Add `ContentCard`; accept `"card"` |
| `internal/mediaobject/processor.go` | Downgrade rule (§4.5); schedule lazy generation (§4.3) |
| `internal/processing/worker.go` | Add `cardMaxEdge`; third spec entry; capacity 2 → 3 |
| `internal/processing/` (new) | Lazy generator: single-flight, global cap, permanent-failure recording |
| `cmd/main.go` | Wire the lazy generator; new concurrency config key |

The lazy generator reuses the existing decode/resize/encode/upload logic rather
than duplicating it. `Processor` reaches it through a port, in the same style as
the existing `VariantLookup` port (`processor.go:62-67`), so `mediaobject` does
not take a dependency on `processing`.

### `apps/web`

| Area | Change |
|---|---|
| `src/types/models/media.ts` | Add `'card'` to `MediaVariant` |
| `src/components/features/vehicles/VehiclePhotoThumbnail.tsx` | Request `'card'` |
| `src/services/api/MediaService.ts` | Update the no-fallback doc comment |

### Not affected

`apps/fleet-service`, `apps/auth-service`, `apps/notification-service`,
`deploy/k8s` (the new config key has an in-code default and needs no ConfigMap
entry unless the default is overridden).

## 8. Non-Functional Requirements

### 8.1 Sizing rationale

The list grid is `grid-cols-1 sm:grid-cols-2 lg:grid-cols-3` with `gap-4`
(`VehicleList.tsx:12`), inside `main` with `p-6` beside a `w-56` sidebar
(`AppLayout.tsx:26,63`). There is no `max-w` container, so the hero width is
`(viewport − 304) / 3` at the three-column breakpoint:

| Viewport | Card CSS px | Device px @2x |
|---|---|---|
| 1024 | 240 | 480 |
| 1280 | 325 | 650 |
| 1440 | 379 | 758 |
| 1920 | 539 | 1078 |

768 covers 1x through roughly a 2600px viewport, and 2x up to roughly 1450px —
which includes the 1440px high-DPI case `manual-verification.md` flagged. It
remains meaningfully cheaper than `display`: linear edge 768 vs 1280 is a 0.6x
scale, roughly 2.8x fewer pixels.

640 was considered and rejected: it falls short at 2x on anything above about a
1024px viewport, leaving the reported defect partly unfixed. 960 was considered
and rejected in the other direction: at only 1.33x narrower than the existing
1280 `display`, "just use `display`" becomes the simpler answer and the variant
stops earning its keep.

- **NFR-1** A `card` variant of a typical vehicle photo must be materially
  smaller than that photo's `display` variant. Verified by measurement during
  manual verification, not asserted here as a fixed byte target.
- **NFR-2** Adding `card` increases per-upload processing work by one
  decode-and-resize pass over an already-decoded source image; the original is
  decoded once and shared across all three variants (`worker.go:160,176`).
- **NFR-3** Storage per image object grows by one `card` rendition.

### 8.2 Request-path performance

- **NFR-4** The content endpoint must not perform image decoding synchronously.
  This is why generation is asynchronous (§9.1): a synchronous decode of a 25 MiB
  original on a streaming endpoint, twelve times over on a cold grid, is the
  failure mode being avoided.
- **NFR-5** A cold twelve-card grid must not spawn twelve concurrent full-size
  decodes (FR-4.2).

### 8.3 Security

- **NFR-6** Authorization precedes any scheduling decision (FR-3.2). The
  existing property — a variant is never reachable by a caller who could not read
  the original (`processor.go:314-316`) — extends unchanged to `card`.
- **NFR-7** A cross-fleet caller continues to receive 404, never 403, so the
  downgrade does not reintroduce an existence oracle.
- **NFR-8** The lazy path must not be a work-amplification vector: single-flight
  plus the global cap plus the permanent-failure record together bound how much
  work any sequence of requests can induce.

### 8.4 Observability

- **NFR-9** Log at `Info` when a lazy generation completes, at `Warn` when one
  fails permanently, and at `Debug` when one is skipped (already present, already
  in flight, ineligible, or dropped at the cap). This mirrors the level
  discipline already used in `Content` (`processor.go:350-353,378-383`).
- **NFR-10** A downgrade is a `Debug`, not a `Warn`. During the lazy-fill period
  it is expected and common; logging it loudly would be noise.

### 8.5 Client cache behaviour

`useMediaContentUrl` caches by `['media','content',id,variant]` with
`staleTime: 5 min` / `gcTime: 6 min` (`media.ts:150-156`). A downgraded response
is cached under the `card` key, so after background generation finishes the user
may keep seeing thumbnail bytes for up to the stale window or until the query
refetches.

- **NFR-11** This is accepted for v1: the visible consequence is that a
  pre-existing photo stays soft for the first visit and sharpens on a later one,
  which is strictly better than today and requires no new invalidation
  machinery. See §9.6.

### 8.6 Testing

- **NFR-12** `ResizeDims` behaviour at 768 including the never-upscale path (an
  original whose longest edge is already ≤ 768).
- **NFR-13** The worker persists exactly three variants for an image, and remains
  idempotent across a redelivered event.
- **NFR-14** `ParseContentVariant` accepts `"card"` and still rejects unknown
  values with 400.
- **NFR-15** Downgrade: `card` missing + `thumbnail` present → 200 thumbnail
  bytes; `card` missing + `thumbnail` missing → 404; `display` missing → still
  404 (proving the rule did not generalise).
- **NFR-16** Single-flight: concurrent requests for the same missing `card`
  trigger exactly one generation.
- **NFR-17** Lazy generation does not destroy the object's existing `thumbnail`
  and `display` rows (the FR-3.4 regression guard).
- **NFR-18** An ineligible object (document, non-`ready`, or one with a recorded
  permanent failure) schedules no generation.
- **NFR-19** Frontend: `VehiclePhotoThumbnail` requests the `card` variant.

## 9. Open Questions

Items 9.1–9.3 were resolved during the spec interview and are recorded here with
their reasoning. 9.4–9.7 are genuinely open and belong to the design phase.

### 9.1 Backfill mechanism — RESOLVED: lazy, asynchronous

Lazy generation on first request, over a batch sweep job, an event replay, or no
backfill. It needs no migration window and no operator action, and it does work
strictly proportional to what is actually viewed — a fleet's rarely-opened photos
never cost anything.

Event replay was rejected on a concrete blocker: `worker.handle` short-circuits
when `obj.Status() == StatusReady` (`worker.go:149-157`), and every pre-existing
image is `ready`, so republishing `media.uploaded` would record the event and
generate nothing. Making replay work would mean weakening a guard that exists to
keep redelivery cheap.

Generation is asynchronous rather than synchronous because the alternative puts
image decoding on an endpoint that today only streams bytes (NFR-4). This is also
what makes the §4.5 downgrade a real, continuously-exercised path rather than a
failure-only branch.

### 9.2 Downgrade breadth — RESOLVED: `card → thumbnail` only

A general next-smaller-available ladder was rejected: it would let a detail view
asking for `display` silently receive a 768px image with no way to detect it. One
narrow documented exception is the smaller change to the contract
`processor.go:320-329` sets out.

### 9.3 Max edge — RESOLVED: 768

See §8.1 for the arithmetic and for why 640 and 960 were both rejected. One fixed
variant serves every breakpoint; `srcset` is a non-goal.

### 9.4 Where is the additive-write race enforced?

FR-3.4 and §6.2 require an idempotent single-variant write. Is a unique index on
`(media_object_id, variant)` added — which also documents an invariant the
`ReplaceForMediaObject` path already happens to satisfy — or is the race handled
entirely in the write (upsert / on-conflict-do-nothing)? A new index is a schema
change on an existing table and needs checking against current data.

### 9.5 What shape is the permanent-failure record?

§6.3 leaves this to design. Candidates: a nullable column on `media_objects`; a
dedicated small table keyed by (media object, variant); or a sentinel row in
`media_variants` with an empty object key. The sentinel is the least invasive but
risks a row that looks like a servable variant to any future reader that does not
know the convention — a real hazard given `ListByMediaObject` exists.

### 9.6 Should the `card` query use a shorter `staleTime`?

Per NFR-11 the v1 answer is no. Worth revisiting if the first-visit-soft window
proves annoying in practice: a shorter stale window for `card` specifically, or
having the downgrade path surface something the client can react to (the header
rejected in FR-5.7), would both close it.

### 9.7 Does `VehicleDetailPage` want `card` too?

Out of scope here, but the detail view is the other place a vehicle photo is
rendered. Whether it currently over- or under-fetches has not been checked and is
not part of this task.

## 10. Acceptance Criteria

Backend — variant and generation:

- [ ] `mediavariant.VariantCard` exists with value `"card"`.
- [ ] `processing.cardMaxEdge` is 768.
- [ ] A newly uploaded JPEG produces exactly three variant rows: `thumbnail`,
      `card`, `display`, with `card`'s longest edge equal to 768 (or the
      original's longest edge, if smaller).
- [ ] A newly uploaded PNG produces a PNG `card` variant.
- [ ] Redelivering `media.uploaded` for the same object still yields exactly three
      rows.
- [ ] An uploaded PDF still produces zero variant rows.

Backend — content endpoint:

- [ ] `?variant=card` returns 200 with the card bytes when the row and object
      exist.
- [ ] `?variant=card` on a pre-existing object with no card row returns 200 with
      thumbnail bytes.
- [ ] `?variant=card` with neither card nor thumbnail returns 404.
- [ ] `?variant=display` with no display row still returns 404.
- [ ] `?variant=bogus` still returns 400.
- [ ] `?variant=original` and the no-parameter form are byte-identical to before,
      including `Content-Length`.
- [ ] A cross-fleet caller requesting `?variant=card` receives 404.

Backend — lazy generation:

- [ ] Requesting `card` for a pre-existing ready image causes the card row to
      exist shortly afterwards, without the request having blocked on it.
- [ ] That object's `thumbnail` and `display` rows still exist afterwards.
- [ ] Concurrent requests for the same missing card variant produce exactly one
      generation and exactly one row.
- [ ] Concurrent requests across many objects never exceed the configured global
      cap of in-flight generations.
- [ ] A document, a non-`ready` object, and an object with a recorded permanent
      failure each schedule no generation.
- [ ] An undecodable original records a permanent failure; a second request does
      not re-attempt decoding.
- [ ] A client disconnecting immediately after its downgraded response does not
      cancel the generation it triggered.

Frontend:

- [ ] `MediaVariant` includes `'card'`.
- [ ] `VehiclePhotoThumbnail` requests `'card'`.
- [ ] `MediaThumbnail` still requests its existing variant.
- [ ] The card's four render states, box sizing, and no-toast-on-failure
      behaviour are unchanged.

Verification:

- [ ] `make ci` passes.
- [ ] Manual: a pre-existing vehicle photo renders sharp on the vehicles list at
      `lg:grid-cols-3` on a high-DPI display after the lazy fill, closing the
      `manual-verification.md` "Deferred, note only" item from task-007.
- [ ] Manual: the `card` variant's transferred size is measurably smaller than
      the same photo's `display` variant (NFR-1).
