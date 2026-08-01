# Final whole-branch review — task-005-vehicle-card-photo-actions

Range reviewed: `352e8c1..84b025e` (12 code commits + 3 doc commits).
Worktree HEAD `84b025e`, working tree clean.

---

## Verdict

**Ready to merge.**

No Critical findings. One Important finding is a recommended follow-up, not a
merge blocker — the feature is correct and fails safe in the scenario it
describes. All four deferred ledger items are ruled ship-able.

One outstanding item is *not* a review finding but must not be lost: the
Task 11 `OPEN FOR HUMAN` browser checks (single-column overflow, card alignment
with/without photos, skeleton-to-content jump at `h-44`, Tab focus ring,
`lg:grid-cols-3` density at the taller card height) were never run, because no
browser was available. Nothing in this review substitutes for them.

**Findings: 0 Critical, 1 Important, 4 Minor.**

---

## Findings

### Important

#### I-1 — A vehicle whose variants were never generated pulls the *full-size original* into every list card, and nothing bounds it

`apps/media-service/internal/mediaobject/processor.go:296-306` (fallback),
`apps/web/src/components/features/vehicles/VehiclePhotoThumbnail.tsx:64`
(the new consumer),
`apps/web/src/lib/hooks/api/media.ts:135-141` (`staleTime` 5 min / `gcTime` 6 min).

**What is wrong.** The backend's variant fallback and the frontend's list view
are individually correct and were reviewed individually. Composed, they produce
a case neither task-scoped review could see: when a media object has no
`thumbnail` row, `?variant=thumbnail` returns the **original** bytes — up to
`MEDIA_MAX_UPLOAD_BYTES` (25 MiB, `media.ts:47`). `VehiclePhotoThumbnail` then
decodes that into an `<img>` sized `h-20 w-20`, and React Query holds the `Blob`
plus a live object URL for six minutes. On a 12-card grid the worst case is
~300 MB of retained blobs and 12 full-size decodes.

**Why it matters.** This is not only a transient post-upload state. In
`apps/media-service/internal/processing/worker.go:160-167`, a `generateVariant`
error aborts `processMessage` for the whole message, so **no** variants are
written and the object never leaves `processing`. Any media the decoder cannot
handle (HEIC, a corrupt JPEG, a broken worker/Kafka path) is therefore
*permanently* in the fallback state, and the list page permanently ships the
original for it. On mobile this is a plausible tab kill, and it is invisible to
the client: the response is a 200 with the original's `Content-Type`, and
nothing distinguishes "this is your thumbnail" from "this is a 25 MB original".

**Concrete fix (follow-up).** The processor already computes exactly the signal
needed — `ContentInfo.Served` (see M-2) — and throws it away. Surface it as a
response header (`X-Media-Variant-Served: original`) in
`resource.go:150-190`, have `MediaService.getContentBlob` return it alongside
the blob, and let `VehiclePhotoThumbnail` render `PhotoPlaceholder` instead of
decoding a fallback original on a list card. A cheaper interim mitigation is to
cap the fallback in `Content`: when `want != ContentOriginal` and
`m.Size()` exceeds some threshold, return the miss to the caller rather than the
original. Either is a small change; neither belongs in this branch.

**Not a blocker because**: the image still renders correctly, the behaviour is
the plan's documented fallback (context.md §2 D4, design §3.2), and the failure
mode is degraded performance rather than incorrectness or a security issue.

---

### Minor

#### M-1 — Three copies of the Carfax template, no automated gate against drift

`apps/web/src/lib/config/runtimeConfig.ts:25`,
`apps/web/public/config/config.json:2`,
`deploy/k8s/base/web/configmap.yaml:17`.

**Verified byte-identical** (SHA-256 of each extracted string:
`766cf0adba79ba2b…`, all three). So there is no drift today.

**What is wrong.** Nothing in `make ci` — not `make fe-test`, not
`tools/check-manifests.sh` — asserts the three stay equal, and the only value
any test exercises is the one hard-coded in `carfax.test.ts:4` and
`VehicleCard.test.tsx:18`. A future edit to the ConfigMap alone is untested by
construction, and `buildCarfaxUrl` fails *silently* closed (M-4), so the symptom
of a bad edit is "the Carfax button quietly disappeared fleet-wide", with
nothing in a log anywhere.

