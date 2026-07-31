# Frontend Audit — task-002-k3s-cluster-deployment

- **Audit Scope:** TypeScript/React changes in `16e4aa7..a8d5221` (media bytes proxied through media-service instead of presigned MinIO URLs)
- **Guidelines Source:** `.claude/skills/frontend-dev-guidelines` (SKILL.md + resources/)
- **Date:** 2026-07-30
- **Build:** PASS
- **Tests:** 63 passed / 0 failed (apps/web), 5 passed / 0 failed (@myfleet/shared-ts)
- **Overall:** NEEDS-WORK

## Build & Test Results

```
> npm run -w apps/web build
> tsc -b && vite build
vite v5.4.21 building for production...
✓ 1756 modules transformed.
dist/index.html                   0.39 kB │ gzip:   0.26 kB
dist/assets/index-ahPuPw2H.css   23.64 kB │ gzip:   5.06 kB
dist/assets/index-BkfyF9u3.js   516.78 kB │ gzip: 152.93 kB
(!) Some chunks are larger than 500 kB after minification.
✓ built in 4.30s
```

```
> npm run -w apps/web test
RUN  v2.1.9
 ✓ src/lib/schemas/fuel.test.ts (12 tests)
 ✓ src/lib/schemas/maintenanceSchedule.test.ts (15 tests)
 ✓ src/lib/api/client.test.ts (1 test)
 ✓ src/lib/hooks/api/activity.test.ts (5 tests)
 ✓ src/lib/hooks/api/mileage.test.ts (4 tests)
 ✓ src/lib/hooks/api/vehicles.test.ts (1 test)
 ✓ src/lib/hooks/api/notifications.test.ts (5 tests)
 ✓ src/lib/hooks/api/dashboard.test.ts (4 tests)
 ✓ src/context/AuthContext.test.tsx (3 tests)
 ✓ src/lib/hooks/api/members.test.ts (7 tests)
 ✓ src/lib/hooks/api/media.test.ts (6 tests)
 Test Files  11 passed (11)
      Tests  63 passed (63)
```

```
> npm run -w @myfleet/shared-ts test
RUN  v2.1.9
 ✓ src/errors.test.ts (2 tests)
 ✓ src/apiClient.test.ts (3 tests)
 Test Files  2 passed (2)
      Tests  5 passed (5)
```

The build/test gate passes. Overall status is NEEDS-WORK because of FAIL checks below, not the gate.

## File Inventory

| File | Classification |
|------|----------------|
| `packages/shared-ts/src/apiClient.ts` | Other (shared API client) |
| `packages/shared-ts/src/apiClient.test.ts` | Other (test, new) |
| `apps/web/src/services/api/MediaService.ts` | Service |
| `apps/web/src/services/api/BaseService.ts` | Service (comment-only change) |
| `apps/web/src/lib/api/client.ts` | Other (comment-only change) |
| `apps/web/src/lib/hooks/api/media.ts` | Hook |
| `apps/web/src/lib/hooks/api/media.test.ts` | Other (test) |
| `apps/web/src/components/features/vehicles/media/MediaThumbnail.tsx` | Component |
| `apps/web/src/components/features/vehicles/media/MediaUploadButton.tsx` | Component (comment-only change) |
| `apps/web/src/components/features/vehicles/media/VehicleMediaGallery.tsx` | Component (comment-only change) |
| `apps/web/src/types/models/media.ts` | Type |

## Anti-Pattern Checklist

