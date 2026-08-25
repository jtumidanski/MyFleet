# Plan Audit — task-030-one-time-maintenance-schedules

**Plan Path:** docs/tasks/task-030-one-time-maintenance-schedules/plan.md
**Audit Date:** 2026-08-25
**Branch:** task-030-one-time-maintenance-schedules (head `36145e0`)
**Base Branch:** main (merge base `285e175`)

## Executive Summary

All 16 plan tasks are implemented. 24 commits, 38 branch files, 41 files in the
diffstat. Every backend functional requirement (FR-OT-*, FR-ANCHOR-*,
FR-COMPLETE-*, FR-UPD-*) and every frontend one (FR-CONV-*, FR-UI-*) maps to
real, exercised code, not just to a checked box. All five Global Constraints
hold on inspection: no fourth `recurrenceType` value exists anywhere;
`oneTime` is serialized without `omitempty` while `dueDate`/`dueMileage` carry
it; every validation failure returns `server.ErrValidation`; and no touched
route changed its authorization. Builds, vet, and tests pass for fleet-service,
shared-go, and the affected web suites; `tsc -b --noEmit` exits 0.

Two process/hygiene findings, neither blocking: the plan's checkboxes were never
ticked (all 100+ steps still read `- [ ]`, so the file is not a usable
completion record), and Task 9's builder-level create tests were kept alongside
the handler-level replacement rather than removed, leaving duplicated coverage.

**Recommendation: READY_TO_MERGE.**

## Task Completion

| # | Task | Status | Evidence / Notes |
|---|------|--------|------------------|
| 1 | `server.Nullable[T]` tri-state | DONE | `packages/shared-go/server/nullable.go:16-39` — struct + `UnmarshalJSON` verbatim to plan §Task 1 Step 3. Tests `nullable_test.go:8,46,57`. Commit `01c0ae0`. |
| 2 | Three columns / three model fields | DONE | `entity.go:18-25` (`OneTime`/`DueDate`/`DueMileage`, `not null;default:false` and int-0 default), `entity.go:45-68` + `:79-101` (`Make`/`ToEntity` both directions), `model.go:13-15,33-35,52-54,68-77` (fields, accessors, `AsSchedule` carry, `WithOneTime`/`WithDuePoint`), `recurrence.go:10-12` (`Schedule` fields), `admin/admintest/db.go:87-88` (sqlite DDL). Tests `entity_test.go:12,43,53,82`. Commits `b7eb5b7`, `24d9128`. |
| 3 | Revised `NextDue` | DONE | `recurrence.go:25-58` — per-axis switch, stored point outranks arithmetic, `!s.OneTime` guard makes a completed one-time axis terminal. Tests `recurrence_test.go:245` (matrix), `:350`. Commit `ffa1f34`. |
| 4 | Shared `validate` + new setters | DONE | `builder.go:56-98` (`validate`), `:99-113` (`Build` delegates), `:40,42-53,57` (`SetOneTime`/`SetDuePoint`/`SetCurrentMileage`), `:24-27` (`currentMileage` off-model field). Design rules 5/6/7 all present at `builder.go:66-70` and `:82-92`. Tests `builder_test.go:21,104,119,137,153,171,192`. Commit `828819f`. |
| 5 | `Processor.Update` validates the result | DONE | `processor.go:81-83` — `validate(updated)` before recompute and before write (FR-UPD-2). Tests `processor_update_test.go:33,55,70,120`. Commit `6b10646`. |
| 6 | Persist columns; clear anchor + deactivate on completion | DONE | `administrator.go:53-58` (`one_time`/`due_date`/`due_mileage` in the `Update` column map, `due_date` written as `*time.Time` so a cleared anchor is SQL NULL), `:92-99` (in-transaction inactive guard → `server.ErrValidation`), `:107` (`WithDuePoint(time.Time{}, 0)` before `NextDue`), `:112-118` (`state = "ok"` for a completed one-time), `:124-125` + `:135-137` (`due_date: nil`, `due_mileage: 0`, `active: false`). Tests `completion_db_test.go:181,238,287`. Commits `b1c2990`, `a61fd96`. |
| 7 | Legacy backfill | DONE | `backfill.go:30-89` — selects only never-completed, never-anchored, non-deleted, non-one-time rows joined to the vehicle odometer; anchors through the same `Schedule`/`NextDue`/`DueState` path; wired at `cmd/main.go:71-75` after `maintenancecategory.Seed` and fatal on error, as prescribed. Tests `backfill_test.go:28,96,140,185`. Commit `68181f6`. |
| 8 | Transport: create, PATCH, complete | DONE | `resource.go:63,66-67` (create attrs), `:85-96` (RFC3339 parse → `ErrValidation` on garbage), `:99-105` (`SetOneTime`/`SetDuePoint`/`SetCurrentMileage`); `:139,145-148` (PATCH attrs, `server.Nullable[string]` for `dueDate`, `*int` for `dueMileage`), `:167-175` + `:190-205` (present-vs-null handling, FR-UPD-3); `:270-278` (inactive pre-check placed *after* the authz checks so a non-member still gets 403/404). `rest.go:31-35` (`oneTime` without `omitempty`, `dueDate`/`dueMileage` with it), `:55-56,69-71` (`Transform`). Commit `06d953c`. |
| 9 | Transport tests: emission rules + token stability | DONE | `rest_test.go:28,45,70` (always-emit `oneTime`, conditional due fields, `DueCycleToken` stable across recomputes for a one-time row). Plus the ruled-in create-path coverage: `resource_test.go:73,151,189` (handler-level, real router, incl. viewer-forbidden) and `rest_test.go:128,176`. notification-service genuinely untouched — it appears nowhere in `git diff 285e175..36145e0 --stat`. Commits `bad549a`, `abe6c08`. |
| 10 | Frontend types + Zod schema | DONE | `types/models/maintenanceSchedule.ts:13-17` (read attrs, `oneTime` non-optional), `:37,40-42` (create), `:47-58` (update, `dueDate?: string \| null`). `lib/schemas/maintenanceSchedule.ts:36-72` (`kind` discriminant + four-rule `superRefine`), `:105-138` (`convertToRecurrenceSchema`). Commit `acf5a28`; reject coverage restored at `maintenanceSchedule.test.ts:59,70` in `f26f01f`. |
| 11 | Schedule form: kind selector, conditional fields, anchor defaulting | DONE | `MaintenanceScheduleForm.tsx:133-153` (Schedule Type select), `:178-228` (intervals gated on `recurring`), `:230-281` (due-point fields for both kinds, relabelled at `:107-108`), `:79-92` (FR-ANCHOR-4 defaulting behind the `useRef` touched flags), `:101-105` (stale-interval clear on switch to one-time). `AddScheduleDialog.tsx:42-54` (payload mapping, YYYY-MM-DD → RFC3339, interval suppression for one-time), `:73` + `VehicleDetailPage.tsx:247` (odometer passed through). Commits `0857fec`, `feca432`. |
| 12 | Extract `ScheduleCard` | DONE | `ScheduleCard.tsx:23-40` (one-time due-point line vs interval line), `:47-53` (completion line, FR-COMPLETE-3), `:80-86` (tone from `status`, none for inactive), `:97-101` (One-time badge, FR-UI-2), `:109-119` (Complete only when active; Set up recurrence only when inactive + one-time + writable). Test `ScheduleCard.test.tsx`. Commit `afa567f`. |
| 13 | Two-tier sorting in the strip | DONE | `UpcomingScheduleStrip.tsx:37-43` (active/inactive tier first, then `rankSchedule`, inactive by most-recent completion), `:59` (non-mutating copy), `:76-88` (delegates to `ScheduleCard`, threads `onConvert`). Test `UpcomingScheduleStrip.test.tsx`. Commit `b52a7c8`. |
| 14 | `ConvertToRecurrenceDialog` | DONE | `ConvertToRecurrenceDialog.tsx:66-89` — exactly one PATCH with `oneTime:false`, `active:true`, explicit `dueDate: null` and `dueMileage: 0` (FR-CONV-3, depends on FR-UPD-3); failure keeps the dialog open with an error toast (FR-CONV-4, `:85-88`). Prefill context at `:60-64,98-105` (FR-CONV-2). Test `ConvertToRecurrenceDialog.test.tsx`. Commit `102966e`. |
| 15 | Completion toast action + page wiring | DONE | `CompleteScheduleDialog.tsx:69-81` — action attached only when `schedule.attributes.oneTime` (FR-CONV-1/FR-CONV-5). `VehicleDetailPage.tsx:57,91-94,204,259,262-275` — `convertingSchedule` state, `'convert'` dialog id, `onRequestConvert`, `onConvert`, category-name resolution. Closes the intra-branch break from Task 13. Test `CompleteScheduleDialog.test.tsx`. Commit `93bdeec`. |
| 16 | Whole-repo verification | DONE | Commit `36145e0`. Controller-run `make ci` (all stages but the pre-existing `lint-check` toolchain failure), byte-identical overlay renders, untouched-service confirmation. This audit is Step 4. |

