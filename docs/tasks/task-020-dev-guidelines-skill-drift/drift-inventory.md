# Drift Inventory — Verified Evidence

Compiled during specification on 2026-08-03 against `main` @ `d3c9eaa`. Every row was checked against repository source; nothing here is inferred from issue #23's text alone.

This is a *starting* inventory, not a complete one. Files marked **unverified** below still need a pass during design.

## A. Backend — confirmed drift

| # | Skill claims | Repository has | Evidence |
| --- | --- | --- | --- |
| A1 | `services/<service-name>/` | `apps/<service-name>/` | `apps/{auth,fleet,media,notification}-service`, `apps/web`; no `services/` dir exists |
| A2 | api2go library handles the JSON:API envelope | Hand-rolled `Resource` / `Document` structs | `packages/shared-go/server/jsonapi.go:12-24`; zero `api2go` refs in `go.work.sum` or any source |
| A3 | `jsonapi.ServerInformation` | Does not exist | no matches in `apps/` or `packages/` |
| A4 | `InitializeRoutes(si jsonapi.ServerInformation) func(*gorm.DB) server.RouteInitializer` | `InitializeRoutes(log, db, deps...) func(chi.Router)` | `apps/fleet-service/internal/vehicle/resource.go:29` |
| A5 | `server.RegisterHandler(l)(si)("name", h)` | No `RegisterHandler` at all; routes go directly on chi | `packages/shared-go/server/handler.go` (whole file); `resource.go:33` uses `r.Get(...)` |
| A6 | `RegisterInputHandler` is curried | `RegisterInputHandler[T any](fn func(http.ResponseWriter, *http.Request, T)) http.HandlerFunc` | `packages/shared-go/server/handler.go:42` |
| A7 | `server.MarshalResponse[T](l)(w)(si)(map[string][]string{})(res)` | `server.WriteJSON(w, status, body)` / `server.WriteError(w, err)` | `packages/shared-go/server/jsonapi.go:26` |
| A8 | `Transform(Model) (RestModel, error)`, `TransformSlice([]Model) ([]RestModel, error)` | `Transform(m Model) server.Resource`, `TransformSlice(ms []Model) []server.Resource` — no error return, no `RestModel` | `apps/fleet-service/internal/vehicle/rest.go:58,97` |
| A9 | `GetName()` must use a value receiver (api2go interface) | No `GetName` anywhere; JSON:API `type` is a literal field | `rest.go:76` (`Type: "vehicles"`) |
| A10 | `model.Provider[T]`, `Map`, `SliceMap`, `ParallelMap`, `ErrorProvider`, `FixedProvider`, `database.EntityProvider` | **None exist.** Real pattern: eager per-domain `Provider` interface returning `(Model, error)` | `apps/fleet-service/internal/vehicle/provider.go:13-16`; grep for each identifier returns nothing |
| A11 | `NewProcessor(l logrus.FieldLogger, ctx context.Context, db *gorm.DB)` | `NewProcessor(log logrus.FieldLogger, p Provider, a Administrator, ...)` | `apps/fleet-service/internal/maintenancerecord/processor.go:19`; option variant at `apps/media-service/internal/mediaobject/processor.go:202` |
| A12 | `server.New(logger)...Run()` startup | `New` / `Use` / `AddRouteInitializer(func(chi.Router))` / `Router()`; no `Run()` | `packages/shared-go/server/handler.go:11-45` |
| A13 | Entity IDs are `uint32` | String UUIDs | `apps/fleet-service/internal/vehicle/builder.go:13` (`uuid.NewString()`) |
| A14 | Examples use buckets / policies / replication | Vehicles, fleets, maintenance, media, invites | 24 `bucket` hits / 4 files, 22 `policy` / 3, 2 `replication` / 2 |
| A15 | `SKILL.md` declares `name: golang-microservice` | Directory is `backend-dev-guidelines`; `skill-rules.json` keys it by directory name | `.claude/skills/backend-dev-guidelines/SKILL.md:2` |

