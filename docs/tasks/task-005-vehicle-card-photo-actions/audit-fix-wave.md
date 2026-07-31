# Final review — fix wave report

Branch: `task-005-vehicle-card-photo-actions`
Worktree: `/home/tumidanski/source/MyFleet/.worktrees/task-005-vehicle-card-photo-actions`

| Commit | Scope |
| --- | --- |
| `66e2f53` | Backend — fixes 1–4 |
| `992803f` | Frontend — fixes 5–12 |
| `611af03` | Build/CI — fix 13 |

All 13 rulings applied. `make ci` green; both server dry-runs clean against `bee`.

---

## Backend

### 1. DOM-10 — lazy provider helper

`apps/media-service/internal/mediavariant/provider.go` rewritten to the
`apps/auth-service/internal/user/provider.go:35` shape: both reads go through
`database.Query` / `database.SliceQuery`, and `GetByMediaObjectAndVariant` drops
the `(Model, bool, error)` triple for `(Model, error)` with a new package-local
`ErrNotFound` sentinel — the same convention `mediaobject` already uses.

`ListByMediaObject` was also wrapped in `SliceQuery`. It has no callers outside
the interface declaration; the finding only cited `GetByMediaObjectAndVariant`,
but leaving one eager read next to a lazy one in the same tiny file re-creates
exactly the inconsistency DOM-10 exists to prevent. **Judgement call, small.**

The `VariantLookup` port itself kept `found bool`. The alternative — exporting
`mediavariant.ErrNotFound` across the port — would make `mediaobject` import the
sibling package's sentinel, breaking the tree-shaped dependency graph that port
was designed for. The composition-root adapter in `apps/media-service/cmd/main.go`
does the translation instead.

Tests: `TestGetByMediaObjectAndVariant_missIsNotAnError` renamed and inverted to
`..._missIsErrNotFound`.

### 2. `context.Context` through the port

`VariantLookup.Lookup(ctx context.Context, mediaObjectID, variant string)`.
`Processor.Content` passes the ctx it already holds; the provider runs
`db.WithContext(ctx)`. The port's doc comment claimed "the same shape as
ObjectStore" — now true. Adapter, `fakeVariants`, and every call site updated.

Tests added:
- `TestGetByMediaObjectAndVariant_honoursContextCancellation` — a cancelled ctx
  produces `context.Canceled`, proving the context reaches the driver rather
  than being accepted and dropped.
- `TestContent_variantFoundServesVariantBytes` extended: `fakeVariants` now
  records the contexts it was handed, and the test asserts a value put on
  `Content`'s ctx arrives at `Lookup`.

### 3. Unservable derived variant → 404 (reverses the plan)

`apps/media-service/internal/mediaobject/processor.go`:

| Request | Before | After |
| --- | --- | --- |
| `?variant=thumbnail`, no variant row | 200 + original bytes | **404** (`Debug` log kept) |
| `?variant=thumbnail`, row exists, object missing from store | 200 + original bytes | **404** (`Warn` log kept) |
| `?variant=thumbnail`, row + object present | 200 + variant, no `Content-Length` | unchanged |
| `?variant=original` | 200 + original + `Content-Length` | **unchanged** |
| no `?variant=` | 200 + original + `Content-Length` | **unchanged** |
| `Lookup` returns an error | 500 | unchanged (now logged, fix 4) |
| `?variant=bogus` | 400 | unchanged |
| cross-fleet | 404, no lookup, no store read | unchanged |

`ContentInfo.Served` removed — with the fallback gone it could only ever echo the
request. `ContentInfo` is now `{ContentType, Size}`. Handler and tests updated.

Doc comments on `Content` and `openOriginal` rewritten (they described the
fallback and were now false), plus the `?variant=` comment on the route in
`resource.go`.

Tests inverted (renamed to describe the new behaviour), each asserting 404 **and**
that the original's bytes were never read from the store:
- `TestContent_variantMissingFallsBackToOriginal` → `TestContent_variantMissingIs404AndServesNoOriginal`
- `TestContent_variantObjectMissingFallsBackToOriginal` → `TestContent_variantObjectMissingIs404AndServesNoOriginal`
- `TestGetContent_variantWithNoRowFallsBackToOriginal` → `TestGetContent_variantWithNoRowIs404WithNoOriginalBytes`