**Completion Rate:** 16/16 tasks (100%)
**Skipped without approval:** 0
**Partial implementations:** 0

## Skipped / Deferred Tasks

None. No plan-mandated behaviour is missing from the branch.

Two ruled-in deviations, both justified and both delivered:
- Task 9 gained a create-path test beyond the plan (see ruling 2 below).
- Task 11 substituted `useRef` touched-flags for the plan's `formState.dirtyFields`
  guard, which the plan's own test could not have passed (see ruling 4).

## Global Constraints Verification

| Constraint | Verdict | Evidence |
|---|---|---|
| No fourth `recurrenceType` value | HOLDS | `builder.go:13` `validRecurrence` unchanged (`time`/`mileage`/`hybrid`); `lib/schemas/maintenanceSchedule.ts:3` `recurrenceTypes` unchanged; `kind` is a separate enum at `:4`. |
| `oneTime` without `omitempty`; `dueDate`/`dueMileage` with it | HOLDS | `rest.go:31-35`. Pinned by `rest_test.go:28` and `:45`. |
| Validation errors are always `server.ErrValidation` | HOLDS | `builder.go:57-95`, `processor.go:82`, `administrator.go:97`, `resource.go:90,171,277`. No bespoke error type or per-field message anywhere in the Go diff. |
| Authorization unchanged on every touched route | HOLDS | The create/PATCH/complete handlers' identity, fleet-membership, and `authz.RequireWrite` blocks are outside the diff hunks; the new inactive pre-check is inserted *after* them (`resource.go:270`) so a non-member still gets 403/404 rather than 400. `resource_test.go:189` pins viewer-forbidden on create. |
| No change to the frozen symbols / `dashboard` / `vehicle` / `notification-service` | HOLDS | `DueState`, `DueBreaches`, `Severity`, `AxisBreach`, `Thresholds`, `DueCycleToken`, `TransformInternalDue`, `Queue`, `RecomputeAll`, `ScheduleDueByVehicle` are all absent from the diff. `RecomputeTx` (`administrator.go:144-164`) writes only `next_due_*`/`status`/`severity`, so the hourly job cannot erase an anchor. |
| No change to `deploy/k8s` | HOLDS | No path under `deploy/` in the diffstat. |
| No hardcoded palette classes | HOLDS | `ScheduleCard.tsx:80-86,91,98` use semantic tokens only; `conventions.test.ts` passes. |

## Assessment of the Five Controller Rulings

**1. Task 6 — reverting the `Entity.Active` `default:true` removal. CORRECT.**
The reasoning holds, and I verified each leg. `Active bool \`gorm:"not null;default:true"\`` is intact at `entity.go:32`. The only `Create` path in the package is `dbAdministrator.Insert` (`administrator.go:37-45`) and `grep` confirms `NewBuilder()` for schedules is called nowhere outside this package's routes and tests — so no production path attempts to insert an inactive schedule. Both deactivation paths write through explicit column maps, which GORM's `default:` tag has no bearing on: `AdvanceTx`'s `updates["active"] = false` (`administrator.go:135-137`) and `Administrator.Update`'s `"active": e.Active` (`administrator.go:60`). The fixture fix (seed active → explicit `UPDATE` → re-read and assert, `completion_db_test.go:300-313`) proves the seed took rather than assuming it. Removing the tag would have made `AutoMigrate` emit `ALTER ... DROP DEFAULT` against the shared schema for a test's convenience — the ruling avoided a real production-schema change.

**2. Task 8→9 — ruling one extra create-path test into Task 9. CORRECT, with a
loose end.** Task 4 made `validate` require a due point, which breaks every
create until Task 8's setters are wired; without a durable test, that wiring is
unproven by anything the branch keeps. The handler-level replacement
(`resource_test.go:73` drives the real chi router with the real
`RegisterInputHandler`) is the right level — it catches JSON tag typos and the
RFC3339 parse, which the builder-level version could not. **Loose end:** the
first-attempt builder-level tests were *not* removed; `rest_test.go:128`
(`TestCreate_succeedsWithDuePoint`) and `:176`
(`TestCreate_requiresDuePointForNeverCompletedSchedule`) still exist alongside
`resource_test.go:73` and `:151`, which cover the same two cases through the
real handler. That is duplicated coverage under misleading `TestCreate_` names
in a file about `Transform`. Cheap tidy-up, not a defect.

**3. Task 10 — requiring the deleted `intervalMiles` reject tests be restored.
CORRECT.** A plan-prescribed test *rewrite* is not a licence to lose coverage
relative to `main`. Both cases are back at `maintenanceSchedule.test.ts:59`
(`requires intervalMiles for a mileage schedule`) and `:70` (`rejects a zero
intervalMiles`), and commit `f26f01f` isolates the restoration.

**4. Task 11 — accepting the `useRef` substitute for `formState.dirtyFields`.
CORRECT, and the substitute preserves the design's intent.** The plan defect is
real: `dirtyFields` is a diff against `defaultValues`, so the effect's own
`setValue('dueDate', …)` away from `undefined` marks the field dirty and the
guard latches after the very first write — the anchor would freeze at the value
computed for the first interval the user typed, which is precisely what
FR-ANCHOR-4 needs *not* to happen. The refs at
`MaintenanceScheduleForm.tsx:79-80` are flipped only inside the fields' own
`onChange` (`:242`, `:269`), so they track user edits exclusively and the effect
keeps recomputing until one occurs. That is the design's intent stated exactly.
One behavioural nuance worth recording rather than fixing: the refs never reset,
so once a user hand-edits the anchor, switching kind away and back will not
re-default it. That reads as correct deference to a deliberate choice, and the
comment at `:71-78` documents the whole reasoning.

