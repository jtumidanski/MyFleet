# Plan Adherence Audit — task-002-k3s-cluster-deployment

**Plan reviewed:** `docs/tasks/task-002-k3s-cluster-deployment/plan.md` (10 tasks, 89 steps)
**Design reviewed:** `docs/tasks/task-002-k3s-cluster-deployment/design.md` (incl. amendments A1/A2/A3)
**Range reviewed:** merge-base `16e4aa7` → head `a8d5221` (21 commits)
**Branch:** `task-002-k3s-cluster-deployment`   **Base:** `main`
**Audit date:** 2026-07-30
**Scope:** plan adherence only. Backend/frontend guideline conformance is covered by the two sibling reviewers.

## Executive Summary

All 10 plan tasks are implemented. Every plan-mandated artifact exists, every plan-mandated
verification reproduces cleanly, and every value the plan specified matches the shipped code
except where one of the four maintainer rulings or the final-review fix wave deliberately
superseded it — each of which I verified was carried through completely and consistently
(including into `deploy/compose`, which the rulings did not explicitly mention).

`make ci` passes end to end. Both overlays render, both server-side dry-runs against `bee`
are clean, the web image builds and serves `200` on `/` and a deep link under
`--read-only --user 101`, the workflow YAML parses with the expected matrices, and the
`bump-overlay` sed rewrite hits exactly 5 tags / 10 diff lines.

Two findings, neither of which is a skipped task:

1. **`packages/shared-ts` tests are not executed by any automated gate.** `apiClient.test.ts`
   — the test plan Task 5 mandated — exists, is substantive, and passes when run by hand, but
   neither `make ci` nor `.github/workflows/main.yml` ever runs it. This is a pre-existing gap
   the branch widened. **This is the one thing I would fix before merge.**
2. **All 89 checkboxes in `plan.md` are still `- [ ]`.** Progress was never recorded, so
   checkbox state carries zero signal. Hygiene only — the work is demonstrably done.

## Task Completion

| # | Task | Status | Evidence |
|---|------|--------|----------|
| 1 | Split Kustomize base — apps only, infra to `infra-local/` | DONE | `deploy/k8s/infra-local/{postgres,minio,redpanda,traefik,ingressroute,secrets-local,kustomization}.yaml`; `deploy/k8s/base/routing/middlewares.yaml`; `deploy/k8s/secrets.example.yaml`; `deploy/k8s/base/kustomization.yaml:1-31`. Commit `4ebc6d7`. Verified below. |
| 2 | Web SPA on unprivileged nginx | DONE | `apps/web/Dockerfile:27-32` (`nginxinc/nginx-unprivileged:alpine`, `EXPOSE 8080`); `apps/web/nginx.conf:7` (`listen 8080`); `deploy/k8s/base/web/deployment.yaml`, `service.yaml`. Commit `b1bcb0d`. Image built + run read-only, both curls `200`. |
| 3 | Proxy media upload bytes | DONE (improved) | `processor.go:32-36` (`ObjectStore`), `:84-105` (`InitUpload`, no URL), `:131-147` (`StoreContent`); `resource.go:44` (4-arg `InitializeRoutes`), `:74-101` (`PUT /media/{id}/content`); `errors.go:11,27-28` (413 sentinel); `cmd/main.go:91,124`; `configmap.yaml:14`. Commits `3268329`, `413ddcf`. |
| 4 | Proxy media download bytes | DONE | `processor.go:34` (`GetObject`), `:208-218` (`Content`); `resource.go:134-158` (`GET /media/{id}/content`); `rest.go:6-15` (no URL fields); `storage/minio.go:1-6,71-74` (presign gone, doc corrected). Commit `e408037`. `grep -rn 'Presign\|DownloadURL\|UploadURL\|/download' apps/media-service --include='*.go'` → **clean**. |
| 5 | `requestBlob` on the shared ApiClient | DONE (per ruling 1) | `packages/shared-ts/src/apiClient.ts:20-40` (`fetchAuthenticated`), `:59-66` (`requestBlob`); `apiClient.test.ts` (3 tests). Commits `d41c4ef`, `36b5000`. See Finding 1 re: CI. |
| 6 | Point the web client at the proxied endpoints | DONE (per ruling 2) | `MediaService.ts:5-17,37-57`; `media.ts:17-18,27-53,70-114,138-143`; `MediaThumbnail.tsx:15-47`; `types/models/media.ts:3-18`; comments at `MediaUploadButton.tsx:13-17`, `VehicleMediaGallery.tsx:22`. Commits `05f6fe5`, `92b8da4`, `bba0829`. Stale-API grep clean. |
| 7 | Fix `GOOGLE_REDIRECT_URL` | DONE (per ruling 3) | `deploy/k8s/base/auth-service/configmap.yaml:16`; `overlays/main/patches/auth-service-config.yaml:10`. Commits `1efa175`, `6b1cb4d`. Detail below. |
| 8 | `overlays/main` for bee | DONE (+ fix wave) | `overlays/main/{kustomization,ingressroute}.yaml` + 5 patch files. Commits `c77e42b`, `665a153`. Render + dry-run verified below. |
| 9 | CI — publish web, bump overlay SHAs | DONE (per ruling 4) | `.github/workflows/main.yml:6-11` (`paths-ignore`), `:59`/`:94` (matrices), `:140-191` (`bump-overlay`, `needs: [publish, trivy]`, `concurrency: bump-overlay`, stale-run guard). Commits `6e340bd`, `fbae196`. |
| 10 | Runbook + CLAUDE.md verification commands | DONE | `docs/runbooks/k3s-deployment.md` (304 lines, all 7 sections + Known constraints); `CLAUDE.md` Build & Verification rewritten. Commits `bb0dc90`, `65ffe29`, `4f5ea4e`, `a8d5221`. |

