# task-002 — Implementation Context

Companion to `plan.md`. Everything below was verified against local source or the
live bee cluster during planning, not recalled.

**Worktree:** `/home/tumidanski/source/MyFleet/.worktrees/task-002-k3s-cluster-deployment`
**Branch:** `task-002-k3s-cluster-deployment`

---

## Phase-artifact note

This task has **no `prd.md`**. design.md §1 records that `/spec-task` was
intentionally skipped (CLAUDE.md's documented escape hatch — requirements were
settled directly with the maintainer). `/plan-task` normally gates on both
`prd.md` and `design.md`; the maintainer approved proceeding from `design.md`
alone. **design.md is therefore the sole requirements record for this task** —
which is why the planning-phase amendment below was written into it rather than
left only in the plan.

---

## Key files

### Kustomize — the thing being restructured

| File | Current state |
|---|---|
| `deploy/k8s/base/kustomization.yaml` | `namespace: myfleet`; lists infra + 4 services incl. their `secret.yaml` |
| `deploy/k8s/base/infra/traefik.yaml` | One file holding SA + ClusterRole + CRB + Deployment + NodePort Service + **4 Middlewares** + IngressRoute |
| `deploy/k8s/base/infra/{postgres,minio,redpanda}.yaml` | Deployment + Service + PVC (**no `storageClassName`**) + placeholder Secret (postgres, minio) |
| `deploy/k8s/base/*-service/secret.yaml` | 4 files, `PLACEHOLDER_*` values — deleted in Task 1 |
| `deploy/k8s/overlays/local/kustomization.yaml` | `../../base`; 8 `replicas`; 4 image `newTag: local`; 4 `IfNotPresent` patches |
| `deploy/k8s/overlays/main/` | **Does not exist** |
| `deploy/k8s/secrets.example.yaml` | **Does not exist** |
| `apps/web` manifests | **Do not exist** |

Baseline: `kustomize build deploy/k8s/overlays/local` renders **39 resources** —
1 ClusterRole, 1 ClusterRoleBinding, 5 ConfigMap, 8 Deployment, 1 IngressRoute,
4 Middleware, 1 Namespace, 3 PVC, 6 Secret, 8 Service, 1 ServiceAccount. The 5th
ConfigMap is `postgres-init-sql`; the 5th and 6th Secrets are `postgres-secret`
and `minio-secret`. Task 1 Step 12 asserts this count is preserved.

### media-service — the byte-proxy change

| File:line | Fact |
|---|---|
| `internal/storage/minio.go:106-107` | `PresignPut` → `mc.PresignedPutObject`; URL host comes from `MINIO_ENDPOINT` |
| `internal/storage/minio.go:125,132,141` | `PutObject`, `GetObject`, `RemoveObject` **already exist** — the proxy needs no new storage methods |
| `internal/mediaobject/processor.go:26` | `presignTTL = 15 * time.Minute` |
| `internal/mediaobject/processor.go:30-34` | `Presigner` interface: `PresignPut`, `PresignGet`, `Bucket` |
| `internal/mediaobject/processor.go:102` | `InitUpload` presigns and returns `(Model, string, error)` |
| `internal/mediaobject/processor.go:206` | `getActive(id)` — 404 on deleted, 410 on purgeable |
| `internal/mediaobject/rest.go:15-16` | `UploadURL`/`DownloadURL` attributes, `omitempty` |
| `internal/mediaobject/resource.go:83-97` | `GET /media/{id}/download` returns the presigned GET URL |
| `model.go:45,51,57` | `WithStatus`, **`WithSize(int64)`**, `WithSoftDelete` — immutable updates |
| `administrator.go:11-18` | `Insert`, **`Update`**, `UpdateInTx`, `SoftDelete` |
| `cmd/main.go:85` | `config.GetInt("MEDIA_WORKERS", 2)` — the pattern for `MEDIA_MAX_UPLOAD_BYTES` |

Existing tests in `processor_test.go` cover pure functions and `Confirm` only —
no storage double exists yet, so Task 3 introduces `fakeStore`.

### Web — the client side

| File:line | Fact |
|---|---|
| `packages/shared-ts/src/apiClient.ts:12-29` | `request` **always** `res.json()` — cannot carry binary; `init.headers` spread last, so a `Content-Type` override wins |
| `apps/web/src/lib/api/client.ts:19-20` | `baseUrl: ''`, so callers pass full `/api/<service>/...` paths |
| `apps/web/src/lib/api/token.ts:4-5` | **Auth is `Authorization: Bearer` from an in-memory/localStorage access token.** The refresh token is the only cookie. |
| `apps/web/src/services/api/MediaService.ts:40-49` | `putToPresignedUrl` — raw `fetch`, no auth header, bypasses `apiClient` |
| `apps/web/src/lib/hooks/api/media.ts:50-56` | `performMediaUpload` reads `attributes.uploadUrl` |
| `apps/web/src/components/.../MediaThumbnail.tsx:21` | `<img src={data?.attributes.downloadUrl}>` |