**5. Task 13→15 — accepting a deliberate intra-branch build break. ACCEPTABLE.**
`UpcomingScheduleStrip` required `onConvert` from `b52a7c8` and
`VehicleDetailPage` only supplied it in `93bdeec`. This is a per-commit `tsc`
break inside a topic branch, closed two commits later; the branch head is clean
(`tsc -b --noEmit` exits 0) and the branch merges as a unit. The cost is that
`git bisect` over a TypeScript build is unreliable across that window. Tolerable
here; worth avoiding as a habit.

## Triage of the Deferred Minor Findings

**None of the twelve block merge.** Ranked by whether they should be picked up
opportunistically:

*Worth doing soon (cheap, small real value):*
- **Task 4 — no reject case for a one-time hybrid missing one axis.** The
  matrix at `builder_test.go:46-52` covers recurring-hybrid on both sides but
  one-time-hybrid only at `:69` (the passing case). Design rules 6/7
  (`builder.go:82-92`) are kind-independent by construction, so this is a
  symmetry gap rather than an untested branch. Two table rows.
- **Task 5 — `TestProcessorUpdate_rejectsOneTimeWithIntervals`
  (`processor_update_test.go:55`) checks only the error, no row re-read.** The
  sibling `TestProcessorUpdate_rejectsZeroIntervalOnHybrid` at `:33` does
  re-read, so the "a rejected PATCH leaves the row untouched" claim in
  `processor.go:71-74` is proven once. Low value to duplicate.
- **Task 6 — the `default:true` hazard comment duplicated near-verbatim in
  three places** (`administrator.go:38-41`, `completion_db_test.go:300-302`,
  `processor_update_test.go:84-86`). Harmless; if it drifts it will drift
  confusingly.

*Correctly deferred, no action:*
- **Task 1 — no `MarshalJSON` on `Nullable[T]`.** The plan never asked for one
  and the type is decode-side only (`resource.go:145`). Adding an unused
  encoder would be speculative surface.
- **Task 6 — the redundant seed-inactive block in
  `TestProcessorUpdate_conversionDerivesFromCompletionPoint`
  (`processor_update_test.go:83-96`).** It genuinely does not affect what the
  test proves, since `validate` runs only on the *resulting* model. But it makes
  the fixture faithful to the real pre-conversion state (a completed one-time
  row *is* inactive), which is worth ~14 lines. Keep.
- **Task 7 — N+1 `Updates` loop (`backfill.go:45-87`).** One-shot startup
  migration over a bounded legacy set; a batched rewrite would trade clarity for
  a cost paid once.
- **Task 7 — non-idempotence when `CurrentMileage + IntervalMiles == 0`.**
  Unreachable: pre-task-030 validation already required `intervalMiles > 0` for
  a mileage schedule, and odometers are non-negative. Even if reached, the
  re-run writes identical values.
- **Task 7 — no test for a partially-anchored hybrid row.** The `WHERE` clause
  at `backfill.go:38` (`due_date IS NULL AND due_mileage = 0`) excludes such a
  row by construction, and `validate` prevents one from being created. Testing
  an unconstructable state.
- **Task 9 — two builder-level create tests could be table-driven.** See ruling
  2; the real point is that they duplicate `resource_test.go`, not that they
  aren't tabular.
- **Task 10 — no explicit test for `dueMileage: 0` / empty-string `dueDate`.**
  Both flow through the same falsy check in the `superRefine`
  (`maintenanceSchedule.ts:57`, `:70`) that the `undefined` cases already
  exercise.
- **Task 12 — an active one-time schedule with neither `dueDate` nor
  `dueMileage` falls back to a bare `recurrenceType` string**
  (`ScheduleCard.tsx:39`). Unreachable from a valid row: `validate`
  (`builder.go:82-92`) requires a due point on every covered axis of an active
  one-time schedule, and every recurrence type covers at least one axis. Dead
  defensive branch; leave it.
- **Task 13 — `completedAt()` returns `NaN` for a malformed
  `lastCompletedDate`** (`UpcomingScheduleStrip.tsx:22-25`), making the
  inactive-tier comparator non-total. Unreachable via the backend, which always
  formats RFC3339 (`rest.go:9`, `:62-64`). A non-total comparator is a genuine
  latent hazard, but the input is server-controlled.
- **Task 14 — no `DialogDescription`/`aria-describedby`.** Matches the sibling
  dialog convention; fixing it here alone would make the codebase less
  consistent, not more. Worth a repo-wide a11y sweep as separate work.
- **Task 15 — tests assert on the mocked `sonner` `action` shape rather than a
  rendered `<button>`.** `sonner` renders into a portal outside the test tree;
  asserting the contract handed to the library is the standard workaround and is
  what the plan prescribed.

## Build & Test Results

| Service | Build | Tests | Vet | Notes |
|---------|-------|-------|-----|-------|
| apps/fleet-service | PASS | PASS | PASS | 20 packages ok, `-count=1`. |
| packages/shared-go | PASS | PASS | PASS | 9 packages ok, `-count=1`. |
| apps/web | PASS (`tsc -b --noEmit` exit 0) | PASS | n/a | Targeted run: 18 files, 203 tests passed (schemas + `features/vehicles` + `vehicleStats`). |

Not re-run per instruction: the full `make ci`. Controller results stand for
head `36145e0` — all stages pass except `lint-check`, which fails on 6 Go
modules from a golangci-lint go1.27-vs-go1.26 toolchain mismatch reproduced
identically on a pristine merge-base checkout. **Pre-existing; not
branch-introduced; not a merge blocker for this branch.** It should be tracked
separately, because a permanently red `lint-check` trains everyone to ignore it.

## Overall Assessment

- **Plan Adherence:** FULL
- **Recommendation:** READY_TO_MERGE

## Findings

### Critical

None.

### Important

1. **The plan's checkboxes were never ticked.** Every step in `plan.md` still
   reads `- [ ]` despite all 16 tasks being complete. The file is the branch's
   own record of what was done, and as committed it asserts that nothing was.
   Anyone reading it later — including a future bisect or post-mortem — gets the
   wrong answer. Mark them off, or note in the plan header that
   `.superpowers/sdd/plan/progress.md` is the authoritative tracker.
2. **`lint-check` is red on `main` and stays red here.** Out of scope for this
   branch, but it means the branch cannot be verified green end-to-end by the
   project's own documented command (`make ci`, per CLAUDE.md). File it.

### Minor

3. **Duplicated create-path coverage.** `rest_test.go:128,176` and
   `resource_test.go:73,151` test the same two cases at different levels, and
   the `TestCreate_*` names sit in a file otherwise about `Transform`. Drop the
   builder-level pair or move them.
4. **`TestBuild_validationMatrix` lacks one-time-hybrid single-axis reject
   rows** (`builder_test.go:69`). Two table entries for matrix symmetry.
