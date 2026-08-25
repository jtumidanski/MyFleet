# Service Scaffolding Checklist

When scaffolding a new MyFleet service, complete these steps. Do not skip any.

## 1. Service Directory
**Directory:** `apps/<service-name>/`

Required structure, matching `apps/fleet-service/`:
```
apps/<service-name>/
├── go.mod
├── go.sum
├── cmd/
│   └── main.go
├── internal/
│   └── <domain>/
│       ├── model.go
│       ├── entity.go
│       ├── builder.go
│       ├── processor.go
│       ├── provider.go
│       ├── administrator.go
│       ├── resource.go
│       └── rest.go
└── Dockerfile
```

There is no `migrations/` directory. Migrations run through GORM's
`AutoMigrate`, driven from a `Migration` function declared alongside each
domain's entity, e.g. `apps/auth-service/internal/user/entity.go:42`:
```go
func Migration(db *gorm.DB) error { return db.AutoMigrate(&Entity{}) }
```

Add the module to `go.work` at the repo root.

## 2. Kubernetes Manifest
**Files:** `deploy/k8s/base/<service-name>/{configmap,deployment,service}.yaml`
— three files, following the shape in `deploy/k8s/base/fleet-service/`.

Register all three in `deploy/k8s/base/kustomization.yaml`'s `resources:`
list (see the `fleet-service/*.yaml` block at lines 17-19 for the pattern) —
a service left out of that list is not deployed.

Deployment shape, from `deploy/k8s/base/fleet-service/deployment.yaml`:
- Pod-level `securityContext`: `runAsNonRoot: true`, a fixed `runAsUser` /
  `fsGroup` (fleet-service uses `10001`, matching the Dockerfile's non-root
  user — see §3)
- Container-level `securityContext`: `allowPrivilegeEscalation: false`,
  `readOnlyRootFilesystem: true`, `capabilities.drop: ["ALL"]`
- `envFrom` a `<service-name>-config` ConfigMap and a `<service-name>-secret`
  Secret
- `readinessProbe` on `/readyz`, `livenessProbe` on `/healthz`
- `resources.requests` / `resources.limits` for memory and cpu

Service shape: `type: ClusterIP`, one port block matching the container port.

Per CLAUDE.md, the `main` overlay must still render with no
PersistentVolumeClaims, no Secrets, and no ClusterRole. The base Deployment's
`secretRef` is filled out-of-band by the overlay-level secret process — do not
add a Secret manifest under `deploy/k8s/base/` or `deploy/k8s/overlays/main/`.

`deploy/k8s/base/routing/middlewares.yaml` holds the Traefik `Middleware`
objects (one stripprefix per service) referenced by the IngressRoutes in §5.

## 3. Dockerfile
**File:** `apps/<service-name>/Dockerfile`

Multi-stage Go build, following `apps/fleet-service/Dockerfile`:
- Builder stage: `FROM golang:1.27-alpine AS build`; copy `go.work`,
  `go.work.sum`, and every module's `go.mod`/`go.sum` first for layer
  caching, then `go build -o /out/<service-name> ./cmd`
- Runtime stage: `FROM alpine:3.24`; create and switch to a non-root user
  (`adduser -D -u 10001 app` / `USER app`)
- Copy the built binary to `/<service-name>` (not `/server`) and set it as
  `ENTRYPOINT`
- `EXPOSE 8080`

Build context is the repo root for every service, including `apps/web`
(CLAUDE.md):
```sh
docker build -f apps/<service-name>/Dockerfile .
```

## 4. Docker Compose Entry
**File:** `deploy/compose/docker-compose.yml`

Add a service block alongside the existing ones (`fleet-service:` starts at
line 142). Following that block's shape, a new entry needs:
- `build.context: ../..`, `build.dockerfile: apps/<service-name>/Dockerfile`
- `environment:` (or `env_file: .env`) with the service's config — the other
  four service blocks all set `DATABASE_URL`, `JWKS_URL`, and `LOG_LEVEL`
  the same way
- `depends_on` with `condition: service_healthy` for whatever the service
  needs at boot (`postgres`, `redpanda`, `auth-service`, ...)