**The Bearer-token detail is load-bearing.** Because auth is a header and not a
cookie, `<img src="/api/media/{id}/content">` would arrive unauthenticated — the
browser does not attach `Authorization` to image requests. Hence Task 5's
`requestBlob` and Task 6's object-URL hook. A cookie-based scheme would not have
needed either.

### CI

`.github/workflows/main.yml` — jobs `build-test` → `publish` (matrix of 4) →
`trivy` (matrix of 4, `fail-fast: false`) and `tag`. `permissions: contents:
write` is already set. The push trigger has **no `paths-ignore`**. The publish
step uses `context: .` + `file: apps/${{ matrix.service }}/Dockerfile`, which
`apps/web/Dockerfile` already matches (its context is the repo root because the
npm workspace lives there).

---

## Verified facts about bee

Checked live via `kubectl` (context `bee`) during planning:

| Fact | Evidence |
|---|---|
| Cluster is reachable from this environment | `kubectl cluster-info` → `https://aeon.tumidanski:6443` |
| Traefik CRDs are `traefik.io/v1alpha1` | `ingressroutes.traefik.io`, `middlewares.traefik.io` present |
| Cluster Traefik LB | `kube-system/traefik`, `EXTERNAL-IP 192.168.23.230`, ports incl. `80`, `443`, `5432`, `9092`, `9093` — the TCP entrypoints behind `postgres.home` and `kafka.home` |
| Shared MinIO | `minio/minio` ClusterIP on `9000`/`9001` → `minio.minio.svc.cluster.local:9000` |
| `myfleet` namespace | **Does not exist** — Argo CD's `CreateNamespace=true` handles it |
| Two default StorageClasses | `local-path (default)` and `longhorn (default)` — an unpinned PVC is non-deterministic |

