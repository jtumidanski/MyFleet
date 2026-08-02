# Backend Audit — fleet-service (task-012)

- **Scope:** Go files in `git diff main...HEAD` — `apps/fleet-service/internal/maintenancecategory/` (entity, model, provider, processor, administrator, rest, resource), `internal/maintenancerecord/resource.go`, `internal/mileage/{provider,resource}.go`, `cmd/main.go`, `packages/shared-go/database/database.go`
- **Guidelines Source:** `backend-dev-guidelines` skill
- **Date:** 2026-08-02
- **Build:** PASS (`go build ./...` in `apps/fleet-service`, exit 0)
- **Tests:** PASS (`maintenancecategory`, `mileage`, `maintenancerecord` — all `ok`, `-count=1`)
- **Overall:** NEEDS-WORK

Build and tests are clean. Two blocking checklist violations remain, both in
`maintenancecategory` and both traceable to the same handler.

## Domain Checklist Results

### maintenancecategory (domain package — has `model.go`)

| ID | Check | Status | Evidence |
|----|-------|--------|----------|
| DOM-01 | `builder.go` exists | **FAIL** | No `builder.go` in the package. `processor.go:81` builds the Model with a raw struct literal `pr.a.Insert(Model{id: ..., name: ..., kind: ..., fleetID: ...})` |
| DOM-02 | `ToEntity()` method | PASS | `entity.go:57` `func (m Model) ToEntity() Entity` |
| DOM-03 | `Make(Entity)` function | PASS | `entity.go:43` `func Make(e Entity) Model` — no error return, matching all 11 sibling domains (see note below) |
| DOM-04 | `Transform` function | PASS | `rest.go:16` `func Transform(m Model) server.Resource` |
| DOM-05 | `TransformSlice` function | PASS | `rest.go:30`; list handler uses it at `resource.go:44`, no inline loop |
| DOM-06 | Processor accepts `FieldLogger` | PASS | `processor.go:32` `NewProcessor(log logrus.FieldLogger, p Provider, a Administrator)` |
| DOM-07 | Handlers pass injected logger | PASS | `resource.go:21` passes `log` (the `logrus.FieldLogger` param at `resource.go:20`); zero `logrus.StandardLogger()` in the package |
| DOM-08 | POST uses `RegisterInputHandler` | **FAIL** | `resource.go:52` registers a bare `func(w, req)`; body decoded by hand at `resource.go:64-69` via `json.NewDecoder` |
| DOM-09 | Transform errors handled | PASS (N/A) | `rest.go:16` `Transform` returns no error, so there is none to discard |
| DOM-10 | Providers use lazy evaluation | N/A | This codebase has no `database.Query`/`database.SliceQuery`; all 12 fleet-service domains use interface-based providers. Not a task-012 deviation |
| DOM-11 | No `os.Getenv()` in handlers | PASS | Zero matches in `resource.go` |
| DOM-12 | No cross-domain logic in handlers | PASS | Both handlers call only `proc.List` (`resource.go:37`) / `proc.Create` (`resource.go:80`) |
| DOM-13 | Handlers don't call providers directly | PASS | `NewProvider(db)` appears only at `resource.go:21` as processor wiring, mirroring `maintenancerecord/resource.go:56` |
| DOM-14 | No direct entity creation in handlers | PASS | Zero `db.Create`/`db.Save`/`db.Delete` in `resource.go` |
| DOM-15 | `administrator.go` exists for writes | PASS | `administrator.go:20-34`; the only `db.Create` is `administrator.go:30` |
| DOM-16 | Domain error → HTTP status mapping | PASS | `server.WriteError` at `resource.go:32,41,55,60,71,77,83` maps via `server.StatusFor` (`packages/shared-go/server/errors.go:17-40`). Uses this codebase's 422-for-validation convention (`errors.go:35`) rather than the guideline's 400 |
| DOM-17 | JSON:API interface on REST models | N/A | This codebase emits `server.Resource{Type, ID, Attributes}` (`rest.go:16-27`) across all 12 domains; there is no api2go layer requiring `GetName()`/`GetID()`/`SetID()` |
| DOM-18 | Request models use flat structure | **FAIL** | `rest.go:41` `CreateAttributes` is correctly flat, but `resource.go:64-68` wraps it in a hand-rolled `struct{ Data struct{ Attributes CreateAttributes } }` — the nested Data/Attributes envelope the guideline forbids |
| DOM-19 | Table-driven tests | WARN | 25 test functions across `entity_test.go`, `provider_test.go`, `processor_test.go`; zero use `t.Run` or a `[]struct` table. Coverage is thorough, style is one-func-per-case |

**DOM-03 note.** The checklist's pass criterion is `(Model, error)`. Every domain
in this service returns a bare `Model` (`grep '^func Make(' internal/*/entity.go`
— 11 of 11). Scored PASS as consistent with the codebase, not with the generic
guideline text.

### DOM-01 detail

`maintenancecategory` is the only domain in `apps/fleet-service/internal/` that
both writes models and lacks a builder. Ten siblings have `builder.go`
(`activity`, `fleet`, `fuel`, `invite`, `maintenancerecord`,
`maintenanceschedule`, `membership`, `mileage`, `vehicle`, `vehiclemedia`), and
every write path in the service goes through `NewBuilder()` —
`vehicle/resource.go:82`, `fuel/resource.go:108`,
`maintenancerecord/resource.go:178`, `mileage/resource.go:115`,
`membership/administrator.go:41`, `activity/administrator.go:47`, and others.

A repo-wide grep for raw model literals on write paths returns exactly one hit:

```
maintenancecategory/processor.go:81:	created, err := pr.a.Insert(Model{
```

The validation this branch added (`processor.go:62-71`: non-empty fleetID,
trimmed non-empty name, ≤60 runes, kind in the enum) is the invariant set that
`Build()` is specified to enforce (`file-responsibilities.md:22-24`,
`patterns-functional.md:16-25`). It currently lives in the processor and is
reachable only through `Create`; any future second write path re-implements it or
silently skips it.

### DOM-08 / DOM-18 detail

`resource.go:52-72`:

```go
r.Post("/maintenance-categories", func(w http.ResponseWriter, req *http.Request) {
	...
	var body struct {
		Data struct {
			Attributes CreateAttributes `json:"attributes"`
		} `json:"data"`
	}
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
```

This is the only hand-rolled JSON:API envelope in the fleet service. Every other
body-carrying endpoint uses `server.RegisterInputHandler` —
`vehicle/resource.go:61,125,225`, `fuel/resource.go:66,153`,
`mileage/resource.go:79`, `maintenanceschedule/resource.go:60,119,193`,
`maintenancerecord/resource.go:108,225`, `fleet/resource.go:35,69`,
`invite/resource.go:39`, `vehiclemedia/resource.go:45`, `dashboard/resource.go:71`.
The two other bare `r.Post` handlers (`vehicle/resource.go:191` restore,
`invite/resource.go:149` accept) are body-less action endpoints, which
`patterns-rest-jsonapi.md:69-71` explicitly exempts. This one carries a body, so
it is not exempt.

