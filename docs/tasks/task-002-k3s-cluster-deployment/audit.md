# task-002 — Code Review Audit

Consolidated result of the three modular reviewers required by CLAUDE.md's Code
Review Pattern, run against the full branch before opening a PR.

**Branch:** `task-002-k3s-cluster-deployment`
**Range reviewed:** `16e4aa7` (merge-base with `main`) → `a8d5221`
**Head after remediation:** see `git log`; the fixes below land in `9c93b6b`,
`b2b7fed`, `8f50a66`, `e3fbe77`, `4bf732c`, `7bd24a6`.

| Reviewer | Verdict at review time | Detail |
|---|---|---|
| `plan-adherence-reviewer` | Plan faithfully executed, 10/10 tasks | [audit-plan-adherence.md](audit-plan-adherence.md) |
| `backend-guidelines-reviewer` | NEEDS-WORK — 2 Critical | [audit-backend.md](audit-backend.md), [audit-backend.json](audit-backend.json) |
| `frontend-guidelines-reviewer` | NEEDS-WORK — 3 must-fix + 413 UX | [audit-frontend.md](audit-frontend.md) |

All must-fix findings were remediated before the PR was opened. The per-reviewer
files above are the full reports and are the authoritative detail; this file is
the index and the disposition record.

---

## Must-fix findings and their disposition

### B1 — `size = -1` allocated 528 MiB before reading a byte (Critical, fixed)

`resource.go` set `size = -1` when `Content-Length` was absent **or above the
cap**. minio-go routes a negative size to `putObjectMultipartStreamNoLength`,
which allocates `OptimalPartInfo(-1, 0)` up front. Measured against the real
module: **553648128 bytes (528.0 MiB) across 9930 parts**, against a 256 MiB pod
limit (`deploy/k8s/base/media-service/deployment.yaml:56`). `MaxBytesReader`
bounds bytes *read*, not a buffer allocated in advance, so it did not help.
Triggerable from the internet by a 1-byte body advertising a large
`Content-Length` — the branch's own defensive line turned a cheap 413 into an
OOMKill.

**Fixed** in `8f50a66`: an over-cap `Content-Length` is now rejected with 413
before any storage call, and `PutObjectOptions.PartSize` is pinned to 5 MiB
(`absMinPartSize`). Measured after the change: `OptimalPartInfo(-1, 5 MiB)` →
5.0 MiB across 10000 parts, ~48.8 GiB capacity, ~2000× the 25 MiB cap. 5 MiB was
chosen over the SDK's 16 MiB default because the buffer is per in-flight upload:
20 concurrent uploads cost 100 MiB rather than 320 MiB against the 256 MiB
limit. Known sizes above 5 MiB now take the multipart path with a 5 MiB buffer
instead of `putObject`'s `make([]byte, size)`, i.e. strictly less memory.

Why three prior review layers missed it: the tests covering this path assert
against `fakeStore`, which never allocates.

### B2 — `GET /media/{id}/content` returned 200 with an empty body (Critical, fixed)

`minio.GetObject` is lazy — the request is issued on first read — so a missing
key returned `err == nil`, and the handler had already committed
`WriteHeader(http.StatusOK)`. Reachable through the normal flow: `POST /media`
creates the row with `size = 0` and writes nothing to storage, and a failed
`StoreContent` leaves exactly that state. Before this branch the presigned URL
surfaced MinIO's real 404; the proxy converted a hard error into a silent
success.

**Fixed** in `8f50a66` via a 32 KiB read-prime inside `storage.GetObject`
(chosen over `Stat()`, which issues a separate HEAD and leaves a TOCTOU window).
Bytes are replayed with `io.MultiReader`, so the response is still streamed and
nothing is double-read. `NoSuchKey` → `storage.ErrObjectNotFound` → **404**,
matching what clients saw pre-branch; everything else stays 500.

A copy that fails *after* headers are on the wire still cannot be signalled by
status — the client sees 200 with a short body, and the existing `Warn` log
remains the only signal. Stated explicitly rather than papered over.

### FE-02 — `cn()` bypassed, and a live skeleton bug (fixed)

`MediaThumbnail.tsx` concatenated `className` with template literals instead of
the project's `cn()` helper. The same defect dropped `rounded` from the loading
skeleton whenever a caller passed a `className` — which the gallery does — so
the skeleton rendered square while the image it stood in for was rounded.
**Fixed** in `b2b7fed`.

### FE-09 — error states discarded (regression, fixed)

`useMediaContentUrl` dropped `isError`/`error`, so 403, 404 and 500 all rendered
the identical "No image" placeholder. A regression: the `useMediaDownloadUrl` it
replaced returned the whole query result. **Fixed** in `b2b7fed`: 401/403 → "No
access", 404 → "Missing", otherwise "Load failed", rendered as a
destructive-tinted tile with `role="img"` and a descriptive `aria-label`. No
per-thumbnail toast, which would burst in a gallery.

The object-URL lifecycle — which went through two maintainer rulings — was
preserved exactly: the change is additive, `isLoading: isLoading || (!!data &&
url === null)` and the `entry.blob === data` identity guard are untouched, and
all four lifecycle tests still pass.

### FE-16 — the refactor's key invariant was untested (fixed)