## B. Frontend — confirmed drift

| # | Skill claims | Repository has | Evidence |
| --- | --- | --- | --- |
| B1 | Jest (`jest.mock`, Jest config) | Vitest | `apps/web/package.json:8` (`"test": "vitest run"`), `:56`; 25 `jest` hits across `testing-guide.md`, `anti-patterns.md` |
| B2 | `services/api/bans.service.ts`, `buckets.service.ts` | PascalCase `VehicleService.ts`, `MediaService.ts`, … | `apps/web/src/services/api/` listing |
| B3 | Types re-exported from `services/api/index.ts` | **No `index.ts` exists** in that directory | `patterns-types.md:207`, `ai-guidance.md:72` vs directory listing |
| B4 | Examples use buckets / bans | Vehicles, fleets, maintenance, fuel, mileage, media, invites, members, notifications | 129 `bucket` hits / 9 files, 81 `ban` / 5 |

## C. Verified accurate — preserve

- Layer model and file roles: `model.go`, `entity.go`, `builder.go`, `processor.go`, `provider.go`, `resource.go`, `rest.go` — all present in `apps/fleet-service/internal/vehicle/`.
- Immutability: private fields, value-receiver accessors, `With*` copy-returning mutators (`vehicle/model.go:30-58`).
- Builder: fluent `*Builder`, validation in `Build() (Model, error)` (`vehicle/builder.go:13-26`).
- `logrus.FieldLogger` (interface) over `*logrus.Logger` (concrete) in processor constructors — holds across every real `NewProcessor`.
- `TransformSlice` must exist alongside `Transform`; list handlers must not inline transform loops (`vehicle/rest.go:97`).
- POST/PATCH bodies via `RegisterInputHandler[T]`; the breaking-change warning at `patterns-rest-jsonapi.md:56` is a real hazard.
- Frontend `services/api/` path (accurate) and `BaseService` inheritance, FE-11 (`apps/web/src/services/api/BaseService.ts:22`).
- Frontend `skill-rules.json` trigger `**/services/api/**/*.ts` — matches real paths, leave alone.

## D. Real patterns worth *adding*

Present in the code, absent from the guidelines:

- `db.Save` UPDATEs every column, so a field the Model does not carry is silently zeroed on any write. Caught by `entityguard`. See the comment at `apps/fleet-service/internal/vehicle/model.go:20-27` — this is a live, documented defect class.
- Narrow named request structs (`createAttributes`, `patchAttributes`) as the read-only enforcement mechanism; pointer fields on patch structs distinguish absent from zero (`vehicle/rest.go:33-52`).
- `gorm.ErrRecordNotFound` → domain `ErrNotFound` translation at the provider boundary (`vehicle/provider.go:26-30`).
- `server.Page` pagination threaded through provider list methods (`vehicle/provider.go:15`).
- `SetErrorLogger` / `WriteError` 5xx redaction: the real implementation of what SEC-09 describes (`packages/shared-go/server/jsonapi.go:38-66`).

## E. Unverified — design phase must inventory

- `backend-dev-guidelines/resources/testing-guide.md` (267 lines) — not checked against real `*_test.go` conventions.
- `backend-dev-guidelines/resources/ai-guidance.md` (277 lines) — only the drift greps were run; the rest of the checklist is unaudited.
- All frontend resource files except `testing-guide.md`, `patterns-types.md`, `patterns-service-layer.md`, `anti-patterns.md`, `architecture-overview.md` — specifically `patterns-react-query.md`, `patterns-forms-validation.md`, `patterns-styling.md`, `patterns-routing.md`, `patterns-components.md`, `patterns-api-client.md`.
- `.claude/agents/backend-guidelines-reviewer.md` lines 7, 9, 24, 39 carry `services/auth-service`. Out of scope per the scoping decision; see PRD §9.
