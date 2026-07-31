# Deploy MyFleet to the bee k3s cluster — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Run MyFleet (four Go services + the web SPA) on the bee k3s cluster against shared cluster infrastructure, exposed through bee's Traefik on two hostnames and continuously deployed by Argo CD.

**Architecture:** The Kustomize tree is split so `base/` holds apps only, bundled dev infrastructure moves to a sibling `infra-local/` that only the local overlay references, and a new `overlays/main` targets bee with real image tags and shared-infra endpoints. Media bytes stop flowing through browser-facing presigned MinIO URLs and are proxied through media-service instead, so MinIO is never exposed outside the cluster. CI gains a `web` image and a job that bumps the overlay's image SHAs so Argo CD sees a real manifest diff on every merge to `main`.

**Tech Stack:** Kustomize v5, Traefik v3 CRDs (`traefik.io/v1alpha1`), k3s, Argo CD, Go 1.25/1.26, chi, GORM, MinIO Go SDK, React 19 + TypeScript + Vite, TanStack React Query, GitHub Actions.

## Global Constraints

- Namespace for every MyFleet resource: `myfleet`.
- Shared infrastructure endpoints (verified live on bee, do not change):
  - Postgres: `postgres.home:5432`, database `myfleet`, role `myfleet`, schemas `auth`, `fleet`, `media`, `notification`.
  - Kafka: `kafka.home:9093`.
  - MinIO: `minio.minio.svc.cluster.local:9000`, bucket `myfleet-media`.
- Public hosts: `myfleet.tumidanski.com` (Cloudflare, TLS at edge) and `myfleet.home` (LAN, plain HTTP). Both are served by one route set.
- bee's Traefik LoadBalancer is `192.168.23.230` in namespace `kube-system`. Traefik CRDs on bee are `traefik.io/v1alpha1` (verified: `ingressroutes.traefik.io`, `middlewares.traefik.io`).
- The main overlay must contain **zero** PersistentVolumeClaims, **zero** Secrets, and **zero** ClusterRole/ClusterRoleBinding/ServiceAccount for Traefik. Secrets are applied out-of-band so Argo CD's `prune: true` cannot remove them.
- Kafka topics stay a flat namespace — no per-environment prefixing, no code changes to topic constants.
- Image references are `ghcr.io/jtumidanski/myfleet-<name>`; the main overlay uses `imagePullPolicy: Always` and image tags that CI rewrites to the commit SHA.
- Go services keep their hardened `securityContext`: `runAsNonRoot: true`, `readOnlyRootFilesystem: true`, `allowPrivilegeEscalation: false`, `capabilities.drop: ["ALL"]`. The web container must match this posture.
- The correct public OAuth callback path is `/api/auth/auth/callback` (routes mount at the chi root — `packages/shared-go/server/handler.go:33`; the OIDC callback is `/auth/callback` — `apps/auth-service/internal/oidc/resource.go:50`; Traefik strips `/api/auth`).
- Every `Bash` call in an implementer subagent must be prefixed with `cd /home/tumidanski/source/MyFleet/.worktrees/task-002-k3s-cluster-deployment && ...`. Never `git add -A` or `git add .` — stage named paths only. No destructive git operations.
- Node is not on `PATH` by default in this environment. Before any `npm`/`make fe-*` command run:
  `export NVM_DIR="$HOME/.nvm" && . "$NVM_DIR/nvm.sh" && nvm use 22`
  (`node` v22.22.2 is installed via nvm). `node_modules/` is absent in this worktree — run `npm ci` once before the first frontend command.

---

## Design Amendment Incorporated Here

design.md §4.3/§7/§8 assumed browser-facing presigned MinIO URLs remain in use. During planning this was found to be broken under decision #1: `POST /api/media` returns a presigned URL built from `MINIO_ENDPOINT` (`apps/media-service/internal/storage/minio.go:106-107`), the browser PUTs bytes straight to it (`apps/web/src/services/api/MediaService.ts:37-41`), and `minio.minio.svc.cluster.local:9000` is not resolvable from a browser. The S3 v4 signature covers the Host header, so the URL cannot be rewritten client-side.

**Decision (maintainer, during planning): proxy the bytes through media-service. Do not expose MinIO publicly** — not on its own hostname and not on a path under the MyFleet hosts, because the shared MinIO also holds `atlas-*` buckets.

This adds Tasks 3–6 and a `MEDIA_MAX_UPLOAD_BYTES` config key, and removes all presign plumbing. design.md carries this as Amendment A1 (§10).

---

## File Structure

**Kustomize (Task 1, 2, 8)**

| Path | Responsibility |
|---|---|
| `deploy/k8s/base/kustomization.yaml` | Apps only: namespace, routing middlewares, four services, web |
| `deploy/k8s/base/routing/middlewares.yaml` | The four `stripPrefix` Middleware CRs — needed by both overlays |
| `deploy/k8s/base/web/{deployment,service}.yaml` | SPA Deployment (hardened) + Service 80→8080 |
| `deploy/k8s/infra-local/kustomization.yaml` | Aggregates dev-only infra; referenced only by `overlays/local` |
| `deploy/k8s/infra-local/{postgres,minio,redpanda}.yaml` | Bundled dev stores, PVCs pinned to `storageClassName: longhorn` |
| `deploy/k8s/infra-local/traefik.yaml` | Dev Traefik Deployment + ServiceAccount + ClusterRole/Binding + NodePort Service. **Must never reach bee.** |
| `deploy/k8s/infra-local/ingressroute.yaml` | Local host-less IngressRoute |
| `deploy/k8s/infra-local/secrets-local.yaml` | Placeholder Secrets for local dev only (keeps `overlays/local` usable) |
| `deploy/k8s/overlays/local/kustomization.yaml` | `../../base` + `../../infra-local`, `:local` tags, `IfNotPresent` |
| `deploy/k8s/overlays/main/kustomization.yaml` | SHA-pinned images, `imagePullPolicy: Always`, ConfigMap patches |
| `deploy/k8s/overlays/main/ingressroute.yaml` | Two hosts, per-route middlewares, explicit priorities |
| `deploy/k8s/overlays/main/patches/*.yaml` | ConfigMap value overrides per service |
| `deploy/k8s/secrets.example.yaml` | Template for the four out-of-band Secrets. Never applied directly, never in any kustomization. |

**media-service byte proxy (Tasks 3, 4)**

| Path | Responsibility |
|---|---|
| `apps/media-service/internal/mediaobject/processor.go` | `ObjectStore` interface, `StoreContent`, `Content`; presign code removed |
| `apps/media-service/internal/mediaobject/resource.go` | `PUT /media/{id}/content`, `GET /media/{id}/content`; `/download` removed |
| `apps/media-service/internal/mediaobject/rest.go` | `Attributes` without `uploadUrl`/`downloadUrl` |
| `apps/media-service/internal/storage/minio.go` | `PresignPut`/`PresignGet` removed (unused after this change) |
| `apps/media-service/cmd/main.go` | Reads `MEDIA_MAX_UPLOAD_BYTES`, passes it to routes |

**Web (Tasks 5, 6)**

| Path | Responsibility |
|---|---|
| `packages/shared-ts/src/apiClient.ts` | `requestBlob` — authenticated binary GET sharing the 401-refresh path |
| `apps/web/src/services/api/MediaService.ts` | `putContent`, `getContentBlob`; presigned methods removed |
| `apps/web/src/lib/hooks/api/media.ts` | `performMediaUpload` via proxy; `useMediaContentUrl` (blob object URL + revoke) |
| `apps/web/src/components/features/vehicles/media/MediaThumbnail.tsx` | Renders the blob object URL |
| `apps/web/src/types/models/media.ts` | `MediaObjectAttributes` without the two URL fields |

**CI & docs (Tasks 9, 10)**

| Path | Responsibility |
|---|---|
| `.github/workflows/main.yml` | `web` in `publish`+`trivy`; `bump-overlay` job; `paths-ignore` |
| `docs/runbooks/k3s-deployment.md` | One-time bootstrap: Postgres DDL, MinIO bucket/user/policy, Secrets, DNS, Google console, Argo CD |
| `CLAUDE.md` | Build & Verification commands |

---

## Task 1: Split Kustomize base — apps only, infra to `infra-local/`

**Files:**
- Create: `deploy/k8s/base/routing/middlewares.yaml`
- Create: `deploy/k8s/infra-local/kustomization.yaml`
- Create: `deploy/k8s/infra-local/traefik.yaml`
- Create: `deploy/k8s/infra-local/ingressroute.yaml`
- Create: `deploy/k8s/infra-local/secrets-local.yaml`
- Create: `deploy/k8s/secrets.example.yaml`
- Move: `deploy/k8s/base/infra/{postgres,minio,redpanda}.yaml` → `deploy/k8s/infra-local/`
- Delete: `deploy/k8s/base/infra/traefik.yaml`, `deploy/k8s/base/{auth,fleet,media,notification}-service/secret.yaml`
- Modify: `deploy/k8s/base/kustomization.yaml`, `deploy/k8s/overlays/local/kustomization.yaml`
- Modify: all 12 `base/*-service/{configmap,deployment,service}.yaml` (strip `metadata.namespace`)

**Interfaces:**
- Consumes: nothing (first task).
- Produces: `base/` renders apps only — Middlewares named `auth-stripprefix`, `fleet-stripprefix`, `media-stripprefix`, `notifications-stripprefix`; ConfigMaps `auth-service-config`, `fleet-service-config`, `media-service-config`, `notification-service-config`; Deployments/Services `auth-service`, `fleet-service`, `media-service`, `notification-service` (all port 8080). `infra-local/` is a standalone kustomize directory referenced as `../../infra-local`.

- [ ] **Step 1: Record the pre-change baseline**

```bash
cd /home/tumidanski/source/MyFleet/.worktrees/task-002-k3s-cluster-deployment
kustomize build deploy/k8s/overlays/local | grep -c '^kind:'
```

Expected: `39`. Write this number down — Step 12 compares against it.

- [ ] **Step 2: Move the three stateful infra manifests with git**

```bash
cd /home/tumidanski/source/MyFleet/.worktrees/task-002-k3s-cluster-deployment
mkdir -p deploy/k8s/infra-local deploy/k8s/base/routing
git mv deploy/k8s/base/infra/postgres.yaml deploy/k8s/infra-local/postgres.yaml
git mv deploy/k8s/base/infra/minio.yaml    deploy/k8s/infra-local/minio.yaml
git mv deploy/k8s/base/infra/redpanda.yaml deploy/k8s/infra-local/redpanda.yaml
```

- [ ] **Step 3: Pin the three PVCs to `longhorn`**

Add `storageClassName: longhorn` to each PVC spec. In `deploy/k8s/infra-local/postgres.yaml` the PVC becomes:

```yaml
---
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: postgres-pvc
spec:
  accessModes:
    - ReadWriteOnce
  storageClassName: longhorn
  resources:
    requests:
      storage: 5Gi
```

Do the same for `minio-pvc` in `minio.yaml` (`storage: 10Gi`) and `redpanda-pvc` in `redpanda.yaml` (`storage: 5Gi`).

bee has two StorageClasses both marked default (`local-path` and `longhorn`), so an unpinned PVC is non-deterministic. The main overlay has no PVCs at all, so this only matters for anyone applying the local overlay to a real cluster.

- [ ] **Step 4: Extract the local Traefik into `infra-local/traefik.yaml`**

Create `deploy/k8s/infra-local/traefik.yaml` containing **only** the ServiceAccount, ClusterRole, ClusterRoleBinding, Deployment and NodePort Service from the old `base/infra/traefik.yaml` — copy those five documents across verbatim except for dropping every `namespace: myfleet` line from `metadata`. Add this header comment at the top of the file:

```yaml
---
# Dev-only Traefik. This MUST NOT be applied to a shared cluster: the
# ClusterRole below grants a cluster-wide watch on Traefik CRDs, and bee's
# k3s Traefik already runs --providers.kubernetescrd cluster-wide. Two
# controllers reconciling the same IngressRoutes would fight over every
# route on the cluster, atlas's included. Referenced only by overlays/local.
```

- [ ] **Step 5: Move the four Middlewares to `base/routing/middlewares.yaml`**

Both overlays need these, so they belong in the base. Create `deploy/k8s/base/routing/middlewares.yaml`:

```yaml
---
# Strip-prefix Middlewares for each gateway path prefix. Needed by both the
# local overlay (its own Traefik) and the main overlay (bee's cluster Traefik).
apiVersion: traefik.io/v1alpha1
kind: Middleware
metadata:
  name: auth-stripprefix
spec:
  stripPrefix:
    prefixes:
      - /api/auth

---
apiVersion: traefik.io/v1alpha1
kind: Middleware
metadata:
  name: fleet-stripprefix
spec:
  stripPrefix:
    prefixes:
      - /api/fleet

---
apiVersion: traefik.io/v1alpha1
kind: Middleware
metadata:
  name: media-stripprefix
spec:
  stripPrefix:
    prefixes:
      - /api/media

---
apiVersion: traefik.io/v1alpha1
kind: Middleware
metadata:
  name: notifications-stripprefix
spec:
  stripPrefix:
    prefixes:
      - /api/notifications
```

- [ ] **Step 6: Move the local IngressRoute to `infra-local/ingressroute.yaml`**