`fetchAuthenticated` merges headers as `...defaultHeaders` → `Authorization` →
`...init.headers`. That spread order is what lets a caller's `Content-Type` beat
the JSON:API default, and the binary media PUT depends on it — yet it sat in a
freshly-refactored merge site with no test. **Fixed** in `9c93b6b`. The test was
proven to bite: reordering the spreads produced `expected
'application/vnd.api+json' to be 'image/jpeg'`, and `apiClient.ts` is
byte-identical afterwards.

### Gate gap — `packages/shared-ts` tests ran in no automated gate (fixed)

`Makefile` ran `npm run -w apps/web test` and the workflow inlined `npm run -w
@myfleet/web test`; neither covered `packages/shared-ts`, and there is no root
vitest workspace. Pre-existing debt, but this branch raised the stakes:
`fetchAuthenticated` is now the single 401-refresh path for every SPA call.
**Fixed** in `e3fbe77` (Makefile) and `7bd24a6` (the workflow now calls `make
fe-test` and `make fe-build`, so CI and local cannot drift again).

### 413 upload experience (improved)

`MaxBytesReader` responds 413 and closes without draining the in-flight body, so
a browser mid-upload most likely saw a connection reset → `TypeError` → a
"Failed to fetch" toast. **Improved** in `b2b7fed` with a client-side size guard
placed *before* `initUpload`, so an oversized file never reaches the network and
never leaves an orphaned `uploaded` row. The client constant mirrors
`MEDIA_MAX_UPLOAD_BYTES` and carries a comment naming the ConfigMap key it
tracks; it is UX, not a security control, and the server path still governs on
drift. The drift branch keys on `status === 413` rather than `code`, because
`codeFor` has no 413 case (see deferred below).

---

## Deferred — accepted, not fixed on this branch

Recorded so they are visible rather than lost. None block merge.

**Backend**
- `codeFor()` in `packages/shared-go/server/server.go` has no `case 413`, so the
  413 document carries `code: "internal_error"`. `status` and `title` are
  correct and the web surfaces `title`.
- Response `Content-Length` is sourced from the DB row rather than the object.
- No `nosniff` / `Content-Disposition` on the content response. Severity is
  limited because auth is Bearer-header-only, so no ambient-credential
  navigation is possible.
- `fakeStore.getErr` is defined but unused; `Content`'s `GetObject` error branch
  has no direct assertion.
- Not covered by tests: the allocation itself (needs a live MinIO for an RSS
  assertion), that `Client.PutObject` passes `putOptions` over the wire, and the
  mid-copy failure.

**Frontend**
- No test for `MediaService.putContent`.
- Blob `gcTime` pins full-size originals in the JS heap to render 96×96
  thumbnails, while the backend already sets `Cache-Control: private,
  max-age=300`.
- `useDeleteMedia` does not invalidate `mediaKeys.content(id)` or the detail
  entry.
- `MediaThumbnail` issues a second request per thumbnail solely for the `alt`
  string.
- No test file for `MediaThumbnail.tsx`.
- The StrictMode test's comment overstates what it observes (the replay runs
  while `data` is still `undefined`); the lifecycle was verified independently.

**Routing / platform**
- `/api/fleet/metrics`, `/healthz` and `/readyz` are publicly reachable and
  unauthenticated — the same exposure class as the `/internal/*` finding,
  pre-existing and outside this branch's scope.
- The `internal-deny` control **fails open**: if the Middleware object is absent
  while the IngressRoute exists (first-sync ordering, partial prune), Traefik
  disables the router and requests fall through to the priority-100
  `/api/fleet` route, returning the unauthenticated cross-fleet data with 200.
  The runbook's §7 403 checks are the mitigation and say why they exist.
- The `internal-deny` regex's `[^/]*/*` is load-bearing — a mandatory-slash edit
  reopens 34 bypasses. Guarded by a runbook curl against
  `/api/fleetinternal/maintenance/due` and a YAML comment; a manifest-level test
  in `make ci` would be the durable fix.
- fleet-service's `/internal/*` routes remain unauthenticated at the app layer;
  blocked at the edge only. Recorded as design.md amendment A3 and should become
  a tracked backlog item so it outlives task-002.
- `overlays/local` deliberately ships no `internal-deny` (NodePort dev-only).
  Correct scoping, but currently reads as an omission rather than a decision.
- `plan.md` is knowingly stale: its 89 checkboxes are unchecked, and `:2138`
  asserts the main overlay renders 4 Middlewares where it now renders 5.

---

## Verification at the time of writing

- `make ci` — exit 0 (`lint-check`, `vet`, `test`, `build`, `fe-test` including
  the newly-wired `packages/shared-ts` suite, `fe-build`).
- `kustomize build deploy/k8s/overlays/local` — 41 resources; server-side
  dry-run against `bee` clean.
- `kustomize build deploy/k8s/overlays/main` — 21 resources; server-side dry-run
  against `bee` clean; renders zero PersistentVolumeClaims, Secrets,
  ClusterRoles, ClusterRoleBindings and ServiceAccounts.
- `actionlint .github/workflows/main.yml` — exit 0.
- No resources were applied to the shared cluster at any point; `kubectl -n
  myfleet get all` returns nothing.