**Completion rate:** 10/10 (100%)
**Skipped without approval:** 0
**Partial implementations:** 0
**Deferred:** 0

## Verification of the Four Maintainer Rulings

Each was checked for completeness, not just presence.

**Ruling 1 — shared `fetchAuthenticated`.** `apiClient.ts:20-40` is a single private helper
carrying the bearer token and the one-shot 401 retry; `request` (`:42-52`) passes
`{'Content-Type': 'application/vnd.api+json'}`, `requestBlob` (`:59-66`) passes `{}`. Header
precedence is `defaults → Authorization → init.headers`, so a caller override still wins —
matching the pre-ruling per-method behaviour. `apiClient.test.ts:63-65` asserts the *retry*
sends `Bearer tok-new`, not the stale token; that assertion is the one that would catch a
refresh path that re-reads a captured token. Not vacuous.

**Ruling 2 — plain state+effect in `useMediaContentUrl`.** `media.ts:95-113`. No `useMemo`,
no module-level cache, no refcounting. `:111` gates on `entry.blob === data` so an
already-revoked URL is never returned, and `:113` folds that frame into `isLoading` — which is
exactly what lets `MediaThumbnail.tsx:19-21` hold the skeleton instead of flashing "No image".
Backed by four real tests in `media.test.ts:118,144,175,192`, including a StrictMode
double-invocation leak check.

**Ruling 3 — `/api`-only strip for auth and media.** `middlewares.yaml`: `auth-stripprefix`
and `media-stripprefix` → `/api`; `fleet-stripprefix` → `/api/fleet` and
`notifications-stripprefix` → `/api/notifications`, both unchanged from `16e4aa7`. I verified
this against source rather than the comments:

- auth routes are `/auth/me`, `/auth/login/google`, `/auth/callback`, `/auth/refresh`,
  `/auth/logout` (`internal/{user,oidc,session}/resource.go`), mounted at the chi root
  (`packages/shared-go/server/handler.go:32-38`). Every frontend caller
  (`client.ts:27`, `fleets.ts:22`, `AuthContext.tsx:49`, `auth.ts:20,46`) resolves correctly
  under `/api`-only stripping.
- media routes carry their own `/media` segment (`resource.go:48,74,104,120,134,161`);
  frontend `basePath = '/api/media'` (`MediaService.ts:19`) resolves correctly.
- The old value was genuinely broken: `git show 16e4aa7:deploy/k8s/base/auth-service/configmap.yaml:13`
  is `http://localhost/api/auth/auth/google/callback` — no such route exists.
- **The ruling was also propagated to `deploy/compose`** (`docker-compose.yml:91,159`,
  commit `6b1cb4d`) and to the two client doc comments (`client.ts`, `BaseService.ts`,
  commit `05f6fe5`). This was not called out in my brief and is the correct call — the compose
  gateway would otherwise have diverged from k8s.