Create `deploy/k8s/infra-local/ingressroute.yaml` with the host-less route set from the old `base/infra/traefik.yaml` (local dev reaches Traefik by NodePort, so there is no Host match):

```yaml
---
# Local-dev routing: no Host match, since local dev hits the NodePort directly.
# The main overlay ships its own host-matched IngressRoute.
apiVersion: traefik.io/v1alpha1
kind: IngressRoute
metadata:
  name: myfleet-routes
spec:
  entryPoints:
    - web
  routes:
    - match: PathPrefix(`/api/auth`)
      kind: Rule
      middlewares:
        - name: auth-stripprefix
      services:
        - name: auth-service
          port: 8080
    - match: PathPrefix(`/api/fleet`)
      kind: Rule
      middlewares:
        - name: fleet-stripprefix
      services:
        - name: fleet-service
          port: 8080
    - match: PathPrefix(`/api/media`)
      kind: Rule
      middlewares:
        - name: media-stripprefix
      services:
        - name: media-service
          port: 8080
    - match: PathPrefix(`/api/notifications`)
      kind: Rule
      middlewares:
        - name: notifications-stripprefix
      services:
        - name: notification-service
          port: 8080
```

- [ ] **Step 7: Delete the old traefik base file and the four service Secrets**

```bash
cd /home/tumidanski/source/MyFleet/.worktrees/task-002-k3s-cluster-deployment
git rm deploy/k8s/base/infra/traefik.yaml
git rm deploy/k8s/base/auth-service/secret.yaml \
       deploy/k8s/base/fleet-service/secret.yaml \
       deploy/k8s/base/media-service/secret.yaml \
       deploy/k8s/base/notification-service/secret.yaml
rmdir deploy/k8s/base/infra 2>/dev/null || true
```

- [ ] **Step 8: Move the placeholder Secrets into `infra-local/secrets-local.yaml`**

design.md decision #5 removes placeholder Secrets from the tracked base. Deleting them outright would leave `overlays/local` unusable (every service `envFrom`s a `*-secret`), so they move to `infra-local/` — dev-only, and never reachable from the main overlay.

Create `deploy/k8s/infra-local/secrets-local.yaml`:

```yaml
---
# LOCAL-DEV PLACEHOLDER SECRETS. Referenced only by overlays/local.
# These values are deliberately non-functional. The main overlay ships NO
# Secrets at all — see deploy/k8s/secrets.example.yaml for the real ones,
# which are applied out-of-band so Argo CD's prune cannot remove them.
apiVersion: v1
kind: Secret
metadata:
  name: auth-service-secret
type: Opaque
stringData:
  DATABASE_URL: "postgres://myfleet:myfleet@postgres:5432/myfleet?sslmode=disable&search_path=auth"
  GOOGLE_CLIENT_ID: "PLACEHOLDER_GOOGLE_CLIENT_ID"
  GOOGLE_CLIENT_SECRET: "PLACEHOLDER_GOOGLE_CLIENT_SECRET"
  JWT_PRIVATE_KEY_PEM: "PLACEHOLDER_JWT_PRIVATE_KEY_PEM"
  OIDC_STATE_SECRET: "PLACEHOLDER_OIDC_STATE_SECRET"

---
apiVersion: v1
kind: Secret
metadata:
  name: fleet-service-secret
type: Opaque
stringData:
  DATABASE_URL: "postgres://myfleet:myfleet@postgres:5432/myfleet?sslmode=disable&search_path=fleet"

---
apiVersion: v1
kind: Secret
metadata:
  name: media-service-secret
type: Opaque
stringData:
  DATABASE_URL: "postgres://myfleet:myfleet@postgres:5432/myfleet?sslmode=disable&search_path=media"
  MINIO_ACCESS_KEY: "myfleet"
  MINIO_SECRET_KEY: "myfleet-local-dev"

---
apiVersion: v1
kind: Secret
metadata:
  name: notification-service-secret
type: Opaque
stringData:
  DATABASE_URL: "postgres://myfleet:myfleet@postgres:5432/myfleet?sslmode=disable&search_path=notification"
```

Then align the bundled stores with those credentials so local dev actually connects. In `deploy/k8s/infra-local/postgres.yaml` replace the `postgres-secret` `stringData` with:

```yaml
stringData:
  POSTGRES_USER: "myfleet"
  POSTGRES_PASSWORD: "myfleet"
  POSTGRES_DB: "myfleet"
```

and in `deploy/k8s/infra-local/minio.yaml` replace the `minio-secret` `stringData` with:

```yaml
stringData:
  MINIO_ROOT_USER: "myfleet"
  MINIO_ROOT_PASSWORD: "myfleet-local-dev"
```

- [ ] **Step 9: Create `deploy/k8s/secrets.example.yaml`**

```yaml
---
# TEMPLATE ONLY — never applied directly, never referenced by a kustomization.
#
# The main overlay ships no Secrets: they are applied out-of-band so they are
# untracked by Argo CD and `prune: true` cannot remove them. Copy this file,
# fill in real values, apply with `kubectl apply -n myfleet -f <copy>`, and
# keep the copy out of git.
#
# DATABASE_URL pattern (one schema per service):
#   postgres://myfleet:<password>@postgres.home:5432/myfleet?sslmode=disable&search_path=<schema>
apiVersion: v1
kind: Secret
metadata:
  name: auth-service-secret
  namespace: myfleet
type: Opaque
stringData:
  DATABASE_URL: "postgres://myfleet:REPLACE_ME@postgres.home:5432/myfleet?sslmode=disable&search_path=auth"
  GOOGLE_CLIENT_ID: "REPLACE_ME"
  GOOGLE_CLIENT_SECRET: "REPLACE_ME"
  # openssl genrsa 2048
  JWT_PRIVATE_KEY_PEM: "REPLACE_ME"
  # openssl rand -hex 32
  OIDC_STATE_SECRET: "REPLACE_ME"

---
apiVersion: v1
kind: Secret
metadata:
  name: fleet-service-secret
  namespace: myfleet
type: Opaque
stringData:
  DATABASE_URL: "postgres://myfleet:REPLACE_ME@postgres.home:5432/myfleet?sslmode=disable&search_path=fleet"

---
apiVersion: v1
kind: Secret
metadata:
  name: media-service-secret
  namespace: myfleet
type: Opaque
stringData:
  DATABASE_URL: "postgres://myfleet:REPLACE_ME@postgres.home:5432/myfleet?sslmode=disable&search_path=media"
  # Scoped MinIO user — policy limited to the myfleet-media bucket.
  MINIO_ACCESS_KEY: "myfleet"
  MINIO_SECRET_KEY: "REPLACE_ME"

---
apiVersion: v1
kind: Secret
metadata:
  name: notification-service-secret
  namespace: myfleet
type: Opaque
stringData:
  DATABASE_URL: "postgres://myfleet:REPLACE_ME@postgres.home:5432/myfleet?sslmode=disable&search_path=notification"
```

- [ ] **Step 10: Strip `metadata.namespace` from the base manifests**

The base kustomization sets `namespace: myfleet`, so per-manifest namespaces are redundant and stop an overlay from relocating the app.

```bash
cd /home/tumidanski/source/MyFleet/.worktrees/task-002-k3s-cluster-deployment
sed -i '/^  namespace: myfleet$/d' \
  deploy/k8s/base/auth-service/*.yaml \
  deploy/k8s/base/fleet-service/*.yaml \
  deploy/k8s/base/media-service/*.yaml \
  deploy/k8s/base/notification-service/*.yaml
grep -rn 'namespace: myfleet' deploy/k8s/base/ || echo "clean"
```

Expected: `clean`. (`base/namespace.yaml` uses `name: myfleet`, not `namespace:`, so it is untouched.)

- [ ] **Step 11: Rewrite `base/kustomization.yaml` — apps only**

```yaml
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization

namespace: myfleet

# Apps only. Bundled dev infrastructure lives in ../infra-local and is
# referenced solely by overlays/local. Secrets are NOT here — the main overlay
# takes them out-of-band (see ../secrets.example.yaml).
resources:
  - namespace.yaml
  - routing/middlewares.yaml

  - auth-service/configmap.yaml
  - auth-service/deployment.yaml
  - auth-service/service.yaml

  - fleet-service/configmap.yaml
  - fleet-service/deployment.yaml
  - fleet-service/service.yaml

  - media-service/configmap.yaml
  - media-service/deployment.yaml
  - media-service/service.yaml

  - notification-service/configmap.yaml
  - notification-service/deployment.yaml
  - notification-service/service.yaml
```

Create `deploy/k8s/infra-local/kustomization.yaml`:

```yaml
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization

# Dev-only infrastructure + placeholder Secrets + dev Traefik. Referenced only
# by overlays/local. Never include this from an overlay that targets a shared
# cluster — see traefik.yaml for why.
resources:
  - secrets-local.yaml
  - postgres.yaml
  - minio.yaml
  - redpanda.yaml
  - traefik.yaml
  - ingressroute.yaml
```

Then point the local overlay at both directories — in `deploy/k8s/overlays/local/kustomization.yaml` replace the `resources` block with:

```yaml
resources:
  - ../../base
  - ../../infra-local
```

Leave the `replicas`, `images` and `patches` blocks in that file untouched for now; Task 2 adds the `web` entries.

- [ ] **Step 12: Verify the local overlay still renders identically in substance**

```bash
cd /home/tumidanski/source/MyFleet/.worktrees/task-002-k3s-cluster-deployment
kustomize build deploy/k8s/overlays/local > /tmp/local-after.yaml && echo "BUILD OK"
grep '^kind:' /tmp/local-after.yaml | sort | uniq -c
```

Expected: `BUILD OK`, and the same 39 resources as Step 1 with the same kind counts:
`1 ClusterRole, 1 ClusterRoleBinding, 5 ConfigMap, 8 Deployment, 1 IngressRoute, 4 Middleware, 1 Namespace, 3 PersistentVolumeClaim, 6 Secret, 8 Service, 1 ServiceAccount`.

- [ ] **Step 13: Verify the base alone is app-only and clean**

```bash
cd /home/tumidanski/source/MyFleet/.worktrees/task-002-k3s-cluster-deployment
kustomize build deploy/k8s/base > /tmp/base-after.yaml && echo "BUILD OK"
echo "--- must all be 0 ---"
grep -c 'kind: Secret'             /tmp/base-after.yaml || true
grep -c 'kind: PersistentVolumeClaim' /tmp/base-after.yaml || true
grep -c 'kind: ClusterRole'        /tmp/base-after.yaml || true
grep -ci 'placeholder'             /tmp/base-after.yaml || true
echo "--- must be 4 ---"
grep -c 'kind: Middleware'         /tmp/base-after.yaml
```

Expected: the four counts are `0`, and Middleware is `4`.

- [ ] **Step 14: Verify PVC storage classes are pinned**

```bash
cd /home/tumidanski/source/MyFleet/.worktrees/task-002-k3s-cluster-deployment
grep -c 'storageClassName: longhorn' /tmp/local-after.yaml
```

Expected: `3`.

- [ ] **Step 15: Commit**

```bash
cd /home/tumidanski/source/MyFleet/.worktrees/task-002-k3s-cluster-deployment
git add deploy/k8s/base deploy/k8s/infra-local deploy/k8s/overlays/local/kustomization.yaml deploy/k8s/secrets.example.yaml
git commit -m "refactor(k8s): split apps from dev infra, drop tracked placeholder secrets

Moves postgres/minio/redpanda/traefik out of base/ into infra-local/, which
only overlays/local references. The four stripPrefix Middlewares stay in the
base because both overlays need them. Placeholder Secrets move to
infra-local/secrets-local.yaml so the base ships none and the coming main
overlay can take them out-of-band. PVCs pin storageClassName: longhorn
because bee has two default StorageClasses."
git rev-parse --show-toplevel   # must end with /.worktrees/task-002-k3s-cluster-deployment
git branch --show-current       # must be task-002-k3s-cluster-deployment
```

---

## Task 2: Add the web SPA to the base on unprivileged nginx

**Files:**
- Modify: `apps/web/Dockerfile` (stage 2)
- Modify: `apps/web/nginx.conf:11` (`listen`)
- Create: `deploy/k8s/base/web/deployment.yaml`
- Create: `deploy/k8s/base/web/service.yaml`
- Modify: `deploy/k8s/base/kustomization.yaml`
- Modify: `deploy/k8s/overlays/local/kustomization.yaml`

**Interfaces:**
- Consumes: Task 1's `base/kustomization.yaml` resource list and `overlays/local/kustomization.yaml` structure.
- Produces: Deployment `web` and Service `web` (port `80` → targetPort `8080`), image `ghcr.io/jtumidanski/myfleet-web`. Task 8's IngressRoute routes the catch-all to `web:80`; Task 9 builds this image.

- [ ] **Step 1: Switch stage 2 to unprivileged nginx**

Stock `nginx:alpine` starts as root to bind :80 and writes to `/var/cache/nginx` and `/var/run`, so it cannot run under the same hardened `securityContext` as the Go services. `nginxinc/nginx-unprivileged` runs as UID 101 and binds an unprivileged port.

In `apps/web/Dockerfile` replace the stage-2 block:

