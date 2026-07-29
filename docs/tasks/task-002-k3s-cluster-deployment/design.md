# task-002 — Deploy MyFleet to the bee k3s cluster

Version: v1
Status: Approved (design phase)
Created: 2026-07-29
Branch: `task-002-k3s-cluster-deployment`

---

## 1. Overview

MyFleet has a Kustomize tree at `deploy/k8s` today, but it is local-dev shaped and
cannot be applied to bee. It ships its own Postgres, MinIO, Redpanda and Traefik
inside the `myfleet` namespace, its only overlay retags every image to `:local`
with `imagePullPolicy: IfNotPresent`, and the web UI has no manifests at all.

This task produces a `main` overlay that runs MyFleet on bee against the cluster's
**shared** infrastructure, exposes it through bee's k3s Traefik on two hostnames,
and wires it to Argo CD for continuous deployment — matching the pattern already
established by `Chronicle20/atlas` and `home-hub`.

This task intentionally skips `/spec-task`. Requirements were settled directly with
the maintainer (CLAUDE.md's documented escape hatch for changes that do not warrant
a PRD).

### Goals

- MyFleet reachable at `https://myfleet.tumidanski.com` (Cloudflare-proxied) and
  `http://myfleet.home` (LAN).
- Four Go services plus the web UI running in namespace `myfleet`, backed by
  `postgres.home`, `kafka.home` and the shared MinIO.
- Argo CD auto-syncs `deploy/k8s/overlays/main` and rolls pods when CI publishes
  new images.
- Google OAuth login actually works.

### Non-goals