**Concrete fix.** Add one assertion to `runtimeConfig.test.ts` that reads
`apps/web/public/config/config.json` from disk and compares it to
`DEFAULT_RUNTIME_CONFIG`, and one grep-style assertion in
`tools/check-manifests.sh` that the rendered `web-config` ConfigMap's
`config.json` parses and its `carfaxUrlTemplate` matches the file. That closes
the loop at CI cost of two lines.

#### M-2 — `ContentInfo.Served` is computed but never read outside tests

`apps/media-service/internal/mediaobject/processor.go:73-76` (declaration),
`:293` and `:328` (both writers). Grep across `apps/media-service` shows the only
readers are `processor_test.go:644,682,705,741`.

**What is wrong.** The handler at `resource.go:150-190` uses `info.ContentType`
and `info.Size` and ignores `info.Served`. The field is a real, correctly
maintained contract that is half-wired: the domain publishes it, the transport
drops it, so no client can ever learn a fallback happened.

**Why it matters.** It is the exact signal I-1 needs, and its absence is why
I-1 exists. Left as-is it reads as test-only API surface, which invites a later
reader to delete it.

**Concrete fix.** Either emit it (`w.Header().Set("X-Media-Variant-Served",
string(info.Served))` in `resource.go`, which is also a useful operational
signal for how often the fallback fires), or drop the field and let the tests
assert on the bytes/content-type they already assert on. Emitting it is the
better call — see I-1.

#### M-3 — A misconfigured template removes the Carfax button with no diagnostic

`apps/web/src/lib/carfax.ts:24-35`.

**What is wrong.** `buildCarfaxUrl` returns `null` for four distinct reasons —
no VIN (`:24`), template missing `{vin}` (`:25`), non-`https:` scheme (`:32`),
unparseable URL (`:34`) — and the caller (`VehicleCard.tsx:32,73`) cannot tell
them apart. "No VIN on this vehicle" is normal and per-row; the other three are
a global misconfiguration affecting every card, and they produce identical,
silent output.

**Why it matters.** Combined with M-1 this is the whole diagnostic story for a
bad ConfigMap edit: nothing. Failing closed is correct (and is why this is
Minor, not Important — a `javascript:` template genuinely cannot reach an
`href`); failing *silently* is the gap.

**Concrete fix.** In `runtimeConfig.ts`'s zod schema (`:40-44`), or once at
module load in `carfax.ts`, `console.warn` when the configured template is
non-empty but unusable (no `{vin}`, or a non-`https:` scheme after
substitution). One warning per page load, not per card.

#### M-4 — `stubObjectUrl` permanently mutates the global `URL` constructor

`apps/web/src/test/objectUrl.ts:16`:
`vi.stubGlobal('URL', Object.assign(URL, { createObjectURL, revokeObjectURL }))`.

**What is wrong.** `Object.assign(URL, …)` mutates the *existing* global `URL`
class in place and then stubs the global with that same (now-mutated) object.
`vi.unstubAllGlobals()` in the `afterEach` of `VehicleCard.test.tsx:38-41` and
`VehiclePhotoThumbnail.test.tsx:18-21` therefore restores a reference to the
already-mutated constructor: `URL.createObjectURL` stays a `vi.fn` for the rest
of the file, and the teardown does not do what it reads as doing.

**Why it matters.** Harmless today (Vitest isolates per file, and every affected
test re-stubs in `beforeEach`), but it is a booby trap for the next test that
asserts on cleanup, and this file is a shared helper introduced precisely to be
reused.

**Concrete fix.** Stub the two methods rather than the class:
`vi.stubGlobal('URL', Object.assign(Object.create(URL), URL, { createObjectURL, revokeObjectURL }))`,
or more simply keep the originals and restore them explicitly in the returned
teardown.

---

## Deferred-item triage