```dockerfile
# --- Stage 2: static server -------------------------------------------------
# Unprivileged nginx (UID 101, binds :8080) so the container runs with the same
# hardened securityContext as the Go services: runAsNonRoot + readOnlyRootFilesystem.
FROM nginxinc/nginx-unprivileged:alpine
COPY apps/web/nginx.conf /etc/nginx/conf.d/default.conf
COPY --from=build /app/apps/web/dist /usr/share/nginx/html
EXPOSE 8080
```

- [ ] **Step 2: Point nginx at port 8080**

In `apps/web/nginx.conf` change `listen 80;` to:

```nginx
    listen 8080;
```

- [ ] **Step 3: Build the image to prove the switch works**

```bash
cd /home/tumidanski/source/MyFleet/.worktrees/task-002-k3s-cluster-deployment
docker build -f apps/web/Dockerfile -t myfleet-web:verify .
```

Expected: build succeeds.

- [ ] **Step 4: Verify the image runs unprivileged with a read-only root filesystem**

```bash
docker run --rm -d --name web-verify \
  --read-only \
  --user 101 \
  --tmpfs /tmp \
  -p 18080:8080 myfleet-web:verify
sleep 3
curl -sf -o /dev/null -w '%{http_code}\n' http://localhost:18080/
curl -sf -o /dev/null -w '%{http_code}\n' http://localhost:18080/vehicles
docker rm -f web-verify
```

Expected: `200` for both — the second proves the SPA fallback serves `index.html` on a deep link.

If nginx fails to start because it still needs a writable path, add the offending path as an `emptyDir` in Step 5's Deployment and a matching `--tmpfs` here, then re-run. Do not relax `readOnlyRootFilesystem`.

- [ ] **Step 5: Create the web Deployment**

Create `deploy/k8s/base/web/deployment.yaml`:

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: web
  labels:
    app: web
spec:
  replicas: 1
  selector:
    matchLabels:
      app: web
  template:
    metadata:
      labels:
        app: web
    spec:
      securityContext:
        runAsNonRoot: true
        # UID 101 = nginx in nginxinc/nginx-unprivileged.
        runAsUser: 101
        fsGroup: 101
      containers:
        - name: web
          image: ghcr.io/jtumidanski/myfleet-web:latest
          ports:
            - containerPort: 8080
              name: http
          securityContext:
            allowPrivilegeEscalation: false
            readOnlyRootFilesystem: true
            capabilities:
              drop: ["ALL"]
          readinessProbe:
            httpGet:
              path: /
              port: 8080
            initialDelaySeconds: 3
            periodSeconds: 10
            failureThreshold: 3
          livenessProbe:
            httpGet:
              path: /
              port: 8080
            initialDelaySeconds: 10
            periodSeconds: 20
            failureThreshold: 3
          resources:
            requests:
              memory: "16Mi"
              cpu: "10m"
            limits:
              memory: "64Mi"
              cpu: "200m"
          volumeMounts:
            # nginx writes its pid and temp bodies at runtime; the root
            # filesystem is read-only, so give it tmpfs.
            - name: nginx-tmp
              mountPath: /tmp
      volumes:
        - name: nginx-tmp
          emptyDir: {}
```

- [ ] **Step 6: Create the web Service**

Create `deploy/k8s/base/web/service.yaml`. Port 80 is kept as the Service port so the IngressRoute and every other consumer address `web:80` regardless of the container's port:

```yaml
apiVersion: v1
kind: Service
metadata:
  name: web
  labels:
    app: web
spec:
  type: ClusterIP
  selector:
    app: web
  ports:
    - port: 80
      targetPort: 8080
      name: http
```

- [ ] **Step 7: Register web in the base**

Append to the `resources` list in `deploy/k8s/base/kustomization.yaml`:

```yaml
  - web/deployment.yaml
  - web/service.yaml
```

- [ ] **Step 8: Give the local overlay matching web entries**

Without these, local dev would pull a `:latest` web image from GHCR while every other service ran locally built code. In `deploy/k8s/overlays/local/kustomization.yaml`:

Add to `replicas`:

```yaml
  - name: web
    count: 1
```

Add to `images`:

```yaml
  - name: ghcr.io/jtumidanski/myfleet-web
    newTag: local
```

Add to `patches`:

```yaml
  - target:
      kind: Deployment
      name: web
    patch: |-
      - op: add
        path: /spec/template/spec/containers/0/imagePullPolicy
        value: IfNotPresent
```

- [ ] **Step 9: Verify both renders**

```bash
cd /home/tumidanski/source/MyFleet/.worktrees/task-002-k3s-cluster-deployment
kustomize build deploy/k8s/base > /tmp/base-web.yaml && echo "BASE OK"
kustomize build deploy/k8s/overlays/local > /tmp/local-web.yaml && echo "LOCAL OK"
grep -c '^kind:' /tmp/local-web.yaml
grep -A2 'myfleet-web' /tmp/local-web.yaml | head -5
```

Expected: both build; local is now `41` resources (39 + web Deployment + web Service); the web image renders as `ghcr.io/jtumidanski/myfleet-web:local`.

- [ ] **Step 10: Commit**

```bash
cd /home/tumidanski/source/MyFleet/.worktrees/task-002-k3s-cluster-deployment
git add apps/web/Dockerfile apps/web/nginx.conf deploy/k8s/base/web deploy/k8s/base/kustomization.yaml deploy/k8s/overlays/local/kustomization.yaml
git commit -m "feat(web): ship the SPA on unprivileged nginx with k8s manifests

Switches the runtime image to nginxinc/nginx-unprivileged:alpine on :8080 so
the web container matches the Go services' hardened securityContext
(runAsNonRoot + readOnlyRootFilesystem + all caps dropped). Adds base/web
manifests with the Service on 80 -> 8080, and the matching local-overlay
replicas/image/pull-policy entries."
git rev-parse --show-toplevel
git branch --show-current
```

---

## Task 3: Proxy media upload bytes through media-service

**Files:**
- Modify: `apps/media-service/internal/mediaobject/processor.go:25-33` (interface), `:80-107` (`InitUpload`)
- Modify: `apps/media-service/internal/mediaobject/resource.go:27-52`
- Modify: `apps/media-service/internal/mediaobject/rest.go:15,37-45`
- Modify: `apps/media-service/cmd/main.go`
- Test: `apps/media-service/internal/mediaobject/processor_test.go`

**Interfaces:**
- Consumes: existing `storage.Client.PutObject(ctx, key string, r io.Reader, size int64, contentType string) error`; `Model.WithSize(int64) Model`; `Administrator.Update(Model) (Model, error)`; `Processor.getActive(id string) (Model, error)`; `AuthorizeAccess(m Model, identityFleetID string) error`.
- Produces:
  - `type ObjectStore interface { PutObject(ctx context.Context, key string, r io.Reader, size int64, contentType string) error; Bucket() string }` — Task 4 adds `GetObject` to this same interface.
  - `func (pr *Processor) StoreContent(ctx context.Context, id, identityFleetID string, r io.Reader, size int64) (Model, error)`
  - `func (pr *Processor) InitUpload(fleetID, userID, contentType, filename string) (Model, error)` — note: **no longer returns a URL**.
  - `func InitializeRoutes(log logrus.FieldLogger, db *gorm.DB, st ObjectStore, maxUploadBytes int64) func(chi.Router)`
  - Route `PUT /media/{id}/content` (public: `PUT /api/media/{id}/content`) → `200` + JSON:API document.

- [ ] **Step 1: Write the failing test**

Append to `apps/media-service/internal/mediaobject/processor_test.go`:

```go
// fakeStore records what was written so the proxy path can be asserted without
// a live MinIO.
type fakeStore struct {
	bucket    string
	putKey    string
	putBody   []byte
	putSize   int64
	putCT     string
	putErr    error
}

func (f *fakeStore) Bucket() string { return f.bucket }

func (f *fakeStore) PutObject(ctx context.Context, key string, r io.Reader, size int64, contentType string) error {
	if f.putErr != nil {
		return f.putErr
	}
	b, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	f.putKey, f.putBody, f.putSize, f.putCT = key, b, size, contentType
	return nil
}

func TestStoreContent_streamsToObjectStoreAndRecordsSize(t *testing.T) {
	db := newConfirmTestDB(t)
	store := &fakeStore{bucket: "myfleet-media"}
	pr := NewProcessor(logrus.New(), NewProvider(db), NewAdministrator(db), store)

	created, err := pr.InitUpload("fleet-a", "u1", "image/jpeg", "photo.jpg")
	if err != nil {
		t.Fatalf("init upload: %v", err)
	}

	body := []byte("jpeg-bytes")
	updated, err := pr.StoreContent(context.Background(), created.ID(), "fleet-a", bytes.NewReader(body), int64(len(body)))
	if err != nil {
		t.Fatalf("store content: %v", err)
	}

	if store.putKey != created.ObjectKey() {
		t.Fatalf("wrote to key %q, want %q", store.putKey, created.ObjectKey())
	}
	if string(store.putBody) != string(body) {
		t.Fatalf("wrote body %q, want %q", store.putBody, body)
	}
	// The content type must come from the row created at init, not the request.
	if store.putCT != "image/jpeg" {
		t.Fatalf("wrote content-type %q, want image/jpeg", store.putCT)
	}
	if updated.Size() != int64(len(body)) {
		t.Fatalf("size = %d, want %d", updated.Size(), len(body))
	}
	if updated.Status() != StatusUploaded {
		t.Fatalf("status = %q, want uploaded (confirm does the transition)", updated.Status())
	}
}

func TestStoreContent_crossFleetIs404(t *testing.T) {
	db := newConfirmTestDB(t)
	store := &fakeStore{bucket: "myfleet-media"}
	pr := NewProcessor(logrus.New(), NewProvider(db), NewAdministrator(db), store)

	created, err := pr.InitUpload("fleet-a", "u1", "image/jpeg", "photo.jpg")
	if err != nil {
		t.Fatalf("init upload: %v", err)
	}

	_, err = pr.StoreContent(context.Background(), created.ID(), "fleet-b", bytes.NewReader([]byte("x")), 1)
	if !errors.Is(err, server.ErrNotFound) {
		t.Fatalf("cross-fleet write must be 404, got %v", err)
	}
	if store.putKey != "" {
		t.Fatalf("cross-fleet write must not touch storage, wrote key %q", store.putKey)
	}
}
```

`newConfirmTestDB(t)` already exists at `processor_test.go:102` — it opens an in-memory SQLite, attaches a `media` schema, creates `media.media_objects` and migrates the outbox, which is exactly what these tests need. Reuse it; its Confirm-specific name is legacy, and renaming it would churn the existing tests for nothing.

There is **no** logger helper in this file — these are the first tests here to construct a `Processor` — so pass `logrus.New()` directly.

Add `bytes`, `context`, `io` and `github.com/sirupsen/logrus` to the test file's import block.

- [ ] **Step 2: Run the test to verify it fails**

```bash
cd /home/tumidanski/source/MyFleet/.worktrees/task-002-k3s-cluster-deployment
go test ./apps/media-service/internal/mediaobject/ -run 'TestStoreContent' -v
```

Expected: compile failure — `pr.StoreContent` undefined, and `InitUpload` returns three values not two.

- [ ] **Step 3: Replace the presign interface with `ObjectStore`**

In `apps/media-service/internal/mediaobject/processor.go`, delete the `presignTTL` constant and the `Presigner` interface, and put in their place:

```go
// ObjectStore is the subset of storage.Client the processor needs. Implemented
// by *storage.Client; kept as an interface so the processor is unit-testable.
//
// Bytes are proxied through this service rather than handed to the browser as
// presigned URLs: MinIO is a shared cluster service that also holds other
// applications' buckets, so it is never exposed outside the cluster.
type ObjectStore interface {
	PutObject(ctx context.Context, key string, r io.Reader, size int64, contentType string) error
	Bucket() string
}
```

Update the struct field and constructor to the new type:

```go
type Processor struct {
	log     logrus.FieldLogger
	p       Provider
	a       Administrator
	storage ObjectStore
}