Tests added:
- `TestGetContent_originalIsUnaffectedByAnUnservableVariant` — on the *same*
  object that 404s for a thumbnail, both the bare request and
  `?variant=original` still return 200, `original-bytes`, `Content-Length: 14`,
  `image/png`. This is the backwards-compatibility contract, pinned explicitly.
- The drift test asserts the `Warn` was actually emitted (via a logrus hook), not
  merely that the branch was taken.

> **JUDGEMENT CALL — needs confirmation.** The ruling's literal words covered the
> missing-row case. Extending 404 to the **row-exists-but-object-missing** case
> is my reading, on the stated rationale that the size consequence is identical:
> both would serve a full-size original in response to a thumbnail request. The
> `Warn` log is retained so an operator can still distinguish real DB/store drift
> from the benign not-yet-processed case, even though the client sees the same
> status. If you intended drift to keep falling back, only the second bullet in
> `Content` and `TestContent_variantObjectMissingIs404AndServesNoOriginal` need
> reverting.

### 4. Log the Lookup-error 500

`apps/media-service/internal/mediaobject/resource.go` — the content handler now
logs before writing the error:

```go
if server.StatusFor(err) >= http.StatusInternalServerError {
    log.WithError(err).WithField("media_id", id).
        WithField("variant", string(v)).
        Error("media content read failed")
}
```

> **Minor judgement call.** The siblings at `:64` and `:110` log unconditionally.
> Copying that exactly would emit an `Error` for every 404 — and with fix 3 a
> not-yet-processed thumbnail is now a *routine* 404, so unconditional logging
> would bury the genuine faults under noise on the busiest path in the service.
> The `StatusFor` gate keeps the shape (`log.WithError(...).Error(...)`) while
> restricting it to 5xx. Both halves are tested:
> `TestGetContent_lookupFailureIs500AndIsLogged` and
> `TestGetContent_expectedErrorsAreNotLoggedAsFaults`.

---

## Frontend

### 5. Render first; latch config asynchronously

`apps/web/src/main.tsx` calls `createRoot().render()` immediately, then
`void loadRuntimeConfig()`.

`apps/web/src/lib/config/runtimeConfig.ts` became an observable store:
`getRuntimeConfig()` is the (referentially stable) snapshot getter,
`subscribeRuntimeConfig(cb)` returns an unsubscribe, and an internal `latch()`
notifies on **both** the success and the failure path. The module imports no
React — `buildCarfaxUrl` stays a pure function that can read it directly.

`apps/web/src/lib/hooks/useRuntimeConfig.ts` (new) wraps it in
`useSyncExternalStore(subscribe, getSnapshot, getSnapshot)`.
`VehicleCard` reads through the hook.

Tests: `VehicleCard.test.tsx` no longer mocks the runtime-config module at all —
a mocked getter would satisfy every assertion while the real app ignored its
ConfigMap. A `latchConfig()` helper drives the real store through
`loadRuntimeConfig` with a stubbed `fetch`, and `beforeEach` resets it.
Two new tests:
- `picks up a runtime config that arrives AFTER the card has mounted`
- `drops the Carfax button when a late config makes the template unusable`

`runtimeConfig.test.ts` gains `notifies subscribers so a tree mounted before the
fetch resolves updates` (including that unsubscribe detaches) and `notifies
subscribers on the failure path too`.

**Mutation-verified.** Reverting `VehicleCard` to the plain module read:

```
   × VehicleCard > picks up a runtime config that arrives AFTER the card has mounted
   × VehicleCard > drops the Carfax button when a late config makes the template unusable
      Tests  2 failed | 10 passed (12)
```

### 6. Schema moved, type derived (FE-14)

`apps/web/src/lib/schemas/runtimeConfig.ts` (new) holds
`DEFAULT_CARFAX_URL_TEMPLATE`, `runtimeConfigSchema`, and
`export type RuntimeConfig = z.infer<typeof runtimeConfigSchema>` — matching
`lib/schemas/vehicle.ts` and `lib/schemas/fleet.ts`, which pair an exported
schema with a `z.infer` type.