5. **The `default:true` hazard comment is triplicated** (`administrator.go:38`,
   `completion_db_test.go:300`, `processor_update_test.go:84`).
6. **`completedAt()` is non-total on malformed input**
   (`UpcomingScheduleStrip.tsx:22-25`). Server-controlled input makes it
   unreachable; a `Number.isNaN` fallback to `0` would close it for a line.

## Action Items

None required before merge.

Optional follow-ups, in priority order:

1. Tick `plan.md`'s checkboxes (or redirect readers to `progress.md`) so the
   committed plan stops claiming nothing was done.
2. Open a separate issue for the repo-wide `golangci-lint` toolchain mismatch
   that keeps `make ci`'s `lint-check` red on `main`.
3. Remove the duplicated builder-level create tests at `rest_test.go:128,176`.
4. Add the two one-time-hybrid reject rows to `TestBuild_validationMatrix`.
5. De-duplicate the `default:true` hazard comment down to one canonical home.

## Backend Guidelines Audit

- **Scope:** Go changes on `task-030-one-time-maintenance-schedules` (`285e175..36145e0`)
- **Packages audited:** `apps/fleet-service/internal/maintenanceschedule` (domain), `packages/shared-go/server` (support), `apps/fleet-service/cmd`, `apps/fleet-service/internal/admin/admintest`
- **Guidelines Source:** backend-dev-guidelines skill
- **Date:** 2026-08-25
- **Build:** PASS (`make build`, exit 0)
- **Tests:** PASS (`make test`, exit 0 — every package `ok`/cached, 0 failed)
- **Overall:** NEEDS-WORK

### Build & Test Results

`make build` and `make test` both exited 0 from the repo root; `go.work`'s four
modules all compiled and every test package reported `ok`. No branch-introduced
failures. (`lint-check`'s golangci-lint toolchain panic is pre-existing on the
merge base and out of scope per the audit brief.)

### Phase 2 — Package Classification

| Package | Classification | Evidence |
|---|---|---|
| `apps/fleet-service/internal/maintenanceschedule` | Domain (`model.go` present) | `model.go:6` |
| `packages/shared-go/server` | Support (transport primitives; no `model.go`) | `nullable.go:16` |
| `apps/fleet-service/cmd` | Support (wiring) | `main.go:74` |
| `apps/fleet-service/internal/admin/admintest` | Support (test fixture DDL) | `admintest/db.go:84-90` |

### Domain Checklist Results

#### maintenanceschedule

| ID | Check | Status | Evidence |
|----|-------|--------|----------|
| DOM-01 | `builder.go` exists, `Build()` validates | PASS | `builder.go:105-108` returns `(Model, error)`; invariants enforced in `validate` at `builder.go:61-102`, every failure path `return server.ErrValidation` (`:63,66,75,79,82,95,98`) |
| DOM-02 | `ToEntity()` on Model | PASS | `entity.go:78` `func (m Model) ToEntity() Entity`; new columns assigned at `:99-101` |
| DOM-03 | `Make(Entity) Model`, no error | PASS | `entity.go:44` `func Make(e Entity) Model`; new columns read at `:62-64` |
| DOM-04 | `Transform` in `rest.go` | PASS | `rest.go:48` |
| DOM-05 | `TransformSlice` used by list handler | PASS | `rest.go:76`; list handler uses it at `resource.go:54`. The two remaining loops are derived-value variants, not bare `Transform` loops: `TransformInternalDue` (`rest.go:101`) and `TransformQueue` (`rest.go:122`, overrides status/severity per row) |
| DOM-06 | Processor takes `logrus.FieldLogger` | PASS | `processor.go:26,34` — interface, not `*logrus.Logger` |
| DOM-07 | Logger threaded, not fetched | PASS | `resource.go:30-31` and `:327-328` pass `log` straight into `NewProcessor`; `grep -n 'logrus.StandardLogger' *.go` over all 22 files in the package returned 0 matches |
| DOM-08 | Bodied routes use `RegisterInputHandler` | PASS | `resource.go:60` (POST create), `:137` (PATCH), `:244` (POST complete) all wrapped. `r.Get`/`r.Delete` routes are body-less and correctly plain (`:34,121,220,317,319,331`) |
| DOM-09 | Errors via `server.WriteError` | PASS | 30 `server.WriteError` calls in `resource.go`; `grep 'http.Error('` = 0 matches; the single `w.WriteHeader` (`resource.go:240`) is a 204 success, not an error ladder; no hand-built error envelope |
| DOM-10 | Provider contract | PASS | `provider.go:24` interface, `:38` `dbProvider`, `:41` `NewProvider(db *gorm.DB) Provider`; `gorm.ErrRecordNotFound` translated to `ErrNotFound` at `:46-48` |
| DOM-11 | No `os.Getenv()` in handlers | PASS | `grep -n 'os.Getenv' resource.go` = 0 matches |
| DOM-12 | No cross-domain logic in handlers | PASS | Cross-domain completion orchestration is behind the injected `CompletionDeps.CompleteInTransaction` (`resource.go:292`, implemented `completion_db.go:64-95`); the vehicle lookup goes through the `VehicleAccessor` interface (`resource.go:21-23`) |
| DOM-13 | Handlers don't call providers | PASS | `NewProvider(`/`NewAdministrator(` appear only at `resource.go:31` and `:328`, inside `InitializeRoutes`, never in a handler body |
| DOM-14 | No direct entity creation in handlers | PASS | `grep -n 'db.Create\|db.Save\|db.Delete' resource.go` = 0 matches |
| DOM-15 | `administrator.go` for writes | PASS | `administrator.go:13-30` interface, `:35` `NewAdministrator`; new columns written at `:56,59,60` and `:124,125,136` |
| DOM-16 | Right sentinel returned | PASS | `server.ErrValidation` from `builder.go:63…98`, `administrator.go:99`, `resource.go:92,171,275,283`; `server.ErrNotFound` from `processor.go:52,96`. `StatusFor` maps `ErrValidation`→422, `ErrNotFound`→404 (`packages/shared-go/server/errors.go:43,33`) |
| DOM-17 | Resource type is a literal | PASS | `rest.go:72` `Type: "maintenanceSchedules"`, `ID: m.ID()`; `resource.go:309` `Type: "maintenanceCompletions"` |
| DOM-18 | Input structs narrow and unexported | WARN | Deviation, pre-existing and extended by this branch: the create/patch/complete request models are **anonymous inline structs in the handler signature** (`resource.go:60-68`, `:137-150`, `:244-247`) rather than named unexported `createAttributes`/`patchAttributes` in `rest.go`, which `file-responsibilities.md` (§`rest.go`) and `ai-guidance.md` (§REST Generation Specifics) both call for. They are narrow, unexported, flat, and do not reuse the read `Attributes` (`rest.go:26`), so the security intent of the rule holds. Pre-existing at merge base (`git show 285e175:./resource.go`, same shape). Non-blocking |
| DOM-19 | Tests cover the domain's logic layers | PASS | Builder invariants `builder_test.go:21,104,119,137,153,171,192`; processor logic `processor_update_test.go:33,55,70,120`; provider/DB error paths `completion_db_test.go:181,238,287` and `backfill_test.go:28,96,140,185`; REST/status mapping `rest_test.go:28,45,70` and `resource_test.go:73,151,189`; entity round-trip `entity_test.go:12,43`. Table-driven where it fits, local `cases := []struct` idiom (`builder_test.go:22`, `nullable_test.go:13`) |