- `JWKS_URL` is in-cluster only in every environment (`http://auth-service:8080/...` in all
  four k8s ConfigMaps and all four compose services), so the now-unreachable
  `/api/auth/.well-known/jwks.json` breaks nothing. The stale comment claiming otherwise was
  removed in `009b57d`.
- `deploy/compose/.env.example:19` was already `http://localhost/api/auth/callback` and is
  correct under the new strip, so it needed no edit. Plan Task 7 Step 3 is therefore
  legitimately a no-op, not a skip.

**Ruling 4 — `bump-overlay` gating.** `main.yml:141` `needs: [publish, trivy]`; `:148-151`
`concurrency: {group: bump-overlay, cancel-in-progress: false}`; `:158-168` stale-run guard
comparing `git rev-parse HEAD` to `github.sha` and bailing rather than pinning backwards.

## Verification of the Final-Review Fix Wave

- **`namespace: myfleet` on `infra-local/kustomization.yaml:9`** — present. Its absence was a
  real bug: the local server dry-run rejects `ClusterRoleBinding "myfleet-traefik"` with
  `subjects[0].namespace: Required value` without it. The local dry-run now passes (below).
- **SPA catch-all in `infra-local/ingressroute.yaml`** — present, `PathPrefix('/')` at
  `priority: 1` → `web:80`, with `priority: 100` on the four `/api/*` routes, mirroring main.
- **`internal-deny` in `overlays/main/ingressroute.yaml:1-23` + route at `priority: 200`** —
  present and correct. `fleet-service` really does register `/internal/maintenance/due`
  without JWT (`internal/maintenanceschedule/resource.go:271`), so the exposure was real.
  I executed the regex `(?i)^/+api/+fleet[^/]*/*internal` under Go's `regexp` against the
  bypass shapes the comment claims to cover and against every legitimate fleet route prefix:

  | Path | Match |
  |---|---|
  | `/api/fleet/internal/maintenance/due` | true |
  | `/api//fleet/internal/maintenance/due` | true |
  | `/api/fleet///internal/maintenance/due` | true |
  | `/api/fleetinternal/maintenance/due` | true |
  | `/api/fleet/INTERNAL/maintenance/due` | true |
  | `/api/fleet/{fleets,vehicles,invites,fuel-logs,maintenance-schedules,maintenance-categories}/…` | false (all) |
  | `/api/media/m1/content`, `/api/auth/me` | false |

  The claims hold. I also confirmed the CRD support the manifest depends on: bee runs
  `rancher/mirrored-library-traefik:3.6.13` and `middlewares.traefik.io/v1alpha1` exposes
  `ipAllowList.rejectStatusCode`, so neither `PathRegexp` nor `rejectStatusCode` is a
  version gamble.

- `plan.md:2138`'s `Middleware 4` is stale as briefed — main now renders 5. No action.

## Plan-Mandated Verifications, Reproduced

Every one of these is a command the plan called for. All were re-run for this audit; none were
taken on trust.