The literal template constant had to live in the schema file (the `.catch()`
defaults need it) rather than being imported from `config/runtimeConfig.ts`,
which would be circular. `DEFAULT_RUNTIME_CONFIG` is built from it in
`config/runtimeConfig.ts` — a value move, not a type alias re-export.

### 7. `createErrorFromUnknown` (FE-09)

`loadRuntimeConfig`'s catch block normalises through the project helper and logs
`apiError.message`. `loadRuntimeConfig` still never throws and never rejects;
the existing tests covering 404 / network error / malformed JSON / timeout are
untouched, and a new test asserts the message reaching `console.warn` is the
normalised one while the promise still resolves to the defaults.

### 8. Object-URL stub

`apps/web/src/test/objectUrl.ts` now stubs a **subclass**:

```ts
class StubbedURL extends URL {
  static createObjectURL = createObjectURL;
  static revokeObjectURL = revokeObjectURL;
}
vi.stubGlobal('URL', StubbedURL);
```

The real `URL` is never mutated, so `vi.unstubAllGlobals()` genuinely restores.

**This exposed a second bug the leak had been hiding.** Vitest runs `afterEach`
hooks last-registered-first, and Testing Library registers its auto-cleanup at
import time — before a test file's own `afterEach`. So the file's hook ran first,
restored the real `URL`, and *then* cleanup unmounted components whose effect
teardown calls `URL.revokeObjectURL`, which jsdom does not implement. That only
ever "worked" because the old stub's mutation outlived its own restore. Added
`unstubObjectUrl()` (cleanup, then unstub) and wired it into all three consumers:
`VehicleCard.test.tsx`, `VehiclePhotoThumbnail.test.tsx`, `media.test.ts`.

New `apps/web/src/test/objectUrl.test.ts` pins the helper's own contract: the
real class never grows the methods, the unstub restores identity, and the
subclass still parses URLs (`new URL(...)` is used in code under test).

### 9. Do not claim "No photo" for a vehicle that has one

`VehiclePhotoThumbnail.tsx` — the `isError || !url` branch now renders
`"Photo unavailable"` unconditionally. `"No photo"` is reachable only from the
`!mediaId` guard above it.

Test: `says "Photo unavailable", not "No photo", when a known photo cannot be
fetched` drives the **real** paused state via
`onlineManager.setOnline(false)` (status pending, fetchStatus `paused` →
`isLoading` false, `isError` false, `data` undefined) rather than faking a
loading promise, which would have passed against the buggy code too. `afterEach`
restores `setOnline(true)` so it cannot leak into other files.

### 10. "No toast" invariant (F-2)

Test `fires no toast when a thumbnail fails to load` — `sonner` is mocked with
recording spies on `toast`, `.error`, `.warning`, `.info`, all asserted
un-called after the placeholder renders.

**Mutation-verified.** Adding `toast.error(...)` to the error branch *and*
restoring the old conditional label:

```
   × VehiclePhotoThumbnail > fires no toast when a thumbnail fails to load
   × VehiclePhotoThumbnail > says "Photo unavailable", not "No photo", when a known photo cannot be fetched
      Tests  2 failed | 5 passed (7)
```

### 11. Wire format pinned (F-3)

`apps/web/src/services/api/MediaService.test.ts` (new — `src/services/api/` had
no tests at all). Four assertions on the exact path handed to
`apiClient.requestBlob`:

| call | expected path |
| --- | --- |
| `getContentBlob('m1')` | `/api/media/m1/content` |
| `getContentBlob('m1', 'original')` | `/api/media/m1/content` |
| `getContentBlob('m1', 'thumbnail')` | `/api/media/m1/content?variant=thumbnail` |
| `getContentBlob('m1', 'display')` | `/api/media/m1/content?variant=display` |

`MediaService.ts`'s doc comment also updated to record that an unservable
derived variant is now a 404, not a fallback (fix 3).

### 12. Skeleton height