| ID | Check | Status | Evidence |
|----|-------|--------|----------|
| FE-01 | No `any` type | PASS | Grep for `: any` / `as any` / `<any>` over all in-scope files: zero matches. `tsc -b` clean under strict mode. |
| FE-02 | No manual class concatenation | **FAIL** | `MediaThumbnail.tsx:26` and `MediaThumbnail.tsx:38` build `className` by template-literal interpolation instead of `cn()`. `cn()` exists at `apps/web/src/lib/utils.ts:9` and is used in 14 other components. |
| FE-03 | No direct API client calls in components | PASS | Grep for `lib/api/client` in `components/features/vehicles/media/*.tsx`: zero matches. Components go through hooks (`MediaThumbnail.tsx:2`, `VehicleMediaGallery.tsx:6-10`) → `mediaService` (`media.ts:3`) → `apiClient` (`MediaService.ts:2`). |
| FE-04 | No inline Zod schemas in components | PASS | Grep for `z.object(` / `z.string(` over in-scope files: zero matches. No forms touched. |
| FE-05 | No spinners for content loading | PASS | Only `animate-spin` match is `MediaUploadButton.tsx:61`, on the submit `<Button>` — explicitly allowed. Content loading uses `<Skeleton>` at `MediaThumbnail.tsx:20` and `VehicleMediaGallery.tsx:61`. |
| FE-06 | No hardcoded colors | PASS | Grep for `(bg\|text\|border)-(white\|black\|gray\|slate\|zinc\|red\|blue\|green\|yellow)`: zero matches. Semantic tokens only — `bg-muted`/`text-muted-foreground` (`MediaThumbnail.tsx:26`), `bg-primary`/`text-primary-foreground` (`MediaThumbnail.tsx:41`), `text-destructive` (`VehicleMediaGallery.tsx:93`). |
| FE-07 | No state mutation | PASS | Grep for `.push(` / `.splice(` / `.sort(`: zero matches. `useMediaContentUrl` replaces state wholesale (`media.ts:99`, `media.ts:103`). |
| FE-08 | No default exports for components | PASS | Grep for `export default`: zero matches. Named exports at `MediaThumbnail.tsx:15`, `VehicleMediaGallery.tsx:25`, `MediaUploadButton.tsx:21`, `media.ts:86`, singleton at `MediaService.ts:85`. |
| FE-09 | Error handling with `createErrorFromUnknown` | **FAIL** | Mutation paths comply: `MediaUploadButton.tsx:39-42`, `VehicleMediaGallery.tsx:34-37`, `:44-47`. Client-level transforms comply: `apiClient.ts:50`, `apiClient.ts:63`. **But `useMediaContentUrl` (`media.ts:87-113`) destructures only `{ data, isLoading }` and returns only `{ url, isLoading }` — `isError`/`error` are discarded.** A 403/404/5xx on the content fetch is therefore indistinguishable from "no bytes": `MediaThumbnail.tsx:23-31` renders the literal text "No image" with no toast and no error state. The hook it replaced (`useMediaDownloadUrl`) returned the whole `useQuery` result, so callers previously *could* reach `isError`/`error`. This is a regression in the hook's surface introduced by this branch. |

## Architecture Checklist

| ID | Check | Status | Evidence |
|----|-------|--------|----------|
| FE-10 | JSON:API model shape | PASS | `types/models/media.ts:20` — `MediaObject = JsonApiResource<MediaObjectAttributes>`; `:38` — `VehicleMedia = JsonApiResource<VehicleMediaAttributes>`. `{ id, attributes }` preserved. Removal of `uploadUrl`/`downloadUrl` (`types/models/media.ts:8-18`) matches the backend, which no longer emits them (grep for `UploadURL\|DownloadURL\|uploadUrl\|downloadUrl` over `apps/media-service/internal`: zero matches). |
| FE-11 | Service extends `BaseService` (when applicable) | PASS | `MediaService.ts:18-85` uses the documented direct-API-client pattern (`patterns-service-layer.md` §"2. Direct API Client Pattern"), exported as a singleton at `MediaService.ts:85`. Consistent with the pre-existing shape of this file. |
| FE-12 | Query key factory uses `as const` | PASS | `media.ts:13-21` — every member ends in `as const`, hierarchical via spread. New branch `contents()`/`content(id)` at `media.ts:17-18` follows the pattern; asserted at `media.test.ts:123`. |
| FE-13 | Forms use `react-hook-form` + `zodResolver` | N/A | No form components changed in this diff. |
| FE-14 | Schema in `lib/schemas/` with inferred type | N/A | No schemas changed in this diff. |

### Deeper architecture review (beyond the mechanical table)

**Maintainer ruling 1 — `requestBlob` shares `fetchAuthenticated`: VERIFIED, with a test gap.**
`request`'s observable behaviour is byte-for-byte preserved. `apiClient.ts:29-33` merges `...defaultHeaders` → `...Authorization` → `...(init.headers ?? {})`, and `request` passes `{ 'Content-Type': 'application/vnd.api+json' }` as `defaultHeaders` (`apiClient.ts:46`). The spread order is identical to the pre-refactor code (see diff), so `init.headers` still wins and the binary PUT's `Content-Type: image/jpeg` override at `MediaService.ts:48` genuinely displaces the JSON:API media type. The 401 recursion moved from `request` to `fetchAuthenticated` (`apiClient.ts:35-38`) but still re-reads the token via `getAccessToken()` on the retry (`apiClient.ts:26`), so the refreshed token is used. **However, nothing tests this.** The only `request()` test in the repo is `apps/web/src/lib/api/client.test.ts:14-16`, which asserts `Authorization` only; `packages/shared-ts/src/apiClient.test.ts` covers `requestBlob` exclusively (lines 25, 55, 87, 90). The exact invariant the ruling names — caller `Content-Type` wins — is unpinned in a file that was just refactored. See FE-16.

