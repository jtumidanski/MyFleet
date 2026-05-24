# task-001 — Implementation Context

Companion to `plan.md`. Captures the key files, fixed decisions, dependencies, and gotchas an engineer needs before executing. Read alongside `design.md` (behavior source of truth) and `prd.md` (the contract).

## Scope decision (read first)

`task-001` was designed as a **spec/anchor** task whose `design.md` and `docs/roadmap.md` slice the build into task-002…012. **The user has overridden that:** this single task implements the **entire MVP**. The roadmap remains useful as a dependency map, but there will be no separate slice tasks — `plan.md` folds all slices into one phased plan (phases mirror the roadmap rows).

## Source documents

- `docs/tasks/task-001-household-vehicle-platform/prd.md` — the MVP contract (FR-*, NFR, §10 acceptance).
- `docs/tasks/task-001-household-vehicle-platform/design.md` — architecture; **authoritative for behavior**: §6 service template, §7 events, §9 authz, §10 algorithms, §13 data model, §17 deploy.
- `docs/roadmap.md` — slice dependency ordering (now folded into plan phases).
- `docs/household-vehicle-platform-mvp-prd.md` — raw source brief.

## Repository layout (target, design §4)

```
go.work · package.json · Makefile
apps/{auth,fleet,media,notification}-service · apps/web
packages/{shared-go,dto-go,shared-ts,ui-components}
deploy/{compose,k8s} · scripts/ · docs/
```
- Go module namespace: `github.com/jtumidanski/myfleet/...` (each app + each package is its own module, wired by `go.work`).
- npm workspaces: `apps/web`, `packages/shared-ts`, `packages/ui-components`.
- Images: `ghcr.io/jtumidanski/myfleet-<service>`.

## Fixed cross-cutting decisions (do not relitigate — design §3)

| ID | Decision |
|----|----------|
| D1 | `fleet-service` owns the **entire** core domain; auth/media/notification stay narrow. |
| D2 | One PostgreSQL, **isolated schema per service** (`auth`/`fleet`/`media`/`notification`); no cross-service joins — id refs resolved via internal APIs. |
| D3 | `auth-service` verifies Google OIDC, mints first-party JWTs; all services validate. |
| D4 | Single-origin **Traefik** gateway: TLS/CORS/path-routing only. |
| A1 | Native `go.work` + npm workspaces; Makefile/scripts orchestration. |
| A2 | JWT validated **per-service** in shared middleware (not edge forwardAuth). |
| A3 | **RS256 + JWKS**; access ~15 min, rotating refresh ~30 d (hashed, single-use, family-revoke on reuse). |
| A4 | **Page-number** pagination everywhere (`page[number]`/`page[size]`, `meta.total`/`totalPages`). |
| A5 | Dashboard aggregations **computed on read** (no precompute for MVP). |
| A6 | Notifications: event-driven **+ daily safety-net job**; dedupe per `(trigger, due-cycle)`. |
| A7 | media variant processing in an **in-process Kafka worker pool**. |
| A8 | **Transactional outbox** in producers (fleet-service): domain write + outbox row in one tx; relay publishes under advisory lock. |
| A9 | Background jobs run under a **Postgres advisory lock** (`WithLeaderLock`). |

## Build order & dependencies

Strict order (each phase depends on the prior unless noted):
`P0 foundation → P1 shared-go → P2 dto/shared-ts/ui → P3 infra → P4 auth → P5 fleet core+authz → P6 vehicle/media-refs → P7 media-service → P8 mileage → P9 maintenance → P10 fuel → P11 status/activity/events → P12 notifications → P13 dashboard → P14 web shell+canonical → P15 web features → P16 CI → P17 k8s → P18 e2e`.

Parallelizable once their deps land: P10 (fuel) ∥ P9 (maintenance) after P8; P16/P17 can begin after P3 and grow. P11 (real events) must precede P12 (consumers). The web phases (P14/P15) need the corresponding backend endpoints to exist.

## Canonical patterns (written in full; everything else mirrors them)

- **Backend domain template (8 files)** — `apps/auth-service/internal/user/` (Task 4.2). Files: `model.go` (immutable, accessors, `With*` copies), `entity.go` (GORM tags + `TableName()` schema-qualified + `Migration` + `Make`/`ToEntity`), `builder.go`, `provider.go` (lazy `database.Query`), `processor.go` (pure logic, takes `logrus.FieldLogger` + providers), `administrator.go` (writes/orchestration, transactions), `rest.go` (`Transform`/`TransformSlice`), `resource.go` (`InitializeRoutes`, handlers, `RegisterInputHandler[T]`).
- **CRUD + soft-delete template** — `apps/fleet-service/internal/vehicle/` (Tasks 6.1–6.2).
- **Cross-domain orchestration** — fuel→mileage (Task 10.1); completion→record+mileage+advance (Task 9.3). Lives in processors, not handlers.
- **Frontend feature template** — `apps/web/.../vehicles` (Task 14.3): Service (extends `BaseService`) → query-key factory + RQ hooks → Zod schema in `lib/schemas/` → page + feature components → skeletons + `createErrorFromUnknown`+sonner → role-gated UI.