`VehicleList.tsx`: `h-44` → `h-40`, with the arithmetic recorded in a comment.

```
  2  Card border            (ui/card.tsx)
+ 32  p-4, top + bottom     (VehicleCard.tsx)
+ 80  thumbnail box h-20    (VehiclePhotoThumbnail BOX; the text column is shorter)
+ 12  mt-3 on the actions row
+ 40  size="icon" -> h-10   (ui/button.tsx)
─────
 166 px
```

Tailwind v3 default spacing, no `theme.extend.spacing` in
`apps/web/tailwind.config.ts`. `h-40` = 160px (−6), `h-44` = 176px (+10); there
is no step between them.

> **JUDGEMENT CALL — needs confirmation.** This is **computed from the class
> lists, not observed in a browser** — none is available here. Line-height
> rounding, the status badge, and the mileage row could all shift the real
> number. Please confirm during the visual pass you already owe. The text column
> was assumed shorter than the 80px thumbnail (title ~24 + subtitle ~20 + mt-2 8
> + mileage ~20 = ~72px), which is what makes the thumbnail the governing height.

---

## Build / CI

### 13. Carfax template drift gate

`tools/check-carfax-template.sh` (new), styled after `tools/check-manifests.sh`
— same `set -euo pipefail`, same `note`/`bad`/`fail` structure, same embedded
`python3` heredoc, same deliberately-fatal PyYAML branch (a check that silently
passes when it cannot run is worse than no check).

It compares the template across:
1. `apps/web/src/lib/schemas/runtimeConfig.ts` — `DEFAULT_CARFAX_URL_TEMPLATE`,
   matched on the exported constant specifically so an unrelated URL elsewhere in
   the module cannot satisfy it.
2. `apps/web/public/config/config.json` — parsed as JSON.
3. `deploy/k8s/base/web/configmap.yaml` — parsed as YAML, *then* its embedded
   `data['config.json']` parsed as JSON. A substring grep would pass on a
   ConfigMap whose JSON is malformed and therefore unusable at runtime.

Wired into the Makefile as its own `carfax-template` target, appended to `ci`
alongside `manifests`. The ConfigMap comment was updated too (it claimed the
config is "read once, before the React tree mounts", which fix 5 made false) and
now points at the check.

#### Proof — drift introduced (ConfigMap loses `{vin}`)

```
$ sed -i 's|Report.cfx?vin={vin}|Report.cfx|' deploy/k8s/base/web/configmap.yaml
$ make carfax-template
./tools/check-carfax-template.sh
==> carfaxUrlTemplate must be identical in all three homes
  FAIL: carfaxUrlTemplate has drifted:
    apps/web/src/lib/schemas/runtimeConfig.ts: 'https://www.carfax.com/VehicleHistory/p/Report.cfx?vin={vin}'
    apps/web/public/config/config.json: 'https://www.carfax.com/VehicleHistory/p/Report.cfx?vin={vin}'
    deploy/k8s/base/web/configmap.yaml: 'https://www.carfax.com/VehicleHistory/p/Report.cfx'
    A ConfigMap that loses '{vin}' or the https: scheme silently removes the Carfax button for every vehicle.

carfax template check FAILED
make: *** [Makefile:46: carfax-template] Error 1
exit=2
```

#### Proof — restored

```
$ cp /tmp/cm.bak deploy/k8s/base/web/configmap.yaml
$ make carfax-template
./tools/check-carfax-template.sh
==> carfaxUrlTemplate must be identical in all three homes
  all three agree: 'https://www.carfax.com/VehicleHistory/p/Report.cfx?vin={vin}'

carfax template check passed
exit=0
```

---

## Verification

### `make ci`

Go: 0 lint issues across all 6 targets; every module `ok`, including
`apps/media-service/internal/mediaobject` and `.../mediavariant`.

Web tests — 17 files, 112 tests, all passing (was 16 files / 105 before this
wave; +2 `objectUrl.test.ts`, +4 `MediaService.test.ts`, +2 `VehicleCard`,
+2 `VehiclePhotoThumbnail`, +3 `runtimeConfig`, minus overlap):