| # | Ledger item | Ruling |
| - | ----------- | ------ |
| 1 | **Task 4** — no test for the non-`ErrObjectNotFound` store error on the variant path (`processor.go:302-303`), nor the empty-`ref.ContentType` fallback (`processor.go:288-292`) | **Can ship.** Both branches are one line and neither is reachable from a production writer. The store-error branch is `return …, err` → 500, structurally identical to the already-tested `openOriginal` default at `:320-322`, and the sibling *lookup*-error path **is** covered (`TestContent_lookupErrorIsReturned`). The empty-`ContentType` branch is dead by construction: `worker.go:223` gets its type from `variantEncoding`, which returns `"image/jpeg"` or `"image/png"` and never `""`, and that is the only writer of `media_variants.content_type`. It is defence-in-depth, not behaviour. |
| 2 | **Task 4** — no test asserts the Debug-vs-Warn distinction between a missing variant row and DB/store drift | **Can ship.** Log level is not client-observable behaviour, and the two paths' *functional* difference (fallback with one store call vs. two) is already pinned by `TestContent_variantObjectMissingFallsBackToOriginal`, which asserts `len(store.getCalls) != 2`. If someone wants the gate cheaply later, `logrus/hooks/test` is three lines — but it is not worth blocking a merge. |
| 3 | **Task 7** — commit messages omit the harness `Co-Authored-By` / `Claude-Session` footer | **Can ship.** Consistent across all 12 commits, zero functional impact, and a 12-commit history rewrite on a branch that has already been reviewed end-to-end is strictly more risk than the defect. Concrete mitigation: if the PR is squash-merged, put the footer on the squash commit message, which is the only commit that lands on `main` anyway. |
| 4 | **Task 11** — `min-w-0` on both flex children has no automated gate | **Can ship, but do not close it silently.** I confirmed by reading that it is present and correct on *both* children — `VehicleCard.tsx:41` (`min-w-0 flex-1`) and `:43` (`min-w-0`) — which is what FR-1.6 requires. The claim that jsdom cannot gate it is accurate: jsdom performs no layout, so a test asserting the class name would be a tautology and a test asserting overflow would always pass. The correct gate is the browser pass that is **already outstanding** as Task 11 Step 6 (`OPEN FOR HUMAN`). Fold this item into that check rather than marking it resolved. |

---

## What I verified and found sound

**Integration seams (the three slices actually meet).**
- Frontend→backend wire format: `VehiclePhotoThumbnail.tsx:64` asks for
  `'thumbnail'` → `useMediaContentUrl` (`media.ts:130-141`) →
  `MediaService.getContentBlob` (`MediaService.ts:65-68`) emits
  `/api/media/{id}/content?variant=thumbnail` → `ParseContentVariant`
  (`contentvariant.go:23-35`) accepts it. The TS union `MediaVariant`
  (`types/models/media.ts:8`) is exactly the Go accepted set.
- `original` sends **no query parameter at all** (`MediaService.ts:66`), so every
  pre-existing caller's request is byte-identical on the wire.
  `TestGetContent_noVariantParamIsUnchanged` pins the server side of that,
  Content-Length included.
- Query key / service / hook agree: `mediaKeys.content(id, variant)`
  (`media.ts:26-27`) keeps the `contents()` prefix, so prefix invalidation still
  matches every variant. `mediaKeys` has no consumer outside `media.ts` and its
  test, so no invalidation site was left assuming the old 3-element key.
- **The list endpoint really returns the two attributes the card needs.**
  `apps/fleet-service/internal/vehicle/resource.go:52` uses
  `TransformWithStatus`, which emits both `vin` and `primaryImageMediaId`
  (`rest.go:17,19`). Had the list transform been the thin one, every card would
  have shown "No photo" in production while all tests passed on fixtures.
