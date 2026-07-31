# Frontend Audit — task-005-vehicle-card-photo-actions

- **Audit Scope:** the 18 `apps/web/` files changed in `352e8c1..84b025e` (Go, `deploy/`, and docs changes excluded)
- **Guidelines Source:** `frontend-dev-guidelines` skill (`.claude/skills/frontend-dev-guidelines/`)
- **Date:** 2026-07-31
- **Build:** PASS
- **Lint:** PASS (`eslint src --max-warnings 0`, 0 warnings)
- **Tests:** 42 passed, 0 failed (5 in-scope test files)
- **Overall:** NEEDS-WORK

## Build & Test Results

```
$ npm run build            # apps/web
> tsc -b && vite build
✓ 1759 modules transformed.
dist/assets/index-VIwqDX9Q.js   521.77 kB │ gzip: 154.51 kB
✓ built in 3.58s
(!) Some chunks are larger than 500 kB after minification.   ← pre-existing, not introduced here

$ npm run lint
> eslint src --max-warnings 0
(no output)

$ npx vitest run src/lib/carfax.test.ts src/lib/config/runtimeConfig.test.ts \
    src/lib/hooks/api/media.test.ts \
    src/components/features/vehicles/VehicleCard.test.tsx \
    src/components/features/vehicles/VehiclePhotoThumbnail.test.tsx
 ✓ src/lib/carfax.test.ts (9 tests)
 ✓ src/lib/config/runtimeConfig.test.ts (9 tests)
 ✓ src/components/features/vehicles/VehiclePhotoThumbnail.test.tsx (5 tests)
 ✓ src/components/features/vehicles/VehicleCard.test.tsx (10 tests)
 ✓ src/lib/hooks/api/media.test.ts (9 tests)
 Test Files  5 passed (5)
      Tests  42 passed (42)
```

The full suite was not re-run per the audit invocation (`make ci` confirmed independently); the
five in-scope test files were run directly as evidence.

Also verified: `dist/config/config.json` is emitted by the production build (90 bytes), so the
compiled-in document ships in the image and the ConfigMap mount is an override, not a requirement.

## File Inventory

| File | Classification |
|------|----------------|
| `apps/web/src/types/models/media.ts` | Type |
| `apps/web/src/services/api/MediaService.ts` | Service |
| `apps/web/src/lib/hooks/api/media.ts` | Hook |
| `apps/web/src/lib/hooks/api/media.test.ts` | Test |
| `apps/web/src/lib/config/runtimeConfig.ts` | Other (lib — runtime config + Zod schema) |
| `apps/web/src/lib/config/runtimeConfig.test.ts` | Test |
| `apps/web/src/lib/carfax.ts` | Other (lib — pure function) |
| `apps/web/src/lib/carfax.test.ts` | Test |
| `apps/web/src/main.tsx` | Other (app entry) |
| `apps/web/src/test/renderWithProviders.tsx` | Other (test helper) |
| `apps/web/src/test/objectUrl.ts` | Other (test helper) |
| `apps/web/src/components/features/vehicles/VehiclePhotoThumbnail.tsx` | Component |
| `apps/web/src/components/features/vehicles/VehiclePhotoThumbnail.test.tsx` | Test |
| `apps/web/src/components/features/vehicles/VehicleCard.tsx` | Component |
| `apps/web/src/components/features/vehicles/VehicleCard.test.tsx` | Test |
| `apps/web/src/components/features/vehicles/VehicleList.tsx` | Component |
| `apps/web/public/config/config.json` | Other (static asset) |
| `apps/web/nginx.conf` | Other (serving config) |

## Anti-Pattern Checklist