| Plan step | Command | Result |
|---|---|---|
| T1 S12 / T2 S9 | `kustomize build deploy/k8s/overlays/local` | OK — **41** resources (39 baseline + web Deployment + Service), kinds: 1 ClusterRole, 1 ClusterRoleBinding, 5 ConfigMap, 9 Deployment, 1 IngressRoute, 4 Middleware, 1 Namespace, 3 PVC, 6 Secret, 9 Service, 1 ServiceAccount |
| T1 S13 | `kustomize build deploy/k8s/base` | OK — 0 Secret, 0 PVC, 0 ClusterRole, 0 placeholder; **4** Middleware |
| T1 S14 | `grep -c 'storageClassName: longhorn'` on local render | **3** |
| T2 S3 | `docker build -f apps/web/Dockerfile .` | PASS (exit 0) |
| T2 S4 | `docker run --read-only --user 101 --tmpfs /tmp` + curl | `/` → **200**, `/vehicles` → **200** |
| T3 S12 / T4 S10 | `go test -race ./apps/media-service/... ./packages/shared-go/...` | PASS (all `ok`) |
| T4 S9 | `grep -rn 'Presign\|presignTTL\|DownloadURL\|UploadURL\|/download' apps/media-service --include='*.go'` | **clean** |
| T6 S8 | `grep -rn 'uploadUrl\|downloadUrl\|putToPresignedUrl\|useMediaDownloadUrl\|getDownloadUrl\|presigned' apps/web/src packages/shared-ts` | clean (sole hit is the intentional "not presigned" comment at `MediaService.ts:15`) |
| T8 S5 | main-overlay content assertions | 0 PVC, 0 Secret, 0 ClusterRole, 0 ServiceAccount, 0 placeholder/REPLACE_ME, 0 `redpanda`/`postgres:5432`/`minio:9000`; `imagePullPolicy: Always` **5**; Deployment **5**; Middleware **5** (see stale-line note); `KAFKA_BROKERS: kafka.home:9093` ×3; `MINIO_ENDPOINT: minio.minio.svc.cluster.local:9000`; `COOKIE_SECURE: "true"`; `GOOGLE_REDIRECT_URL: https://myfleet.tumidanski.com/api/auth/callback` |
| T8 S6 / T10 S5 | `kustomize build .../main \| kubectl apply --dry-run=server -f -` | PASS — 21 resources, all `(server dry run)`, no errors |
| T10 S5 (extended) | `kustomize build .../local \| kubectl apply --dry-run=server -f -` | PASS — 41 resources, all `(server dry run)`, no errors |
| T9 S4 | PyYAML parse of `main.yml` | `jobs: ['build-test','publish','trivy','tag','bump-overlay']`; both matrices `[…, 'web']`; `paths-ignore: ['deploy/k8s/overlays/main/**']`; `bump needs: ['publish','trivy']` |
| T9 S5 | sed dry-run of the SHA rewrite | count **5**, diff **10** lines |
| T10 S3 | runbook claims vs repo | callback URL matches the overlay patch (`k3s-deployment.md:146` ↔ `patches/auth-service-config.yaml:10`); `MEDIA_MAX_UPLOAD_BYTES` in both ConfigMap and `cmd/main.go:91`; Argo path `deploy/k8s/overlays/main` matches; `secrets.example.yaml` exists |
| T10 S4 | `make ci` | **PASS** (exit 0) |

Cluster safety: only `kubectl config current-context`, read-only gets, and
`--dry-run=server` were used. `kubectl -n myfleet get all` returns
`No resources found` — nothing was applied. The `myfleet` namespace pre-exists (the dry-run
reports it `configured` rather than `created`), which plan Task 8 Step 6 explicitly permits.

## Build & Test Results

| Component | Build | Tests | Vet / Lint | Notes |
|---|---|---|---|---|
| Go workspace (all 4 services + `packages/*`) | PASS | PASS | PASS | `make ci`; `go test -race` across the workspace, all `ok`. Note: bare `go build ./...` from repo root does not work here (`go.work`); the Makefile's module-path form does. |
| `apps/media-service` (targeted, `-race`) | PASS | PASS | PASS | 6 new/changed tests in `mediaobject` + 2 in `resource_test.go` |
| `apps/web` | PASS (`tsc -b && vite build`) | PASS | PASS | 11 files / 63 tests |
| `packages/shared-ts` | PASS | PASS **(not run by CI)** | PASS | 2 files / 5 tests when invoked manually — see Finding 1 |
| `apps/web` Docker image | PASS | n/a | n/a | runs read-only as UID 101, serves `/` and deep links |
| `deploy/k8s` local + main | PASS (render) | PASS (server dry-run) | n/a | |

## Findings

### Finding 1 — `packages/shared-ts` tests are never executed by an automated gate (fix before merge)

`apiClient.test.ts` is the test plan Task 5 required. It exists, it is substantive, and it
passes — but nothing runs it:

- `Makefile:20-21` — `fe-test: npm run -w apps/web test`
- `.github/workflows/main.yml:48` — `run: npm run -w @myfleet/web test`

`vitest` is configured per-workspace (`apps/web/vite.config.ts`); there is no root
`vitest.workspace.*`. Running `npm run -w @myfleet/shared-ts test` by hand gives
`2 files / 5 tests passed` — including the pre-existing `src/errors.test.ts`, which has been
orphaned the same way since before this branch.