func NewProcessor(log logrus.FieldLogger, p Provider, a Administrator, st ObjectStore) *Processor {
	return &Processor{log: log, p: p, a: a, storage: st}
}
```

Add `"io"` to the import block and drop `"time"` only if nothing else in the file uses it (`Confirm` uses `time.Now()`, so keep it).

- [ ] **Step 4: Make `InitUpload` stop presigning**

Replace the tail of `InitUpload` — everything from `created, err := pr.a.Insert(m)` onward — and its doc comment:

```go
// InitUpload creates a media-object row in the uploaded state. The client then
// PUTs the bytes to /media/{id}/content; this service proxies them to object
// storage so MinIO is never reachable from the browser.
func (pr *Processor) InitUpload(fleetID, userID, contentType, filename string) (Model, error) {
```

and:

```go
	created, err := pr.a.Insert(m)
	if err != nil {
		return Model{}, err
	}
	return created, nil
}
```

Every `return Model{}, "", err` inside the function becomes `return Model{}, err`.

- [ ] **Step 5: Add `StoreContent`**

Insert after `InitUpload`:

```go
// StoreContent streams the request body into object storage for an object still
// in the uploaded state and records the byte count. Fleet-scoped; the content
// type comes from the row created at init, never from the request, so a client
// cannot relabel someone else's bytes. The status transition and the
// media.uploaded event stay in Confirm.
func (pr *Processor) StoreContent(ctx context.Context, id, identityFleetID string, r io.Reader, size int64) (Model, error) {
	m, err := pr.getActive(id)
	if err != nil {
		return Model{}, err
	}
	if err := AuthorizeAccess(m, identityFleetID); err != nil {
		return Model{}, err
	}
	if m.Status() != StatusUploaded {
		return Model{}, server.ErrConflict
	}
	if err := pr.storage.PutObject(ctx, m.ObjectKey(), r, size, m.ContentType()); err != nil {
		return Model{}, err
	}
	return pr.a.Update(m.WithSize(size))
}
```

- [ ] **Step 6: Run the tests to verify they pass**

```bash
cd /home/tumidanski/source/MyFleet/.worktrees/task-002-k3s-cluster-deployment
go test ./apps/media-service/internal/mediaobject/ -run 'TestStoreContent' -v
```

Expected: both tests PASS.

- [ ] **Step 7: Drop `uploadUrl` from the transport layer**

In `apps/media-service/internal/mediaobject/rest.go` remove the `UploadURL` field from `Attributes` and delete `TransformWithUploadURL` entirely. Leave `DownloadURL` and `TransformWithDownloadURL` alone — Task 4 removes those.

- [ ] **Step 8: Wire the upload route**

In `apps/media-service/internal/mediaobject/resource.go`, change the signature and the init handler's response, and add the content route. The `InitializeRoutes` signature becomes:

```go
func InitializeRoutes(log logrus.FieldLogger, db *gorm.DB, st ObjectStore, maxUploadBytes int64) func(chi.Router) {
```

In the `POST /media` handler, replace the `proc.InitUpload(...)` call and its response with:

```go
			m, err := proc.InitUpload(identity.ActiveFleetID, identity.UserID, attrs.ContentType, attrs.OriginalFilename)
			if err != nil {
				log.WithError(err).Error("media init upload failed")
				server.WriteError(w, err)
				return
			}
			server.WriteJSON(w, http.StatusCreated, server.Document{Data: Transform(m)})
```

and update that handler's comment to `// POST /media — init upload: create the row in the uploaded state.`

Then add this route immediately after the `POST /media` block:

```go
		// PUT /media/{id}/content — proxy the raw bytes to object storage.
		// Bounded by maxUploadBytes so a client cannot stream unbounded data;
		// Cloudflare also caps request bodies at the edge for the public host.
		r.Put("/media/{id}/content", func(w http.ResponseWriter, req *http.Request) {
			identity := auth.IdentityFromContext(req.Context())
			id := chi.URLParam(req, "id")
			if err := requireWrite(identity); err != nil {
				server.WriteError(w, err)
				return
			}
			body := http.MaxBytesReader(w, req.Body, maxUploadBytes)
			defer func() { _ = body.Close() }()

			// Content-Length is advisory here: MaxBytesReader is the real
			// bound. -1 lets the SDK stream an unknown-length body.
			size := req.ContentLength
			if size < 0 || size > maxUploadBytes {
				size = -1
			}
			m, err := proc.StoreContent(req.Context(), id, identity.ActiveFleetID, body, size)
			if err != nil {
				var maxErr *http.MaxBytesError
				if errors.As(err, &maxErr) {
					server.WriteError(w, server.ErrRequestEntityTooLarge)
					return
				}
				log.WithError(err).Error("media content upload failed")
				server.WriteError(w, err)
				return
			}
			server.WriteJSON(w, http.StatusOK, server.Document{Data: Transform(m)})
		})
```

Add `"errors"` to the import block.

- [ ] **Step 9: Add the 413 error sentinel**

`packages/shared-go/server/errors.go` has no 413 sentinel (verified: it declares `ErrUnauthorized`, `ErrForbidden`, `ErrNotFound`, `ErrConflict`, `ErrGone`, `ErrValidation`). Add one in exactly the existing style — a sentinel in the `var` block plus a case in `StatusFor`.

In the `var` block, after `ErrGone`:

```go
	ErrRequestEntityTooLarge = errors.New("request entity too large") // 413
```

In `StatusFor`, after the `ErrGone` case:

```go
	case errors.Is(err, ErrRequestEntityTooLarge):
		return 413
```

Realign the `var` block's comments with `gofmt` after the insertion — the new name is longer than the existing ones, so the whole block re-indents:

```bash
cd /home/tumidanski/source/MyFleet/.worktrees/task-002-k3s-cluster-deployment
gofmt -w packages/shared-go/server/errors.go
go build ./packages/shared-go/... && echo "SHARED-GO OK"
```

Expected: `SHARED-GO OK`.

- [ ] **Step 10: Read the upload cap from config and pass it through**

In `apps/media-service/cmd/main.go`, find the `mediaobject.InitializeRoutes(` call and add the new argument. Read the cap next to the existing `MEDIA_WORKERS` lookup:

```go
	// Cap proxied uploads. 25 MiB sits under Cloudflare's free-plan request-body
	// ceiling, so the edge is not the first thing a user discovers.
	maxUploadBytes := int64(config.GetInt("MEDIA_MAX_UPLOAD_BYTES", 26214400))
```

Then pass `maxUploadBytes` as the fourth argument to `mediaobject.InitializeRoutes`.

Check the real name and signature of the config getter before writing this — `config.GetInt("MEDIA_WORKERS", 2)` is used at `apps/media-service/cmd/main.go:85`. If `GetInt` returns `int`, the conversion above is correct as written.

- [ ] **Step 11: Add the config key to the base ConfigMap**

Append to `data` in `deploy/k8s/base/media-service/configmap.yaml`:

```yaml
  MEDIA_MAX_UPLOAD_BYTES: "26214400"
```

- [ ] **Step 12: Verify the whole service builds and its tests pass**

```bash
cd /home/tumidanski/source/MyFleet/.worktrees/task-002-k3s-cluster-deployment
go build ./apps/media-service/... && echo "BUILD OK"
go vet ./apps/media-service/... && echo "VET OK"
go test -race ./apps/media-service/... && echo "TESTS OK"
```

Expected: all three OK. If other call sites of `InitUpload` or `InitializeRoutes` fail to compile, update them — they are the only consumers and must move to the new signatures.

- [ ] **Step 13: Commit**

```bash
cd /home/tumidanski/source/MyFleet/.worktrees/task-002-k3s-cluster-deployment
git add apps/media-service packages/shared-go/server deploy/k8s/base/media-service/configmap.yaml
git commit -m "feat(media): proxy upload bytes instead of presigning MinIO

The browser previously PUT bytes straight to a presigned MinIO URL built from
MINIO_ENDPOINT. Against the shared cluster MinIO that host is unresolvable from
a browser, and the v4 signature covers Host so it cannot be rewritten. Adds
PUT /media/{id}/content, which streams the body to object storage under a
MEDIA_MAX_UPLOAD_BYTES bound, and drops uploadUrl from the API. MinIO stays
unreachable from outside the cluster."
git rev-parse --show-toplevel
git branch --show-current
```

---

## Task 4: Proxy media download bytes through media-service

**Files:**
- Modify: `apps/media-service/internal/mediaobject/processor.go` (`ObjectStore`, remove `DownloadURL`)
- Modify: `apps/media-service/internal/mediaobject/resource.go` (replace `/download` with `/content`)
- Modify: `apps/media-service/internal/mediaobject/rest.go` (remove `DownloadURL`)
- Modify: `apps/media-service/internal/storage/minio.go` (remove presign methods)
- Test: `apps/media-service/internal/mediaobject/processor_test.go`

**Interfaces:**
- Consumes: Task 3's `ObjectStore` interface and `fakeStore` test double; existing `storage.Client.GetObject(ctx, key string) (io.ReadCloser, error)`.
- Produces:
  - `ObjectStore` gains `GetObject(ctx context.Context, key string) (io.ReadCloser, error)`.
  - `func (pr *Processor) Content(ctx context.Context, id, identityFleetID string) (Model, io.ReadCloser, error)`
  - Route `GET /media/{id}/content` (public: `GET /api/media/{id}/content`) → `200`, raw bytes, `Content-Type` from the row.
  - `GET /media/{id}/download` and `attributes.downloadUrl` no longer exist.

- [ ] **Step 1: Write the failing test**

Append to `apps/media-service/internal/mediaobject/processor_test.go`:

```go
func TestContent_returnsBytesAndModelForOwnFleet(t *testing.T) {
	db := newConfirmTestDB(t)
	store := &fakeStore{bucket: "myfleet-media", getBody: []byte("jpeg-bytes")}
	pr := NewProcessor(logrus.New(), NewProvider(db), NewAdministrator(db), store)

	created, err := pr.InitUpload("fleet-a", "u1", "image/jpeg", "photo.jpg")
	if err != nil {
		t.Fatalf("init upload: %v", err)
	}

	m, rc, err := pr.Content(context.Background(), created.ID(), "fleet-a")
	if err != nil {
		t.Fatalf("content: %v", err)
	}
	defer func() { _ = rc.Close() }()

	if store.getKey != created.ObjectKey() {
		t.Fatalf("read key %q, want %q", store.getKey, created.ObjectKey())
	}
	if m.ContentType() != "image/jpeg" {
		t.Fatalf("content type = %q, want image/jpeg", m.ContentType())
	}
	got, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(got) != "jpeg-bytes" {
		t.Fatalf("body = %q, want jpeg-bytes", got)
	}
}

func TestContent_crossFleetIs404(t *testing.T) {
	db := newConfirmTestDB(t)
	store := &fakeStore{bucket: "myfleet-media", getBody: []byte("jpeg-bytes")}
	pr := NewProcessor(logrus.New(), NewProvider(db), NewAdministrator(db), store)

	created, err := pr.InitUpload("fleet-a", "u1", "image/jpeg", "photo.jpg")
	if err != nil {
		t.Fatalf("init upload: %v", err)
	}

	if _, _, err := pr.Content(context.Background(), created.ID(), "fleet-b"); !errors.Is(err, server.ErrNotFound) {
		t.Fatalf("cross-fleet read must be 404, got %v", err)
	}
	if store.getKey != "" {
		t.Fatalf("cross-fleet read must not touch storage, read key %q", store.getKey)
	}
}
```

Extend `fakeStore` from Task 3 with the read side:

```go
	getKey  string
	getBody []byte
	getErr  error
```

```go
func (f *fakeStore) GetObject(ctx context.Context, key string) (io.ReadCloser, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	f.getKey = key
	return io.NopCloser(bytes.NewReader(f.getBody)), nil
}
```

- [ ] **Step 2: Run the test to verify it fails**

```bash
cd /home/tumidanski/source/MyFleet/.worktrees/task-002-k3s-cluster-deployment
go test ./apps/media-service/internal/mediaobject/ -run 'TestContent' -v
```

Expected: compile failure — `pr.Content` undefined.

- [ ] **Step 3: Add `GetObject` to the interface**

In `apps/media-service/internal/mediaobject/processor.go` add the method to `ObjectStore`:

```go
type ObjectStore interface {
	PutObject(ctx context.Context, key string, r io.Reader, size int64, contentType string) error
	GetObject(ctx context.Context, key string) (io.ReadCloser, error)
	Bucket() string
}
```

- [ ] **Step 4: Replace `DownloadURL` with `Content`**

Delete the `DownloadURL` method and put this in its place:

```go
// Content authorizes by fleet and opens the object's bytes for streaming to the
// client. The caller owns closing the returned ReadCloser. Bytes are proxied
// rather than presigned so MinIO stays unreachable from the browser.
func (pr *Processor) Content(ctx context.Context, id, identityFleetID string) (Model, io.ReadCloser, error) {
	m, err := pr.GetByID(id, identityFleetID)
	if err != nil {
		return Model{}, nil, err
	}
	rc, err := pr.storage.GetObject(ctx, m.ObjectKey())
	if err != nil {
		return Model{}, nil, err
	}
	return m, rc, nil
}
```

- [ ] **Step 5: Run the tests to verify they pass**

```bash
cd /home/tumidanski/source/MyFleet/.worktrees/task-002-k3s-cluster-deployment
go test ./apps/media-service/internal/mediaobject/ -run 'TestContent|TestStoreContent' -v
```

Expected: all four tests PASS.

- [ ] **Step 6: Replace the `/download` route with `/content`**

In `apps/media-service/internal/mediaobject/resource.go` delete the whole `GET /media/{id}/download` block and add:

```go
		// GET /media/{id}/content — stream the bytes after authz. Proxied, not
		// presigned: MinIO is a shared cluster service and is never exposed
		// outside the cluster.
		r.Get("/media/{id}/content", func(w http.ResponseWriter, req *http.Request) {
			identity := auth.IdentityFromContext(req.Context())
			id := chi.URLParam(req, "id")
			m, rc, err := proc.Content(req.Context(), id, identity.ActiveFleetID)
			if err != nil {
				server.WriteError(w, err)
				return
			}
			defer func() { _ = rc.Close() }()

			if ct := m.ContentType(); ct != "" {
				w.Header().Set("Content-Type", ct)
			}
			if size := m.Size(); size > 0 {
				w.Header().Set("Content-Length", strconv.FormatInt(size, 10))
			}
			// Per-fleet authorized bytes — never store in a shared cache.
			w.Header().Set("Cache-Control", "private, max-age=300")
			w.WriteHeader(http.StatusOK)
			if _, err := io.Copy(w, rc); err != nil {
				// Headers are already written, so the status cannot be changed;
				// log and let the client see a truncated body.
				log.WithError(err).Warn("media content stream interrupted")
			}
		})
```

Add `"io"` and `"strconv"` to the import block.

- [ ] **Step 7: Remove `downloadUrl` from the transport layer**

In `apps/media-service/internal/mediaobject/rest.go` remove the `DownloadURL` field from `Attributes` and delete `TransformWithDownloadURL`. `Attributes` should now carry no URL fields at all.

- [ ] **Step 8: Delete the now-unused presign methods**

In `apps/media-service/internal/storage/minio.go` delete `PresignPut` and `PresignGet`. Update the package doc comment on line 2 and the `New` comment on line 72, which both still claim bytes are exchanged via presigned URLs:

```go
// always private; bytes are exchanged with clients exclusively by proxying
// through media-service, never by presigned URL — MinIO is a shared cluster
// service and is not exposed outside the cluster.
```

Remove `"net/url"` from the imports if `PresignGet` was its only consumer.

- [ ] **Step 9: Verify nothing references the removed API**

```bash
cd /home/tumidanski/source/MyFleet/.worktrees/task-002-k3s-cluster-deployment
grep -rn 'Presign\|presignTTL\|DownloadURL\|UploadURL\|/download' apps/media-service --include='*.go' || echo "clean"
```

Expected: `clean`.

- [ ] **Step 10: Full service verification**

```bash
cd /home/tumidanski/source/MyFleet/.worktrees/task-002-k3s-cluster-deployment
go build ./apps/media-service/... && echo "BUILD OK"
go vet ./apps/media-service/... && echo "VET OK"
go test -race ./apps/media-service/... && echo "TESTS OK"
```

Expected: all three OK. `apps/media-service/internal/storage/minio_test.go` may reference the deleted presign methods — remove those cases; they are testing code that no longer exists.

- [ ] **Step 11: Commit**

```bash
cd /home/tumidanski/source/MyFleet/.worktrees/task-002-k3s-cluster-deployment
git add apps/media-service
git commit -m "feat(media): proxy download bytes, delete presign plumbing

Replaces GET /media/{id}/download (presigned GET URL) with
GET /media/{id}/content, which streams the object through the service after
fleet authorization. Removes PresignPut/PresignGet from the storage client and
both URL fields from the API attributes: no code path hands a MinIO URL to a
browser any more."
git rev-parse --show-toplevel
git branch --show-current
```

---

## Task 5: Add `requestBlob` to the shared ApiClient

**Files:**
- Modify: `packages/shared-ts/src/apiClient.ts`
- Test: `packages/shared-ts/src/apiClient.test.ts` (create)

**Interfaces:**
- Consumes: existing `ApiClient` with `opts.getAccessToken` and `opts.onRefresh`.
- Produces: `requestBlob(path: string, init?: RequestInit, retried?: boolean): Promise<Blob>` — sends the bearer token, retries once through `onRefresh` on 401, throws via `createErrorFromUnknown` on non-OK, and does **not** set a JSON `Content-Type`.

- [ ] **Step 1: Write the failing test**

`request` always calls `res.json()`, so it cannot carry binary bodies. Create `packages/shared-ts/src/apiClient.test.ts`:

```ts
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { ApiClient } from './apiClient';

describe('ApiClient.requestBlob', () => {
  beforeEach(() => {
    vi.restoreAllMocks();
  });

  it('sends the bearer token and returns the blob', async () => {
    const blob = new Blob(['bytes'], { type: 'image/jpeg' });
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      blob: async () => blob,
    });
    vi.stubGlobal('fetch', fetchMock);

    const client = new ApiClient({
      baseUrl: '',
      getAccessToken: () => 'tok-123',
      onRefresh: async () => null,
    });

    const out = await client.requestBlob('/api/media/m1/content');

    expect(out).toBe(blob);
    const headers = fetchMock.mock.calls[0][1].headers as Record<string, string>;
    expect(headers.Authorization).toBe('Bearer tok-123');
    // A binary GET must not claim a JSON:API content type.
    expect(headers['Content-Type']).toBeUndefined();
  });

  it('refreshes once on 401 and retries', async () => {
    const blob = new Blob(['bytes']);
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce({ ok: false, status: 401, blob: async () => blob })
      .mockResolvedValueOnce({ ok: true, status: 200, blob: async () => blob });
    vi.stubGlobal('fetch', fetchMock);

    const onRefresh = vi.fn().mockResolvedValue('tok-new');
    const client = new ApiClient({
      baseUrl: '',
      getAccessToken: () => 'tok-old',
      onRefresh,
    });

    const out = await client.requestBlob('/api/media/m1/content');

    expect(out).toBe(blob);
    expect(onRefresh).toHaveBeenCalledTimes(1);
    expect(fetchMock).toHaveBeenCalledTimes(2);
  });

  it('throws on a non-OK response', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue({
        ok: false,
        status: 404,
        json: async () => ({ errors: [{ detail: 'not found' }] }),
        blob: async () => new Blob([]),
      }),
    );

    const client = new ApiClient({
      baseUrl: '',
      getAccessToken: () => 'tok-123',
      onRefresh: async () => null,
    });

    await expect(client.requestBlob('/api/media/missing/content')).rejects.toBeDefined();
  });
});
```

- [ ] **Step 2: Run the test to verify it fails**

```bash
cd /home/tumidanski/source/MyFleet/.worktrees/task-002-k3s-cluster-deployment
export NVM_DIR="$HOME/.nvm" && . "$NVM_DIR/nvm.sh" && nvm use 22
npm ci
npm run -w @myfleet/shared-ts test 2>/dev/null || npx vitest run packages/shared-ts/src/apiClient.test.ts
```

Expected: FAIL — `client.requestBlob is not a function`.

If `@myfleet/shared-ts` has no `test` script, run the file through the repo's vitest as shown by the fallback, and use that same command in Step 4.

- [ ] **Step 3: Implement `requestBlob`**

Add to the `ApiClient` class in `packages/shared-ts/src/apiClient.ts`, after `request`:

```ts
  /**
   * Authenticated binary GET. Shares request's bearer-token and one-shot
   * 401-refresh behaviour, but returns the raw Blob instead of parsed JSON and
   * sets no Content-Type — used for media bytes proxied through the API.
   */
  async requestBlob(path: string, init: RequestInit = {}, retried = false): Promise<Blob> {
    const token = this.opts.getAccessToken();
    const res = await fetch(this.opts.baseUrl + path, {
      ...init,
      headers: {
        ...(token ? { Authorization: `Bearer ${token}` } : {}),
        ...(init.headers ?? {}),
      },
    });
    if (res.status === 401 && !retried) {
      const refreshed = await this.opts.onRefresh();
      if (refreshed) return this.requestBlob(path, init, true);
    }
    if (!res.ok) {
      const body = await res.json().catch(() => null);
      throw createErrorFromUnknown({ status: res.status, body });
    }
    return res.blob();
  }
```

- [ ] **Step 4: Run the test to verify it passes**

```bash
cd /home/tumidanski/source/MyFleet/.worktrees/task-002-k3s-cluster-deployment
export NVM_DIR="$HOME/.nvm" && . "$NVM_DIR/nvm.sh" && nvm use 22
npm run -w @myfleet/shared-ts test 2>/dev/null || npx vitest run packages/shared-ts/src/apiClient.test.ts
```

Expected: 3 tests PASS.

- [ ] **Step 5: Commit**

```bash
cd /home/tumidanski/source/MyFleet/.worktrees/task-002-k3s-cluster-deployment
git add packages/shared-ts/src/apiClient.ts packages/shared-ts/src/apiClient.test.ts
git commit -m "feat(shared-ts): add ApiClient.requestBlob for authenticated binary GETs

request() always parses JSON, so it cannot fetch media bytes. requestBlob
reuses the bearer-token and 401-refresh path but returns the raw Blob and
sends no Content-Type."
git rev-parse --show-toplevel
git branch --show-current
```

---

## Task 6: Point the web client at the proxied media endpoints

**Files:**
- Modify: `apps/web/src/services/api/MediaService.ts`
- Modify: `apps/web/src/lib/hooks/api/media.ts`
- Modify: `apps/web/src/components/features/vehicles/media/MediaThumbnail.tsx`
- Modify: `apps/web/src/components/features/vehicles/media/MediaUploadButton.tsx:14-15` (comment)
- Modify: `apps/web/src/components/features/vehicles/media/VehicleMediaGallery.tsx:22` (comment)
- Modify: `apps/web/src/types/models/media.ts`
- Test: `apps/web/src/lib/hooks/api/media.test.ts`

**Interfaces:**
- Consumes: Task 5's `apiClient.requestBlob`; Task 3's `PUT /api/media/{id}/content`; Task 4's `GET /api/media/{id}/content`.
- Produces: `mediaService.putContent(id: string, file: File): Promise<JsonApiResource<MediaObjectAttributes>>`; `mediaService.getContentBlob(id: string): Promise<Blob>`; `performMediaUpload(file, deps)` with `UploadDeps = { initUpload, putContent, confirm }`; hook `useMediaContentUrl(id)` returning `{ url: string | null; isLoading: boolean }`; `mediaKeys.content(id)`.

- [ ] **Step 1: Update the failing test to the proxied flow**

Rewrite the upload-sequence cases in `apps/web/src/lib/hooks/api/media.test.ts`. The flow is now init → putContent → confirm, and there is no `uploadUrl` to check:

```ts
  it('calls init → putContent → confirm in order', async () => {
    const calls: string[] = [];

    const mockInit = vi.fn().mockImplementation(async () => {
      calls.push('init');
      return {
        id: 'm1',
        type: 'media-objects',
        attributes: {
          fleetId: 'f1',
          uploadedByUserId: 'u1',
          bucket: 'myfleet-media',
          objectKey: 'f1/m1/photo.jpg',
          status: 'uploaded',
        },
      };
    });

    const mockPut = vi.fn().mockImplementation(async () => {
      calls.push('put-content');
      return {
        id: 'm1',
        type: 'media-objects',
        attributes: { status: 'uploaded' },
      };
    });

    const mockConfirm = vi.fn().mockImplementation(async () => {
      calls.push('confirm');
      return {
        id: 'm1',
        type: 'media-objects',
        attributes: { status: 'processing' },
      };
    });

    const fakeFile = new File(['bytes'], 'photo.jpg', { type: 'image/jpeg' });

    const result = await performMediaUpload(fakeFile, {
      initUpload: mockInit,
      putContent: mockPut,
      confirm: mockConfirm,
    });

    expect(calls).toEqual(['init', 'put-content', 'confirm']);
    // The bytes go to the media row's own id — never to an external URL.
    expect(mockPut).toHaveBeenCalledWith('m1', fakeFile);
    expect(result.attributes.status).toBe('processing');
  });
```

Delete the `throws if init returns no uploadUrl` case outright — `uploadUrl` no longer exists, and the id is always present on a created resource. Keep every other test in the file as-is.

- [ ] **Step 2: Run the test to verify it fails**

```bash
cd /home/tumidanski/source/MyFleet/.worktrees/task-002-k3s-cluster-deployment
export NVM_DIR="$HOME/.nvm" && . "$NVM_DIR/nvm.sh" && nvm use 22
npm run -w apps/web test
```

Expected: FAIL — `performMediaUpload` still calls `deps.putToPresignedUrl`, which the new deps object does not provide.

- [ ] **Step 3: Replace the presigned methods on `MediaService`**

In `apps/web/src/services/api/MediaService.ts` update the class doc comment and swap the two methods. Replace the header comment's route list with:

```ts
/**
 * Media service — wraps the media-service endpoints (gateway prefix /api/media).
 * Backend routes (apps/media-service/internal/mediaobject/resource.go):
 *   POST   /api/media               — init upload: creates the row (uploaded)
 *   PUT    /api/media/{id}/content  — upload the raw bytes (proxied to MinIO)
 *   POST   /api/media/{id}/confirm  — mark uploaded→processing
 *   GET    /api/media/{id}          — get metadata
 *   GET    /api/media/{id}/content  — stream the bytes (proxied from MinIO)
 *   DELETE /api/media/{id}          — soft delete
 *
 * Bytes are proxied through media-service, not presigned: MinIO is a shared
 * cluster service and is never reachable from the browser.
 */
```

Delete `putToPresignedUrl` and `getDownloadUrl`, and add:

```ts
  /**
   * PUT /api/media/{id}/content — upload the raw bytes through the API. Goes
   * via apiClient so the bearer token and 401-refresh apply; the Content-Type
   * override replaces the default JSON:API media type.
   */
  async putContent(id: string, file: File): Promise<JsonApiResource<MediaObjectAttributes>> {
    const doc = await apiClient.request<JsonApiDocument<JsonApiResource<MediaObjectAttributes>>>(
      `${this.basePath}/${id}/content`,
      {
        method: 'PUT',
        body: file,
        headers: { 'Content-Type': file.type || 'application/octet-stream' },
      },
    );
    return doc.data;
  }

  /** GET /api/media/{id}/content — the raw bytes, authenticated. */
  async getContentBlob(id: string): Promise<Blob> {
    return apiClient.requestBlob(`${this.basePath}/${id}/content`);
  }
```

- [ ] **Step 4: Rework the upload orchestration and the content hook**

In `apps/web/src/lib/hooks/api/media.ts`:

Replace the `download` keys with `content`:

```ts
  contents: () => [...mediaKeys.all, 'content'] as const,
  content: (id: string) => [...mediaKeys.contents(), id] as const,
```

Replace `UploadDeps` and `performMediaUpload`:

```ts
export interface UploadDeps {
  initUpload: (attrs: InitMediaUploadAttributes) => Promise<JsonApiResource<MediaObjectAttributes>>;
  putContent: (id: string, file: File) => Promise<JsonApiResource<MediaObjectAttributes>>;
  confirm: (id: string) => Promise<JsonApiResource<MediaObjectAttributes>>;
}

/**
 * Orchestrates the three-step upload sequence:
 *  1. Init — creates the media row in the uploaded state.
 *  2. PUT the bytes to /api/media/{id}/content (proxied to MinIO by the service).
 *  3. Confirm — transitions the row from uploaded → processing.
 *
 * After confirm, the caller should poll GET /media/{id} until status === 'ready'.
 */
export async function performMediaUpload(
  file: File,
  deps: UploadDeps,
): Promise<JsonApiResource<MediaObjectAttributes>> {
  const media = await deps.initUpload({
    contentType: file.type,
    originalFilename: file.name,
  });

  await deps.putContent(media.id, file);

  return deps.confirm(media.id);
}
```

Replace `useMediaDownloadUrl` with a hook that turns the blob into an object URL and revokes it on change/unmount:

```ts
/**
 * GET /api/media/{id}/content — fetches the bytes and exposes them as an object
 * URL suitable for an <img src>. A plain src cannot be used: the API needs an
 * Authorization header, which the browser does not send for image requests.
 */
export function useMediaContentUrl(id: string | null | undefined) {
  const { data, isLoading } = useQuery({
    queryKey: mediaKeys.content(id ?? ''),
    queryFn: () => mediaService.getContentBlob(id as string),
    enabled: !!id,
    staleTime: 5 * 60 * 1000,
    gcTime: 6 * 60 * 1000,
  });

  const [url, setUrl] = useState<string | null>(null);

  useEffect(() => {
    if (!data) {
      setUrl(null);
      return;
    }
    const objectUrl = URL.createObjectURL(data);
    setUrl(objectUrl);
    return () => URL.revokeObjectURL(objectUrl);
  }, [data]);

  return { url, isLoading };
}
```

Update `useUploadMedia`'s deps object:

```ts
      performMediaUpload(file, {
        initUpload: (attrs) => mediaService.initUpload(attrs),
        putContent: (id, f) => mediaService.putContent(id, f),
        confirm: (id) => mediaService.confirm(id),
      }),
```

Add `import { useEffect, useState } from 'react';` at the top of the file.

- [ ] **Step 5: Update `MediaThumbnail`**

Replace the body of `apps/web/src/components/features/vehicles/media/MediaThumbnail.tsx`:

```tsx
import { Skeleton } from '../../../ui/skeleton';
import { useMediaContentUrl, useMediaObject } from '../../../../lib/hooks/api/media';

interface MediaThumbnailProps {
  mediaId: string;
  isPrimary?: boolean;
  className?: string;
}

/**
 * Renders a media object's bytes, fetched through the API and held as an object
 * URL. Metadata comes from the separate detail query (both are cached by React
 * Query, so a gallery re-render costs no extra requests).
 */
export function MediaThumbnail({ mediaId, isPrimary, className }: MediaThumbnailProps) {
  const { url, isLoading } = useMediaContentUrl(mediaId);
  const { data: meta } = useMediaObject(mediaId);

  if (isLoading) {
    return <Skeleton className={className ?? 'h-24 w-24 rounded'} />;
  }

  if (!url) {
    return (
      <div
        className={`flex items-center justify-center rounded bg-muted text-xs text-muted-foreground ${className ?? 'h-24 w-24'}`}
      >
        No image
      </div>
    );
  }

  return (
    <div className="relative">
      <img
        src={url}
        alt={meta?.attributes.originalFilename ?? 'Vehicle photo'}
        className={`rounded object-cover ${className ?? 'h-24 w-24'}`}
      />
      {isPrimary && (
        <span className="absolute bottom-1 left-1 rounded bg-primary px-1 py-0.5 text-[10px] font-medium text-primary-foreground">
          Primary
        </span>
      )}
    </div>
  );
}
```

- [ ] **Step 6: Drop the URL fields from the shared type**

In `apps/web/src/types/models/media.ts` remove `uploadUrl?: string;` and `downloadUrl?: string;` from `MediaObjectAttributes`, and replace the doc comment above it:

```ts
/**
 * Mirrors apps/media-service/internal/mediaobject/rest.go Attributes.
 * Bytes are fetched from /api/media/{id}/content, not from a URL in the
 * payload — see MediaService.
 */
```

- [ ] **Step 7: Fix the two stale comments**

In `apps/web/src/components/features/vehicles/media/MediaUploadButton.tsx` replace lines 14-15 with:

```tsx
 *  1. POST /api/media (init) → media row
 *  2. PUT file bytes to /api/media/{id}/content (proxied to MinIO)
```

In `apps/web/src/components/features/vehicles/media/VehicleMediaGallery.tsx` line 22, replace `renders thumbnails via presigned GET URLs,` with `renders thumbnails via proxied content URLs,`.

- [ ] **Step 8: Verify nothing still references the removed API**

```bash
cd /home/tumidanski/source/MyFleet/.worktrees/task-002-k3s-cluster-deployment
grep -rn 'uploadUrl\|downloadUrl\|putToPresignedUrl\|useMediaDownloadUrl\|getDownloadUrl\|presigned' apps/web/src packages/shared-ts --include='*.ts' --include='*.tsx' || echo "clean"
```

Expected: `clean`.

- [ ] **Step 9: Run the frontend tests and build**

```bash
cd /home/tumidanski/source/MyFleet/.worktrees/task-002-k3s-cluster-deployment
export NVM_DIR="$HOME/.nvm" && . "$NVM_DIR/nvm.sh" && nvm use 22
npm run -w apps/web test && echo "FE TEST OK"
npm run -w apps/web build && echo "FE BUILD OK"
```

Expected: both OK. TypeScript will flag any leftover reference to the removed attributes — fix them at the call site rather than re-adding the fields.

- [ ] **Step 10: Commit**

```bash
cd /home/tumidanski/source/MyFleet/.worktrees/task-002-k3s-cluster-deployment
git add apps/web/src packages/shared-ts
git commit -m "feat(web): upload and render media through the API, not MinIO

Switches the upload sequence to PUT /api/media/{id}/content and thumbnails to
GET /api/media/{id}/content, held as object URLs because the browser will not
send an Authorization header for an <img src>. Drops uploadUrl/downloadUrl from
the media type."
git rev-parse --show-toplevel
git branch --show-current
```

---

## Task 7: Fix `GOOGLE_REDIRECT_URL` in both environments

**Files:**
- Modify: `deploy/k8s/base/auth-service/configmap.yaml:13`
- Modify: `deploy/compose/.env.example:19`

**Interfaces:**
- Consumes: nothing.
- Produces: the correct callback path `/api/auth/auth/callback`, which Task 8's main-overlay patch overrides with the public host.

- [ ] **Step 1: Confirm the two wrong values and the correct path**

```bash
cd /home/tumidanski/source/MyFleet/.worktrees/task-002-k3s-cluster-deployment
grep -n 'GOOGLE_REDIRECT_URL' deploy/k8s/base/auth-service/configmap.yaml deploy/compose/.env.example
grep -n 'auth/callback\|auth/login/google' apps/auth-service/internal/oidc/resource.go
```

Expected: the ConfigMap has `.../api/auth/auth/google/callback` (strips to `/auth/google/callback` — no such route) and `.env.example` has `.../api/auth/callback` (strips to `/callback` — no such route). The resource file registers `/auth/callback`. With `/api/auth` stripped by Traefik, the only correct public path is `/api/auth/auth/callback`.

- [ ] **Step 2: Fix the Kubernetes ConfigMap**

In `deploy/k8s/base/auth-service/configmap.yaml` set:

```yaml
  # Traefik strips /api/auth, and the OIDC callback route is /auth/callback
  # (apps/auth-service/internal/oidc/resource.go), so the public path is
  # /api/auth/auth/callback. The main overlay swaps in the public host.
  GOOGLE_REDIRECT_URL: "http://localhost/api/auth/auth/callback"
```

- [ ] **Step 3: Fix the compose env template**

In `deploy/compose/.env.example` line 19 set:

```
GOOGLE_REDIRECT_URL=http://localhost/api/auth/auth/callback
```

- [ ] **Step 4: Verify**

```bash
cd /home/tumidanski/source/MyFleet/.worktrees/task-002-k3s-cluster-deployment
grep -rn 'GOOGLE_REDIRECT_URL' deploy/ | grep -v 'docker-compose.yml'
```

Expected: both files now read `http://localhost/api/auth/auth/callback`.

- [ ] **Step 5: Commit**

```bash
cd /home/tumidanski/source/MyFleet/.worktrees/task-002-k3s-cluster-deployment
git add deploy/k8s/base/auth-service/configmap.yaml deploy/compose/.env.example
git commit -m "fix(auth): correct GOOGLE_REDIRECT_URL in both environments

Routes mount at the chi root and the OIDC callback is /auth/callback, so with
/api/auth stripped the only working public path is /api/auth/auth/callback.
The k8s ConfigMap pointed at /auth/google/callback and compose at /callback —
neither route exists. Both appear to have conflated the login path with the
callback."
git rev-parse --show-toplevel
git branch --show-current
```

---

## Task 8: Create `overlays/main` for bee

**Files:**
- Create: `deploy/k8s/overlays/main/kustomization.yaml`
- Create: `deploy/k8s/overlays/main/ingressroute.yaml`
- Create: `deploy/k8s/overlays/main/patches/auth-service-config.yaml`
- Create: `deploy/k8s/overlays/main/patches/fleet-service-config.yaml`
- Create: `deploy/k8s/overlays/main/patches/media-service-config.yaml`
- Create: `deploy/k8s/overlays/main/patches/notification-service-config.yaml`
- Create: `deploy/k8s/overlays/main/patches/pull-policy.yaml`

**Interfaces:**
- Consumes: Task 1's `base/` (apps only, Middlewares present), Task 2's `web` Deployment/Service, Task 3's `MEDIA_MAX_UPLOAD_BYTES` key, Task 7's corrected callback path.
- Produces: `deploy/k8s/overlays/main` — the Argo CD source path. Task 9's `bump-overlay` job rewrites the `newTag` values in its `kustomization.yaml`.

- [ ] **Step 1: Write the ConfigMap patches**

These are strategic-merge patches on the base ConfigMaps — only the listed keys change.

`deploy/k8s/overlays/main/patches/auth-service-config.yaml`:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: auth-service-config
data:
  APP_BASE_URL: "https://myfleet.tumidanski.com"
  JWT_ISSUER: "https://myfleet.tumidanski.com"
  GOOGLE_REDIRECT_URL: "https://myfleet.tumidanski.com/api/auth/auth/callback"
  # Required for the Cloudflare host. Note this makes myfleet.home unable to
  # hold a session: browsers do not send Secure cookies over plain HTTP.
  COOKIE_SECURE: "true"
```

`deploy/k8s/overlays/main/patches/fleet-service-config.yaml`:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: fleet-service-config
data:
  KAFKA_BROKERS: "kafka.home:9093"
```

`deploy/k8s/overlays/main/patches/media-service-config.yaml`:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: media-service-config
data:
  KAFKA_BROKERS: "kafka.home:9093"
  MINIO_ENDPOINT: "minio.minio.svc.cluster.local:9000"
```

`deploy/k8s/overlays/main/patches/notification-service-config.yaml`:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: notification-service-config
data:
  KAFKA_BROKERS: "kafka.home:9093"
```

`MEDIA_BUCKET`, `MINIO_USE_SSL`, `JWKS_URL`, `FLEET_SERVICE_URL` and `FLEET_INTERNAL_URL` are deliberately absent — the base values are already correct for bee (in-cluster DNS, plaintext to the in-cluster MinIO).

- [ ] **Step 2: Write the pull-policy patch**

`deploy/k8s/overlays/main/patches/pull-policy.yaml` — a JSON6902 patch body applied to all five Deployments by the kustomization:

```yaml
- op: add
  path: /spec/template/spec/containers/0/imagePullPolicy
  value: Always
```

- [ ] **Step 3: Write the IngressRoute**

`deploy/k8s/overlays/main/ingressroute.yaml`. An `IngressRoute` is used rather than a plain `Ingress` because each service needs a *different* `stripPrefix`, and the `router.middlewares` annotation applies to a whole Ingress object — a plain Ingress would need five separate objects.

```yaml
---
# Both public hosts share one route set. Priorities are explicit rather than
# relying on Traefik's rule-length ordering: the SPA catch-all must lose to
# every /api/* route. Entry point `web` is bee's k3s Traefik :80; TLS for the
# Cloudflare host terminates at the edge.
apiVersion: traefik.io/v1alpha1
kind: IngressRoute
metadata:
  name: myfleet-routes
spec:
  entryPoints:
    - web
  routes:
    - match: (Host(`myfleet.tumidanski.com`) || Host(`myfleet.home`)) && PathPrefix(`/api/auth`)
      kind: Rule
      priority: 100
      middlewares:
        - name: auth-stripprefix
      services:
        - name: auth-service
          port: 8080
    - match: (Host(`myfleet.tumidanski.com`) || Host(`myfleet.home`)) && PathPrefix(`/api/fleet`)
      kind: Rule
      priority: 100
      middlewares:
        - name: fleet-stripprefix
      services:
        - name: fleet-service
          port: 8080
    - match: (Host(`myfleet.tumidanski.com`) || Host(`myfleet.home`)) && PathPrefix(`/api/media`)
      kind: Rule
      priority: 100
      middlewares:
        - name: media-stripprefix
      services:
        - name: media-service
          port: 8080
    - match: (Host(`myfleet.tumidanski.com`) || Host(`myfleet.home`)) && PathPrefix(`/api/notifications`)
      kind: Rule
      priority: 100
      middlewares:
        - name: notifications-stripprefix
      services:
        - name: notification-service
          port: 8080
    - match: Host(`myfleet.tumidanski.com`) || Host(`myfleet.home`)
      kind: Rule
      # Catch-all for the SPA: must lose to every /api/* route above.
      priority: 1
      services:
        - name: web
          port: 80
```

- [ ] **Step 4: Write the kustomization**

`deploy/k8s/overlays/main/kustomization.yaml`:

```yaml
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization

namespace: myfleet

# The bee cluster overlay. Argo CD syncs this path (see argocd-myfleet.yml in
# the k3s/bee repo). It references base/ only — never infra-local/ — because
# Postgres, Kafka and MinIO are pre-existing shared cluster services and the
# dev Traefik would fight bee's cluster Traefik over every IngressRoute.
#
# No Secrets here: they are applied out-of-band so Argo CD's prune cannot
# remove them. See ../../secrets.example.yaml.
resources:
  - ../../base
  - ingressroute.yaml

replicas:
  - name: auth-service
    count: 1
  - name: fleet-service
    count: 1
  - name: media-service
    count: 1
  - name: notification-service
    count: 1
  - name: web
    count: 1

# newTag values are rewritten to the commit SHA by the bump-overlay CI job on
# every push to main, so `git log` is an accurate record of what is deployed.
# `latest` is only the seed for the first sync.
images:
  - name: ghcr.io/jtumidanski/myfleet-auth-service
    newTag: latest
  - name: ghcr.io/jtumidanski/myfleet-fleet-service
    newTag: latest
  - name: ghcr.io/jtumidanski/myfleet-media-service
    newTag: latest
  - name: ghcr.io/jtumidanski/myfleet-notification-service
    newTag: latest
  - name: ghcr.io/jtumidanski/myfleet-web
    newTag: latest

patches:
  - path: patches/auth-service-config.yaml
  - path: patches/fleet-service-config.yaml
  - path: patches/media-service-config.yaml
  - path: patches/notification-service-config.yaml
  - path: patches/pull-policy.yaml
    target:
      kind: Deployment
      name: auth-service
  - path: patches/pull-policy.yaml
    target:
      kind: Deployment
      name: fleet-service
  - path: patches/pull-policy.yaml
    target:
      kind: Deployment
      name: media-service
  - path: patches/pull-policy.yaml
    target:
      kind: Deployment
      name: notification-service
  - path: patches/pull-policy.yaml
    target:
      kind: Deployment
      name: web
```

- [ ] **Step 5: Verify the overlay renders and contains nothing it must not**

```bash
cd /home/tumidanski/source/MyFleet/.worktrees/task-002-k3s-cluster-deployment
kustomize build deploy/k8s/overlays/main > /tmp/main.yaml && echo "BUILD OK"
echo "--- must all be 0 ---"
grep -c 'kind: PersistentVolumeClaim' /tmp/main.yaml || true
grep -c 'kind: Secret'                /tmp/main.yaml || true
grep -c 'kind: ClusterRole'           /tmp/main.yaml || true
grep -c 'kind: ServiceAccount'        /tmp/main.yaml || true
grep -ci 'placeholder\|REPLACE_ME'    /tmp/main.yaml || true
grep -c 'redpanda\|postgres:5432\|minio:9000' /tmp/main.yaml || true
echo "--- expected values ---"
grep -c 'imagePullPolicy: Always' /tmp/main.yaml   # 5
grep -c 'kind: Middleware'        /tmp/main.yaml   # 4
grep -c 'kind: Deployment'        /tmp/main.yaml   # 5
grep -n 'kafka.home:9093\|minio.minio.svc.cluster.local:9000\|COOKIE_SECURE\|GOOGLE_REDIRECT_URL' /tmp/main.yaml
```

Expected: the six "must be 0" counts are all `0`; `imagePullPolicy: Always` is `5`, Middleware `4`, Deployment `5`; `KAFKA_BROKERS` is `kafka.home:9093` in three ConfigMaps, `MINIO_ENDPOINT` is the in-cluster MinIO, `COOKIE_SECURE` is `"true"`, and `GOOGLE_REDIRECT_URL` is `https://myfleet.tumidanski.com/api/auth/auth/callback`.

- [ ] **Step 6: Server-side dry-run against bee**

The bee cluster is reachable from this environment (`kubectl config current-context` → `bee`) and has the Traefik CRDs, so the rendered manifests can be validated for real.

```bash
cd /home/tumidanski/source/MyFleet/.worktrees/task-002-k3s-cluster-deployment
kubectl config current-context   # must print: bee
kubectl create namespace myfleet --dry-run=client -o yaml | kubectl apply --dry-run=server -f -
kustomize build deploy/k8s/overlays/main | kubectl apply --dry-run=server -f -
```

Expected: every resource reports `(server dry run)` with no errors. This validates the IngressRoute and Middleware CRs against the real CRD schemas.

If the namespace does not exist yet the dry-run may reject namespaced resources. In that case create it for real — Argo CD's `CreateNamespace=true` would create it anyway — then re-run:

```bash
kubectl create namespace myfleet
kustomize build deploy/k8s/overlays/main | kubectl apply --dry-run=server -f -
```

Do **not** apply the manifests for real. This task ends at dry-run.

- [ ] **Step 7: Verify the local overlay is still intact**

```bash
cd /home/tumidanski/source/MyFleet/.worktrees/task-002-k3s-cluster-deployment
kustomize build deploy/k8s/overlays/local > /dev/null && echo "LOCAL STILL OK"
```

Expected: `LOCAL STILL OK`.

- [ ] **Step 8: Commit**

```bash
cd /home/tumidanski/source/MyFleet/.worktrees/task-002-k3s-cluster-deployment
git add deploy/k8s/overlays/main
git commit -m "feat(k8s): add overlays/main targeting the bee cluster

References base/ only — shared postgres.home, kafka.home and the cluster MinIO
replace the bundled infra, and the dev Traefik is excluded so it cannot fight
bee's cluster Traefik. One IngressRoute serves both myfleet.tumidanski.com and
myfleet.home with explicit priorities so the SPA catch-all loses to every
/api/* route. No PVCs, no Secrets, imagePullPolicy: Always."
git rev-parse --show-toplevel
git branch --show-current
```

---

## Task 9: CI — publish the web image and bump the overlay SHAs

**Files:**
- Modify: `.github/workflows/main.yml:3-5` (trigger), `:52-54` and `:88-89` (matrices), and append a new job

**Interfaces:**
- Consumes: Task 2's `apps/web/Dockerfile`; Task 8's `deploy/k8s/overlays/main/kustomization.yaml` with five `newTag` entries.
- Produces: image `ghcr.io/jtumidanski/myfleet-web`; a `bump-overlay` job that rewrites all five `newTag` values to `${{ github.sha }}` and pushes to `main`.

- [ ] **Step 1: Add `paths-ignore` to the push trigger**

The bump job pushes to `main`, so the workflow must not retrigger itself. Two independent guards are used because either alone is fragile. In `.github/workflows/main.yml` replace the `on:` block:

```yaml
on:
  push:
    branches: [main]
    # Guard 1 of 2 against a self-retrigger loop: the bump-overlay job below
    # pushes a commit that touches only this path. Guard 2 is [skip ci] in that
    # commit's message.
    paths-ignore:
      - 'deploy/k8s/overlays/main/**'
```

- [ ] **Step 2: Add `web` to the publish and trivy matrices**

The web Dockerfile takes the repo root as its context and lives at `apps/web/Dockerfile`, exactly matching the existing `apps/${{ matrix.service }}/Dockerfile` pattern, so adding `web` to the matrix needs no other change.

In the `publish` job change the matrix to:

```yaml
        service: [auth-service, fleet-service, media-service, notification-service, web]
```

In the `trivy` job change its matrix identically:

```yaml
        service: [auth-service, fleet-service, media-service, notification-service, web]
```

- [ ] **Step 3: Add the `bump-overlay` job**

Append to the end of `.github/workflows/main.yml`:

```yaml
  # ── Bump the deployed image SHAs ──────────────────────────────────────────────
  bump-overlay:
    needs: publish
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
        with:
          ref: main

      - name: Pin overlay images to this commit
        run: |
          set -euo pipefail
          f=deploy/k8s/overlays/main/kustomization.yaml
          # Rewrite every `newTag:` in the images block to this commit's SHA.
          # Argo CD then sees a real manifest diff and rolls the Deployments.
          sed -i -E "s/^(    newTag: ).*$/\1${{ github.sha }}/" "$f"
          # `if` guards the check so a clean diff does not trip `set -e`.
          if git diff --quiet "$f"; then
            echo "already pinned to this SHA — nothing to bump"
            exit 0
          fi
          # Fail loudly if the rewrite missed an image rather than deploying a
          # partially-pinned overlay.
          test "$(grep -c "    newTag: ${{ github.sha }}" "$f")" -eq 5

          git config user.name  "github-actions[bot]"
          git config user.email "github-actions[bot]@users.noreply.github.com"
          git add "$f"
          # [skip ci] is guard 2 of 2 against retriggering this workflow;
          # paths-ignore on the push trigger is guard 1.
          git commit -m "chore(deploy): pin main overlay to ${{ github.sha }} [skip ci]"
          git push origin main
```

The existing `permissions: contents: write` at the top of the file already covers this push.

- [ ] **Step 4: Validate the workflow YAML**

```bash
cd /home/tumidanski/source/MyFleet/.worktrees/task-002-k3s-cluster-deployment
python3 -c "import yaml,sys; d=yaml.safe_load(open('.github/workflows/main.yml')); print('jobs:', list(d['jobs'])); print('publish matrix:', d['jobs']['publish']['strategy']['matrix']['service']); print('trivy matrix:', d['jobs']['trivy']['strategy']['matrix']['service']); print('paths-ignore:', d[True]['push']['paths-ignore'])"
```

Expected: `jobs` includes `bump-overlay`; both matrices list five services ending in `web`; `paths-ignore` is `['deploy/k8s/overlays/main/**']`.

(`d[True]` is not a typo — PyYAML parses the bare key `on` as the boolean `True`.)

- [ ] **Step 5: Dry-run the sed rewrite locally**

Prove the rewrite hits exactly the five image tags and nothing else:

```bash
cd /home/tumidanski/source/MyFleet/.worktrees/task-002-k3s-cluster-deployment
cp deploy/k8s/overlays/main/kustomization.yaml /tmp/kust-orig.yaml
SHA=0123456789abcdef0123456789abcdef01234567
sed -E "s/^(    newTag: ).*$/\1${SHA}/" /tmp/kust-orig.yaml > /tmp/kust-bumped.yaml
grep -c "    newTag: ${SHA}" /tmp/kust-bumped.yaml
diff /tmp/kust-orig.yaml /tmp/kust-bumped.yaml | grep -c '^[<>]'
kustomize build deploy/k8s/overlays/main >/dev/null && echo "unchanged file still builds"
```

Expected: the count is `5`, the diff touches `10` lines (5 removed + 5 added), and the untouched overlay still builds. If the count is not 5, the indentation in the kustomization does not match the sed pattern — fix the pattern, not the manifest.

- [ ] **Step 6: Commit**

```bash
cd /home/tumidanski/source/MyFleet/.worktrees/task-002-k3s-cluster-deployment
git add .github/workflows/main.yml
git commit -m "ci: publish myfleet-web and bump the main overlay image SHAs

Adds web to the publish and trivy matrices, and a bump-overlay job that pins
the five newTag values to the built commit so Argo CD sees a real manifest diff
and git log records what is deployed. Loop prevention uses both [skip ci] in
the bump commit and paths-ignore on the push trigger — either alone is fragile."
git rev-parse --show-toplevel
git branch --show-current
```

---

## Task 10: Deployment runbook and CLAUDE.md verification commands

**Files:**
- Create: `docs/runbooks/k3s-deployment.md`
- Modify: `CLAUDE.md` ("Build & Verification" section)

**Interfaces:**
- Consumes: everything above — the runbook documents the manual bootstrap that the manifests deliberately do not automate.
- Produces: the operator-facing procedure and the repo's canonical verification command list.

- [ ] **Step 1: Write the runbook**

Create `docs/runbooks/k3s-deployment.md`:

```markdown
# Runbook — deploying MyFleet to the bee k3s cluster

MyFleet runs in namespace `myfleet` on bee, served by the cluster's k3s Traefik
at `192.168.23.230` on two hostnames:

- `https://myfleet.tumidanski.com` — Cloudflare-proxied, TLS terminates at the edge
- `http://myfleet.home` — LAN, plain HTTP

Nothing stateful is deployed. Postgres, Kafka and MinIO are pre-existing shared
cluster services.

Argo CD syncs `deploy/k8s/overlays/main` from `main`. The steps below are the
one-time bootstrap; none of it is automated, by design — a PreSync hook would
put credential handling in the manifests.

> **Ordering:** if Argo CD syncs before step 1, all four Go services
> `CrashLoopBackOff` on a missing schema. That is expected and self-resolves
> once the DDL below runs. No manifest change is needed.

## 1. Postgres — role, database, schemas

GORM `AutoMigrate` creates tables inside these schemas on first start, but it
does **not** create the schemas. This step is a hard prerequisite.

Pick a password and keep it for step 3.

```sh
POD=$(kubectl -n postgres get pods -l app=postgres -o jsonpath='{.items[0].metadata.name}')

kubectl -n postgres exec "$POD" -- psql -U postgres <<'SQL'
CREATE ROLE myfleet LOGIN PASSWORD '<password>';
CREATE DATABASE myfleet OWNER myfleet;
SQL

kubectl -n postgres exec "$POD" -- psql -U postgres -d myfleet <<'SQL'
CREATE SCHEMA IF NOT EXISTS auth         AUTHORIZATION myfleet;
CREATE SCHEMA IF NOT EXISTS fleet        AUTHORIZATION myfleet;
CREATE SCHEMA IF NOT EXISTS media        AUTHORIZATION myfleet;
CREATE SCHEMA IF NOT EXISTS notification AUTHORIZATION myfleet;
SQL
```

Verify:

```sh
kubectl -n postgres exec "$POD" -- psql -U postgres -d myfleet -c '\dn'
```

Expected: the four schemas, all owned by `myfleet`.

## 2. MinIO — bucket and a scoped user

The credential must not be able to reach any `atlas-*` bucket, so it gets a
policy scoped to `myfleet-media` alone rather than the root user.

Write the policy:

```sh
cat > /tmp/myfleet-media-policy.json <<'JSON'
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": ["s3:*"],
      "Resource": [
        "arn:aws:s3:::myfleet-media",
        "arn:aws:s3:::myfleet-media/*"
      ]
    }
  ]
}
JSON
```

Then create the bucket and user:

```sh
kubectl -n minio port-forward svc/minio 9000:9000 &
mc alias set bee http://localhost:9000 <root-user> <root-pass>
mc mb bee/myfleet-media
mc admin user add bee myfleet <minio-secret>
mc admin policy create bee myfleet-media-rw /tmp/myfleet-media-policy.json
mc admin policy attach bee myfleet-media-rw --user myfleet
```

Verify the scoping — the first must succeed, the second must be denied:

```sh
mc alias set beemyfleet http://localhost:9000 myfleet <minio-secret>
mc ls beemyfleet/myfleet-media
mc ls beemyfleet/atlas-assets   # must fail with AccessDenied
```

Media bytes are proxied through media-service, so MinIO is never reachable from
a browser and this credential never leaves the cluster.

## 3. Kubernetes Secrets

The main overlay ships no Secrets — they are applied out-of-band so Argo CD does
not manage them and `prune: true` cannot remove them.

Copy the template, fill it in, apply it, and keep the copy out of git:

```sh
cp deploy/k8s/secrets.example.yaml /tmp/myfleet-secrets.yaml
# edit /tmp/myfleet-secrets.yaml — replace every REPLACE_ME
kubectl create namespace myfleet --dry-run=client -o yaml | kubectl apply -f -
kubectl apply -n myfleet -f /tmp/myfleet-secrets.yaml
shred -u /tmp/myfleet-secrets.yaml
```

| Secret | Keys |
|---|---|
| `auth-service-secret` | `DATABASE_URL`, `GOOGLE_CLIENT_ID`, `GOOGLE_CLIENT_SECRET`, `JWT_PRIVATE_KEY_PEM`, `OIDC_STATE_SECRET` |
| `fleet-service-secret` | `DATABASE_URL` |
| `media-service-secret` | `DATABASE_URL`, `MINIO_ACCESS_KEY`, `MINIO_SECRET_KEY` |
| `notification-service-secret` | `DATABASE_URL` |

Generate the two auth-service values with:

```sh
openssl genrsa 2048        # JWT_PRIVATE_KEY_PEM
openssl rand -hex 32       # OIDC_STATE_SECRET
```

## 4. DNS

- **Cloudflare:** an `A`/`CNAME` record for `myfleet.tumidanski.com`, proxied,
  pointing at the same origin as `tumidanski.com`.
- **Pi-hole:** a local record `myfleet.home` → `192.168.23.230`, on **both**
  Pi-hole servers.

## 5. Google Cloud Console

Register this as an authorised redirect URI on the OAuth client:

```
https://myfleet.tumidanski.com/api/auth/auth/callback
```

This is external and cannot be automated or verified from the repo. Login stays
broken until it is done.

The path looks doubled but is correct: Traefik strips `/api/auth`, and the OIDC
callback route is `/auth/callback`.

## 6. Argo CD

Add `argocd-myfleet.yml` to the `~/source/k3s/bee` repo — a separate repo and a
separate commit from the MyFleet PR:

```yaml
apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: myfleet-main
  namespace: argocd
spec:
  project: default
  source:
    repoURL: https://github.com/jtumidanski/MyFleet.git
    targetRevision: main
    path: deploy/k8s/overlays/main
  destination:
    server: https://kubernetes.default.svc
    namespace: myfleet
  syncPolicy:
    automated:
      selfHeal: true
      prune: true
    syncOptions:
      - ServerSideApply=true
      - CreateNamespace=true
```

The MyFleet repository is public, so no `repo-creds` Secret is needed.

`prune: true` is on from the start. Atlas deferred it because Kustomize-hashed
ConfigMaps left orphans; this overlay generates no hashed resources. Anything
that must survive a manifest drop needs
`argocd.argoproj.io/sync-options: Prune=false`.

Apply it:

```sh
kubectl apply -f argocd-myfleet.yml
```

## 7. Post-deploy verification

```sh
kubectl -n myfleet get deploy
kubectl -n myfleet logs deploy/auth-service | grep -i migrat
curl -H 'Host: myfleet.home' http://192.168.23.230/api/fleet/healthz
curl -H 'Host: myfleet.home' http://192.168.23.230/ -o /dev/null -w '%{http_code}\n'
curl -H 'Host: myfleet.home' http://192.168.23.230/vehicles -o /dev/null -w '%{http_code}\n'
```

Expected: all five Deployments `Available` with no `ImagePullBackOff`;
AutoMigrate completes against the `auth` schema; `/api/fleet/healthz` returns
200; `/` and the deep link `/vehicles` both return 200 — the deep link proves
the catch-all priority is right.

Then, in a browser on `https://myfleet.tumidanski.com`:

- complete a full Google login round-trip and confirm a session cookie is set
- upload a vehicle photo and confirm the object lands in `myfleet-media`
- confirm a commit to `main` bumps the overlay SHA and Argo CD rolls the
  affected Deployments with no manual intervention

## Known constraints

**`myfleet.home` cannot hold a session.** `COOKIE_SECURE=true` is required for
the Cloudflare host, and browsers do not send `Secure` cookies over plain HTTP.
The LAN host is useful for `/healthz`, `/readyz` and confirming the stack is up
without depending on Cloudflare, but not for using the app. Sessions are
per-host anyway. Fixing this properly means a real certificate on
`myfleet.home`, not a config change.

**Cloudflare caps request bodies.** Media uploads are proxied through
media-service, so they traverse Cloudflare. `MEDIA_MAX_UPLOAD_BYTES` defaults to
25 MiB, under Cloudflare's free-plan ceiling; a client hitting the service limit
gets a `413`, and one hitting the edge limit is rejected before Traefik.

**No `myfleet` Postgres backup story.** The database is created by hand on a
shared Postgres whose backup policy is out of scope here. Worth a follow-up.

**Argo CD `selfHeal` reverts manual edits.** Anything changed with
`kubectl edit` in the `myfleet` namespace is undone on the next sync.
Out-of-band Secrets are unaffected, being untracked.

**A second MyFleet environment on this broker would cross-talk.** Kafka topics
are a flat namespace and the consumer group is fixed at `notification`, so two
environments would share both. Isolation means making those constants
configurable — a code change, not a manifest change.
```

- [ ] **Step 2: Update CLAUDE.md's Build & Verification section**

Replace the placeholder paragraph in `CLAUDE.md` under `## Build & Verification` with:

```markdown
Run before claiming a branch is done:

```sh
make ci        # lint-check, vet, test, build, fe-test, fe-build
```

Individually: `make vet`, `make test`, `make build` (Go); `make fe-test`,
`make fe-build` (web). `make lint-check` is the check-only lint CI runs; `make
lint` fixes what it can.

Node is not always on `PATH` — if `npm` is missing, load it first:

```sh
export NVM_DIR="$HOME/.nvm" && . "$NVM_DIR/nvm.sh" && nvm use 22
```

Deployment manifests (`deploy/k8s`) have no test suite, so render them:

```sh
kustomize build deploy/k8s/overlays/local   # dev: bundled infra + dev Traefik
kustomize build deploy/k8s/overlays/main    # bee: shared infra, no PVCs, no Secrets
```

The `main` overlay must render with no PersistentVolumeClaims, no Secrets, no
ClusterRole and no placeholder values. Against a reachable cluster, also run:

```sh
kustomize build deploy/k8s/overlays/main | kubectl apply --dry-run=server -f -
```

Container builds: `docker build -f apps/<service>/Dockerfile .` (context is the
repo root for every service, including `apps/web`).
```

- [ ] **Step 3: Verify the runbook's claims against the repo**

Documentation drifts silently, so check the concrete values rather than trusting the prose:

```bash
cd /home/tumidanski/source/MyFleet/.worktrees/task-002-k3s-cluster-deployment
grep -n 'myfleet.tumidanski.com/api/auth/auth/callback' docs/runbooks/k3s-deployment.md deploy/k8s/overlays/main/patches/auth-service-config.yaml
grep -n 'MEDIA_MAX_UPLOAD_BYTES' deploy/k8s/base/media-service/configmap.yaml apps/media-service/cmd/main.go
grep -n 'path: deploy/k8s/overlays/main' docs/runbooks/k3s-deployment.md
ls deploy/k8s/secrets.example.yaml
```

Expected: the callback URL in the runbook matches the overlay patch exactly; `MEDIA_MAX_UPLOAD_BYTES` appears in both the ConfigMap and the service; the Argo CD path matches the real overlay directory; the secrets template exists.

- [ ] **Step 4: Run the full verification suite**

```bash
cd /home/tumidanski/source/MyFleet/.worktrees/task-002-k3s-cluster-deployment
export NVM_DIR="$HOME/.nvm" && . "$NVM_DIR/nvm.sh" && nvm use 22
make ci
```

Expected: `lint-check`, `vet`, `test`, `build`, `fe-test`, `fe-build` all clean. Fix anything that fails before committing — this is the gate for the whole branch.

- [ ] **Step 5: Final manifest verification**

```bash
cd /home/tumidanski/source/MyFleet/.worktrees/task-002-k3s-cluster-deployment
kustomize build deploy/k8s/overlays/local > /dev/null && echo "LOCAL OK"
kustomize build deploy/k8s/overlays/main  > /dev/null && echo "MAIN OK"
kustomize build deploy/k8s/overlays/main | kubectl apply --dry-run=server -f - >/dev/null && echo "DRY-RUN OK"
```

Expected: all three OK.

- [ ] **Step 6: Commit**

```bash
cd /home/tumidanski/source/MyFleet/.worktrees/task-002-k3s-cluster-deployment
git add docs/runbooks/k3s-deployment.md CLAUDE.md
git commit -m "docs: add k3s deployment runbook and real verification commands

Documents the one-time bootstrap the manifests deliberately do not automate:
Postgres role/database/schemas, the scoped MinIO user, the four out-of-band
Secrets, both DNS records, the Google redirect URI, and the Argo CD Application
for the bee repo. Records the known constraints, including that myfleet.home
cannot hold a session under COOKIE_SECURE=true."
git rev-parse --show-toplevel
git branch --show-current
```

---

## Out of Scope

Carried from design.md §1 and §7, plus what the amendment adds:

- Ephemeral per-PR environments (atlas's `ApplicationSet`/`overlays/pr` model).
- Per-environment Kafka topic or consumer-group isolation.
- Observability wiring (Prometheus scrape configs, Tempo tracing, Loki labels).
- Migrating `deploy/compose` off its bundled infra.
- Backup/restore policy for the `myfleet` Postgres database.
- Fixing bee's two-default-StorageClass misconfiguration.
- A certificate for `myfleet.home` (the only real fix for LAN sessions).
- Serving generated variants rather than originals from the thumbnail endpoint.

## Manual, external steps (cannot be done from this repo)

These belong to the operator, and the runbook is their reference: Cloudflare DNS
record, Pi-hole DNS record, Google Cloud Console redirect URI, Postgres DDL,
MinIO bucket/user/policy, the four Kubernetes Secrets, and committing
`argocd-myfleet.yml` to `~/source/k3s/bee`.