| ID | Check | Status | Evidence |
|----|-------|--------|----------|
| FE-01 | No `any` type | PASS | `grep -n ': any\|as any'` across all 18 in-scope files → 0 matches. Corroborated by `npm run lint` passing at `--max-warnings 0`. Unknown input is typed `unknown` and narrowed by Zod: `runtimeConfig.ts:47`. |
| FE-02 | No manual class concatenation | PASS | `cn()` used at `VehiclePhotoThumbnail.tsx:31`, `:70`, `:74`, `:81`. `grep -nE 'className=\{[^}]*[+\`]'` over `components/features/vehicles/*.tsx` → 0 matches. `VehicleCard.tsx` uses only static class strings (`:35`, `:36`, `:41`, `:63`), so no `cn()` is required. |
| FE-03 | No direct API client calls in components | PASS | Neither `VehicleCard.tsx` nor `VehiclePhotoThumbnail.tsx` imports `lib/api/client`; the thumbnail goes through the hook (`VehiclePhotoThumbnail.tsx:4`, `:64`) which goes through the service (`media.ts:3`, `:136`). |
| FE-04 | No inline Zod schemas in components | PASS | Zero `z.` usage in any `.tsx` in scope. The only new Zod schema is in a lib module, not a component (`runtimeConfig.ts:40`) — see FE-14. |
| FE-05 | No spinners for content loading | PASS | `grep -rn 'animate-spin'` over `src/components/features/vehicles/*.tsx` and `src/lib/` → 0 matches in changed files. Loading renders `<Skeleton>` (`VehiclePhotoThumbnail.tsx:70`, `VehicleList.tsx:15`). Ordering is correct: `isLoading` is checked at `:69`, **before** the error/no-url branch at `:72`, so the placeholder cannot flash. |
| FE-06 | No hardcoded colors | PASS | `grep -nE '(bg\|text\|border)-(white\|black\|gray\|slate\|red\|green\|blue\|yellow\|zinc\|neutral)-?[0-9]*'` over the three changed components → 0 matches. Semantic tokens only: `bg-muted text-muted-foreground` (`VehiclePhotoThumbnail.tsx:33`), `text-foreground` / `text-muted-foreground` (`VehicleCard.tsx:44`, `:45`, `:53`). |
| FE-07 | No state mutation | PASS | The only new state is `useState<{blob;url}\|null>` (`media.ts:142`) and it is always replaced with a fresh object literal (`media.ts:150`) or `null` (`media.ts:146`). No `.push`/`.splice`/`.sort` on state anywhere in scope. |
| FE-08 | No default exports for components | PASS | `grep -rn 'export default' src/` → 0 matches repo-wide. Named exports at `VehicleCard.tsx:24`, `VehiclePhotoThumbnail.tsx:26` and `:59`. |
| FE-09 | Error handling via `createErrorFromUnknown` | WARN | Component path is fine: the thumbnail surfaces failure through error **state**, not a toast (`VehiclePhotoThumbnail.tsx:64`, `:72-75`) — deliberate, and `createErrorFromUnknown` already ran inside `ApiClient.requestBlob` (`packages/shared-ts/src/apiClient.ts:62`). **Deviation:** `runtimeConfig.ts:80-82` catches with a bare `console.warn`, which `anti-patterns.md:12` names as the anti-pattern. Defensible (runs before the `<Toaster>` exists — `AppProviders.tsx:26`) but the codebase's classification helper is bypassed. Non-blocking. |

## Architecture Checklist

| ID | Check | Status | Evidence |
|----|-------|--------|----------|
| FE-10 | JSON:API model shape | PASS | `types/models/media.ts:28` — `MediaObject = JsonApiResource<MediaObjectAttributes>`; `:46` — `VehicleMedia = JsonApiResource<VehicleMediaAttributes>`. The new `MediaVariant` (`:9`) is a request enum, not a model, so `{id, attributes}` does not apply. |
| FE-11 | Service extends `BaseService` (when applicable) | PASS | `MediaService` uses the documented **Direct API Client Pattern** for simple resources (`patterns-service-layer.md:56`): private `basePath` at `MediaService.ts:24`, singleton export at `:96`. Content bytes cannot go through `BaseService`, whose verbs all return `JsonApiDocument` (`BaseService.ts:27`, `:36`); `requestBlob` returns a `Blob` (`apiClient.ts:59`). Correct choice. |
| FE-12 | Query key factory uses `as const` | PASS | `media.ts:19-31`, every branch terminates in `as const`; the new variant branch at `:27-28`. Hierarchy is preserved — `contents()` (`:22`) is still the prefix of `content(id, variant)`, so prefix invalidation matches every variant of an id. Pinned by `media.test.ts:24-25` for both the defaulted and explicit variant. No caller invalidates `contents()` today (`grep -rn 'mediaKeys.content'` → only the factory and the hook), so no invalidation regression exists. |
| FE-13 | Forms use `react-hook-form` + `zodResolver` | N/A | No form component was added or modified in this branch. |
| FE-14 | Schema in `lib/schemas/` with inferred type | FAIL | `runtimeConfigSchema` is declared at `runtimeConfig.ts:40-44`, outside `src/lib/schemas/`, and is **not** paired with `z.infer` — the exported type is a hand-written `interface RuntimeConfig` (`runtimeConfig.ts:15-17`). Every pre-existing schema follows the convention, e.g. `src/lib/schemas/fleet.ts:5` + `:9`. Non-blocking in practice (the shapes agree today) but see Finding 4 for the drift direction that types out silently. |