- Ephemeral per-PR environments (atlas's `ApplicationSet`/`overlays/pr` model). Single
  `main` env only.
- Per-environment Kafka topic or consumer-group isolation. See §4.4.
- Observability wiring (Prometheus scrape configs, Tempo tracing, Loki labels).
- Migrating `deploy/compose` off its bundled infra. Compose stays self-contained.
- Backup/restore policy for the `myfleet` Postgres database.

---

## 2. Settled decisions

| # | Decision |
|---|---|
| 1 | Use bee's **shared** infra: `postgres.home:5432`, `kafka.home:9093`, `minio.minio.svc.cluster.local:9000`. Drop bundled infra from the cluster overlay. |
| 2 | Add `deploy/k8s/overlays/main` with real GHCR tags and `imagePullPolicy: Always`. |
| 3 | Drop the bundled Traefik; expose via bee's cluster Traefik at `192.168.23.230`. |
| 4 | Set `storageClassName: longhorn` explicitly on any retained PVC. |
| 5 | Move placeholder Secrets out of the tracked base. |
| 6 | Add `argocd-myfleet.yml` to `~/source/k3s/bee`; add Pi-hole DNS for `myfleet.home`. |
| 7 | Kafka topics stay a **flat namespace** — no per-env prefixing. |
| 8 | Deploy the web UI too (CI image + manifests). |
| 9 | Postgres bootstrap is a **one-time manual** `psql` step, documented in a runbook. |
| 10 | Images are **SHA-pinned**, bumped into the overlay by a CI job. |
| 11 | Hosts: `myfleet.tumidanski.com` (Cloudflare, TLS at edge) **and** `myfleet.home` (LAN, HTTP). |
| 12 | MinIO access via a **scoped** `myfleet` user + policy limited to `myfleet-media`. |
| 13 | `argocd-myfleet.yml` is authored directly in the bee repo (two repos, two commits). |

### Facts verified against the live cluster

These were checked rather than assumed, and they shape the design:

- **GHCR needs no pull secret.** `myfleet-auth-service`, `-fleet-service`,
  `-media-service`, `-notification-service` all serve anonymous pulls (HTTP 200 with
  an anonymous token), same as atlas's public packages. `myfleet-web` does not exist
  yet.
- **The MyFleet GitHub repo is public**, so Argo CD needs no repo credentials.
- **No `myfleet` database and no `myfleet` role** exist on `postgres.home`. Existing
  roles: `postgres`, `atlas`, `home-hub`, `scratch` — bee's convention is one role
  per app.
- **No `myfleet-media` bucket** on the shared MinIO (only `atlas-assets`,
  `atlas-canonical`, `atlas-renders`, `atlas-wz`).
- **No Kafka topic collision.** Atlas topics are uppercase `COMMAND_TOPIC_*` /
  `EVENT_TOPIC_*`; MyFleet's are lowercase dotted (`vehicle.created`, `media.uploaded`).
  This is what makes decision #7 safe.
- **`tumidanski.com` already resolves into Cloudflare space** (`2606:4700:30xx::`),
  and there is no `cloudflared` workload in-cluster — so Cloudflare reaches origin
  over the existing public IP / port-forward, exactly as `home-hub` does.
- **bee has two default StorageClasses** (`local-path` and `longhorn`), which is why
  #4 matters — though see §4.6 for why it barely applies.

---

## 3. Target architecture

```
                    Internet
                       |
                 Cloudflare (TLS terminates here)
                       |
    LAN ──┐            | :80
          |            v
      Pi-hole    192.168.23.230  (k3s Traefik LoadBalancer, kube-system)
   myfleet.home ──────►│
                       │  IngressRoute (namespace myfleet, entryPoint web)
          ┌────────────┼──────────────┬──────────────┬─────────────────┐
          │            │              │              │                 │
    /api/auth    /api/fleet     /api/media   /api/notifications       /
    stripPrefix  stripPrefix    stripPrefix    stripPrefix        (no strip)
          │            │              │              │                 │
     auth-service  fleet-service  media-service  notification-svc     web
       :8080         :8080          :8080           :8080            :80
          │            │              │              │
          └────────────┴──────┬───────┴──────────────┘
                              │
        ┌─────────────────────┼──────────────────────┐
        │                     │                      │
  postgres.home:5432    kafka.home:9093    minio.minio.svc.cluster.local:9000
   db: myfleet            (flat topics)        bucket: myfleet-media
   schemas: auth,                              user: myfleet (scoped policy)
   fleet, media,
   notification
```

Everything MyFleet owns lives in namespace `myfleet`. Nothing stateful is deployed —
all three backing stores are pre-existing cluster services.

---

## 4. Repository changes (MyFleet)

### 4.1 Kustomize restructure

The base currently mixes app services with infra, and `overlays/local` depends on that
infra. Deleting infra from the base would break local dev, and Kustomize has no clean
way for an overlay to *remove* a base resource. So the fix is structural: split infra
into a sibling directory that only the local overlay references.

```
deploy/k8s/
├── base/                       # apps only — no infra, no secrets
│   ├── kustomization.yaml
│   ├── namespace.yaml
│   ├── routing/middlewares.yaml    # 4 stripPrefix Middleware CRs (both overlays need these)
│   ├── auth-service/           {configmap,deployment,service}.yaml
│   ├── fleet-service/          {configmap,deployment,service}.yaml
│   ├── media-service/          {configmap,deployment,service}.yaml
│   ├── notification-service/   {configmap,deployment,service}.yaml
│   └── web/                    {deployment,service}.yaml        # NEW
├── infra-local/                # NEW — referenced only by overlays/local
│   ├── kustomization.yaml
│   ├── postgres.yaml           # + storageClassName: longhorn
│   ├── minio.yaml              # + storageClassName: longhorn
│   ├── redpanda.yaml           # + storageClassName: longhorn
│   ├── traefik.yaml            # Deployment + RBAC + NodePort Service
│   └── ingressroute.yaml       # local routing (no host match)
├── overlays/
│   ├── local/kustomization.yaml    # ../../base + ../../infra-local
│   └── main/                       # NEW
│       ├── kustomization.yaml      # SHA-pinned images, config patches
│       ├── ingressroute.yaml       # two hosts, per-route middlewares
│       └── patches/*.yaml
└── secrets.example.yaml        # NEW — template, never applied directly
```

Every `secret.yaml` is deleted from `base/` (decision #5). The four `Secret` objects
are applied out-of-band; because they are not in git, Argo CD does not manage them and
`prune: true` will not remove them.

The `namespace: myfleet` field stays on the base kustomization, and `metadata.namespace`
is stripped from individual manifests so the overlay controls placement.

### 4.2 Routing

`base/infra/traefik.yaml` is split. The four `stripPrefix` Middleware CRs are needed by
**both** overlays and move to `base/routing/middlewares.yaml`. The Traefik Deployment,
ClusterRole/Binding, ServiceAccount and NodePort Service move to `infra-local/` — they
must never reach bee, because that ClusterRole grants a cluster-wide watch on
`ingressroutes` and bee's k3s Traefik already runs `--providers.kubernetescrd`
cluster-wide. Two controllers reconciling the same CRs would have them fighting over
every IngressRoute on the cluster, atlas's included.

The main overlay uses a Traefik **`IngressRoute`**, not a plain `Ingress`. This is
forced by prefix stripping: the `traefik.ingress.kubernetes.io/router.middlewares`
annotation applies to an entire Ingress object, but each service needs a *different*
`stripPrefix`. A plain Ingress would therefore require five separate Ingress objects.
`IngressRoute` supports per-route middlewares natively, and bee already uses it
(`argocd` namespace, `atlas-pr-1135`).

Both hostnames are served by one route set:

```yaml
routes:
  - match: (Host(`myfleet.tumidanski.com`) || Host(`myfleet.home`)) && PathPrefix(`/api/auth`)
    kind: Rule
    priority: 100
    middlewares: [{name: auth-stripprefix}]
    services: [{name: auth-service, port: 8080}]
  # ... fleet / media / notifications, priority 100
  - match: Host(`myfleet.tumidanski.com`) || Host(`myfleet.home`)
    kind: Rule
    priority: 1                      # catch-all: SPA, must lose to every /api/* route
    services: [{name: web, port: 80}]
```

Explicit priorities are set rather than relying on Traefik's default rule-length
ordering, mirroring what `deploy/compose` already does for the web router.

### 4.3 Configuration

Applied in `overlays/main` as ConfigMap patches over the base values.

| Key | Service | Base (local) | main overlay |
|---|---|---|---|
| `KAFKA_BROKERS` | fleet, media, notification | `redpanda:9092` | `kafka.home:9093` |
| `MINIO_ENDPOINT` | media | `minio:9000` | `minio.minio.svc.cluster.local:9000` |
| `MEDIA_BUCKET` | media | `myfleet-media` | unchanged |
| `APP_BASE_URL` | auth | `http://localhost` | `https://myfleet.tumidanski.com` |
| `JWT_ISSUER` | auth | `https://myfleet.example.com` | `https://myfleet.tumidanski.com` |
| `GOOGLE_REDIRECT_URL` | auth | *(wrong — see §4.5)* | `https://myfleet.tumidanski.com/api/auth/auth/callback` |
| `COOKIE_SECURE` | auth | `false` | `true` |
| `JWKS_URL`, `FLEET_SERVICE_URL`, `FLEET_INTERNAL_URL` | various | in-cluster DNS | unchanged |

`DATABASE_URL` moves entirely into the out-of-band Secrets:

```
postgres://myfleet:<password>@postgres.home:5432/myfleet?sslmode=disable&search_path=<schema>
```

with `<schema>` one of `auth`, `fleet`, `media`, `notification`.

### 4.4 Kafka

No changes to application code. Topics stay the compile-time constants they are today —
`schedule.overdue`, `maintenance.completed`, `fuel.logged`, `vehicle.created`,
`member.invited` (`apps/notification-service/internal/consumer/consume.go:27`) and
`media.uploaded` (`apps/media-service/internal/mediaobject/processor.go:23`) — with the
consumer group fixed at `notification`.

This is safe **today** because atlas's topics are uppercase-prefixed and cannot collide.
It is recorded here as an accepted constraint, not an oversight: a second MyFleet
environment on the same broker would cross-talk, since two envs would share both topic
names and the consumer group. Adding isolation later means making those constants
configurable — a code change, not a manifest change.

### 4.5 Bug fix folded in: `GOOGLE_REDIRECT_URL`

Routes are registered on the root chi router with no base mount
(`packages/shared-go/server/handler.go:33`), and the OIDC callback is `/auth/callback`
(`apps/auth-service/internal/oidc/resource.go:50`). With `/api/auth` stripped, the only
correct public callback path is **`/api/auth/auth/callback`**.

Both committed configs are wrong, in different ways:

| File | Value | Strips to | Route exists? |
|---|---|---|---|
| `deploy/k8s/base/auth-service/configmap.yaml:13` | `/api/auth/auth/google/callback` | `/auth/google/callback` | No |
| `deploy/compose/.env.example:19` | `/api/auth/callback` | `/callback` | No |

Both appear to conflate the login path (`/auth/login/google`) with the callback. Since
this task must set a working value anyway, it fixes both — the compose one too, so the
two environments do not disagree.

### 4.6 Storage

After decision #1 the main overlay contains **zero PVCs** — Postgres, Kafka and MinIO
are all shared cluster services. So bee's two-default-StorageClass ambiguity does not
actually bite MyFleet in production. Decision #4 is still honoured by setting
`storageClassName: longhorn` explicitly on the three `infra-local` PVCs, so that anyone
applying the local overlay to a real cluster gets a deterministic class rather than
whichever default wins.

The ambiguity itself (two classes both marked default) is a pre-existing bee
misconfiguration and is out of scope here.

### 4.7 Web UI

`apps/web/Dockerfile` already builds a self-contained SPA image, and the app is
environment-agnostic — no `VITE_*` variables are baked at build time, and every service
client uses a relative path (`/api/fleet/vehicles`, etc.). One image works in every
environment; routing is purely an Ingress concern.

Two changes are needed:

1. **CI** — add `web` to the `publish` matrix in `.github/workflows/main.yml`
   (context is the repo root, file `apps/web/Dockerfile`), producing
   `ghcr.io/jtumidanski/myfleet-web`. Add it to the `trivy` matrix too, for parity
   with the Go services.
2. **Manifests** — `base/web/{deployment,service}.yaml`, Service on port 80.

Because `web` joins the base, `overlays/local` must gain matching entries too: a
`replicas` count, a `ghcr.io/jtumidanski/myfleet-web` → `newTag: local` image override,
and the `imagePullPolicy: IfNotPresent` patch. Omitting these would leave local dev
pulling a `:latest` web image from GHCR while every other service ran locally built code.

**Security posture.** The four Go services run `runAsNonRoot`, `runAsUser: 10001`,
`readOnlyRootFilesystem: true`, all capabilities dropped. Stock `nginx:alpine` cannot
match that: it starts as root to bind :80 and writes to `/var/cache/nginx` and
`/var/run`.

*Recommended:* switch stage 2 of `apps/web/Dockerfile` to
`nginxinc/nginx-unprivileged:alpine` and change `apps/web/nginx.conf` to `listen 8080`.
The container then runs as UID 101 with a read-only root filesystem, and the web
Deployment gets the same hardened `securityContext` as everything else. The Service
maps port 80 → targetPort 8080, so nothing downstream changes.

*Alternative, if the image change is unwanted:* keep `nginx:alpine` and relax the web
Deployment's `securityContext` (`readOnlyRootFilesystem: false`, no `runAsNonRoot`),
with `emptyDir` mounts for the writable paths. This is the smaller diff but leaves one
container meaningfully less hardened than its siblings, and it is the kind of thing a
security review will flag.

This design assumes the recommended option.

### 4.8 CI image-tag bump

A new `bump-overlay` job in `.github/workflows/main.yml`, gated on `publish`, rewrites
the five `newTag` values in `deploy/k8s/overlays/main/kustomization.yaml` to
`${{ github.sha }}` and pushes the commit to `main`. Argo CD sees a real manifest diff
and rolls the Deployments; `git log` becomes an accurate record of what is deployed.

Loop prevention needs both belts:

- `[skip ci]` in the bump commit message, **and**
- `paths-ignore: ['deploy/k8s/overlays/main/**']` on the workflow's `push` trigger.

Either alone is fragile; together the bump commit cannot retrigger the workflow. The
existing `contents: write` permission already covers the push.

---

## 5. Cluster-side changes (`~/source/k3s/bee`)

A new `argocd-myfleet.yml`, modelled on `argocd-atlas.yml` but far smaller — one
`Application`, no `ApplicationSet`, no cleanup CronJobs, no RBAC (all of which exist in
atlas only to serve per-PR environments, which are a non-goal here).

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

No `repo-creds` Secret is required — the repository is public.

`prune: true` is enabled from the start rather than after a stability window. Atlas
deferred it because its Kustomize-hashed ConfigMaps left orphans; MyFleet's main overlay
generates no hashed resources, so the failure mode that motivated the delay does not
apply. Any resource that must survive a manifest drop should carry
`argocd.argoproj.io/sync-options: Prune=false`.

---

## 6. One-time bootstrap

None of this is automated, by decision #9. It belongs in
`docs/runbooks/k3s-deployment.md`, committed in this task's PR.

**1. Postgres** — role, database, four schemas:

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

GORM `AutoMigrate` creates tables inside these schemas on first start; it does **not**
create schemas, which is why this step is a hard prerequisite rather than a convenience.

**2. MinIO** — bucket plus a scoped user:

```sh
kubectl -n minio port-forward svc/minio 9000:9000 &
mc alias set bee http://localhost:9000 <root-user> <root-pass>
mc mb bee/myfleet-media
mc admin user add bee myfleet <secret>
mc admin policy create bee myfleet-media-rw ./myfleet-media-policy.json
mc admin policy attach bee myfleet-media-rw --user myfleet
```

The policy scopes `s3:*` to `arn:aws:s3:::myfleet-media` and `.../*` only, so the
credential cannot reach any `atlas-*` bucket.

**3. Secrets** — from `deploy/k8s/secrets.example.yaml`, filled in and applied by hand:

| Secret | Keys |
|---|---|
| `auth-service-secret` | `DATABASE_URL`, `GOOGLE_CLIENT_ID`, `GOOGLE_CLIENT_SECRET`, `JWT_PRIVATE_KEY_PEM`, `OIDC_STATE_SECRET` |
| `fleet-service-secret` | `DATABASE_URL` |
| `media-service-secret` | `DATABASE_URL`, `MINIO_ACCESS_KEY`, `MINIO_SECRET_KEY` |
| `notification-service-secret` | `DATABASE_URL` |

**4. DNS** — a Cloudflare record for `myfleet.tumidanski.com` (proxied, pointing at the
same origin as `tumidanski.com`), and a Pi-hole record for `myfleet.home` →
`192.168.23.230` on both Pi-hole servers.

**5. Google Cloud Console** — register
`https://myfleet.tumidanski.com/api/auth/auth/callback` as an authorised redirect URI.
This is external and cannot be automated or verified from the repo; login stays broken
until it is done.

**6. Argo CD** — `kubectl apply -f argocd-myfleet.yml` from the bee repo.

---

## 7. Consequences and risks

**Secure cookies make `myfleet.home` unauthenticated in practice.** `COOKIE_SECURE=true`
is required for the Cloudflare host, and browsers do not send `Secure` cookies over
plain HTTP. So on `myfleet.home` a user cannot hold a session: it is useful for
`/healthz`, `/readyz` and unauthenticated endpoints, and for confirming the stack is up
without depending on Cloudflare — but not for using the app. Sessions are per-host
regardless, so logging in on one host would never have authenticated the other. If LAN
login matters later, the fix is a real certificate on `myfleet.home`, not a config
change.

**Cloudflare caps upload size.** media-service handles vehicle photo uploads, and
Cloudflare's proxy enforces a request-body ceiling (100 MB on free plans; `home-hub`
pins `proxy-body-size: 10m`). Uploads above the limit fail at the edge, before Traefik.
Worth an explicit limit in the media-service config rather than discovering it from a
user report.

**No `myfleet` Postgres backup story.** The database is created by hand on a shared
Postgres whose PVC is 10 Gi and whose backup policy is out of scope here. Worth a
follow-up.

**Argo CD `selfHeal` will revert manual edits.** Anything changed with `kubectl edit`
in the `myfleet` namespace is reverted on the next sync. Out-of-band Secrets are
unaffected, being untracked.

**First sync ordering.** If Argo CD syncs before the manual Postgres step, all four
services `CrashLoopBackOff` on a missing schema. This is expected and self-resolves once
the DDL runs — no manifest change needed. It is the accepted cost of decision #9 over a
PreSync hook.

---

## 8. Verification

Pre-merge, in the worktree:

- `kustomize build deploy/k8s/overlays/main` renders with no placeholders and no PVCs.
- `kustomize build deploy/k8s/overlays/local` still renders — proving the infra split
  did not break local dev.
- `kubectl apply --dry-run=server -f -` against the rendered main overlay.
- `make ci` clean (`lint-check`, `vet`, `test`, `build`, `fe-test`, `fe-build`).

Post-deploy, on bee:

- All five Deployments `Available`; no `ImagePullBackOff`.
- `kubectl -n myfleet logs deploy/auth-service` shows AutoMigrate completing against
  the `auth` schema.
- `curl -H 'Host: myfleet.home' http://192.168.23.230/api/fleet/healthz` → 200.
- `https://myfleet.tumidanski.com/` serves the SPA; a deep link (e.g. `/vehicles`)
  returns index.html rather than 404, confirming the catch-all priority is right.
- Full Google login round-trip completes and sets a session cookie.
- A media upload lands an object in the `myfleet-media` bucket.
- A commit to `main` bumps the overlay SHA and Argo CD rolls the affected Deployments
  without manual intervention.

---

## 9. Deliverables

**MyFleet repo (this branch, one PR):**

- `deploy/k8s/base/` restructured; secrets removed; `web/` added; `routing/middlewares.yaml` added
- `deploy/k8s/infra-local/` extracted from the old base infra, with explicit `longhorn` storage class
- `deploy/k8s/overlays/main/` created
- `deploy/k8s/overlays/local/kustomization.yaml` updated for the new layout
- `deploy/k8s/secrets.example.yaml` added
- `apps/web/Dockerfile` + `apps/web/nginx.conf` switched to unprivileged nginx
- `.github/workflows/main.yml`: `web` added to `publish` and `trivy`; new `bump-overlay` job; `paths-ignore` on the push trigger
- `deploy/k8s/base/auth-service/configmap.yaml` and `deploy/compose/.env.example`: `GOOGLE_REDIRECT_URL` corrected
- `docs/runbooks/k3s-deployment.md` added
- `CLAUDE.md` "Build & Verification" updated with the kustomize render commands

**bee repo (separate commit):**

- `argocd-myfleet.yml`

**External, manual:**

- Cloudflare DNS record; Pi-hole DNS record; Google Cloud Console redirect URI;
  Postgres DDL; MinIO bucket, user and policy; four Kubernetes Secrets

---

## 10. Amendments

### A1 — Media bytes are proxied through media-service (approved during planning)

Found while writing `plan.md`: §4.3's move of `MINIO_ENDPOINT` to
`minio.minio.svc.cluster.local:9000` is incompatible with the presigned-URL
upload flow this design left in place.

- `POST /api/media` presigns a PUT URL built from `MINIO_ENDPOINT`
  (`apps/media-service/internal/storage/minio.go:106-107`).
- The browser PUTs the bytes straight to that URL
  (`apps/web/src/services/api/MediaService.ts:37-41`).
- `minio.minio.svc.cluster.local` does not resolve in a browser, and the S3 v4
  signature covers the `Host` header, so the URL cannot be rewritten client-side.

As designed, uploads and thumbnails both fail and §8's "a media upload lands an
object in the `myfleet-media` bucket" cannot pass.

**Decision: proxy the bytes through media-service. Do not expose MinIO
publicly** — neither on its own hostname nor on a path under the MyFleet hosts,
because the shared MinIO also holds the `atlas-*` buckets and proxying the S3
API would expose that surface as well.

Changes this makes to the sections above:

| Section | Amendment |
|---|---|
| §4.3 | Adds `MEDIA_MAX_UPLOAD_BYTES` (default `26214400` = 25 MiB) to the media-service config. `MINIO_ENDPOINT` stays internal, so server-side S3 traffic never leaves the cluster. |
| §4.4 | Unaffected — no topic or consumer-group changes. |
| §7 | The Cloudflare body-size note is no longer hypothetical: uploads now traverse the edge. The service returns `413` at its own limit; the edge rejects earlier. |
| §8 | Post-deploy media-upload check stands, and now can actually pass. |
| §9 | Adds `apps/media-service` (new content routes, presign plumbing deleted), `packages/shared-ts` (`ApiClient.requestBlob`), and `apps/web` (proxied upload + object-URL thumbnails) to the MyFleet-repo deliverables. |

New API surface, replacing the presigned flow:

- `PUT /api/media/{id}/content` — streams the body to object storage under the
  `MEDIA_MAX_UPLOAD_BYTES` bound. Content type comes from the row created at
  init, never from the request.
- `GET /api/media/{id}/content` — streams the bytes after fleet authorization.
- Removed: `GET /api/media/{id}/download`, `attributes.uploadUrl`,
  `attributes.downloadUrl`, `storage.Client.PresignPut`/`PresignGet`.

One non-obvious consequence: MyFleet authenticates with an `Authorization:
Bearer` header rather than a session cookie, and browsers do not attach that
header to `<img src>` requests. Thumbnails therefore fetch their bytes through
the API client and render an object URL, which is why this amendment also touches
`packages/shared-ts`.

Implementation lands in `plan.md` Tasks 3–6.
