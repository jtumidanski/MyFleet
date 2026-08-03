# Platform Admin Console — Implementation Context

Companion to `plan.md`. Read this first; it is the map the plan assumes.

---

## 1. What is being built

A platform-admin tier orthogonal to fleet membership, plus a `/admin` console in
`fleet-service` + `apps/web` that can inspect the whole platform and purge data
at three granularities (record / fleet / system) with a 5-day recovery window.

Authoritative documents, in order of precedence:

1. `design.md` — resolves all six PRD open questions and records four findings
   (F1–F4) that changed the work. **Where design.md and prd.md disagree,
   design.md wins** (its §14 lists all nine deviations).
2. `prd.md` — requirement ids (`FR-ADMIN-*`) referenced throughout the plan.
3. `risks.md` — R1–R10, re-scored in design.md §15.
4. `ui-directions.html` — the visual reference for every console screen
   (Direction C). Open it before building any UI task.

---

## 2. Repository shape

Go workspace with four services plus shared packages; one Vite/React SPA.

```
apps/auth-service/          users, sessions, OIDC, JWKS
apps/fleet-service/         fleets, vehicles, maintenance, fuel, mileage, activity, dashboards
apps/media-service/         media objects + variants, MinIO
apps/notification-service/  notifications, preferences, reminder job
apps/web/                   React SPA
packages/shared-go/         auth, config, database, events, health, jobs, server, telemetry
packages/shared-ts/         ApiClient, JSON:API types, error helpers
packages/dto-go/            cross-service event payloads
packages/ui-components/     StatusBadge, formatters
deploy/k8s/                 base + overlays/{local,main}
```

### Domain package convention (every Go domain package)

`entity.go` (GORM struct + `TableName()` + `Migration()` + `Make`/`ToEntity`),
`model.go` (immutable struct, unexported fields, getter methods),
`builder.go` (fluent constructor), `provider.go` (read interface),
`administrator.go` (write interface), `processor.go` (business logic),
`resource.go` (chi handlers + `InitializeRoutes`), `rest.go` (JSON:API
transform).

Wiring happens only in `cmd/main.go` — the composition root. Cross-domain
dependencies are injected as function values or small interfaces
(`vehicle.StatusDeps`, `maintenanceschedule.WithOverdueHooks`,
`NewCompletionDeps().WithActivityRecorder`). Never import a sibling domain's
concrete type across a service boundary.

---

## 3. Facts verified against source (do not re-derive)

| Fact | Evidence |
|---|---|
| `media.media_objects` HAS `fleet_id` (not null, indexed) | `apps/media-service/internal/mediaobject/entity.go:12` |
| `notification.notifications` HAS `fleet_id` (nullable) | `apps/notification-service/internal/notification/entity.go:20` |
| No foreign keys anywhere; nothing cascades | every entity uses plain indexed `uuid`, no `constraint:` tags |
| `database.Migration` is `func(*gorm.DB) error` — raw DDL is allowed | `packages/shared-go/database/database.go:11` |
| `jobs.Every` fires first at `T+interval`, never at startup | `packages/shared-go/jobs/scheduler.go:11-20` |
| `server.ParsePage` reads `page[number]` / `page[size]`, default 25, cap 100 | `packages/shared-go/server/pagination.go:20-33` |
| `server.Detailed(base, detail)` wraps a sentinel with a JSON:API `detail` | `packages/shared-go/server/errors.go:52` |
| `database.WithLeaderLock(db, name, fn)` is the advisory-lock wrapper | `packages/shared-go/database/lock.go:12` |
| `newPrincipalResolver` is the SOLE `session.Principal` construction site, enforced by an arch test | `apps/auth-service/cmd/main.go:120`, `apps/auth-service/internal/arch/arch_test.go:29` |
| `TestMintAccess_mapsEveryPrincipalField` `t.Fatalf`s on any non-string `Principal` field | `apps/auth-service/internal/session/processor_test.go:258-262` |
| `notifications-stripprefix` strips the FULL `/api/notifications` prefix | `deploy/k8s/base/routing/middlewares.yaml:45-52` |
| `internal-deny` today covers only fleet-service and media-service | `deploy/k8s/overlays/main/ingressroute.yaml:89,111` |
| `main` overlay's TLS twin copies `spec.routes` via `replacements` | `deploy/k8s/overlays/main/ingressroute.yaml:159-189` |
| Tests run on SQLite with `ATTACH DATABASE ':memory:' AS fleet` + hand-written DDL | `apps/fleet-service/internal/invite/resource_test.go:33-57`, `apps/fleet-service/internal/fuel/processor_test.go:38-72` |
| `vehicle.PurgeExpired` hard-deletes with no cascade (the §11 defect) | `apps/fleet-service/internal/vehicle/purge.go:22-24` |
| media-service runs a SECOND `purge_after` sweep that also deletes MinIO objects | `apps/media-service/cmd/main.go:109-117`, `internal/mediaobject/purge.go:22` |
| `fleet.dashboards` has NO real unique index despite its doc comment | `apps/fleet-service/internal/dashboard/entity.go:11-17` |
| `notification.ExistsByDedupeKey` counts without a `deleted_at` filter | `apps/notification-service/internal/notification/administrator.go:26-32` |
| `RequireAuth` redirects fleetless users to `/onboarding` | `apps/web/src/components/RequireAuth.tsx:29` |
| Theme tokens `danger-subtle` / `-subtle-foreground` / `-border` exist; `--destructive` is reserved for controls | `apps/web/src/index.css` (light `:root`, dark `.dark`) |
| Existing web deps include `@radix-ui/react-{label,select,slot,switch}` — `react-dialog` is NEW | `apps/web/package.json:15-18` |
| `make ci` = `lint-check vet test build fe-test fe-build manifests carfax-template` | `Makefile:48` |