**The fix is wire-compatible.** `server.RegisterInputHandler`
(`packages/shared-go/server/handler.go:42-54`) decodes exactly
`{data:{attributes:T}}` and returns `server.ErrValidation` on a decode failure —
byte-for-byte what `resource.go:64-72` does by hand. Passing `CreateAttributes`
as `T` produces an identical request contract and identical error behaviour, so
`patterns-rest-jsonapi.md:56-73`'s warning about `RegisterInputHandler`
conversions breaking frontend callers does **not** apply here. No frontend change
is required.

## Sub-Domain Checklist Results

No sub-domain (action-event) packages were added or modified on this branch.
`maintenancecategory` has `model.go` and is scored as a full domain above.

## Security Review — tenant isolation

The central concern for this branch: every read and write path scoped to the
caller's active fleet, and the no-active-fleet branch handled safely.

| ID | Check | Status | Evidence |
|----|-------|--------|----------|
| SEC-A | Read path scoped to caller's fleet | PASS | `provider.go:33-43` `visibleTo` constrains every read to `fleet_id IS NULL OR fleet_id = ?`; applied at `provider.go:48,49` (List), `provider.go:73` (IDsByKind), `provider.go:98` (FindByName) — all three Provider methods, no unscoped query |
| SEC-B | Fleet ID comes from identity, never the body | PASS | `resource.go:37` and `resource.go:80` pass `identity.ActiveFleetID` from `auth.IdentityFromContext` (`resource.go:25,53`). `CreateAttributes` (`rest.go:41-44`) has only `Name` and `Kind` — no fleet field to spoof, documented at `rest.go:38-40` |
| SEC-C | `fleetID == ""` handled safely on reads | PASS | `provider.go:34-41` returns `fleet_id IS NULL` — system rows only, never a bind of `""` into a uuid column and never an unfiltered scan. Verified by `provider_test.go:191-220` |
| SEC-D | `fleetID == ""` handled safely on writes | PASS | Two independent guards: `resource.go:59-62` rejects at the HTTP layer with `ErrValidation`; `processor.go:62-64` rejects again for any caller bypassing the handler. Covered by `processor_test.go:156` |
| SEC-E | Write authorization | PASS | `resource.go:54-57` `authz.RequireWrite(identity)` → 403 for viewers (`authz/authz.go:20-25`) before any body read |
| SEC-F | Cross-fleet leakage via record `?kind=` filter | PASS | `maintenancerecord/resource.go:83` passes `v.FleetID()`, but only after `authz.RequireSameFleet(identity, v.FleetID())` at `maintenancerecord/resource.go:68`, which returns 404 unless `identity.ActiveFleetID == v.FleetID()` and is non-empty (`authz/authz.go:12-17`). The value is therefore provably the caller's own fleet |
| SEC-G | Fleet isolation proven by test | PASS | `provider_test.go:108-148` (fleet B cannot see fleet A's custom row, both still see system rows), `provider_test.go:152-180` (`IDsByKind` bounded per fleet), `processor_test.go:89` (same name in another fleet stays distinct) |
| SEC-H | No hardcoded secrets | PASS | Zero credential literals in the changed files; `database.go:28` still reads `config.MustGet("DATABASE_URL")` |

The GET list endpoint (`resource.go:24`) deliberately has no role guard — any
authenticated caller may list. That is safe and intentional
(`resource.go:16-19`): the result set is already bounded by
`identity.ActiveFleetID` through `visibleTo`, and an unauthenticated context
yields a zero `auth.Identity` (`packages/shared-go/auth/identity.go:20-25`),
i.e. `ActiveFleetID == ""`, which falls into the system-rows-only branch. There
is no input that widens the result set beyond system rows plus the caller's own.

Tenant isolation is sound. No blocking security finding.

## Summary

### Blocking (must fix)

- **DOM-01** — `maintenancecategory` has no `builder.go`. `processor.go:81`
  constructs the Model with a raw struct literal; it is the only write-path model
  literal in the service, and the invariants it should enforce currently live
  loose in `processor.go:62-71`.
- **DOM-08** — `resource.go:52` registers POST `/maintenance-categories` as a
  bare handler instead of `server.RegisterInputHandler[CreateAttributes]`, with
  manual `json.NewDecoder` at `resource.go:69`. Only hand-decoded body in the
  service. Fix is wire-compatible — no frontend change needed.
- **DOM-18** — `resource.go:64-68` hand-rolls the nested `Data/Attributes`
  envelope. Same root cause as DOM-08; resolved by the same fix, since the flat
  `CreateAttributes` (`rest.go:41`) already exists and is exactly the `T` the
  typed handler needs.

### Non-Blocking (should fix)

- **DOM-19** — No table-driven tests in `maintenancecategory` (25 test functions,
  zero `t.Run`). `testing-guide.md:39` states a preference, not a requirement;
  `maintenanceschedule/recurrence_test.go`, `invite/processor_test.go`,
  `dashboard/processor_test.go` and `status/derive_test.go` are the in-repo
  precedent.
- **Untested branch, self-disclosed.** `provider.go:34-41`'s `fleetID == ""`
  branch is not regression-guarded — SQLite does not type-check bind parameters,
  so deleting the branch leaves the suite green while breaking PostgreSQL with
  SQLSTATE 22P02. Honestly documented at `provider_test.go:182-190`; noted here
  only so it is not lost.
- **No handler-level test for the new POST.** There is no
  `maintenancecategory/resource_test.go`. `processor.go` is well covered
  (`processor_test.go:27-233`), but the resource layer — precisely where DOM-08
  and DOM-18 sit — has zero coverage. `invite/resource_test.go` is the in-repo
  precedent for testing a handler.
- **Pagination ordering has no tiebreak.** `mileage/provider.go:38`
  (`recorded_at desc`) and `maintenancecategory/provider.go:60` (`name asc`) page
  with `Offset`/`Limit` over a non-unique sort key, so rows with equal keys can
  repeat or vanish across page boundaries. Consistent with every sibling
  (`fuel/provider.go:45`, `maintenancerecord/provider.go:67`,
  `activity/provider.go:43`), so this is a pre-existing service-wide pattern, not
  a task-012 regression — recorded for completeness only.

### Verified clean (no action)

- `database.go:28` — `TranslateError: true` is what makes
  `processor.go:88`'s `errors.Is(err, gorm.ErrDuplicatedKey)` race recovery work
  without importing a cgo-gated driver error type. Correctly documented at
  `database.go:28-31` and `administrator.go:7-19`.
- `cmd/main.go:168` — the second `NewProcessor` correctly gained the
  `NewAdministrator(db)` argument; the `CategoryAccessor` interface change
  (`maintenancerecord/resource.go:34`) has its one call site updated
  (`maintenancerecord/resource.go:83`).
- `mileage/provider.go:38` `asc` → `desc` aligns mileage with its fuel and
  maintenance-record siblings; the doc comments at `mileage/provider.go:13-14`
  and `mileage/resource.go:32` were both updated to match.

---

# Plan Adherence Audit — task-012-vehicle-detail-redesign

**Plan Path:** docs/tasks/task-012-vehicle-detail-redesign/plan.md
**Audit Date:** 2026-08-02
**Branch:** task-012-vehicle-detail-redesign (34 commits ahead of main, clean tree)
**Base Branch:** main
**Reviewer:** plan-adherence-reviewer (this section is independent of the backend-guidelines audit above)

## Executive Summary

All 17 tasks are implemented. The plan's checkboxes were never ticked, so every task was
verified against `git diff main...HEAD` and the working tree instead; 17/17 are DONE, with 0
SKIPPED and 0 PARTIAL. The three deliverables flagged as easy to silently skip were all
genuinely done: the Task 6 Step 4 consumer sweep (every pre-existing consumer of the three
list hooks lived inside the three section components deleted in Task 17; all four surviving
consumers read `data?.rows`), the Task 7 dedupe-by-id (`apps/web/src/lib/vehicleRecords.ts:83,110`
plus three dedicated tests), and the Task 17 Step 3 grep (six residual hits, all in
explanatory comments — zero imports or JSX references). Backend build, vet and package tests
pass; the 97 frontend tests covering the new modules pass. Several deliberate deviations from
the plan's reference code are documented below; one real gap remains — `isFetchingNextPage`
is produced by `useVehicleRecords` but never consumed, so the Load more button is still not
disabled while a page is in flight.

## Task Completion

| # | Task | Status | Evidence / Notes |
|---|------|--------|------------------|
| 1 | Fleet-scope the maintenance category domain | DONE | `apps/fleet-service/internal/maintenancecategory/entity.go:35` (`FleetID *string`, plus an unplanned `uniqueIndex:idx_maintenance_categories_scope`), `entity.go:50` (`Make` wires `fleetID`), `model.go:41,49`, `provider.go:33` (`visibleTo`), `provider.go:45,71` (`List`/`IDsByKind` take `fleetID`), `processor.go:37,44`, `resource.go` GET passes `identity.ActiveFleetID`, `maintenancerecord/resource.go:34,83` (interface widened; call site passes `v.FleetID()` — the only `IDsByKind(` call site in the file). Both planned tests exist: `provider_test.go:108` `TestListScopesToFleet`, `:152` `TestIDsByKindScopesToFleet`, plus unplanned `:191` (no-active-fleet) and `:235`. |
| 2 | Create custom categories | DONE (deviation) | `provider.go:84` `FindByName`; `rest.go:41` `CreateAttributes`; POST route in `resource.go` with `authz.RequireWrite` + empty-fleet guard; `processor.go:61` `Create`. **Deviation:** the plan's `Provider.Create(e Entity)` was replaced by a separate write interface — `administrator.go` `Administrator.Insert(Model)` — matching the project's DOM guidelines; `cmd/main.go:168` wires `NewAdministrator(db)`. Hardening beyond plan: `fleetID == ""` rejected at the processor, `maxCategoryNameLen` counted in runes (`processor.go:20,66`), and `gorm.ErrDuplicatedKey` race recovery (requires `TranslateError: true`, added at `packages/shared-go/database/database.go:28`). All 7 planned `TestCreate_*` tests present in `processor_test.go` plus 4 unplanned (`:156`, `:166`, `:212`). |
| 3 | Frontend category create service and hook | DONE | `apps/web/src/types/models/maintenanceCategory.ts` (`CreateMaintenanceCategoryAttributes`), `services/api/MaintenanceCategoryService.ts` (BaseService generic widened), `lib/hooks/api/maintenance.ts:101` `useCreateMaintenanceCategory` invalidating `maintenanceCategoryKeys.all`. |
| 4 | CategoryCombobox | DONE | Deps in `apps/web/package.json:15,17,24` (`@radix-ui/react-dialog`, `@radix-ui/react-popover`, `cmdk`); `components/ui/popover.tsx`, `components/ui/command.tsx`; `components/features/vehicles/CategoryCombobox.tsx`. Test file has 7 tests (the 4 planned plus a create-path trio); a `ResizeObserver` stub was added to `src/test/setup.ts:86-100` for cmdk. |
| 5 | Wire the combobox into the maintenance forms | DONE | `maintenance/MaintenanceRecordForm.tsx:13,94-96` (combobox replaces the Select; `kind` made **required** at `:32`, tightening the plan), `MaintenanceScheduleForm.tsx:61-63` (`kind="maintenance"`; its own Select remains only for recurrence type at `:79`). `MaintenanceRecordForm.test.tsx:24` adds the `scrollIntoView` stub, `:44` wraps renders in `QueryClientProvider`, `:48` is the rewritten kind-filter assertion. |
| 6 | Honest pagination in the list hooks | DONE (deviation) | `lib/hooks/api/pageSize.ts`; `services/api/MaintenanceRecordService.ts:30-42` and `services/api/FuelService.ts:27-34` gain `page`/`pageSize`; all three hooks converted to `useInfiniteQuery` (`mileage.ts:57`, `fuel.ts:42`, `maintenance.ts:124`). **Step 4 consumer sweep verified:** on `main` the only consumers were `VehicleFuelSection.tsx:32-33`, `VehicleMaintenanceSection.tsx:55,60` and `VehicleMileageSection.tsx:38` — all three files are deleted in Task 17. Every surviving consumer reads `.rows`: `VehicleTrends.tsx:28`, `CompleteScheduleDialog.tsx:49`, `VehicleRecordDrawer.tsx:100`, `vehicleRecords.ts:60,74,86`. Step 5 accumulation test at `lib/hooks/api/mileage.test.ts:119`. **Deviation:** `total` is read from the *last* page rather than `data.pages[0]`, documented inline (`mileage.ts:75-78`, `fuel.ts:57-59`). |
| 7 | The record merge (pure function) | DONE | `lib/vehicleRecords.ts` with `mergeVehicleRecords`, `filterVehicleRecords`, `RecordSource`, `MergeResult`, `VehicleRecordRow` (incl. Task 9's `gallons?`, `:26`). **Dedupe-by-id requirement met:** `:83` `dedupeById(sources.flatMap(...))`, helper at `:110-119`, and the watermark is deliberately computed from each source's pre-dedupe `s.rows` (`:94`, rationale at `:70-77`). Tests: 13 passing, including `vehicleRecords.test.ts:94` (within-source duplicate), `:107` (cross-source duplicate) and `:129` (watermark survives an all-duplicate source). |
| 8 | useVehicleRecords | DONE (extended) | `lib/hooks/api/vehicleRecords.ts:50`. Takes `CategoriesQueryState` rather than a bare array (plan amended by commit 4e87975), folds `categoriesQuery.isLoading` into `isLoading` (`:127`) and exposes `categoriesError` (`:130`). `loadMore` is a `useCallback` (`:150`) and the whole return value is memoised (`:166`). `isFetchingNextPage` is exposed at `:147` — but see Gaps: nothing consumes it. Unplanned test file `vehicleRecords.test.ts` (16 tests). |
| 9 | Stat derivations (pure functions) | DONE | `lib/vehicleStats.ts:11` `deriveOdometer`, `:25` `deriveTrailingCost`, `:53` `deriveAvgEconomy`, `deriveNextService` + exported `rankSchedule` switching on `status` (not `severity`) as the plan requires. `vehicleStats.test.ts` has 31 tests, well beyond the planned set (adds negative-delta / equal-reading leg handling). |
| 10 | Dialog and sheet primitives | DONE (deviation) | `components/ui/dialog.tsx`, `components/ui/sheet.tsx` (`side` variant API retained). **Deviation:** both use Radix's standard `bg-black/80` scrim, which the repo's `no hardcoded palette classes` convention test forbids; rather than adding an `--overlay` token, `src/test/conventions.test.ts:117-134` gained a two-line file+exact-text allowlist. The guardrail still fires for any other palette usage, including new lines in those same two files. |
| 11 | Dialog wrappers for the existing forms | DONE | All seven exist under `components/features/vehicles/dialogs/`: `EditVehicleDialog`, `LogMileageDialog`, `LogFuelDialog`, `LogMaintenanceDialog` (calls `useMaintenanceCategories()`), `AddScheduleDialog` (maintenance-only filter at `:28`), `CompleteScheduleDialog` (new react-hook-form + Zod form, `:1-3,54-55`), `DeleteVehicleDialog` (`navigate('/vehicles', { replace: true })` at `:34`). `FuelForm` gained an optional `defaultValues` prop so the drawer can prefill edits. |
| 12 | Identity rail and quick actions | DONE | `detail/VehicleIdentityRail.tsx` — primary photo from the media list (`:58`), title + `StatusBadge` (`:74-75`), spec line (`:77`), `<dl>` Odometer/VIN/Notes with `—` fallbacks (`:80-95`), 3-tile strip with `+N` (`:22,97-127`) reusing `VehiclePhotoThumbnail`, Edit gated on `canWrite` (`:129`). `detail/VehicleQuickActions.tsx:40` — `flex-row flex-wrap … lg:flex-col`, delete gated on `canDelete`. |
| 13 | Stat strip and schedule tiles | DONE | `detail/VehicleStatStrip.tsx:82` (`grid-cols-[repeat(auto-fit,minmax(150px,1fr))]`, `—` fallbacks, `partial` → "based on recent records" at `:91,100`, `text-danger`/`text-warning` at `:74-79`). `detail/UpcomingScheduleStrip.tsx:46` sorts with the imported `rankSchedule`, colors on `status` (`:64-70`, `border-danger-border bg-danger-subtle/45`), renders `SeverityChip` on `severity`, and takes resolved `categoryNames` as a prop (`:15`) so it stays presentational. |
| 14 | Records table | DONE (deviation) | `detail/VehicleRecordsTable.tsx` — five chips with `aria-pressed` (`:19-25,128`), Date/Type/Item/Odometer/Cost columns, clickable rows with `role="button"` + keyboard handler (`:162-171`), skeletons (`:64`), "No records yet." (`:154`), Load more only when `hasMore` (`:195`). Violet modification badge and its "Intentional status colors" comment moved from the deleted section (`:40-41`). All 6 planned tests pass. **Deviation:** the footer reads `Showing X of Y` only under "All"; under a narrowing chip it reads `Showing X matching <Kind>` (`:110-113`) because `total` is cross-source — a documented correctness improvement over the plan's unconditional wording. |
| 15 | Record drawer | DONE | `lib/hooks/api/maintenance.ts:178` `useUpdateMaintenanceRecord`. `detail/VehicleRecordDrawer.tsx` — `Sheet side="right"` (`:361`), per-kind `enabled` queries (`:89-100`), `RecordAttachmentList` (`:266`), Edit/Delete for maintenance (`:272,281`) and fuel (`:327,336`). **`kind` is passed explicitly** to `MaintenanceRecordForm` (`:219`), the requirement added by commit ec89fa5. Mileage branch (`:343-356`) is read-only with no Edit/Delete. Unplanned `VehicleRecordDrawer.test.tsx` (5 tests). |
| 16 | Trends block and gallery dialog | DONE (deviation) | `detail/VehicleTrends.tsx` (two-up `grid-cols-[repeat(auto-fit,minmax(260px,1fr))]`, `MileageSparkline` + `VehicleActivityTimeline`); `dialogs/PhotoGalleryDialog.tsx` (git-detected rename of `media/VehicleMediaGallery.tsx`; thumbnail grid + `MediaUploadButton` + set-primary + delete). **Deviation:** `VehicleTrends` drops the planned `mileageRows` prop and calls `useMileageRecords` itself (`:19`) — a cache hit on the query the page already holds; rationale documented at `:10-17`. |
| 17 | Assemble the page and delete the old sections | DONE (Step 5 unverifiable) | `pages/VehicleDetailPage.tsx` rewritten: `OpenDialog` state (`:34,42`), drawer row state (`:43`), `useVehicleRecords(vehicle?.id ?? '', categoriesQuery)` passing the **whole query** (`:52`, comment `:48-51`), `categoriesError` surfaced (`:107-112`), container `max-w-[1600px]` + `lg:grid-cols-[320px_minmax(0,1fr)]` (`:106,114`), sticky rail (`:115`), all eight dialogs + drawer at page level (`:167-225`). Step 2: all four section files are gone from the tree. **Step 3 confirmed:** `grep -rn "VehicleMileageSection\|VehicleMaintenanceSection\|VehicleFuelSection\|VehicleMediaGallery" apps/web/src` returns 6 hits, every one a provenance comment (`AddScheduleDialog.tsx:20`, `PhotoGalleryDialog.tsx:22`, `VehicleRecordsTable.tsx:31`, `CompleteScheduleDialog.tsx:36`, `LogMaintenanceDialog.tsx:22`, `LogFuelDialog.tsx:15`) — no import, JSX or type reference survives. Step 4 (`make ci`) reported passing at HEAD by the caller. Step 5 is a manual browser check that cannot be verified from a static audit. |

**Completion Rate:** 17/17 tasks (100%)
**Skipped without approval:** 0
**Partial implementations:** 0

## Skipped / Deferred Tasks

None. Two items are caveats rather than skipped work:

1. **Task 17 Step 5 — manual in-app verification.** `make up` plus a browser pass over the
   sticky rail, dialog behavior, drawer per-kind actions, inline category creation and the
   sub-1024px stacking cannot be confirmed from repository state. Everything Step 5 would
   exercise has static evidence (see the Task 17 row), but the interaction behavior itself
   is unverified here.
2. **Backend `ORDER BY` tiebreaker** is explicitly deferred by the plan itself (Task 7
   preamble) and remains deferred; the merge's dedupe prevents duplicate renders but cannot
   recover a row the server skipped. `vehicleRecords.ts:58-68` documents the residual
   same-timestamp reordering risk.

## Gaps Found

1. **`isFetchingNextPage` is produced but never consumed.** Task 8 states the flag is
   *required* because "without this flag the button cannot be disabled, and sustained
   clicking starves the fetch so rows never arrive." The hook exposes it
   (`lib/hooks/api/vehicleRecords.ts:147`), but `VehicleRecordsTable`'s prop interface
   (`detail/VehicleRecordsTable.tsx:10-17`) has no such prop, its Load more button
   (`:195-199`) is never disabled, and `VehicleDetailPage.tsx:155-162` does not pass it.
   The failure mode Task 8 was written to prevent is still reachable. Task 14's own
   interface never listed the prop, so this is a seam between two tasks rather than a
   skipped step — but the stated intent is unmet.
2. **`deriveOdometer` is fed the whole merged feed, not mileage rows.** The plan's signature
   is `deriveOdometer(mileageRows, currentMileage)`; both call sites pass every row
   (`VehicleDetailPage.tsx:63`, `VehicleStatStrip.tsx:63`). Because the implementation only
   filters on `typeof r.mileage === 'number'` (`vehicleStats.ts:15-16`), the displayed
   odometer can come from a fuel log or maintenance record rather than an odometer reading.
   That is arguably more accurate (freshest known reading), but it is not what the parameter
   name promises and no test pins the intended behavior.
3. **A convention guardrail was narrowed rather than satisfied.** `bg-black/80` in
   `dialog.tsx` / `sheet.tsx` is allowlisted by file+exact-text in
   `src/test/conventions.test.ts:117-134`. The allowlist is tight and well argued, but the
   plan's global constraint ("status colors use the semantic token families") was met by
   amending the test rather than by adding an `--overlay` token.

## Work Beyond the Plan

Recorded so reviewers do not mistake it for scope creep: a unique index and duplicate-key
race recovery on category creation (Tasks 1-2), `TranslateError: true` in
`packages/shared-go/database/database.go:28`, the mileage list being made newest-first
(`internal/mileage/provider.go`, `resource.go`, with `MileageSparkline.tsx:31` re-commented
accordingly), and three unplanned test files (`lib/hooks/api/vehicleRecords.test.ts`,
`detail/VehicleRecordDrawer.test.tsx`, and the substantially expanded `vehicleStats.test.ts`).

## Build & Test Results

| Service | Build | Tests | Vet | Notes |
|---------|-------|-------|-----|-------|
| fleet-service (+ shared-go) | PASS | PASS | PASS | `go build github.com/jtumidanski/myfleet/...` and `go vet github.com/jtumidanski/myfleet/...` clean; `go test ./apps/fleet-service/internal/{maintenancecategory,maintenancerecord,mileage}/... -count=1` all `ok` |
| apps/web | PASS | PASS | n/a | Targeted `vitest run` over the 9 files this task touched or created: 97 tests passed (vehicleRecords 13, vehicleStats 31, useVehicleRecords 16, mileage 5, RecordsTable 6, RecordDrawer 5, CategoryCombobox 7, MaintenanceRecordForm 3, conventions 11). Full `make ci` reported exit 0 at HEAD by the caller and was not re-run. |

## Overall Assessment

- **Plan Adherence:** FULL
- **Recommendation:** READY_TO_MERGE from a plan-adherence standpoint (the backend-guidelines
  audit above records its own separate blocking findings, which are outside this section's scope)

## Action Items

1. Thread `isFetchingNextPage` from `useVehicleRecords` through `VehicleDetailPage` into
   `VehicleRecordsTable` and disable the Load more button while it is true — the only unmet
   piece of stated intent in the plan (Task 8's second constraint).
2. Decide and document the intended `deriveOdometer` input: either filter to
   `kind === 'mileage'` at the two call sites, or rename the parameter and add a test
   asserting a fuel row's odometer is intentionally eligible.
3. Optional: replace the `bg-black/80` allowlist in `conventions.test.ts` with an `--overlay`
   semantic token so the guardrail needs no exemptions.
4. Run Task 17 Step 5 manually (`make up`) before merge — sticky rail, per-kind drawer
   actions, inline category creation, and sub-1024px stacking have no automated coverage.

---

# Frontend Audit — task-012-vehicle-detail-redesign

- **Audit Scope:** TypeScript/React files changed in `git diff main...HEAD` (51 files under `apps/web/src`)
- **Guidelines Source:** `frontend-dev-guidelines` skill (`.claude/skills/frontend-dev-guidelines/`)
- **Date:** 2026-08-02
- **Build:** PASS
- **Tests:** 275 passed, 0 failed (38 test files)
- **Overall:** NEEDS-WORK

## Build & Test Results

```
$ npm run -w apps/web build
vite v5.4.21 building for production...
✓ 1841 modules transformed.
dist/assets/index-BZ9rLJfb.js   647.12 kB │ gzip: 192.62 kB
✓ built in 3.77s

$ npm run -w apps/web test
 Test Files  38 passed (38)
      Tests  275 passed (275)
   Duration  3.59s
```

Build emits the standard `chunks are larger than 500 kB` advisory (pre-existing, not a
guideline item). No failures.

## File Inventory

**Pages**
- `apps/web/src/pages/VehicleDetailPage.tsx` (M — rewritten as rail + records)

**Components — feature (new)**
- `components/features/vehicles/CategoryCombobox.tsx`
- `components/features/vehicles/detail/VehicleIdentityRail.tsx`
- `components/features/vehicles/detail/VehicleQuickActions.tsx`
- `components/features/vehicles/detail/VehicleStatStrip.tsx`
- `components/features/vehicles/detail/UpcomingScheduleStrip.tsx`
- `components/features/vehicles/detail/VehicleRecordsTable.tsx`
- `components/features/vehicles/detail/VehicleRecordDrawer.tsx`
- `components/features/vehicles/detail/VehicleTrends.tsx`
- `components/features/vehicles/dialogs/{AddSchedule,CompleteSchedule,DeleteVehicle,EditVehicle,LogFuel,LogMaintenance,LogMileage}Dialog.tsx`
- `components/features/vehicles/dialogs/PhotoGalleryDialog.tsx` (R74 from `media/VehicleMediaGallery.tsx`)

**Components — feature (modified/deleted)**
- `components/features/vehicles/maintenance/MaintenanceRecordForm.tsx` (M)
- `components/features/vehicles/maintenance/MaintenanceScheduleForm.tsx` (M)
- `components/features/vehicles/fuel/FuelForm.tsx` (M)
- `components/features/vehicles/mileage/MileageSparkline.tsx` (M)
- `components/features/vehicles/{fuel/VehicleFuelSection,maintenance/VehicleMaintenanceSection,mileage/VehicleMileageSection}.tsx` (D)

**Components — ui (new shadcn primitives)**
- `components/ui/{command,dialog,popover,sheet}.tsx`

**Hooks**
- `lib/hooks/api/vehicleRecords.ts` (A), `lib/hooks/api/pageSize.ts` (A)
- `lib/hooks/api/{fuel,maintenance,mileage}.ts` (M — converted to `useInfiniteQuery`)

**Services**
- `services/api/{FuelService,MaintenanceCategoryService,MaintenanceRecordService,MileageService}.ts` (M)

**Types**
- `types/models/maintenanceCategory.ts` (M — added `CreateMaintenanceCategoryAttributes`)

**Other (pure functions / tests / infra)**
- `lib/vehicleRecords.ts`, `lib/vehicleStats.ts` (A)
- `lib/vehicleRecords.test.ts`, `lib/vehicleStats.test.ts`, `lib/hooks/api/vehicleRecords.test.ts`,
  `lib/hooks/api/mileage.test.ts`, `components/features/vehicles/CategoryCombobox.test.tsx`,
  `.../detail/VehicleRecordsTable.test.tsx`, `.../detail/VehicleRecordDrawer.test.tsx`,
  `.../maintenance/MaintenanceRecordForm.test.tsx`
- `test/setup.ts` (M — `ResizeObserver` stub), `test/conventions.test.ts` (M — palette allowlist)

**Schemas** — no files added or modified under `lib/schemas/` (see FE-04 / FE-14).

## Anti-Pattern Checklist

| ID | Check | Status | Evidence |
|----|-------|--------|----------|
| FE-01 | No `any` type | PASS | `grep -rn ": any\|as any\|<any>\|any\[\]"` over all in-scope components, hooks, services, lib and types files returned zero matches. Dynamic input is typed `unknown` — `lib/hooks/api/vehicleRecords.ts:30` (`error: unknown`), narrowed through `createErrorFromUnknown` at `pages/VehicleDetailPage.tsx:109`. |
| FE-02 | No manual class concatenation | PASS | Grep for `className={…+…}` and `className={\`…\`}` over the in-scope tree returned zero matches. Conditional classes go through `cn()`: `detail/VehicleRecordsTable.tsx:54-57`, `detail/VehicleStatStrip.tsx:31`, `detail/UpcomingScheduleStrip.tsx:74`, `CategoryCombobox.tsx:136`, `ui/command.tsx:12`, `ui/sheet.tsx:59`. |
| FE-03 | No direct API client calls in components | PASS | `grep -rn "lib/api/client" components pages` returned zero matches. Every component reaches data through `lib/hooks/api/*`: `pages/VehicleDetailPage.tsx:6-8`, `detail/VehicleRecordDrawer.tsx:12-20`, `CategoryCombobox.tsx:16`. The only `apiClient` import is in the service layer, `services/api/MileageService.ts:2`. |
| FE-04 | No inline Zod schemas in components | **FAIL** | `components/features/vehicles/dialogs/CompleteScheduleDialog.tsx:16-23` defines `const completeScheduleSchema = z.object({ date: z.string()…, latestMileage: z.number()… })` inside the component file, importing `zod` at line 3. This is a plain object schema, not the permitted cross-field `.refine()` exception. Every sibling form in this task's diff imports from `lib/schemas/` (`dialogs/LogFuelDialog.tsx:6`, `dialogs/LogMileageDialog.tsx:6`, `dialogs/AddScheduleDialog.tsx:10`, `dialogs/LogMaintenanceDialog.tsx:9`), and `lib/schemas/` already holds `fuel.ts`, `mileage.ts`, `maintenanceRecord.ts`, `maintenanceSchedule.ts`, `vehicle.ts` — no `completeSchedule.ts` was added. |
| FE-05 | No spinners for content loading | PASS | All three `animate-spin` occurrences added by this task sit on action buttons gated by a mutation's `isPending`: `detail/VehicleRecordDrawer.tsx:280` and `:335` (destructive Delete buttons, `disabled={deleteRecord.isPending}` / `disabled={deleteFuel.isPending}` at `:277` / `:332`), `dialogs/CompleteScheduleDialog.tsx:129` (`type="submit"`, `disabled` at `:128`). `CategoryCombobox.tsx:170-174` swaps the `Plus` icon for `Loader2` on the "Create …" `CommandItem`, which is a create-mutation trigger disabled at `:168` — an action affordance, not content loading. Content loading uses `Skeleton` throughout: `detail/VehicleRecordsTable.tsx:64-76`, `detail/VehicleRecordDrawer.tsx:214`, `:290`, `:355`, `dialogs/PhotoGalleryDialog.tsx` skeleton grid, `pages/VehicleDetailPage.tsx:89-96`. |
| FE-06 | No hardcoded colors | **FAIL** | `detail/VehicleRecordsTable.tsx:41` — `modification: 'border-violet-200 bg-violet-100 text-violet-800'`. Fixed palette values with no dark-mode counterpart; renders as a light chip against a dark surface. Also `ui/dialog.tsx:18` and `ui/sheet.tsx:19` — `bg-black/80`. Mitigating context (does not clear the check, but bears on severity): the violet chip is a verbatim move of reviewed code that already existed on `main` (`git show main:…/VehicleMaintenanceSection.tsx` lines 312-322 — same classes, same "Intentional status colors" comment), and the project's own `PALETTE` regex at `test/conventions.test.ts:114-115` deliberately omits `violet`; the two `bg-black/80` lines are explicitly allowlisted with a written rationale at `test/conventions.test.ts:117-133`. Everything else in the diff uses semantic tokens — `detail/VehicleRecordsTable.tsx:37-39`, `detail/UpcomingScheduleStrip.tsx:67-69`, `pages/VehicleDetailPage.tsx:108`. |
| FE-07 | No state mutation | PASS | Every sort copies first: `lib/vehicleRecords.ts:84` (`[...all].sort(compareRows)`), `lib/vehicleStats.ts:187` (`[...schedules].sort(...)`), `detail/UpcomingScheduleStrip.tsx:46` (`[...schedules].sort(...)`), `mileage/MileageSparkline.tsx:32` (`[...records].sort(...)`). `lib/vehicleStats.ts:54-56` sorts the array returned by `.filter(...)`, a fresh array, so the caller's `fuelRows` is untouched. `lib/vehicleRecords.ts:110-119` (`dedupeById`) builds a new array rather than splicing. No `.push`/`.splice` on state or props anywhere in scope. |
| FE-08 | No default exports for components | PASS | `grep -rn "export default"` over the in-scope components and pages returned zero matches. All named: `pages/VehicleDetailPage.tsx:36`, `detail/VehicleRecordsTable.tsx:92`, `detail/VehicleRecordDrawer.tsx:62`, `CategoryCombobox.tsx:44`; the `ui/` primitives use named export lists (`ui/command.tsx:109-118`, `ui/dialog.tsx:87-98`, `ui/sheet.tsx:109-120`, `ui/popover.tsx:30`). |
| FE-09 | Error handling with `createErrorFromUnknown` | PASS | 15 of the 16 `catch` blocks in scope follow `const apiError = createErrorFromUnknown(err); toast.error(apiError.message \|\| '…')`: `pages/VehicleDetailPage.tsx:83-86`, `CategoryCombobox.tsx:93-96`, `detail/VehicleRecordDrawer.tsx:157-160`, `:171-174`, `:192-195`, `:204-207`, `dialogs/AddScheduleDialog.tsx:43-46`, `dialogs/CompleteScheduleDialog.tsx:73-76`, `dialogs/DeleteVehicleDialog.tsx:35-38`, `dialogs/EditVehicleDialog.tsx:34-37`, `dialogs/LogFuelDialog.tsx:35-38`, `dialogs/LogMaintenanceDialog.tsx:52-55`, `dialogs/LogMileageDialog.tsx:34-37`, `dialogs/PhotoGalleryDialog.tsx:40-43`, `:50-53`. The one bare `catch` — `detail/VehicleRecordDrawer.tsx:142-144` — is a deliberate per-attachment tally inside a loop whose failures are surfaced in aggregate by the `toast.error` at `:148-150`, and the enclosing `catch` at `:157` handles the record update itself; the user is never left without feedback. Query-level failure is surfaced too: `lib/hooks/api/vehicleRecords.ts:120` returns `categoriesError`, rendered at `pages/VehicleDetailPage.tsx:107-112`. |

## Architecture Checklist

| ID | Check | Status | Evidence |
|----|-------|--------|----------|
| FE-10 | JSON:API model shape | PASS | `types/models/maintenanceCategory.ts:17` — `export type MaintenanceCategory = JsonApiResource<MaintenanceCategoryAttributes>`, and `packages/shared-ts/src/jsonapi.ts:1-6` defines `JsonApiResource<A, R>` as `{ type: string; id: string; attributes: A; relationships?: R }`. The new `CreateMaintenanceCategoryAttributes` (`types/models/maintenanceCategory.ts:20-23`) is an attributes-only write payload, matching the documented "Update Data Types" pattern. `lib/vehicleRecords.ts:5-27` (`VehicleRecordRow`) is a derived UI view-model over three JSON:API sources, not a domain model — correctly placed in `lib/`, not `types/models/`. |
| FE-11 | Service extends `BaseService` (when applicable) | PASS | `services/api/MaintenanceCategoryService.ts:17-20` — `class MaintenanceCategoryService extends BaseService<MaintenanceCategoryAttributes, CreateMaintenanceCategoryAttributes>`; `services/api/FuelService.ts:20-24` and `services/api/MaintenanceRecordService.ts:23-27` likewise extend `BaseService`. `services/api/MileageService.ts:11-48` uses the documented direct-API-client pattern (private `basePath` at `:12`, `apiClient.request` at `:28` and `:39`, singleton export at `:50`), which the guideline permits for simple resources. All four export singletons. |
| FE-12 | Query key factory uses `as const` | PASS | `lib/hooks/api/fuel.ts:21-27` (`fuelKeys`, every member `as const`), `lib/hooks/api/mileage.ts:11-16` (`mileageKeys`), `lib/hooks/api/maintenance.ts:33-39` (`maintenanceRecordKeys`), `:51-57` (`maintenanceScheduleKeys`), `:68-73` (`maintenanceCategoryKeys`). All hierarchical via spread. `lib/hooks/api/vehicleRecords.ts` declares no key of its own — it composes the three source hooks (`:51-53`) — which is correct, not a gap. |
| FE-13 | Forms use `react-hook-form` + `zodResolver` | PASS | `dialogs/CompleteScheduleDialog.tsx:54-60` — `useForm<CompleteScheduleFormInput>({ resolver: zodResolver(completeScheduleSchema), values: {...} })`, rendered through the shadcn `Form`/`FormField`/`FormItem`/`FormMessage` primitives at `:85-122`. The forms this task modified keep the pattern: `maintenance/MaintenanceRecordForm.tsx:66-78` (`resolver: zodResolver(maintenanceRecordSchema)`), `fuel/FuelForm.tsx:36-47` (`zodResolver(fuelSchema)`, with `...defaultValues` merged last at `:46`). Dialogs that wrap a form delegate rather than re-implementing (`dialogs/LogFuelDialog.tsx:47-52`, `dialogs/LogMileageDialog.tsx:46-51`, `dialogs/EditVehicleDialog.tsx:49-64`). |
| FE-14 | Schema in `lib/schemas/` with inferred type | **FAIL** | `dialogs/CompleteScheduleDialog.tsx:16-25` — the schema lives in the component file and its inferred type is declared un-exported (`type CompleteScheduleFormInput = z.infer<typeof completeScheduleSchema>` at `:25`), so neither half of the convention holds. Every schema that *is* in `lib/schemas/` pairs correctly: `lib/schemas/maintenanceRecord.ts:3`+`:34`, `lib/schemas/fuel.ts:10`+`:41`, `lib/schemas/mileage.ts:3`+`:11`, `lib/schemas/maintenanceSchedule.ts:13`+`:50`, `lib/schemas/vehicle.ts:7`+`:26`. |

## Styling Checklist

| ID | Check | Status | Evidence |
|----|-------|--------|----------|
| FE-15 | Interactive elements show `cursor-pointer` | **FAIL** | `components/ui/command.tsx:93` — `CommandItem`'s base class list is `'relative flex cursor-default select-none items-center …'`. `cursor-default` is applied, nothing overrides it, and cmdk renders `Command.Item` as a `div[role="option"]`, not a `<button>`. Every category row and the inline-create row in `CategoryCombobox.tsx` is one of these: `:130-134` (suggested, `onSelect={() => handleSelect(c.id)}`), `:148-152` (custom), `:165-168` (`Create "…"`, `onSelect={() => void handleCreate()}`). Hovering any of them shows an arrow cursor — exactly the failure mode `patterns-styling.md:230-236` describes. Mitigating context: `components/ui/select.tsx:109` carries the same `cursor-default` on `SelectItem` and predates this branch, so the new primitive matches an existing repo wart rather than inventing one. Everything else clickable passes: `detail/VehicleRecordsTable.tsx:164` explicitly adds `cursor-pointer` to the `role="button"` `<tr>` (with `tabIndex={0}` and an Enter/Space handler at `:166-171`); `CategoryCombobox.tsx:101-113` wraps the `PopoverTrigger` in `asChild` around a real `<Button>`, which carries `cursor-pointer` from its CVA base (`components/ui/button.tsx:7`); the photo tiles at `VehicleIdentityRail.tsx:102-112` are native `<button>`s. |

## Testing Checklist

| ID | Check | Status | Evidence |
|----|-------|--------|----------|
| FE-16 | Tests exist for changed components | **WARN** | Covered: `CategoryCombobox.test.tsx` (7 tests, incl. the inline-create path), `detail/VehicleRecordsTable.test.tsx` (6 tests), `detail/VehicleRecordDrawer.test.tsx` (per-kind render + write gating), `maintenance/MaintenanceRecordForm.test.tsx` (updated for the combobox substitution), plus `lib/vehicleRecords.test.ts`, `lib/vehicleStats.test.ts` and `lib/hooks/api/vehicleRecords.test.ts` for the pure functions and the composing hook. Not covered, and non-trivial: **`dialogs/CompleteScheduleDialog.tsx`** — the only dialog in the set with its own form, its own schema (`:16-23`), an auto-fill fallback chain (`:49-50`, `latestLogged ?? vehicle?.attributes.currentMileage`) and a hand-rolled numeric `onChange` mapping `''` to `undefined` (`:111-113`); **`detail/VehicleStatStrip.tsx`** — the `partial` branch at `:90-104` and the `nextServiceClass` mapping at `:74-79` are component-level logic `vehicleStats.test.ts` does not reach; **`detail/UpcomingScheduleStrip.tsx`** — `toneClass` (`:65-70`) and the `categoryNames` fallback (`:71-72`); **`pages/VehicleDetailPage.tsx`** — the `openDialog` state machine (`:67-76`, `:102-103`), notably `handleQuickAction`'s `'upload' → 'gallery'` remap and the `modification → maintenance` kind derivation. The plan's `Files:` sections only ever promised `.test.tsx` for `CategoryCombobox` and `VehicleRecordsTable` (plan.md:47-48), so this is a scope decision rather than a silently skipped step — recorded as WARN, not FAIL. |
| FE-17 | Mocks updated when services changed | PASS (N/A shape) | The repo has no `__mocks__/` directory (`find . -name "__mocks__"` → empty); tests mock service singletons inline with `vi.mock`. Those mocks track the changed interfaces: `detail/VehicleRecordDrawer.test.tsx:11-22` mocks `maintenanceRecordService {get, patch, remove}`, `maintenanceCategoryService {list}`, `fuelService {get, patch, remove}` and `mileageService {listByVehicle}` — matching the signatures after the `page`/`pageSize` additions in `services/api/FuelService.ts:27-33` and `services/api/MaintenanceRecordService.ts:30-41`. `lib/hooks/api/mileage.test.ts` was updated in the same diff for the `useInfiniteQuery` conversion. `test/setup.ts:86-100` adds the `ResizeObserver` stub cmdk requires, without which every `Command`-rendering test would throw. Full suite green (275/275). |

## Additional Guideline Findings (outside the FE-* IDs)

These come from the same skill, so they are reported rather than invented — but they have no
assigned FE-* number, so they are listed separately.

1. **Title-case regression on interactive text** — `patterns-components.md:277-297` requires title
   case for button labels and dialog titles. The code this task replaced complied
   (`git show main:…/VehicleMaintenanceSection.tsx` lines 166, 261, 271 — "Add Schedule",
   "Log Record", "Log Modification"); the replacement does not.
   Buttons: `detail/VehicleQuickActions.tsx:14-19` ("Log mileage", "Log fuel", "Log maintenance",
   "Log modification", "Add schedule", "Upload photo") and `:31` ("Delete vehicle");
   `detail/UpcomingScheduleStrip.tsx:54` ("Add schedule"); `detail/VehicleRecordsTable.tsx:197`
   ("Load more"); `dialogs/CompleteScheduleDialog.tsx:130` ("Mark complete");
   `dialogs/PhotoGalleryDialog.tsx` ("Set primary").
   Dialog titles: `dialogs/AddScheduleDialog.tsx:53`, `dialogs/CompleteScheduleDialog.tsx:83`,
   `dialogs/DeleteVehicleDialog.tsx:45`, `dialogs/EditVehicleDialog.tsx:47`,
   `dialogs/LogFuelDialog.tsx:45`, `dialogs/LogMaintenanceDialog.tsx:63`,
   `dialogs/LogMileageDialog.tsx:44`.
   (Card titles — `detail/UpcomingScheduleStrip.tsx:51` "Upcoming maintenance",
   `detail/VehicleTrends.tsx:26` "Mileage trend" — are outside the rule's enumerated list, noted
   only for consistency.)

2. **`FormControl` dropped around the category field** — `patterns-forms-validation.md:156-160`
   wraps the control in `<FormControl>` so shadcn's `useFormField` can wire `id` /
   `aria-describedby` / `aria-invalid`. The combobox substitution removed that wrapper:
   `maintenance/MaintenanceRecordForm.tsx:91-101` and
   `maintenance/MaintenanceScheduleForm.tsx:58-68` now render `<CategoryCombobox>` as a bare
   sibling of `<FormLabel>`. The label's `htmlFor` therefore points at an id no element carries,
   and the field's error state is not announced. `CategoryCombobox` compensates with its own
   `aria-label` (`CategoryCombobox.tsx:107`, defaulted at `:50`), so the control is not unlabelled
   — but the label/control association and the `FormMessage` wiring at
   `MaintenanceRecordForm.tsx:102` are broken.

## Summary

### Blocking (must fix)

- **FE-04** — inline `z.object(...)` schema in a component:
  `apps/web/src/components/features/vehicles/dialogs/CompleteScheduleDialog.tsx:16-23`.
  Move it to `apps/web/src/lib/schemas/completeSchedule.ts`.
- **FE-14** — the same schema's inferred type is neither co-located in `lib/schemas/` nor exported:
  `apps/web/src/components/features/vehicles/dialogs/CompleteScheduleDialog.tsx:25`.
- **FE-15** — `cursor-default` on the new `CommandItem` primitive leaves every `CategoryCombobox`
  option without a pointer affordance: `apps/web/src/components/ui/command.tsx:93`
  (consumed at `CategoryCombobox.tsx:130`, `:148`, `:165`).

### Non-Blocking (should fix)

- **FE-06** — `border-violet-200 bg-violet-100 text-violet-800` at
  `apps/web/src/components/features/vehicles/detail/VehicleRecordsTable.tsx:41`. Fails the check as
  written, but is a verbatim carry-over of already-reviewed code from `main` and sits deliberately
  outside the project's own `PALETTE` guard (`test/conventions.test.ts:114-115`). The `bg-black/80`
  scrims at `ui/dialog.tsx:18` / `ui/sheet.tsx:19` are explicitly allowlisted with rationale at
  `test/conventions.test.ts:117-133`.
- **FE-16** — no test file for `dialogs/CompleteScheduleDialog.tsx` (its own form + schema +
  auto-fill chain), `detail/VehicleStatStrip.tsx`, `detail/UpcomingScheduleStrip.tsx`, or the
  `openDialog` state machine in `pages/VehicleDetailPage.tsx:67-103`.
- **Title casing** — interactive text regressed to sentence case across the new dialogs and quick
  actions; see Additional Finding 1 for the full line list.
- **`FormControl` wrapper** — restore it (or forward the id/aria props into `CategoryCombobox`) at
  `maintenance/MaintenanceRecordForm.tsx:91-101` and
  `maintenance/MaintenanceScheduleForm.tsx:58-68`; see Additional Finding 2.