### Sub-Domain Checklist Results

| ID | Check | Status | Evidence |
|----|-------|--------|----------|
| SUB-01..04 | Sub-domain checks | OUT-OF-SCOPE | This branch touches no sub-domain package. The tree's only genuine sub-domain packages are `notification-service/internal/admin` and `media-service/internal/admin`; `git diff --stat 285e175..36145e0 -- '*.go'` lists neither |

### Security Review

`fleet-service` is not an auth service, so SEC-01/02/03 (JWT parsing, token
revocation, open redirect) have no subject here. The authorization surface of
the touched routes is audited instead, per the brief.

| ID | Check | Status | Evidence |
|----|-------|--------|----------|
| SEC-01 | JWT validation uses verified parsing | OUT-OF-SCOPE | `grep -n 'ParseUnverified' apps/fleet-service/internal/maintenanceschedule/*.go` = 0 matches; token verification lives in the JWKS middleware wired at `apps/fleet-service/cmd/main.go:82`, untouched by this branch |
| SEC-02 | Token revocation on validated tokens | OUT-OF-SCOPE | No revocation/logout handler in scope |
| SEC-03 | No open redirect | OUT-OF-SCOPE | No callback/redirect handler in scope |
| SEC-04 | No hardcoded secrets | PASS | No credential literal in any changed Go file; `backfill.go`, `nullable.go`, `resource.go` contain none |
| SEC-05 | Route set unchanged (no new authorization surface) | PASS | `diff` of the `r.Get/Post/Patch/Put/Delete(` registration lines between `285e175:resource.go` and head is empty — 8 routes at base, the same 8 at head. `authz.Require*` call count is 9 at base and 9 at head |
| SEC-06 | Same-fleet on reads | PASS | List `resource.go:42`; get-by-id `:129` via `requireScheduleFleet` (`:379-385`); queues `:349` |
| SEC-07 | Same-fleet + `RequireWrite` on create | PASS | `resource.go:77` `RequireSameFleet`, `:81` `RequireWrite` — both before `NewBuilder()` at `:97` |
| SEC-08 | Same-fleet + `RequireWrite` on PATCH | PASS | `resource.go:159` `requireScheduleFleet`, `:163` `RequireWrite`, both before `proc.Update` at `:176`. Ordering preserves the 404/403 no-leak: the fleet check runs before the role check |
| SEC-09 | Same-fleet + `RequireWrite` on DELETE | PASS | `resource.go:228,232` before `proc.Delete` at `:236` |
| SEC-10 | Same-fleet + `RequireWrite` on complete, and the new inactive pre-check does not precede authz | PASS | `resource.go:261` `RequireSameFleet`, `:265` `RequireWrite`; the new `if !sched.Active()` pre-check is at `:274`, **after** both, so a non-member still gets 403/404 rather than a 422 that would confirm the schedule exists. Documented at `:270-273` |
| SEC-11 | New PATCH attributes cannot escalate or reassign | PASS | The PATCH attribute set (`resource.go:138-149`) is `recurrenceType, oneTime, intervalMonths, intervalMiles, dueDate, dueMileage, active` — it contains no `vehicleId`, `categoryId`, or `id`, so a caller cannot move a schedule to another vehicle or fleet. The apply closure (`:176-210`) touches only those fields |
| SEC-12 | 5xx redaction path intact | PASS | Every error in `resource.go` exits through `server.WriteError` (30 call sites), which is the only path that redacts 5xx titles (`packages/shared-go/server/jsonapi.go`) |
| SEC-13 | New shared generic does not leak internals in responses | PASS | `server.Nullable` is referenced in exactly one place tree-wide, a request struct: `grep -rn 'Nullable' --include='*.go' .` returns only `resource.go:142,147` outside its own file/test. It is in no response struct |

### Elevated-Risk Verdicts

**1. `backfill.go` mutating production rows at startup — PARTIAL FAIL (Important).**
The `WHERE` clause is correctly narrow on the four predicates it names
(`backfill.go:37-40`): `one_time = false`, both anchor columns still at their
defaults, both completion columns still at their defaults, `deleted_at IS NULL`.
Combined with `validate` (`builder.go:93-99`), which forces every newly created
never-completed recurring schedule to carry an anchor, no post-deploy row can
match — the selection genuinely targets only pre-task-030 rows. NULL handling is
correct: `due_date IS NULL` and `last_completed_date IS NULL` are the right tests
for the nullable columns, and `= 0` the right test for the `not null default 0`
ones. The idempotency claim at `:24-26` holds for every reachable path; the one
row shape that would write nothing (unrecognized recurrence type) is explicitly
skipped at `:60-64`.

Two defects:
- **Not safe under concurrent execution (Important).** The read at `:34-41` takes
  no lock and the write at `:84-85` is keyed on `id` alone, without re-asserting
  the selection predicate. With multiple replicas starting together the duplicate
  writes are benign (the anchor is deterministic from `created_at`/interval), but
  the SELECT-then-UPDATE window is not: if a user completes a schedule between one
  replica's `Scan` and its `Updates`, the backfill writes an anchor onto a row that
  now has a completion point, and `NextDue` prefers the anchor over the completion
  point per axis (`recurrence.go:43-46,51-54`) — the schedule's next-due is then
  frozen at the backfill anchor until the next completion clears it. `RecomputeAll`'s
  comparable job runs "under an advisory lock" (`processor.go:213`); `Backfill` has
  none. Minimum fix: append the selection predicate to the UPDATE's `WHERE`
  (`due_date IS NULL AND due_mileage = 0 AND last_completed_date IS NULL AND
  last_completed_mileage = 0`), which makes the write self-guarding without a lock.
- **The predicate omits `active` (Minor).** A pre-existing recurring schedule that
  a user deliberately deactivated and never completed is selected and has its
  `status`/`severity` rewritten (`:71-72`). Harmless in the UI but wider than the
  stated intent.
- **N+1 (acceptable).** One `Updates` per row (`:84-85`) for a one-shot startup job
  is acceptable; the population is bounded by pre-deploy schedule count. Note the
  query has no `LIMIT`/batching, so the whole candidate set is materialized in
  memory at `:33`; fine at current scale.

**2. `AdvanceTx` inactive guard — FAIL (Important).**
The guard is on the row the transaction loaded (`administrator.go:88-100`), and
failing it rolls back the record insert and mileage append because it runs inside
`CompleteInTransaction`'s `db.Transaction` (`completion_db.go:66`), verified by
`completion_db_test.go:287`. But the comment at `administrator.go:92-97` claims
this makes the check-then-act race safe — *"Only the check on the row THIS
transaction loaded is authoritative"* — and the code does not deliver that. The
load at `:89` is a plain `tx.First`, with no `clause.Locking{Strength: "UPDATE"}`
and no `SELECT ... FOR UPDATE`. Under Postgres READ COMMITTED (the default) two
concurrent completions of the same one-time schedule both read `active = true`,
both pass the guard, and both write a maintenance record — exactly the outcome
the comment says is prevented. The final `UPDATE` at `:138` is keyed on
`id = ?` only, so it does not serialize them either. Fix: either
`tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&e, "id = ?", id)`, or make
the update conditional (`Where("id = ? AND active = ?", id, true)`) and return
`server.ErrValidation` when `RowsAffected == 0`. Until then the comment is a
false safety claim and should not stand as written.