**Maintainer ruling 2 — state+effect object-URL lifecycle: VERIFIED SOUND. Not re-litigated.**
Every `createObjectURL` at `media.ts:102` is matched by exactly one `revokeObjectURL` at `media.ts:104` in the same effect's cleanup, keyed on `[data]` (`media.ts:105`).
- *Unchanged re-render:* effect does not re-run (deps stable), no create, no revoke.
- *`data` change:* the render that observes the new blob computes `url = null` via the identity guard at `media.ts:111`, so `isLoading` is `true` (`media.ts:113`) and `MediaThumbnail.tsx:19-21` swaps the `<img>` for a `<Skeleton>` **during the commit mutation phase**, which precedes the passive-effect cleanup that revokes. A rendered `<img>` therefore cannot hold a revoked URL.
- *Unmount:* DOM teardown precedes cleanup; same ordering argument.
- *StrictMode replay:* create → revoke → create, with both `setEntry` calls batched inside one passive-effect flush, so no intermediate render ever exposes the revoked first URL. `main.tsx:11` confirms StrictMode is actually active in dev. `media.test.ts:340-358` asserts `createObjectURL.calls === revokeObjectURL.calls` after StrictMode mount + unmount.
- *`setState` after unmount / update-during-render:* none. `setEntry` is only called synchronously in the effect body (`media.ts:99`, `media.ts:103`), never in render, never in an async continuation. The `if (!data) setEntry(null)` branch at `media.ts:98-101` is idempotent (React bails on `Object.is(null, null)`), so no render loop.

**React Query — key shape, `enabled`, invalidation, cache growth.**
- Key shape and stability: PASS, see FE-12.
- `enabled` guards present at `media.ts:90` (and `:64`, `:121`). Note the pre-existing idiom `mediaKeys.content(id ?? '')` at `media.ts:88` collapses all nullish-id observers onto `['media','content','']` — harmless only because they are disabled. Same idiom at `:62` and `:119`, unchanged by this branch.
- **Blob cache growth is a real, this-branch-introduced concern.** `media.ts:91-92` sets `staleTime: 5min`, `gcTime: 6min` on a query whose payload is now the **full original image bytes** (previously it was a presigned-URL string). `MEDIA_MAX_UPLOAD_BYTES` is 25 MiB (`deploy/k8s/base/media-service/configmap.yaml:14`, `apps/media-service/cmd/main.go:91`), and `GET /media/{id}/content` streams the original object — there is no thumbnail variant in the route table (`apps/media-service/internal/mediaobject/resource.go:134-158`). A gallery of N photos therefore pins N originals in the JS heap for up to 6 minutes after the last observer unmounts, to render them at `h-24 w-24` (`VehicleMediaGallery.tsx:73`). The backend already sets `Cache-Control: private, max-age=300` (`resource.go:152`), so the browser HTTP cache would cover repeat fetches without the React Query blob retention.
- **Invalidation gap after delete.** `useDeleteMedia.onSettled` (`media.ts:191-195`) invalidates only `mediaKeys.vehicleMedia(vehicleId)`. It never removes `mediaKeys.content(mediaId)` or `mediaKeys.detail(mediaId)`, so a deleted photo's bytes stay resident for the full `gcTime`. Cheap fix: `queryClient.removeQueries({ queryKey: mediaKeys.content(mediaId) })`.
- Invalidation after upload/confirm is correct-if-redundant: `useUploadMedia.onSettled` (`media.ts:144-148`) and `useAddVehicleMedia.onSettled` (`media.ts:160-164`) both invalidate the same vehicle-media key, and `MediaUploadButton.tsx:36-37` awaits both in sequence. Nothing invalidates `mediaKeys.detail(id)` after `confirm`, so a `useMediaObject` observer shows the pre-confirm status for up to `staleTime` 30s (`media.ts:65`) — pre-existing, unchanged.