- `healthcheck` — only `auth-service` (line 116) and `fleet-service` (line
  165) have one today (`media-service` and `notification-service` don't);
  follow `fleet-service`'s:
  `wget -qO- http://localhost:8080/healthz || exit 1`
- `labels` starting with `traefik.enable=true`, plus
  `traefik.http.routers.<name>.*` entries so the compose Traefik container
  picks the service up (`deploy/compose/traefik/traefik.yml` sets
  `providers.docker.exposedByDefault: false`, so an unlabeled service gets no
  route). Mirror the routing decision made in §5 below.

Bring the stack up with `scripts/dev-up.sh`, which `cd`s into
`deploy/compose` and runs `docker compose --env-file .env up -d --build`.

## 5. Traefik Routing
**Files:** `deploy/k8s/overlays/main/ingressroute.yaml` and
`deploy/k8s/infra-local/ingressroute.yaml`

There is no nginx in the API request path. `apps/web` runs its own
static-file server config for the SPA inside the `web` container, and it
does no proxying. API routing to backend services is two Traefik
`IngressRoute` objects — one per overlay — each with `:80` and `:443`
routers inside it.

Add the new service's route to **both** files with matching route sets.
`.github/workflows/pr.yml`'s `manifests` job explains why
(`pr.yml:131-135`):

> the `:80` and `:443` IngressRoutes must carry identical route sets. They
> are two objects because a Traefik IngressRoute with a `tls` section is
> TLS-only, and the route set contains the internal-deny rule guarding
> fleet-service's unauthenticated `/internal/*` endpoints — so drift between
> them is a security hole.

If the new service exposes an unauthenticated `/internal/*` surface of its
own (the pattern fleet-service, media-service, auth-service, and
notification-service all follow), give it the same priority-200 deny router
ahead of the priority-100 proxy router, in both files.

## 6. CI Configuration
Add the service to:
- `strategy.matrix.service` in the `containers` job of
  `.github/workflows/pr.yml` (currently
  `[auth-service, fleet-service, media-service, notification-service]`)
- `strategy.matrix.service` in `.github/workflows/main.yml` — the same list
  appears in its build, scan, and push jobs, each currently
  `[auth-service, fleet-service, media-service, notification-service, web]`

A service missing from these lists is not built or scanned in CI.

## 7. Post-Scaffold Verification
After scaffolding is complete, audit the work against the MyFleet backend
developer guidelines by dispatching the `backend-guidelines-reviewer` agent
(or run `/audit-plan` if this scaffolding was driven by a task plan).

Also render both kustomize overlays and, against a reachable cluster, run
both server dry-runs (CLAUDE.md). `make manifests` (`Makefile:45`) runs the
local render and its assertions — skipping the *local* dry-run and only ever
checking `main` previously let a missing `namespace:` reach ten reviews.
```sh
kustomize build deploy/k8s/overlays/local
kustomize build deploy/k8s/overlays/main
```

## 8. Compliance Checklist (Commonly Missed Items)

Verify each domain package against these items, which are frequently missed
during initial implementation:

| Check | File | Requirement |
|-------|------|-------------|
| `builder.go` exists | `builder.go` | Fluent builder — `NewBuilder()`, setters, and `Build()`. Returns `(Model, error)` and checks invariants where the domain has them (11 of 17); a bare `Build() Model` is correct where it has none (`DOM-01`) |
| `ToEntity()` method | `entity.go` | `func (m Model) ToEntity() Entity` — present in 17 of the 20 domain packages with an `entity.go` (e.g. `apps/fleet-service/internal/vehicle/entity.go:63`). The three exceptions: `platformadmin` has no `model.go` at all, so no `Model` and nothing to convert; `dashboard`'s types are `Dashboard`/`Widget`, converted one-way from entity via `MakeDashboard` (`entity.go:68`) with no reverse method; `admin` declares `Operation`/`AuditEvent` instead of a single `Model`, each with its own `ToEntity()` (`model.go:114`, `model.go:169`) (`DOM-02`) |
| `Make` constructor | `entity.go` | `func Make(e Entity) Model`; not uniform — a domain whose model needs child rows too takes them as extra args, e.g. `maintenancerecord/entity.go:44` is `func Make(e Entity, docs []DocumentEntity) Model` (`DOM-03`) |
| `TransformSlice` function | `rest.go` | List handlers use `TransformSlice`, not an inline loop — unless decorating each row with a per-row derived value via `TransformDerived` (`DOM-05`) |
| `logrus.FieldLogger` | `processor.go` | Constructor takes `logrus.FieldLogger`, not `*logrus.Logger` (`DOM-06`) |
| `log` parameter in handlers | `resource.go` | Handlers use the `log` parameter `InitializeRoutes` receives — never `logrus.StandardLogger()`, which is reserved for the shared error-logger fallback (`packages/shared-go/server/jsonapi.go:70`) (`DOM-07`) |
| `server.RegisterInputHandler[T]` | `resource.go` | Bodied routes (`r.Post`, `r.Patch`, `r.Put`) wrap their handler in `RegisterInputHandler[T]`; body-less routes (GET, DELETE, path-only actions) register a plain `func(w, r)` (`DOM-08`) |
| Errors via `server.WriteError` | `resource.go` | Domain errors go out through `server.WriteError(w, err)`, not a hand-built envelope (`DOM-09`) |
| `Provider` interface | `provider.go` | `Provider` interface plus a `NewProvider(db)` constructor; a method that fetches a single record translates `gorm.ErrRecordNotFound` to a domain sentinel (or a bool/zero-value, where the caller doesn't need it as an error) (`DOM-10`) |
| No `os.Getenv()` in handlers | `resource.go` | Env vars are read once in config at startup and injected via constructors (`DOM-11`) |

## Database Notes
- Each service owns its own database schema
- UUID primary keys are generated in the application (`uuid.NewString()`,
  e.g. `apps/fleet-service/internal/vehicle/builder.go:13`)
- Migrations run on service startup via each domain's `Migration(db)`
  function