## Styling Checklist

| ID | Check | Status | Evidence |
|----|-------|--------|----------|
| FE-15 | Interactive elements show `cursor-pointer` | PASS | Both actions are `<Button asChild>` (`VehicleCard.tsx:68`, `:78`), and `cursor-pointer` is the last token of the base CVA string in `src/components/ui/button.tsx:7` — `Slot` merges that `className` onto the rendered `<a>`. No other clickable surface was introduced: the card body is no longer a link (`VehicleCard.tsx:35` renders a bare `<Card>`; the old wrapping `<Link className="block">` and its `cursor-pointer` are gone), and correctly so — `VehicleCard.test.tsx:73-80` asserts exactly one link and that the thumbnail has no ancestor `<a>`. The `<img>`/placeholder carry no handler (`VehiclePhotoThumbnail.tsx:28-38`, `:78-82`). |

## Testing Checklist

| ID | Check | Status | Evidence |
|----|-------|--------|----------|
| FE-16 | Tests exist for changed components | WARN | Both new/rebuilt components have tests (`VehicleCard.test.tsx`, `VehiclePhotoThumbnail.test.tsx`); both new lib modules have tests (`carfax.test.ts`, `runtimeConfig.test.ts`); the hook change is covered (`media.test.ts:24-25`, `:191-257`). Three stated invariants are nonetheless unpinned — see Findings 2, 3 and 8. `VehicleList.tsx:15` (`h-28`→`h-44`) has no test; trivial in isolation but see Finding 8. `main.tsx` has no test, so nothing asserts the app still mounts when the config fetch fails — the property `runtimeConfig.test.ts` works hardest to guarantee at the module level. |
| FE-17 | Mocks updated when services changed | PASS | No `__mocks__/` directory exists in this app; the convention is inline `vi.mock`. All three consumers were updated for the widened `getContentBlob` signature: `media.test.ts:14-18`, `VehiclePhotoThumbnail.test.tsx:9-11` (with the two-arg assertion at `:40`), `VehicleCard.test.tsx:10-12`. |

## Verified Design Intents

Checked explicitly and found correctly implemented — recorded so a later reader does not re-litigate them:

- **No build-time config.** `grep -rn 'import.meta.env\|VITE_' src/` returns exactly one hit, and it is prose inside a comment (`runtimeConfig.ts:6`). No `VITE_*` variable is read anywhere.
- **`'original'` sends no query parameter.** `MediaService.ts:66` — `const suffix = variant === 'original' ? '' : \`?variant=${variant}\``. The untouched gallery path (`MediaThumbnail.tsx:42`, `useMediaContentUrl(mediaId)` with no variant) therefore produces a byte-identical request. (Untested — Finding 3.)
- **Carfax fails closed.** `carfax.ts:32` rejects any scheme but `https:`, `:25` rejects a template that ignores `{vin}`, `:24` rejects an empty/whitespace VIN, `:33` rejects an unparseable URL. `encodeURIComponent` at `:29`. Nine cases pinned at `carfax.test.ts:34-59`, including `javascript:`, `http:` and `data:`.
- **Nothing contacts Carfax before a click.** The href is computed by a pure string function (`carfax.ts:22`) and rendered on a plain `<a>` (`VehicleCard.tsx:79`); no prefetch, no `<Link>`, no `fetch`.
- **Both anchors name the vehicle.** `VehicleCard.tsx:69` and `:83`, asserted by name at `VehicleCard.test.tsx:64`, `:70`, `:86`. `target="_blank"` + `rel="noopener noreferrer"` at `:81-82`, asserted at `:92` and `:95`.
- **`min-w-0` on both flex children.** `VehicleCard.tsx:41` (the flex-1 column) and `:43` (the inner text column), with `shrink-0` on the thumbnail (`VehiclePhotoThumbnail.tsx:11`) as the third leg. (Layout-only, so unpinned by jsdom — a `toHaveClass('min-w-0')` assertion would at least catch deletion.)
- **Object-URL lifecycle.** Create and revoke are a matched pair in one effect keyed on `data` (`media.ts:144-152`), and the stale-URL guard at `:158` (`entry.blob === data`) is what prevents an already-revoked URL being handed back during the one render where state lags. Covered by three real regression tests: revoke-once-on-id-change (`media.test.ts:191-220`), revoke-once-on-unmount (`:222-237`), and create/revoke parity under `<React.StrictMode>` (`:239-257`).
- **Loading order.** `isLoading` precedes the error/no-url branch (`VehiclePhotoThumbnail.tsx:69` vs `:72`). This is genuinely caught by `VehiclePhotoThumbnail.test.tsx:65-76`: `PhotoPlaceholder` carries `role="img"` (`:28`) while `Skeleton` is `aria-hidden` (`ui/skeleton.tsx:6`), so swapping the two branches makes `queryByRole('img')` non-null and the test fails.
- **Distinct placeholder labels.** `VehiclePhotoThumbnail.tsx:67` (`"No photo"`) vs `:74` (`"Photo unavailable"`), asserted separately at `VehiclePhotoThumbnail.test.tsx:47` and `:61`.
- **`no-cache` on the config document.** `nginx.conf` exact-match `location = /config/config.json` with `add_header Cache-Control "no-cache"`. It correctly sits outside the SPA fallback, so a missing ConfigMap yields a 404 rather than `index.html`, which `runtimeConfig.ts:76` turns into the built-in defaults.

## Summary

### Blocking (must fix)

None. Build, lint and tests are green and no anti-pattern check fails outright.

### Important (should fix before merge)