Server-side dry-run is therefore viable, and Task 8 Step 6 uses it. From
design.md §2, also already verified: GHCR serves the four service images
anonymously (no pull secret); the MyFleet repo is public (no Argo repo creds); no
`myfleet` role/database on `postgres.home`; no `myfleet-media` bucket; no Kafka
topic collision with atlas (uppercase `COMMAND_TOPIC_*`/`EVENT_TOPIC_*` vs
MyFleet's lowercase dotted names). `ghcr.io/jtumidanski/myfleet-web` **does not
exist yet** — Task 9 creates it.

---

## Design amendment A1 — media bytes are proxied

**The one substantive deviation from approved design.md.**

design.md §4.3 moves `MINIO_ENDPOINT` to `minio.minio.svc.cluster.local:9000`
while leaving the presigned-URL flow intact. Those two cannot both hold:

1. `POST /api/media` presigns a PUT URL from `MINIO_ENDPOINT`
   (`internal/storage/minio.go:106-107`).
2. The browser PUTs bytes straight to it
   (`apps/web/src/services/api/MediaService.ts:37-41`).
3. `minio.minio.svc.cluster.local` does not resolve in a browser.
4. The S3 v4 signature covers the `Host` header, so the URL cannot be rewritten
   client-side.

Result as designed: uploads and thumbnails both fail, and design.md §8's "a media
upload lands an object in the `myfleet-media` bucket" cannot pass.

**Maintainer decision during planning: proxy the bytes through media-service; do
not expose MinIO publicly** — not on its own hostname and not on a path under the
MyFleet hosts, because the shared MinIO also holds `atlas-*` buckets and
proxying the S3 API would expose that surface too.

Consequences folded into the plan:

- `PUT /media/{id}/content` and `GET /media/{id}/content` (Tasks 3, 4).
- All presign plumbing deleted — `PresignPut`/`PresignGet`, `presignTTL`,
  `uploadUrl`/`downloadUrl`, `GET /media/{id}/download`. No dead code left behind.
- `MINIO_ENDPOINT` stays internal, so server-side S3 traffic (bucket check,
  variant worker, purge job) never leaves the cluster.
- New `MEDIA_MAX_UPLOAD_BYTES` (default `26214400` = 25 MiB), enforced with
  `http.MaxBytesReader` → `413`. This also closes design.md §7's open note about
  Cloudflare's body-size cap, which now genuinely applies since uploads traverse
  the edge.
- `ApiClient.requestBlob` + an object-URL hook, because auth is a Bearer header
  (Task 5, 6).

Recorded as Amendment A1 in design.md §10.

---

## Ordering and dependencies

```
1 (kustomize split) ─► 2 (web manifests) ─┐
                                          ├─► 8 (overlays/main) ─► 9 (CI) ─► 10 (docs)
7 (redirect URL fix) ─────────────────────┘                          ▲
                                                                     │
3 (upload proxy) ─► 4 (download proxy) ─┐                            │
                                        ├─► 6 (web client) ──────────┘
5 (requestBlob) ────────────────────────┘
```

- **1 → 2 → 8** is the hard chain: the overlay needs the split base and the web
  manifests.
- **3 → 4** share the `ObjectStore` interface; 3 defines it with `PutObject`, 4
  adds `GetObject`.
- **6 needs 3, 4 and 5.** Running it earlier leaves the frontend calling routes
  that do not exist.
- **8 needs 3** for `MEDIA_MAX_UPLOAD_BYTES` in the base ConfigMap, and **7** so
  the patch overrides a correct base value.
- **9 needs 2** (the web image) and **8** (the five `newTag` lines its `sed`
  rewrites — the step asserts exactly 5 matches).
- **10 last** — it asserts consistency across everything and runs `make ci`.

---

## Environment gotchas

- **`node` is not on `PATH`.** Only a Windows `npm` shim at `/mnt/c/...` is,
  which will not work. nvm has v22.22.2 and v24.12.0 installed but neither is
  active. Before any `npm`/`make fe-*` command:
  `export NVM_DIR="$HOME/.nvm" && . "$NVM_DIR/nvm.sh" && nvm use 22`
- **`node_modules/` is absent in this worktree** — `npm ci` once before the first
  frontend command (Task 5 Step 2 does this).
- `kustomize` v5.4.3, `kubectl` v1.32.3, `go` 1.26.5, `docker` all present.
- CI pins Go `1.25` and Node `22`; local Go is 1.26.5. `go.work` governs the
  module set.
- The git stash stack is shared across worktrees — use a WIP commit, never bare
  `git stash`.

---

## Traps

1. **Never let `infra-local/traefik.yaml` reach bee.** Its ClusterRole grants a
   cluster-wide watch on Traefik CRDs, and bee's k3s Traefik already runs
   `--providers.kubernetescrd` cluster-wide. Two controllers reconciling the same
   IngressRoutes would fight over every route on the cluster, atlas's included.
   `overlays/main` must reference `../../base` only.
2. **Middlewares belong in the base, Traefik itself does not.** Both overlays
   need the four `stripPrefix` CRs; only local needs the controller.
3. **The SPA catch-all must lose to every `/api/*` route.** Explicit
   `priority: 100` vs `priority: 1`, not Traefik's default rule-length ordering.
   Verified by a deep-link `curl` returning 200.
4. **Deleting the base Secrets breaks `overlays/local`** — every service
   `envFrom`s a `*-secret`. They move to `infra-local/secrets-local.yaml`; the
   main overlay ships none.
5. **The public OAuth callback is `/api/auth/callback`.** *(Corrected — this
   entry originally read "`/api/auth/auth/callback` is not a typo", which is the
   opposite of the truth. See design.md amendment A2.)* Both committed configs
   were wrong in different ways (`/auth/google/callback` and `/callback` —
   neither route exists), apparently conflating the login path with the
   callback. A maintainer ruling during execution fixed this on the gateway
   side: `auth-stripprefix` strips only `/api`, not the whole `/api/auth`, and
   the OIDC route is `/auth/callback` — so the correct public path is
   `/api/auth/callback`. Register **that** in Google Cloud Console;
   `docs/runbooks/k3s-deployment.md` is authoritative.
6. **The bump-overlay job must not retrigger itself.** `[skip ci]` **and**
   `paths-ignore` — either alone is fragile.
7. **`kustomize` `newTag` seeding.** The overlay ships `latest` and CI rewrites
   it to the SHA on the first push to `main`. Until then the intent of design.md
   decision #10 ("git log is an accurate record of what is deployed") is not yet
   met, and `myfleet-web` will `ImagePullBackOff` because the image does not
   exist until Task 9's workflow first runs.
8. **First-sync ordering.** If Argo CD syncs before the Postgres DDL, all four
   services `CrashLoopBackOff` on a missing schema. Expected; self-resolves.
   GORM `AutoMigrate` creates tables but not schemas.
9. **`Content-Type` on the proxied upload must come from the stored row**, not
   the request, so a client cannot relabel bytes.
10. **`readOnlyRootFilesystem` on nginx needs a writable `/tmp`** (pid, temp
    bodies) — hence the `emptyDir`. Do not relax the security context to fix a
    startup failure.

---

## Verification commands

```sh
# Go
go build ./... && go vet ./... && go test -race ./...

# Frontend (load node first)
export NVM_DIR="$HOME/.nvm" && . "$NVM_DIR/nvm.sh" && nvm use 22
npm ci && npm run -w apps/web test && npm run -w apps/web build

# Everything CI runs
make ci        # lint-check vet test build fe-test fe-build

# Manifests
kustomize build deploy/k8s/overlays/local
kustomize build deploy/k8s/overlays/main
kustomize build deploy/k8s/overlays/main | kubectl apply --dry-run=server -f -

# Web container
docker build -f apps/web/Dockerfile -t myfleet-web:verify .
```

The `main` overlay must render with **zero** PVCs, Secrets, ClusterRoles,
ServiceAccounts and placeholder strings, and with `imagePullPolicy: Always` on
all five Deployments.

---

## Post-merge, operator-owned

Not verifiable from this repo — see `docs/runbooks/k3s-deployment.md`:
Postgres role/database/4 schemas; MinIO bucket + scoped user/policy; the four
out-of-band Secrets; Cloudflare + Pi-hole DNS records; the Google Cloud Console
redirect URI; and `argocd-myfleet.yml` committed to `~/source/k3s/bee` (a
separate repo, a separate commit).