This is **pre-existing infrastructure debt, not a plan violation** — plan Task 5 Step 4's own
verification command was `npm run -w @myfleet/shared-ts test`, which does pass, and the plan
never claimed the test would be wired into CI. But this branch materially raised the stakes:
`fetchAuthenticated` is now the single 401-refresh path for **every** call the SPA makes, not
just blobs, and a regression in it would ship green. One line in the Makefile
(`npm run -w @myfleet/shared-ts test` alongside the existing `fe-test`) closes it.

### Finding 2 — `plan.md` progress was never recorded (hygiene)

All 89 steps remain `- [ ]`; zero are `- [x]`. Nothing was actually skipped — I verified each
task independently against code and re-ran the verifications — but the plan document is not a
usable record of what happened, and checkbox state gave me no signal to work from.

### Finding 3 — `internal-deny` protects only `overlays/main` (accept as-is)

`infra-local/ingressroute.yaml` has no equivalent block, so `/api/fleet/internal/*` remains
reachable through the local dev Traefik. Correct scoping — local dev is not internet-facing and
the control is explicitly documented as routing-layer-only — but worth stating explicitly so it
is a decision rather than an oversight. The underlying fact that `/internal/*` carries no JWT
is unchanged and out of this plan's scope; the runbook's post-deploy regression check
(`k3s-deployment.md:245-270`, including the no-separator `/api/fleetinternal/...` variant)
correctly warns that the control fails open.

## Deviations From Plan Text — All Justified

None of these are defects; recording them so they are not re-flagged.

| Plan text | Shipped | Justification |
|---|---|---|
| T5: `requestBlob` duplicates the 401 logic | shared `fetchAuthenticated` | Ruling 1 |
| T6: `useMediaContentUrl` returns `{url, isLoading}` from a bare effect | adds the `entry.blob === data` guard and folds the gap frame into `isLoading` | Ruling 2 |
| T1/T7: strip `/api/auth`, `/api/media`; callback `/api/auth/auth/callback` | strip `/api`; callback `/api/auth/callback` | Ruling 3 (design A2) |
| T9: `bump-overlay` `needs: publish` | `needs: [publish, trivy]` + concurrency + stale-run guard | Ruling 4 |
| T1 S11: `infra-local/kustomization.yaml` with no `namespace` | `namespace: myfleet` | Final-review fix; without it the local dry-run fails outright |
| T1 S6: local IngressRoute, four `/api/*` routes | + SPA catch-all, + explicit priorities | Final-review fix |
| T8 S3: five routes | + `internal-deny` Middleware and route at `priority: 200` | Final-review fix (design A3) |
| T3 S5: `StoreContent` persists the caller-supplied `size` | persists bytes actually read via `countingReader` (`processor.go:110-119,142-146`) | Improvement — the plan would record an attacker-supplied length. `TestStoreContent_unknownLengthRecordsActualByteCount` (`processor_test.go:430`) covers it. |
| T3 S8: `MaxBytesError` handled inline | extracted to `classifyUploadError` (`resource.go:32-38`) | Improvement — makes the 413 mapping unit-testable; covered by `resource_test.go:18,37` |
| T7 S3: edit `deploy/compose/.env.example` | unchanged | Already correct under Ruling 3's `/api`-only strip. Verified, not assumed. |
| — | `deploy/compose/docker-compose.yml:91,159` also switched to `/api` | Necessary corollary of Ruling 3; not in the plan or the ruling text, and correct |

## Overall Assessment

- **Plan Adherence:** FULL
- **Recommendation:** READY_TO_MERGE (after Action Item 1)

Nothing was silently skipped or quietly deferred. Every plan-required file exists, every
plan-required verification reproduces, and the only test the plan mandated that is not
exercised by CI (Finding 1) exists and passes — the gap is in the gate, not the work.

## Action Items

1. **Wire `packages/shared-ts` into the test gate.** Add `npm run -w @myfleet/shared-ts test`
   to `Makefile` `fe-test` (and/or a step in `.github/workflows/main.yml`), so
   `apiClient.test.ts` and `errors.test.ts` actually run. `fetchAuthenticated` is now the
   whole SPA's 401-refresh path; an untested regression there ships green today.
2. *(Optional, hygiene)* Tick `plan.md`'s checkboxes, or add a one-line note at the top of
   the plan pointing at the rulings and this audit, so the document reflects reality.
3. *(Optional, no code change)* Note in `design.md` A3 that `internal-deny` is scoped to
   `overlays/main` by design, so the local overlay's omission reads as intentional.