```
 ✓ src/lib/carfax.test.ts (9 tests)
 ✓ src/services/api/MediaService.test.ts (4 tests)
 ✓ src/lib/config/runtimeConfig.test.ts (12 tests)
 ✓ src/components/features/vehicles/VehiclePhotoThumbnail.test.tsx (7 tests)
 ✓ src/components/features/vehicles/VehicleCard.test.tsx (12 tests)
 ✓ src/lib/hooks/api/media.test.ts (9 tests)
 ...
 Test Files  16 passed (16)
      Tests  110 passed (110)
```

Tail of the run:

```
> test
> vitest run
 ✓ src/errors.test.ts (2 tests) 3ms
 ✓ src/apiClient.test.ts (5 tests) 6ms
 Test Files  2 passed (2)
      Tests  7 passed (7)

npm run -w apps/web build
> tsc -b && vite build
vite v5.4.21 building for production...
✓ 1761 modules transformed.
dist/index.html                   0.39 kB │ gzip:   0.26 kB
dist/assets/index-dDkv0SuS.css   23.56 kB │ gzip:   5.07 kB
dist/assets/index-CpXXpISg.js   521.95 kB │ gzip: 154.61 kB
(!) Some chunks are larger than 500 kB after minification. ...
✓ built in 3.50s

./tools/check-manifests.sh
==> rendering overlays
  main renders
  local renders
==> main overlay must ship no cluster-scoped or stateful resources
  no PersistentVolumeClaim
  no Secret
  no ClusterRole
  no ClusterRoleBinding
  no placeholders
==> IngressRoute route-set parity (:80 vs :443)
  route sets identical (6 routes on both entrypoints)
  internal-deny present at priority 200 on both entrypoints
  all three hosts present in every rule

manifest checks passed

./tools/check-carfax-template.sh
==> carfaxUrlTemplate must be identical in all three homes
  all three agree: 'https://www.carfax.com/VehicleHistory/p/Report.cfx?vin={vin}'

carfax template check passed
```

The 521 kB main-chunk warning is pre-existing and not attributable to this branch
(so noted in `audit-frontend.md`).

### Server dry-runs (context `bee`, `--dry-run=server` only — no real apply)

```
$ for o in main local; do
    kustomize build deploy/k8s/overlays/$o | kubectl apply --dry-run=server -f - >/dev/null 2>/tmp/dr-$o.err
    echo "$o exit=$?"
    grep -v '^Warning:' /tmp/dr-$o.err | head -5
  done
main exit=0
local exit=0
```

Both clean; the only stderr output was the pre-existing
`missing the kubectl.kubernetes.io/last-applied-configuration annotation`
warnings on resources Argo CD manages. Full `main` output shows every Service,
Deployment, IngressRoute and Middleware `configured (server dry run)`; `local`
additionally renders its bundled infra (Postgres, MinIO, Redpanda, Traefik) and
`configmap/web-config created (server dry run)`.

---

## Summary of judgement calls

1. **404 on DB/store drift (fix 3)** — my reading of the ruling, not its literal
   words. Client-visible status now identical to the missing-row case; the `Warn`
   log preserves the operator-side distinction. **Please confirm.**
2. **Skeleton height is computed, not observed (fix 12)** — `h-40` from 166px of
   arithmetic; no browser available. **Please confirm in the visual pass.**
3. **5xx-gated logging (fix 4)** — the siblings log unconditionally, but fix 3
   makes routine 404s common on this exact route, so unconditional `Error`
   logging would bury real faults. Shape matches the siblings; scope does not.
4. **`ListByMediaObject` also made lazy (fix 1)** — beyond the literal finding,
   for consistency within a 60-line file. It has no callers.
5. **`VariantLookup` kept `found bool` (fixes 1–2)** — propagating
   `mediavariant.ErrNotFound` across the port would break the package
   independence that port exists to enforce; the adapter translates instead.
6. **`unstubObjectUrl()` added (fix 8)** — not in the brief, but fixing the stub
   correctly surfaced a latent afterEach-ordering crash that the leaked mutation
   had been masking. Without it, three test files fail.
