# MyFleet

A cloud-hosted **household vehicle management platform**. A household is a
"fleet": one or more people collaborating on a shared set of vehicles, their
maintenance history, recurring service schedules, fuel logs, mileage, and
documents.

The product philosophy is household-first collaboration and structured,
durable maintenance records — practical utility and data quality over
AI-driven workflows. See [`docs/household-vehicle-platform-mvp-prd.md`](docs/household-vehicle-platform-mvp-prd.md)
for the full MVP product definition and
[`docs/roadmap.md`](docs/roadmap.md) for the delivery slices.

## Features

- **Fleets & membership** — create a household, invite members by email,
  owner/member/viewer roles, fleet-scoped authorization on every request.
- **Vehicles** — CRUD with soft-delete and a restore window, multiple photos
  per vehicle with a selectable primary image.
- **Mileage** — immutable mileage history with a trend graph; fuel and
  maintenance entries feed it automatically.
- **Maintenance** — seeded categories, maintenance records with document and
  receipt attachments, recurring schedules (time / mileage / hybrid), and
  upcoming + overdue queues with severity.
- **Fuel** — fuel logs capturing date, mileage, volume, and cost.
- **Status & activity** — derived vehicle status (Healthy / Upcoming /
  Overdue / Inactive), a fleet activity feed, and per-vehicle timelines.
- **Notifications** — in-app notifications driven by domain events, with
  per-user per-type preferences and scheduled reminders.
- **Dashboards** — a widget catalog (fleet overview, status cards,
  maintenance queues, recent activity, spend, mileage trends) with per-user
  layout persistence.

## Architecture

A monorepo of four Go services and one React SPA behind a single-origin
reverse proxy. Traefik routes by path prefix; the SPA is the catch-all at `/`.

```
                    ┌──────────── Traefik ────────────┐
  browser ────────► │ /api/auth/*          → auth-service          :8080
                    │ /api/fleet/*         → fleet-service         :8080
                    │ /api/media/*         → media-service         :8080
                    │ /api/notifications/* → notification-service  :8080
                    │ /*                   → web (nginx + SPA)     :80
                    └─────────────────────────────────┘
                              │
              PostgreSQL  ·  MinIO  ·  Kafka/Redpanda
```

| Component | Path | Responsibility |
|---|---|---|
| `auth-service` | `apps/auth-service` | Google OIDC verification, user auto-provisioning, first-party RS256 JWT mint + refresh, JWKS endpoint, `/auth/me`. |
| `fleet-service` | `apps/fleet-service` | All core domain: fleets, membership, invites, vehicles, mileage, maintenance records + schedules, fuel, activity, status, dashboards. Produces domain events. |
| `media-service` | `apps/media-service` | MinIO-backed uploads, object metadata, presigned downloads, async thumbnail/display variant generation. |
| `notification-service` | `apps/notification-service` | Event consumers, in-app notification inbox, per-type preferences, scheduled reminder jobs. |
| `web` | `apps/web` | React 18 + TypeScript SPA (Vite, React Router, TanStack Query, react-hook-form + Zod, Tailwind, shadcn-style components). |
| `shared-go` | `packages/shared-go` | Server bootstrap, JSON:API helpers, pagination, JWT middleware + JWKS client, config, database, health/metrics, telemetry, background jobs. |
| `dto-go` | `packages/dto-go` | Cross-service event payload contracts. |
| `shared-ts` | `packages/shared-ts` | API client (including the single 401-refresh path), JSON:API types, error handling. |
| `ui-components` | `packages/ui-components` | Cross-app React components and formatters. |

### Key decisions

- `fleet-service` owns all core domain; other services stay out of it.
- One PostgreSQL instance, an isolated schema per service (`auth`, `fleet`,
  `media`, `notification`). **No cross-service joins** — services talk over
  HTTP (`/internal/*` endpoints) or events.
- `auth-service` mints first-party JWTs; every other service validates them
  against the shared JWKS URL.
- Transport is **JSON:API**; Go services follow a DDD layout with immutable
  models, functional composition, and GORM entities.
- Domain events (`vehicle.created`, `maintenance.completed`, `fuel.logged`,
  `schedule.overdue`, `member.invited`) are enqueued transactionally and
  published to Kafka.

## Getting started

### Prerequisites

- Go 1.25+
- Node 22 (`nvm use 22`)
- Docker + Docker Compose

Node is not always on `PATH`:

```sh
export NVM_DIR="$HOME/.nvm" && . "$NVM_DIR/nvm.sh" && nvm use 22
```

### Run the full stack locally

```sh
./scripts/dev-up.sh      # copies .env.example → .env if needed, then builds and starts
```