---

## 4. The three structural decisions everything hangs off

### 4.1 One declarative purge manifest (design §3)

`apps/fleet-service/internal/admin/manifest.go` lists every purgeable table once.
Four generic operations run over it — `Count`, `Stamp`, `Restore`, `Reap` — and
they are the only code that writes purge state. `Count` and `Stamp` share the
same `Where`, which is what makes FR-ADMIN-UI-9 ("blast-radius counts match the
purge exactly") true by construction. `Restore` and `Reap` key purely on
`purge_operation_id`, which makes them scope-independent and idempotent.

**The ordering rule that replaces child-to-parent ordering:** a `Where`
predicate may reference parent tables but must **never** filter `deleted_at` on
them. The `AND deleted_at IS NULL` guard belongs to the *target* table only.
With that split, stamp order is irrelevant.

A source-parsing arch test asserts every `TableName()` literal in the service is
either in the manifest or in `excludedTables` with a reason.

### 4.2 Never write `purge_after` on an admin stamp (design F3)

Both legacy sweeps (`vehicle.PurgeExpired`, media's `ListPurgeable`) key on
`purge_after`. An admin stamp writes `deleted_at` + `purge_operation_id` and
nothing else, so those sweeps cannot see admin-stamped rows at all. The
`purge_operation_id IS NULL` narrowing required by FR-ADMIN-RESTORE-7 is applied
to **both** sweeps as belt-and-braces.

### 4.3 The admin route tree is a sibling, not a child (design §5, §10.1)

Server: `/admin` is its own chi group with `authmw.JWT` and nothing else shared;
no handler in it calls `RequireSameFleet`, and no handler outside it reads
`Identity.PlatformAdmin`. An arch test enforces both directions.

Client: the `/admin` branch is a sibling of `<RequireAuth><AppLayout/></RequireAuth>`
in `App.tsx`, so `RequireAuth`'s fleetless redirect never applies and
`RequireAuth.tsx` is not modified. This is R5's structural resolution.

---

## 5. Deviations this plan makes from design.md

Two, both small and argued in-line in `plan.md`:

1. **`RequirePlatformAdmin` lives in `internal/authz`, not `internal/admin`.**
   PRD FR-ADMIN-AUTH-6 names `authz.RequirePlatformAdmin` explicitly, and the
   guard sits naturally beside `RequireSameFleet` / `RequireOwner`. The R7 arch
   test (design §5.5) therefore allowlists `internal/authz/scope.go` and its
   test alongside `internal/admin/` as legitimate reference sites. The property
   the test protects — no *handler* outside `/admin` reads `PlatformAdmin` — is
   unchanged.

2. **`Stamp` takes an explicit `now time.Time` instead of emitting SQL `now()`.**
   SQLite (the whole test harness, design F4) has no `now()`. Every purge test
   would otherwise be unrunnable. The value is `time.Now().UTC()` at the single
   call site.

Two further mechanical notes the design leaves implicit:

- **GORM index renames.** Flipping `uniqueIndex:idx_fleet_user` to `index` and
  then dropping/recreating the index in the same `Migration` produces index
  churn on every boot (AutoMigrate recreates what the DDL just dropped). The
  plan instead gives each GORM-managed index a **new name** and has the DDL drop
  only the **legacy** name, so both steps are idempotent. See Task 1.
- **Nullable uuid columns must be `*string`.** Postgres rejects `''` for a
  `uuid` column, so `purge_operations.target_id` and every audit-row uuid that
  can be absent are `*string`.

---

## 6. Sequencing and why it matters

The plan follows design §13. Tasks 1–8 have no visible payoff and are the ones
most likely to be deferred; they are the foundation:

| Tasks | What | Why first |
|---|---|---|
| 1–2 | Schema: `deleted_at` + `purge_operation_id` + partial unique indexes | nothing can be soft-deleted safely until the lockout risk (R2/F1) is closed |
| 3 | `admintest.NewDB` shared DDL harness | every later test needs the full schema in one database (F4) |
| 4–6 | The data-visibility sweep + per-domain regression tests | adding `deleted_at` silently changes the meaning of every existing query (R1) |
| 7 | Manifest + four operations + completeness arch test | the single source of truth for counting, stamping, restoring, reaping |
| 8 | Orphan cleanup + `PurgeExpired` cascade + both sweeps narrowed | until this lands `/admin/stats` reports numbers no fleet can reconcile (R10) |
| 9–12 | Auth tier: table, seed, claim, guard, `/auth/me`, internal routes | |
| 13–14 | media + notification internal admin routes **with their `internal-deny` rules** | F2: without the rule these are unauthenticated internet-reachable delete endpoints |
| 15–20 | Clients, purge lifecycle, reaper, read endpoints | |
| 21 | Route-group wiring + ConfigMaps | |
| 22–28 | Web console | |
| 29 | Full `make ci` + both overlay dry-runs | |

---

## 7. Files that will be created (new)

```
apps/fleet-service/internal/admin/
  manifest.go  manifest_test.go  arch_test.go
  operations.go  operations_test.go
  orphans.go  orphans_test.go
  entity.go  model.go  builder.go  provider.go  administrator.go
  processor.go  processor_test.go  resource.go  rest.go
  confirmation.go  confirmation_test.go
  stats.go  stats_test.go
  reaper.go  reaper_test.go
  admintest/db.go
apps/fleet-service/internal/adminclient/
  http.go  auth.go  media.go  notification.go  client_test.go
apps/auth-service/internal/platformadmin/
  entity.go  provider.go  administrator.go  seed.go  seed_test.go  resource.go
apps/media-service/internal/admin/
  manifest.go  operations.go  resource.go  arch_test.go  admin_test.go
apps/notification-service/internal/admin/
  manifest.go  operations.go  resource.go  arch_test.go  admin_test.go
apps/web/src/components/ui/{dialog,table,badge}.tsx
apps/web/src/components/admin/{AdminLayout,RequirePlatformAdmin,PurgeConfirmDialog,BlastRadiusPanel}.tsx
apps/web/src/pages/admin/{AdminOverviewPage,AdminFleetsPage,AdminUsersPage,AdminPurgesPage,AdminAuditPage}.tsx
apps/web/src/services/api/AdminService.ts
apps/web/src/lib/hooks/api/admin.ts
apps/web/src/lib/admin/purgeStatus.ts
apps/web/src/types/models/admin.ts
```

---

## 8. Build and verification

```sh
make ci                                   # the gate
make test                                 # go test -race ./...
go test ./apps/fleet-service/internal/admin/...   # while iterating
export NVM_DIR="$HOME/.nvm" && . "$NVM_DIR/nvm.sh" && nvm use 22   # if npm is missing
npm run -w apps/web test
kustomize build deploy/k8s/overlays/main  | kubectl apply --dry-run=server -f -
kustomize build deploy/k8s/overlays/local | kubectl apply --dry-run=server -f -
```

`make manifests` runs `tools/check-manifests.sh`, which asserts the `main`
overlay renders with no PVCs, Secrets, ClusterRole, or placeholders. The local
overlay dry-run is **not** optional — CLAUDE.md records a `namespace:` omission
that slipped through ten reviews because only the `main` dry-run was ever run.