**3. `Administrator.Update` column map — PASS.**
All three new columns are present: `"one_time"` (`administrator.go:56`),
`"due_date"` (`:59`), `"due_mileage"` (`:60`). `due_date` is fed
`e.DueDate`, a `*time.Time` that `ToEntity` leaves nil for a zero time
(`entity.go:88-91`), so a cleared anchor writes SQL NULL rather than year 1 —
pinned by `entity_test.go:43` and `processor_update_test.go:120`.

**4. `Entity.Active` `default:true` hazard — PASS with a caveat (Minor).**
The hazard is real and correctly described: GORM omits a zero-valued field
carrying a `default:` tag from the INSERT, so `Insert` (`administrator.go:43`)
cannot persist `active=false`. It is adequately contained *today* — deactivation
happens only through `AdvanceTx`'s column-map `Updates` (`:136`) and
`Update`'s (`:61`), neither of which the `default` tag affects, and the create
handler never calls `SetActive` (`resource.go:97-106`). The tag itself is
pre-existing (`git show 285e175:./entity.go:23`). The caveat is that the
comment is on the wrong symbol: nothing prevents a future
`NewBuilder().SetActive(false)`, and `Builder.SetActive` (`builder.go:37`)
carries no warning at all. A one-line comment on `SetActive`, or removing the
setter (it has no production caller), would close it properly.

**5. `Nullable[T]` — PASS with a latent trap (Minor).**
`UnmarshalJSON` (`nullable.go:29-38`) is correct on all four inputs: absent (never
called, `Present` stays false — the zero value is the absent state), explicit null
(`:31-33`, `Present` true / `Valid` false), value (`:34-37`), and malformed
(`:35` propagates the error, which `RegisterInputHandler` turns into
`ErrValidation` at `packages/shared-go/server/handler.go:54-56`). `encoding/json`
passes the raw `null` literal with no surrounding whitespace, so the `bytes.Equal`
test is sound. All four states are pinned by `nullable_test.go:20-23,46,57`.
The absence of `MarshalJSON` **is** a latent trap: a `Nullable` placed in a
response struct would serialize as `{"Present":…,"Valid":…,"Value":…}` with no
compile-time or test-time complaint. It is not currently in any response struct
(SEC-13 above), and the doc comment at `:8-15` explains the type but says nothing
about output. Either add `MarshalJSON` or add one line to the doc comment saying
the type is request-only.

**6. `Processor.Update` ordering — PASS.**
The statement sequence is exactly as claimed: `apply` at `processor.go:80`,
`validate(updated)` at `:81-83` with an early return, then `NextDue` at `:85`,
`DueState` at `:87`, and only then `pr.a.Update` at `:89`. A rejected PATCH never
reaches the recompute or the write. Pinned by `processor_update_test.go:33,55`.

### Additional Findings

**A. `Processor.Update` derives status from the wrong mileage baseline (Minor,
branch-introduced asymmetry).** `processor.go:87` passes
`updated.LastCompletedMileage()` as `currentMileage` to `DueState` — the
schedule's completion odometer, not the vehicle's. This branch added
`Builder.SetCurrentMileage` (`builder.go:50-52`, used at `resource.go:105`)
specifically because *"without it a new mileage schedule is stored 'ok'
regardless of how close the vehicle already is to the due point"*
(`builder.go:19-22`), but the PATCH path still has that defect: PATCHing a
mileage schedule resets its stored `status` to `ok` for a vehicle already past
due. Self-healing — read paths compute `DueState` live (`processor.go:119,151,204`)
and the hourly `RecomputeAll` (`:220`) repairs the stored column — so the blast
radius is the stored column for up to an hour. The create path already has the
vehicle in hand at `resource.go:72`; the PATCH path does too, via
`requireScheduleFleet`. Pre-existing at merge base (`285e175:processor.go:79`),
but the branch is what made the inconsistency visible.

**B. Two stale comments say 400 where the code returns 422 (Minor).**
`resource.go:86` — *"an unparseable one is a 400 here"* — and
`nullable_test.go:45` — *"RegisterInputHandler turns a decode error into a 400"*.
Both paths write `server.ErrValidation`, which `StatusFor` maps to **422**
(`packages/shared-go/server/errors.go:43`). The code is right; the comments are
wrong, and one of them is the rationale a future reader would trust.

### Summary

#### Blocking (must fix)
- **Risk-2 / `administrator.go:88-100,138`** — `AdvanceTx`'s inactive guard reads
  the row with an unlocked `tx.First` and updates on `id` alone, so two concurrent
  completions of the same one-time schedule can both pass the guard and both write
  a maintenance record. The comment at `:92-97` asserts the opposite. Add
  `clause.Locking{Strength: "UPDATE"}` to the load, or make the UPDATE conditional
  on `active = true` and check `RowsAffected`.
- **Risk-1 / `backfill.go:84-85`** — the backfill UPDATE does not re-assert the
  SELECT predicate from `:37-40`, so a completion landing inside the
  SELECT→UPDATE window gets a stale anchor written back onto it, and `NextDue`
  prefers that anchor over the completion point (`recurrence.go:43-46,51-54`).
  Not safe under multi-replica startup. Append the four selection predicates to
  the UPDATE's `WHERE`.

#### Non-Blocking (should fix)
- **DOM-18 / `resource.go:60-68,137-150,244-247`** — anonymous inline request
  structs instead of named unexported `createAttributes`/`patchAttributes` in
  `rest.go`. Pre-existing package style; the branch extended it.
- **Risk-1b / `backfill.go:37-40`** — predicate omits `active`, so deliberately
  deactivated never-completed schedules also get `status`/`severity` rewritten.
- **Risk-4 / `builder.go:37`** — `Builder.SetActive` is an unguarded foot-gun
  against the `default:true` tag; the mitigating comment lives on
  `administrator.go:38-41` instead. Comment or remove the setter.
- **Risk-5 / `nullable.go:8-15`** — no `MarshalJSON`; a `Nullable` in a response
  struct would silently serialize its three internal fields. Add the method or a
  request-only doc line.
- **Finding A / `processor.go:87`** — PATCH derives `DueState` from
  `LastCompletedMileage()` rather than the vehicle odometer that
  `Builder.SetCurrentMileage` was added to supply.
- **Finding B / `resource.go:86`, `nullable_test.go:45`** — comments say 400;
  `server.ErrValidation` maps to 422.

---

## Frontend Guidelines Audit

- **Audit Scope:** TS/TSX changed on `task-030-one-time-maintenance-schedules` (merge base `285e175` → head `36145e0`)
- **Guidelines Source:** `.claude/skills/frontend-dev-guidelines` (SKILL.md + all 12 `resources/*.md`)
- **Date:** 2026-08-25
- **Build:** PASS (`make fe-build`, exit 0)
- **Tests:** 822 passed / 0 failed (apps/web, 101 files); 9 passed (shared-ts); 10 passed (ui-components)
- **Overall:** NEEDS-WORK (FE-16 FAIL; three non-checklist Important findings)