## Stable shared symbols (keep signatures consistent)

- `config.Get/MustGet/GetInt`
- `telemetry.NewLogger/InitTracer/CorrelationID/CorrelationIDFromContext`
- `database.Connect/SetMigrations/Query/SliceQuery/WithLeaderLock`
- `server.New/.Use/.AddRouteInitializer/.Run`, `server.WriteJSON/WriteError`, `server.RegisterInputHandler[T]`, `server.{Resource,Document}`, `server.{Page,PageMeta}` + `Page.Offset()/Meta(total)`, `server.Err{Unauthorized,Forbidden,NotFound,Conflict,Gone,Validation}` + `StatusFor`
- `health.Liveness/Readiness`
- `auth.Identity{UserID,Email,ActiveFleetID,Role}`, `auth.JWT/RequireRole/IdentityFromContext/WithIdentity`, `auth.NewJWKSKeyfunc`
- `events.Envelope`, `events.Producer`/`NoopProducer`/`NewKafkaProducer`, `events.{Enqueue,RelayOnce,MigrateOutbox,Consume}`
- `jobs.Every`
- fleet `authz.RequireSameFleet/RequireWrite/RequireOwner`
- `maintenanceschedule.{Schedule,Thresholds,DefaultThresholds,NextDue,DueState,Severity}`; `status.{Input,Derive}`; `fuel.DerivePrice`
- TS: `@myfleet/shared-ts` `ApiClient`, `createErrorFromUnknown`, `ApiError`, `JsonApiResource/Document/PageMeta`; `vehicleKeys` query-key shape.

## Conventions & gotchas

- **Error mapping (design §6):** 401 missing/invalid JWT · 403 role lacks permission · **404** resource in another fleet (never 403 — no existence leak) · 409 invite used/expired or sole-owner self-removal or same-email mismatch · 410 purged soft-deleted entity · 422 validation. Implemented once in `shared-go/server`.
- **Builder return type varies by domain:** no-invariant domains (auth `user`) use `Build() Model`; invariant-enforcing domains (`vehicle`, etc.) use `Build() (Model, error)` returning `server.ErrValidation`. Match the domain's needs.
- **GORM `TableName()` is schema-qualified** (`auth.users`, `fleet.vehicles`, …) to honor D2 isolation on a shared instance. Compose `init-schemas.sql` creates the schemas.
- **Events are stubbed early:** Phases 4–10 inject `events.NoopProducer{}`; Phase 11 swaps in the real Kafka producer + outbox relay and replaces emit calls. Don't wire Kafka before P11.
- **Owner-only mutations** re-check the membership table (authoritative) because the token role may be ≤15 min stale (design §9); media/notification trust the token claim.
- **Append-only tables** (`mileage_records`, `activity_events`): no update/delete; below-latest mileage is **flagged, not dropped** (design §14).
- **Status is never stored** — derived on read via `status.Derive` (design §10.2).
- **Gateway prefix:** Traefik routes `/api/<service>/*` to each service and strips the `/api/<service>` prefix; services register routes at their bare paths (`/auth/me`, `/fleets`, `/vehicles`, …). Confirm strip-prefix middleware in compose/k8s Traefik config. The web client calls full `/api/<service>/...` paths.
- **Internal endpoint** `GET /internal/memberships/active?user_id=` is registered **without** JWT middleware and must be network-restricted (not exposed via the public gateway path).

## Verification gates (CLAUDE.md "done" bar)

- Per backend module: `go build ./...`, `go vet ./...`, `go test -race ./...` clean.
- Frontend: `npm run -w apps/web build`, `npm run -w apps/web test` clean.
- Infra/CI: `docker compose config` parses; `kubectl kustomize` renders; workflow YAML/JSON valid.
- Full: `make ci`.
- **Before PR (mandatory):** `/audit-plan` + `superpowers:requesting-code-review`; findings → `audit.md`.

## External dependencies / setup needed at execution time

- **Google OIDC credentials** (`GOOGLE_CLIENT_ID/SECRET/REDIRECT_URL`) — required for auth login to function end-to-end; obtain from a Google Cloud OAuth client. Not committed; set in `deploy/compose/.env`.
- **RS256 private key** (`JWT_PRIVATE_KEY_PEM`) — generated locally via `scripts/gen-jwt-key.sh`; in k8s a Secret.
- **Docker** (compose v2) for local infra; a kube context only needed for the k8s `--dry-run` apply (rendering via `kubectl kustomize` works without one).
- Key Go deps: GORM (+postgres, +sqlite for tests), logrus, OTel, go-chi/chi v5, golang-jwt/jwt v5, MicahParks/keyfunc v3, segmentio/kafka-go, minio-go v7, google/uuid, golang.org/x/oauth2, google.golang.org/api/idtoken.
- Key TS deps: react, @tanstack/react-query v5, react-hook-form + @hookform/resolvers + zod, react-router-dom v6, sonner, tailwind, shadcn/ui, vite, vitest + @testing-library/react.