- ConfigMap document shape matches the parser: the ConfigMap's `config.json`
  value parses as JSON and its single key is exactly
  `runtimeConfigSchema`'s (`runtimeConfig.ts:40-44`). `kustomize build
  overlays/main` renders the ConfigMap and the deployment's volume reference
  resolves to it (4 matching sites).
- The mount is a **directory** mount, not `subPath`
  (`deployment.yaml:60-63`), so a ConfigMap edit reaches running pods without a
  restart — which is the whole point of the mechanism, and it is done right.
  nginx's exact-match `location = /config/config.json` (`nginx.conf:16-18`) has
  no `try_files`, so a missing file 404s rather than serving `index.html` as
  JSON; `loadRuntimeConfig` treats that as `!res.ok` and falls back.
- `mediaobject` never imports `mediavariant`: the `VariantLookup` port is
  declared in `processor.go:56-60`, `variant` crosses it as a plain `string`,
  and the only adapter is `variantLookup` in `cmd/main.go:161-169`. The tree is
  intact.

**Security.**
- **Authorization ordering.** `Processor.Content` (`processor.go:281-284`) calls
  `GetByID` → `getActive` → `AuthorizeAccess` as its *first* statement, before
  the variant lookup and before any store read. `AuthorizeAccess`
  (`processor.go:80-85`) maps a fleet mismatch to `server.ErrNotFound` → 404,
  never 403, so no existence oracle. Both layers pin it:
  `TestContent_crossFleetNeverTouchesLookupOrStore` asserts the lookup ran zero
  times and storage was touched zero times, and
  `TestGetContent_variantCrossFleetIs404WithNoStoreRead` asserts the same
  through the real router with a 404 status.
- **The `https:`-only guard genuinely holds.** `buildCarfaxUrl:32` parses the
  *substituted* URL and rejects anything whose `protocol` is not `https:`.
  I checked the bypasses: `javascript:`/`data:` are rejected (tested,
  `carfax.test.ts:52-54`); leading whitespace and embedded tab/newline are
  stripped by the `URL` constructor *before* protocol determination, so
  `java\nscript:` still parses as `javascript:` and is rejected; a
  scheme-relative or path-relative template throws in `new URL` (no base) and is
  caught at `:33-35`; and `HTTPS:` is normalised to `https:` by the parser, so
  case does not smuggle anything past. The VIN itself is
  `encodeURIComponent`-ed (`:28`), so it cannot inject query parameters or
  fragments.
- The anchor is a plain `<a>` with `target="_blank"` **and**
  `rel="noopener noreferrer"` (`VehicleCard.tsx:78-84`), asserted at
  `VehicleCard.test.tsx:92-95`. Nothing contacts Carfax before a click: no
  prefetch, no hover handler, no `<link rel="prefetch">` anywhere in the branch.
- A rejected `?variant=` never touches storage
  (`TestGetContent_bogusVariantIs400WithNoBytes` asserts `store.getCalls` is
  unchanged) and returns a JSON:API envelope with `code: "bad_request"` — the
  new `codeFor` case at `server.go:12-13`, which previously rendered as
  `internal_error`.

**Correctness under failure.**
- Missing variant row → fallback to original, logged at Debug
  (`processor.go:298-300`); variant row with no object → fallback, logged at
  Warn (`processor.go:307-310`); lookup error → propagated as 500
  (`processor.go:290-295`). All three have processor-level tests, and the first
  two also have router-level tests.
- `Content-Length` is omitted for a variant and present for an original/fallback
  (`resource.go:180-182` guards on `info.Size > 0`; `ContentInfo.Size` stays 0
  only on the variant path). `media_variants` has no size column, so sending the
  original's length would truncate or hang the response — pinned by
  `TestGetContent_thumbnailServesVariantWithoutContentLength`.
- Config fetch that never settles: bounded by a 2 s `AbortController`
  (`runtimeConfig.ts:35,72-74`) with a fake-timer test that proves the promise
  resolves rather than hanging. Every failure mode — 404, network error,
  malformed JSON, wrong types, non-object, hang — resolves to
  `DEFAULT_RUNTIME_CONFIG` and never rejects, and `main.tsx:22` uses `.then`
  (not `.catch`) deliberately so the tree always mounts.
- Broken thumbnail on a list of twelve: `useMediaContentUrl`'s error state
  surfaces as `isError`, `VehiclePhotoThumbnail:71-75` renders a neutral
  placeholder with a distinct accessible label ("Photo unavailable" vs "No
  photo"), and there is **no toast** — twelve failures produce twelve
  placeholders and zero notifications. The `!mediaId` case short-circuits before
  any request (`:66-68`, asserted twice).

**Nothing dead, duplicated, or half-wired** beyond M-2 and the two items the
plan explicitly scopes out (`mediavariant.ListByMediaObject` with no production
caller; the `display` variant with no consumer — both documented in
context.md §7). `NewProcessor` has exactly one production call site
(`resource.go:45`), so the nil-`VariantLookup` panic path is unreachable in
production, and the test suite passes `&fakeVariants{}` rather than `nil`
everywhere. `useMediaContentUrl`'s object-URL effect is untouched — only the
query key and `queryFn` changed, as the plan required. No stale references to
the deleted card-wrapping `<Link>` remain; `VehicleList.tsx` is the only
consumer.

**Not re-verified** (per instructions, already run and independently confirmed):
`make ci`, the full Go and web test suites, and both `kubectl --dry-run=server`
overlay renders. I re-ran only `kustomize build overlays/main` to confirm the
new ConfigMap renders and its volume reference resolves.