### Build & Test Results

```
make fe-build → exit 0 (vite build; only chunk-size advisories)
make fe-test  → apps/web: Test Files 101 passed (101), Tests 822 passed (822)
                shared-ts: 2 files / 9 tests passed
                ui-components: 1 file / 10 tests passed
```

### File Inventory

- `apps/web/src/types/models/maintenanceSchedule.ts` — **Type**
- `apps/web/src/lib/schemas/maintenanceSchedule.ts` (+ `.test.ts`) — **Schema**
- `apps/web/src/components/features/vehicles/maintenance/MaintenanceScheduleForm.tsx` (+ `.test.tsx`) — **Component (form)**
- `apps/web/src/components/features/vehicles/maintenance/ScheduleCard.tsx` (+ `.test.tsx`) — **Component (new)**
- `apps/web/src/components/features/vehicles/detail/UpcomingScheduleStrip.tsx` (+ `.test.tsx`) — **Component**
- `apps/web/src/components/features/vehicles/dialogs/AddScheduleDialog.tsx` — **Component (dialog)**
- `apps/web/src/components/features/vehicles/dialogs/ConvertToRecurrenceDialog.tsx` (+ `.test.tsx`) — **Component (dialog, new)**
- `apps/web/src/components/features/vehicles/dialogs/CompleteScheduleDialog.tsx` (+ `.test.tsx`) — **Component (dialog)**
- `apps/web/src/pages/VehicleDetailPage.tsx` — **Page**
- `apps/web/src/lib/vehicleStats.test.ts` — **Other (fixture)**

No **Service** and no **Hook** file changed on this branch (`git diff --stat … -- apps/web/src/services apps/web/src/lib/hooks` → empty).

### Anti-Pattern Checklist

| ID | Check | Status | Evidence |
|----|-------|--------|----------|
| FE-01 | No `any` | PASS | `grep -nE ": any\|as any"` over all 9 in-scope source files → 0 matches |
| FE-02 | No manual class concat | PASS | grep for a `className={` opening a string literal, and for `+` inside a `className={…}` expression → 0 matches across the 9 in-scope files; `cn()` used at `ScheduleCard.tsx:89,91` |
| FE-03 | No client/service call in components | PASS | `grep -nE "lib/api/client\|services/api/"` over the 7 component/page files → 0 matches. Every dialog goes through hooks: `ConvertToRecurrenceDialog.tsx:11`, `CompleteScheduleDialog.tsx:10-12`, `AddScheduleDialog.tsx:6-9`. No `useState`+`useEffect`+service shape present. |
| FE-04 | No inline Zod in components | PASS | `grep -n "z.object(\|from 'zod'"` over components → 0 matches; only `lib/schemas/maintenanceSchedule.ts:1,35,115,147` |
| FE-05 | No content spinners | PASS | `animate-spin` at `MaintenanceScheduleForm.tsx:290`, `ConvertToRecurrenceDialog.tsx:189`, `CompleteScheduleDialog.tsx:139` — all three gated on a pending submit inside a submit `<Button>` |
| FE-06 | No hardcoded colors | PASS | `apps/web/src/test/conventions.test.ts:113-115` executes the palette regex repo-wide and passed in Phase 1; `git diff … -- apps/web/src/test/` is empty, so the two-line allowlist (`:130-133`) was not extended. Tones use tokens: `ScheduleCard.tsx:83,85` (`border-danger-border`, `bg-warning-subtle/45`), declared at `index.css:56-62,114-118` |
| FE-07 | No state mutation | PASS | `.push` hits are on function-local arrays (`ScheduleCard.tsx:27,49`); `.sort` at `UpcomingScheduleStrip.tsx:59` sorts a copy (`[...schedules]`) |
| FE-08 | No default exports | PASS | `grep -n "export default"` over in-scope files → 0 matches |
| FE-09 | `createErrorFromUnknown` | PASS | `AddScheduleDialog.tsx:3,59-60`, `ConvertToRecurrenceDialog.tsx:5,86-87`, `CompleteScheduleDialog.tsx:5,84-85` — all import from `@myfleet/shared-ts`, single argument, fallback via `apiError.message \|\|`, `toast.error` on every catch, dialog left open on failure |

### Architecture Checklist

| ID | Check | Status | Evidence |
|----|-------|--------|----------|
| FE-10 | JSON:API model shape | PASS | `types/models/maintenanceSchedule.ts:31` (`JsonApiResource<MaintenanceScheduleAttributes>`), attributes at `:6-29`, separate write types at `:34-43` (create), `:46-60` (update), `:63-67` (complete). Doc comment names the backend struct (`:3-5`); the tri-state on `dueDate` is documented at `:51-56` |
| FE-11 | Service via `apiClient` only | OUT-OF-SCOPE | `git diff --stat 285e175..36145e0 -- apps/web/src/services` → empty; `MaintenanceScheduleService.ts:29-35` (unchanged) still extends `BaseService` |
| FE-12 | Query keys `as const` | PASS | No hook module changed; the keys the new code invalidates through are `lib/hooks/api/maintenance.ts:51-60`, every tier `as const` |
| FE-13 | RHF + `zodResolver` | PASS | `MaintenanceScheduleForm.tsx:47-48`, `ConvertToRecurrenceDialog.tsx:47-48`, `CompleteScheduleDialog.tsx:52-53` |
| FE-14 | Schema in `lib/schemas/` + inferred type | PASS | `lib/schemas/maintenanceSchedule.ts` (no `.schema.` infix); `:107`, `:141`, `:156` export `z.infer` types for all three schemas |

### Styling Checklist

| ID | Check | Status | Evidence |
|----|-------|--------|----------|
| FE-15 | `cursor-pointer` on interactive elements | PASS | Every clickable surface in scope is a `<Button>` (`ScheduleCard.tsx:110,116`, `UpcomingScheduleStrip.tsx:66`, `ConvertToRecurrenceDialog.tsx:185,188`) or a `SelectTrigger`; `button.tsx:7` and `select.tsx:17,111` carry `cursor-pointer` in their base strings. No hand-rolled clickable `div`/row added. |

### Testing Checklist

| ID | Check | Status | Evidence |
|----|-------|--------|----------|
| FE-16 | Tests for changed components | **FAIL** | `AddScheduleDialog.tsx:40-62` gained non-trivial mapping logic (`kind → oneTime`, interval stripping, `YYYY-MM-DD → RFC3339`) and has **no test file**: `ls .../dialogs/` shows no `AddScheduleDialog.test.tsx`, and `grep -rln "AddScheduleDialog" apps/web/src` returns only the component and `VehicleDetailPage.tsx`. Nothing anywhere asserts the create payload. `VehicleDetailPage.tsx` (new `convertingSchedule` state + dialog wiring) is also untested, consistent with its pre-existing absence of a page test. All other changed components have sibling tests. |
| FE-17 | `vi.mock` call sites updated | OUT-OF-SCOPE | No service module changed. The new stubs are correct against current signatures: `ConvertToRecurrenceDialog.test.tsx:10-12` (`patch`), `CompleteScheduleDialog.test.tsx:12-14` (`complete`). Note `CompleteScheduleDialog.test.tsx:18-31` hand-reimplements `vehicleKeys` inside the mock of `lib/hooks/api/vehicles` — a divergence risk if the real factory changes. |