1. **Mounting the whole app is blocked on the config fetch, with no shell to look at.**
   `main.tsx:20-28` defers `createRoot(...).render(...)` until `loadRuntimeConfig()` resolves, and
   `index.html:9` is `<div id="root"></div>` with nothing inside it. A slow or wedged
   `/config/config.json` therefore yields a fully blank page for up to the full
   `FETCH_TIMEOUT_MS = 2000` (`runtimeConfig.ts:34`) on **every** route — for a value whose only
   consumer is one optional icon button (`VehicleCard.tsx:32`). The in-file comment ("the app
   already gates its first meaningful render on the auth bootstrap") does not hold: the auth
   bootstrap runs *inside* React, so the shell is already mounted and painting. Either render
   first and re-render when the config latches, or put a static shell in `index.html` so the
   2 s ceiling is a visible skeleton rather than white. The 2 s bound is the right instinct;
   the placement of the await is the defect.

2. **The "zero notifications" invariant has no regression test.**
   `VehiclePhotoThumbnail.tsx:56-57` states it as a construction guarantee ("N broken thumbnails
   produce N placeholders and zero notifications"), but neither `VehiclePhotoThumbnail.test.tsx`
   nor `VehicleCard.test.tsx` references `sonner` at all — no mock, no assertion
   (`grep -n 'sonner\|toast'` over both files → 0 matches). Adding `toast.error(...)` to the
   `isError` branch would leave all 42 tests green. Add `vi.mock('sonner', …)` plus
   `expect(toast.error).not.toHaveBeenCalled()` to the failed-load test
   (`VehiclePhotoThumbnail.test.tsx:51-63`).

3. **The `'original'` → no-query-parameter contract is untested.**
   `MediaService.ts:65-68` is the single line protecting every pre-existing caller from a changed
   request, and `src/services/api/` contains no test files at all (`find src -name '*.test.ts*'`
   lists 15 files, none under `services/`). The only variant coverage is at the hook boundary
   (`VehiclePhotoThumbnail.test.tsx:40`), which asserts what the hook passes down, not what the
   service puts on the wire. Rewriting line 66 to always emit `?variant=original` would pass CI.
   A four-line test spying on `apiClient.requestBlob` and asserting the exact path for all three
   variants would close it.

### Non-Blocking (should fix)

4. **FE-14 — `runtimeConfigSchema` sits outside `lib/schemas/` and skips `z.infer`.**
   `runtimeConfig.ts:40-44` vs the hand-written `interface RuntimeConfig` at `:15-17`. The two
   agree today, and the *removal* direction is caught by the compiler (`parseRuntimeConfig`'s
   declared return at `:47-49` would no longer satisfy the interface). The *addition* direction is
   not: adding a key to the schema without adding it to the interface silently drops it from every
   consumer's type. `export type RuntimeConfig = z.infer<typeof runtimeConfigSchema>` removes the
   whole class, and matches `lib/schemas/fleet.ts:9`.

5. **FE-09 — `console.warn` instead of `createErrorFromUnknown`.**
   `runtimeConfig.ts:81`. Justified (pre-mount, no `<Toaster>` yet — `AppProviders.tsx:26`) but it
   bypasses the project's error classifier, so the logged value is whatever was thrown: a
   `TypeError`, an `Error('config request returned 404')` (`:77`), or an abort. Running it through
   `createErrorFromUnknown` before logging costs one line and makes the warning structured.

6. **`stubObjectUrl`'s cleanup is a no-op, and three test files rely on it.**
   `test/objectUrl.ts:16` — `vi.stubGlobal('URL', Object.assign(URL, { createObjectURL, revokeObjectURL }))`.
   `Object.assign` mutates the real `URL` constructor *before* `vi.stubGlobal` snapshots the
   "original", so the recorded original **is** the mutated object. `vi.unstubAllGlobals()` at
   `VehicleCard.test.tsx:41`, `VehiclePhotoThumbnail.test.tsx:18` and `media.test.ts:147` therefore
   restores the mocks rather than removing them; the two methods stay bolted to the global `URL`
   for the remainder of the worker's life. Harmless today because Vitest isolates per file and
   every consumer re-stubs in `beforeEach`, but the cleanup reads as protection it does not
   provide. Capture the pre-existing descriptors and restore them, or stub the two methods
   individually rather than reassigning the constructor.

7. **`"No photo"` can be shown for a vehicle that has one.**
   `VehiclePhotoThumbnail.tsx:72-75` reaches the `isError ? … : 'No photo'` ternary whenever
   `isLoading` is false, `isError` is false and `url` is null. React Query produces exactly that
   combination for a **paused** query (offline / no network), where `isPending` is true but
   `isFetching` is false, so `isLoading` is false and `data` is undefined. The user is then told
   the vehicle has no photo when it merely could not be fetched — the precise distinction the two
   labels exist to preserve. `mediaId` is already known non-empty on that line (guarded at `:66`),
   so the label can simply be `'Photo unavailable'` unconditionally.

8. **`h-44` is close to, but not equal to, the rebuilt card.**
   `VehicleList.tsx:15` renders an `h-44` (176 px) skeleton. The card measures roughly
   2 (border, `ui/card.tsx:8`) + 32 (`p-4`, `VehicleCard.tsx:35`) + 80 (thumbnail box,
   `VehiclePhotoThumbnail.tsx:11`; the text column is shorter) + 12 (`mt-3`, `VehicleCard.tsx:63`)
   + 40 (`size="icon"` → `h-10`, `ui/button.tsx:20`) ≈ 166 px, so the skeleton→content swap still
   shifts layout by ~10 px — the exact jitter the height bump was meant to remove. Nothing pins
   the two values together, so they will drift again the next time the card changes.

### Observations (no action required)

- **Any https host is accepted as a Carfax target.** `carfax.ts:32` validates the scheme but not
  the host, so whoever can edit the `web-config` ConfigMap can point the button — and the VIN in
  its query string — at an arbitrary https origin. `rel="noreferrer"` (`VehicleCard.tsx:82`) caps
  the leak at the VIN. Reasonable given ConfigMap edit rights are already privileged; recorded for
  the threat model rather than as a defect.
- **`role="img"` on the no-photo placeholder is announced per card.** `VehiclePhotoThumbnail.tsx:28-29`
  gives every photoless card an accessible name, so a 20-vehicle fleet with no photos yields 20
  "No photo" entries in a screen reader's image list. This is the stated design intent and the
  label is what distinguishes the failure case; if it proves noisy in use, `aria-hidden` on the
  no-photo variant and a label only on `'Photo unavailable'` keeps the distinction that matters
  while dropping the repetition.
- The 521 kB main-chunk warning in `vite build` is pre-existing and not attributable to this branch.