**Upload path.**
- Bytes are sent correctly and are **not** mislabeled: `MediaService.ts:42-52` passes `body: file` with an explicit `Content-Type` override that wins per the spread order at `apiClient.ts:32`. Backend ignores the request content type and uses the row's stored value anyway (`apps/media-service/internal/mediaobject/processor.go`, `StoreContent`), so relabelling is not exploitable.
- **413 does not surface as something actionable.** Two problems:
  1. There is **no client-side size pre-check**. `MediaUploadButton.tsx:47-53` restricts only `accept="image/*"`; any size is streamed to the server. The server enforces the cap with `http.MaxBytesReader` (`resource.go:81`), which responds 413 and closes without draining the remaining request body — a browser still uploading will commonly see a connection reset and `fetch` will reject with `TypeError: Failed to fetch` rather than resolving with the 413 response. That path lands at `MediaUploadButton.tsx:39-41` as `createErrorFromUnknown(TypeError)` → `ApiError(0, 'unknown', 'Failed to fetch')` → toast **"Failed to fetch"**. A pre-flight `file.size` check against the 25 MiB cap makes this deterministic and removes the ambiguity entirely.
  2. Even when the 413 *is* received cleanly, the toast reads **"request entity too large"** — the raw Go error string (`packages/shared-go/server/errors.go:11` → `WriteError` `Title: err.Error()`), with no mention of the limit or the file. Additionally `codeFor` has no `413` case (`packages/shared-go/server/server.go:12-28`), so the JSON:API `code` comes back as `internal_error`; any future frontend branching on `apiError.code` would misclassify a size rejection as a server fault. (Backend-owned; flagged here because it is the frontend's only signal.)
- **Orphaned media rows on a failed PUT.** `performMediaUpload` (`media.ts:45-52`) creates the row via `initUpload` before `putContent`; if `putContent` throws (413, reset, 5xx) the row is stranded in `uploaded` with no bytes and no cleanup. The init-then-upload shape is *pre-existing*, but this branch materially raises exposure: the size cap is now enforced at this exact step, whereas the old presigned PUT went straight to MinIO and never produced a 413 from the API. Worth a follow-up (best-effort `mediaService.remove(media.id)` in a `catch`, or a server-side sweeper for stale `uploaded` rows).

**Error/loading states in `MediaThumbnail`.** With `retry: 1` as the client default (`AppProviders.tsx:16`), a 403/404/5xx settles after one retry into `data === undefined`, `isLoading === false`, `url === null` — rendering the "No image" div (`MediaThumbnail.tsx:23-31`). No toast, no retry affordance, no distinction from a genuinely absent image. See FE-09.

**Type accuracy.** No `any`, no casts papering over the removed attributes, and no dead references: grep for `downloadUrl|uploadUrl|putToPresignedUrl|getDownloadUrl|useMediaDownloadUrl|mediaKeys.download|presign` over `apps/web/src` and `packages/shared-ts/src` returns a single hit — the explanatory prose at `MediaService.ts:15`. `tsc -b` is clean. Minor nit: the `id as string` casts at `media.ts:63`, `:89`, `:120` are non-null assertions in disguise; they are safe (guarded by `enabled: !!id`) and are the file's established idiom, so not a new finding.

**New per-thumbnail metadata request.** `MediaThumbnail.tsx:17` adds `useMediaObject(mediaId)` purely to source the `alt` text at `:37`. The gallery previously issued one request per thumbnail (`useMediaDownloadUrl`, which returned metadata *and* the URL together); it now issues two (`GET /api/media/{id}` + `GET /api/media/{id}/content`), doubling request count for a string used only in `alt`. `VehicleMediaGallery.tsx:68-74` already has `ref.attributes` in hand; consider passing a filename down or dropping the second query for the generic `'Vehicle photo'` fallback.

## Styling Checklist

| ID | Check | Status | Evidence |
|----|-------|--------|----------|
| FE-15 | Interactive elements show `cursor-pointer` | PASS | Every clickable surface in the changed files is a native-`<button>`-backed `<Button>`: `VehicleMediaGallery.tsx:78-87`, `:89-98`, `MediaUploadButton.tsx:54-63`. Per `patterns-styling.md:232`, native `<button>`/`<a>` are exempt. No `PopoverTrigger`/`DialogTrigger`/clickable `<div>`/clickable row in scope. The `<input type="file">` at `MediaUploadButton.tsx:47-53` carries `className="hidden"` and is not a hoverable surface. |

## Testing Checklist

| ID | Check | Status | Evidence |
|----|-------|--------|----------|
| FE-16 | Tests exist for changed components | **FAIL** | Hook coverage is genuinely strong: `media.test.ts:243-269` (no settled-with-null-url frame), `:283-312` (revoke-once on id change), `:314-338` (revoke-once on unmount), `:340-358` (StrictMode create/revoke parity); `apiClient.test.ts:20-38`, `:44-70`, `:72-98` cover `requestBlob`. **Three gaps:** (a) nothing asserts that a caller's `Content-Type` beats the JSON:API default in `request` — the exact invariant maintainer ruling 1 rests on, in a just-refactored merge at `apiClient.ts:29-33`; the only `request()` test is `apps/web/src/lib/api/client.test.ts:14-16` and it checks `Authorization` only. (b) No test for `MediaService.putContent` (`MediaService.ts:42-52`) asserting path/method/body/header. (c) The upload sequence lost all error-path coverage — the old `throws if init returns no uploadUrl` case was deleted and nothing replaced it, so no failure mode of `performMediaUpload` (`media.ts:41-53`) is exercised. `MediaThumbnail.tsx` also has no component test across its three render branches, but that folder has never had one — pre-existing. |
| FE-17 | Mocks updated when services changed | PASS (with note) | No `__mocks__/` directory exists in `apps/web/src`; the project mocks inline. `media.test.ts:112-116` mocks the `MediaService` module for the renamed `getContentBlob`, and `media.test.ts:190` asserts the renamed `putContent` dep. Note: the module mock replaces the whole `mediaService` object with only `getContentBlob`, leaving `initUpload`/`confirm`/`get`/`remove`/`putContent` undefined in that file — safe today because `performMediaUpload` takes injected deps, but brittle if a future test in this file touches the singleton. |

## Summary

### Blocking (must fix)

- **FE-02** — `MediaThumbnail.tsx:26` and `MediaThumbnail.tsx:38` concatenate classes with template literals instead of `cn()`. Mechanical fix; `cn` is at `apps/web/src/lib/utils.ts:9`. (Related live nit in the same edit: `MediaThumbnail.tsx:20` uses `className ?? 'h-24 w-24 rounded'`, so the caller-supplied `className="h-24 w-24"` at `VehicleMediaGallery.tsx:73` silently drops `rounded` — the skeleton renders square while the image it stands in for is rounded. `cn('h-24 w-24 rounded', className)` fixes both at once.)
- **FE-09** — `useMediaContentUrl` (`media.ts:87-113`) swallows `isError`/`error`, so `MediaThumbnail.tsx:23-31` renders "No image" identically for a genuine 404, a 403, and a 500. Return the error state and let the component distinguish it. This is a regression versus the `useMediaDownloadUrl` it replaced.
- **FE-16(a)** — Add a `request()` test in `packages/shared-ts/src/apiClient.test.ts` asserting a caller-supplied `Content-Type` overrides `application/vnd.api+json`. The binary PUT depends on it, the merge site was just refactored, and no test currently pins it.
- **Upload 413 surfacing** — Add a client-side `file.size` guard in `MediaUploadButton.tsx:28-43` against the 25 MiB server cap before calling `upload.mutateAsync`. Without it the most likely user-visible outcome of an oversized upload is the toast "Failed to fetch", because `http.MaxBytesReader` (`apps/media-service/internal/mediaobject/resource.go:81`) closes the connection without draining the in-flight body.

### Non-Blocking (should fix)

- **FE-16(b,c)** — Add a `MediaService.putContent` test (path/method/body/`Content-Type`), and restore error-path coverage for `performMediaUpload` (`media.ts:41-53`), which currently has none.
- **Blob cache growth** — `media.ts:91-92` retains full-size originals (up to 25 MiB each) in the JS heap for 6 minutes to render 96×96 thumbnails. Consider dropping `gcTime` sharply and leaning on the backend's `Cache-Control: private, max-age=300` (`resource.go:152`), and/or exposing a thumbnail variant on the content route.
- **Delete invalidation** — `useDeleteMedia.onSettled` (`media.ts:191-195`) should `removeQueries` on `mediaKeys.content(mediaId)` / `mediaKeys.detail(mediaId)`, not just invalidate the vehicle list.
- **Doubled request count** — `MediaThumbnail.tsx:17` adds a second per-thumbnail query solely for the `alt` string at `:37`. Pass the filename from `VehicleMediaGallery.tsx:68-74` or accept the generic fallback.
- **Orphaned media rows** — a `putContent` failure strands the row created at `media.ts:45`. Pre-existing shape, but newly reachable via the 413 path introduced by this branch.
- **413 `code` mislabel** (backend-owned, degrades the FE error surface) — `codeFor` has no 413 case (`packages/shared-go/server/server.go:12-28`), so size rejections arrive at the frontend as `code: "internal_error"`.
- **`id as string` casts** at `media.ts:63`, `:89`, `:120` — safe under the `enabled: !!id` guards, and the file's established idiom; noted only for completeness.