### Elevated-Risk Verdicts

1. **Anchor defaulting effects — PASS with a Minor.** `MaintenanceScheduleForm.tsx:82-92` and `:101-105` cannot loop: effect 1 early-returns when `!recurring` (`:83`), effect 2 early-returns when `recurring` (`:102`), and neither writes a value that appears in the other's dep array. Dep arrays are complete (`:92`, `:105`); `setValue` is destructured at `:69` so the closure is stable. Ref guards are set only from the field's own `onChange` (`:242`, `:269`) and reset on remount — `AddScheduleDialog.tsx:66-78` renders the form inside `DialogContent` with no `forceMount`, so Radix unmounts it on close and no touched-state survives a dialog reopen. No programmatic `setValue` bypass exists (only these two effects call it). Minor: `toDateInputValue` (`:26-28`) formats a *local* `Date` through `toISOString()`, so the defaulted first-due date is off by one day for users west of UTC late in the day.
2. **`ConvertToRecurrenceDialog` PATCH — PASS.** The payload (`:70-81`) sets `oneTime: false`, `active: true`, both intervals (`?? 0` zeroes the axis the recurrence type does not cover), `dueDate: null` and `dueMileage: 0`. That matches the server contract exactly: `apps/fleet-service/internal/maintenanceschedule/resource.go:138-150` types `DueDate` as `server.Nullable[string]` and `DueMileage` as `*int`, and `:190-205` treats present-but-invalid as "clear". `BaseService.patch` (`BaseService.ts:55-61`) `JSON.stringify`s the attributes verbatim, so `null` survives and `undefined` keys are dropped — the tri-state is preserved end to end. No payload anywhere carries `recurrenceType: 'oneTime'` (`AddScheduleDialog.tsx:45`, `ConvertToRecurrenceDialog.tsx:72` both pass the schema enum). Failure keeps the dialog open (`:85-88` does not call `onOpenChange`), pinned by `ConvertToRecurrenceDialog.test.tsx:98-110`.
3. **`ScheduleCard` state distinctions — PASS.** "One-time" is a text badge (`:97-101`), "completed" is the literal string `Completed …` (`:47-53`, rendered at `:106`) — neither is colour-only; both are asserted by text in `ScheduleCard.test.tsx:77,121`. `canWrite: false` suppresses **both** actions (`:109`, `:115`), pinned at `ScheduleCard.test.tsx:61-64,134-137`.
4. **Comparator — PASS.** `UpcomingScheduleStrip.tsx:37-43` is total and antisymmetric: the boolean tier at `:40`, `rankSchedule` (`vehicleStats.ts:230-238`) returns 0/1/2 for every input, and equal elements return 0 with `Array.prototype.sort` being spec-stable. The `NaN` path needs a malformed `lastCompletedDate`; the backend formats RFC3339 and omits the field when zero (`maintenanceschedule/rest.go:36,69-70`), so it is unreachable from this API. Missing dates coerce to `0`, not `NaN` (`:24`).
5. **Accessibility — PASS with a repo-wide Minor.** The toast action (`CompleteScheduleDialog.tsx:73-78`) is sonner's `action` object, which renders a real `<button>` whose accessible name is the `label` string, inside the app-level `<Toaster>` (`AppProviders.tsx:19,51`). Caveat: sonner is mocked in `CompleteScheduleDialog.test.tsx:38-43`, so the assertion is on the options object (`:100-101`), never on a rendered role — the control's reachability is unverified by the suite. Neither new dialog sets `DialogDescription`, which makes Radix log a missing-`aria-describedby` warning; this is repo-wide (`grep -rn "DialogDescription" apps/web/src/components` returns only `ui/dialog.tsx` and the `AlertDialog` variant in `MemberList.tsx`), so it is a convention worth fixing globally rather than a branch defect.
6. **React Query — FAIL (Important).** The conversion reuses `useUpdateMaintenanceSchedule` (`lib/hooks/api/maintenance.ts:298-315`), which invalidates only `maintenanceScheduleKeys.lists()` and `.detail(id)`. Reactivating a schedule also changes (a) the vehicle's server-derived `nextDue` under `vehicleKeys.detail(vehicleId)` and (b) the fleet upcoming/overdue queues under `maintenanceScheduleKeys.queues()` — neither is invalidated. The sibling completion mutation does invalidate `vehicleKeys.detail` (`:347-351`), so the asymmetry is inside one user flow. No mock hides this: `ConvertToRecurrenceDialog.test.tsx` stubs only the service and uses a real `QueryClient`, so the real `onSettled` runs — nothing asserts on it.

### Summary

#### Blocking (must fix)
- **FE-16** — `AddScheduleDialog.tsx:40-62` has no test; the create-payload mapping (`kind → oneTime`, interval stripping, date→RFC3339) is asserted nowhere in the suite.
- **Important / invalidation** — `lib/hooks/api/maintenance.ts:298-315` (used by `ConvertToRecurrenceDialog.tsx:68`) does not invalidate `vehicleKeys.detail(vehicleId)` or `maintenanceScheduleKeys.queues()` after a conversion reactivates a schedule.
- **Important / stale props** — `ConvertToRecurrenceDialog.tsx:60-64` reads `lastCompletedDate` / `lastCompletedMileage` off the schedule object captured *before* the completion PATCH (`CompleteScheduleDialog.tsx:76` → `VehicleDetailPage.tsx:91-93`). On the primary toast path, a first-time completion has no prior completion, so `anchorParts` is empty and the "Repeats from the completion you just recorded" line (`:100-104`) never renders — or, for a re-completed schedule, renders the *previous* completion. The PATCH itself is unaffected (the server derives the anchor).

#### Non-Blocking (should fix)
- **Minor / timezone** — `MaintenanceScheduleForm.tsx:26-28` (`toISOString` on a local `Date`) yields a next-day default anchor for users west of UTC; `CompleteScheduleDialog.tsx:50,65` and `AddScheduleDialog.tsx:53` round-trip `YYYY-MM-DD` through UTC midnight with the same class of drift.
- **Minor / UX** — once the user edits the first-due field, `dueDateTouched` (`MaintenanceScheduleForm.tsx:79`) latches for the life of the mount; clearing the field afterwards leaves it empty with a validation error and no way to recover the computed default.
- **Minor / UX** — an auto-computed `dueDate` survives a `recurring → oneTime` switch (only intervals are cleared, `:101-105`), so a one-time schedule can inherit a due date the user never chose. It is visible in the field, so it is disclosed rather than silent.
- **Minor / convention** — none of the three dialogs guards close while the mutation is pending (`dismissible` / `onOpenChange`), which `patterns-forms-validation.md` documents as the reference behaviour; only `VehiclesPage.tsx` does it today.
- **Minor / a11y** — no `DialogDescription` / `aria-describedby` on the new dialogs (repo-wide convention gap).
- **Minor / test fragility** — `CompleteScheduleDialog.test.tsx:18-31` hand-reimplements `vehicleKeys` inside a module mock; it will silently diverge if the real factory changes.