That brings up Traefik, PostgreSQL, MinIO, Redpanda, all four services, and
the SPA. The app is at <http://localhost>; the Traefik dashboard is at
<http://localhost:8081>.

Equivalent Make targets:

```sh
make up      # docker compose up -d --build
make down    # docker compose down -v  (destroys volumes)
```

### Configuration

Local config lives in `deploy/compose/.env`, seeded from
[`deploy/compose/.env.example`](deploy/compose/.env.example). Two values have
no usable default and must be filled in before login works:

- `GOOGLE_CLIENT_ID` / `GOOGLE_CLIENT_SECRET` — a Google OAuth client whose
  redirect URI matches `GOOGLE_REDIRECT_URL`
  (`http://localhost/api/auth/callback` by default).
- `JWT_PRIVATE_KEY_PEM` — an RS256 private key in PKCS#1 PEM form. Generate
  one with `./scripts/gen-jwt-key.sh`.

Never commit real credentials; Gitleaks runs in CI.

### Frontend dev server

For iterating on the UI against the compose backend:

```sh
npm install
npm run -w apps/web dev
```

## Build & verification

Run the whole gate before claiming a branch is done:

```sh
make ci      # lint-check, vet, test, build, fe-test, fe-build, manifests, carfax-template
```

Individually:

| Command | What it does |
|---|---|
| `make vet` / `make test` / `make build` | Go workspace vet, race-enabled tests, build |
| `make fe-test` / `make fe-build` | Vitest across `apps/web` and `packages/shared-ts`; production web build |
| `make lint-check` | Check-only lint + format guard (what CI runs) |
| `make lint` | Same, in fix mode |
| `make fmt` | Formatter layer only (gofumpt, goimports, prettier) |
| `make tidy` | `go mod tidy` in every workspace module |
| `make manifests` | Render the k8s overlays and assert their invariants |

### Deployment manifests

`deploy/k8s` has no test suite, so render both overlays:

```sh
kustomize build deploy/k8s/overlays/local   # dev: bundled infra + dev Traefik
kustomize build deploy/k8s/overlays/main    # cluster: shared infra, no PVCs, no Secrets
```

The `main` overlay must render with no PersistentVolumeClaims, no Secrets, no
ClusterRole, and no placeholder values. Against a reachable cluster, run
**both** server dry-runs — rendering alone does not catch namespace or
cross-resource-reference errors:

```sh
kustomize build deploy/k8s/overlays/main  | kubectl apply --dry-run=server -f -
kustomize build deploy/k8s/overlays/local | kubectl apply --dry-run=server -f -
```

### Container images

Every service builds from the repo root as context, including the web app:

```sh
docker build -f apps/<service>/Dockerfile .
```

## Deployment

`main` publishes images to `ghcr.io/jtumidanski/myfleet/<service>`, scans them
with Trivy, computes the next semver tag, and pins
`deploy/k8s/overlays/main` to the built commit. Argo CD syncs that overlay to
the `myfleet` namespace on the k3s cluster.

Cluster bootstrap (Postgres role/database/schemas, secrets, TLS, hostnames) is
deliberately manual and documented in
[`docs/runbooks/k3s-deployment.md`](docs/runbooks/k3s-deployment.md). GORM
`AutoMigrate` creates tables but **not** schemas — create those first or the
services crash-loop.

## Repository layout

```
apps/                 auth-service, fleet-service, media-service,
                      notification-service, web
packages/             shared-go, dto-go, shared-ts, ui-components
deploy/compose/       local docker-compose stack + Traefik config
deploy/k8s/           kustomize base + local/main overlays
docs/                 PRD, roadmap, runbooks, per-task specs under docs/tasks/
scripts/              dev-up.sh, gen-jwt-key.sh
tools/                lint.sh, check-manifests.sh, task-numbers.sh, …
.github/workflows/    pr.yml, main.yml
```

## Development workflow

Non-trivial changes go through four phases, each in its own session, all
inside a dedicated git worktree under `.worktrees/task-NNN-slug/`:

1. `/spec-task <idea>` — PRD interview; creates the worktree and branch.
2. `/design-task <task-id>` — produces `design.md`.
3. `/plan-task <task-id>` — produces `plan.md` + `context.md`.
4. `/execute-task <task-id>` — implements the plan in the existing worktree.

Artifacts live in `docs/tasks/task-NNN-slug/`. Task numbers come from
`tools/task-numbers.sh next`. Code review (`/audit-plan` or
`superpowers:requesting-code-review`) runs before every PR. Full details,
including the Go and React guideline checklists, are in
[`CLAUDE.md`](CLAUDE.md).
