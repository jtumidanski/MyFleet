# Task 030 — Implementation Context

Companion to `plan.md`. Everything an implementer needs that is not a plan
step: where the code lives, why the decisions went the way they did, and what
will bite.

---

## 1. The problem in one paragraph

Every maintenance schedule in MyFleet is recurring, and next-due is *derived*
rather than stored: `NextDue()` returns `lastCompletedDate + intervalMonths` and
`lastCompletedMileage + intervalMiles` (`recurrence.go:22`). There is no way to
express "state inspection due 2026-11-30" or "new tires at 60,000 miles". The
same missing primitive causes an adjacent defect — a newly created recurring
schedule has a zero `lastCompletedDate`, so its computed next-due is year 1 and
it is born `overdue` (`builder.go:49`). Both are the same thing: an absolute due
point stored on the row that `NextDue` prefers over interval arithmetic.
One-time schedules keep it forever; recurring schedules keep it for exactly one
cycle and drop it at the first completion.

---

## 2. Why the change is small

`DueState`, `DueBreaches`, `Severity`, `AxisBreach`, `Thresholds`,
`DueCycleToken`, the queue handlers, the internal due feed, the vehicle status
banner and the dashboard counts all consume `NextDue` and the `recurrenceType`
axis flags. **Neither changes meaning.** Every one of those consumers therefore
inherits one-time support without being touched, which is why the acceptance
criteria can demand "appears in the queues, banner, dashboard and internal
feed" while the plan lists no work in any of those places.

The frontend gets the same free ride: `nextDueDate` / `nextDueMileage` continue
to carry the *effective* due point, so `deriveNextService`, `rankSchedule`,
`vehicleBanner`, `MaintenanceQueueView` and `VehicleStatStrip` need no change.

`recurrenceType` keeps meaning **which axes the schedule is judged on**, not
**how often it repeats**. `oneTime` is the orthogonal flag. The name invites the
opposite reading, and a reviewer who assumes `oneTime` should have been a fourth
`recurrenceType` value will keep asking why it isn't — the answer is in
design §1: a one-off can be due by date, by odometer, or by either, so `once`
would have to become `once`/`once_mileage`/`once_hybrid` and every one of the
six `type == "time" || type == "hybrid"` axis tests would grow a parallel arm.

---

## 3. Key files

### Read before starting (backend)

| File | Why |
|---|---|
| `apps/fleet-service/internal/maintenanceschedule/recurrence.go:22-30` | `NextDue` — the whole feature is two `switch` statements here |
| `apps/fleet-service/internal/maintenanceschedule/recurrence.go:33-50` | `DueState` — reads a zero due-date as `overdue`, which is why validation and the backfill both exist |
| `apps/fleet-service/internal/maintenanceschedule/builder.go:35-49` | Where the invariants live today, and the `lastCompletedMileage`-as-`currentMileage` defect |
| `apps/fleet-service/internal/maintenanceschedule/processor.go:68-78` | `Update` — applies, recomputes, writes; never validates. FR-UPD-2 exists because of this |
| `apps/fleet-service/internal/maintenanceschedule/administrator.go:47-60` | `Update`'s explicit column map — omitting a column here is the "PATCH succeeds and changes nothing" failure mode |
| `apps/fleet-service/internal/maintenanceschedule/administrator.go:76-101` | `AdvanceTx` — already loads the entity and issues the one atomic `Updates`, which is why deactivation belongs here |
| `apps/fleet-service/internal/maintenanceschedule/completion_db.go:63-95` | `CompleteInTransaction` — the single `db.Transaction` the record, mileage and advance all run in |
| `apps/fleet-service/internal/maintenanceschedule/rest.go:11-24` | `DueCycleToken` — must stay byte-identical to notification-service's derivation |
| `apps/fleet-service/internal/maintenancecategory/entity.go:103` | `Seed` — the idempotent-startup-job pattern `Backfill` follows |
| `apps/fleet-service/cmd/main.go:67` | Where `Seed` is wired, and where `Backfill` goes |
| `packages/shared-go/server/handler.go:45-58` | `RegisterInputHandler` decodes straight into the attrs struct — why `Nullable` has to be a field type |

### Read before starting (frontend)

| File | Why |
|---|---|
| `apps/web/src/components/features/vehicles/detail/UpcomingScheduleStrip.tsx` | Despite the name, the vehicle's whole schedule list — `VehicleDetailPage.tsx:192` feeds it `ListByVehicle`, inactive rows included |
| `apps/web/src/lib/vehicleStats.ts:230-239` | `rankSchedule` — reads `status`, which is meaningless for a deactivated row |
| `apps/web/src/lib/schemas/maintenanceSchedule.ts` | The existing `superRefine`, which the new matrix extends rather than replaces |
| `apps/web/src/components/features/vehicles/dialogs/CompleteScheduleDialog.tsx:56-70` | The success branch the conversion toast action attaches to |
| `apps/web/src/pages/VehicleDetailPage.tsx:37,53-55,84-87` | `OpenDialog`, the dialog state, and the `handleComplete` pattern `handleConvert` copies |
| `apps/web/src/components/features/vehicles/maintenance/MaintenanceRecordForm.test.tsx:1-46` | The form-test conventions: `QueryClientProvider` wrapper, `scrollIntoView` stub, role-based queries |

### Not changed, and deliberately so

- **`apps/notification-service` — no code change.** `reminder.DueCycleToken`
  builds `"<next_due_date>|<next_due_mileage>"` from the feed
  (`internal/reminder/job.go:107`) and `consumer.OverdueDedupeKey` composes
  `"<user>:overdue:<schedule>:<cycle>"` (`consume.go:164`), which
  `notification.Processor.Generate` dedupes on. A one-time schedule's anchor
  never moves, so the token is constant for the row's whole life and each user
  gets **exactly one** overdue notification no matter how long it stays overdue.
  This resolves the PRD's open question, and Task 9 pins it with a fleet-service
  test so a future `NextDue` change cannot quietly break the dedupe.
- **`apps/web/src/services/api/MaintenanceScheduleService.ts` and
  `lib/hooks/api/maintenance.ts` — no change needed.** `BaseService` is generic
  over the attribute types, so extending
  `Create/UpdateMaintenanceScheduleAttributes` (Task 10) is enough; the
  conversion reuses `useUpdateMaintenanceSchedule` as-is.
- **`deploy/k8s` — no change.** Both overlays must render byte-identical.

---

## 4. Decisions and their reasoning

### `Nullable[T]` rather than an empty-string sentinel

FR-UPD-3 requires `"dueDate": null` to clear the anchor. `encoding/json` cannot
distinguish an absent key from an explicit `null` on a `*string`. Two
alternatives were weighed: an **empty-string sentinel** (`"dueDate": ""` means
clear) needs no shared-go change but deviates from the PRD's literal wording and
bakes a magic value into the public API; **deriving the clear server-side**
(clear whenever `oneTime` flips false) removes the client's explicit control and
couples two independent fields. `Nullable[T]` costs about thirty lines plus a
table test and is reusable by every future PATCH in the repo, several of which
already carry the absent-vs-null ambiguity silently.

`dueMileage` stays a `*int`: `0` is already its "unset" encoding at the column,
the model and `NextDue`, so `{"dueMileage": 0}` is unambiguous without new
machinery. Only `dueDate`, whose cleared state is a `NULL` column, needs
`Nullable`.

### `due_date` nullable, `due_mileage` not

This matches how the table already treats the two kinds:
`LastCompletedDate`/`NextDueDate` are `*time.Time`,
`LastCompletedMileage`/`NextDueMileage` are plain `int` with `0` meaning unset.
Following that split rather than "correcting" it keeps `Make`/`ToEntity`
symmetric and avoids a `*int` no other column in the table uses.

### The anchor requirement, and where it deviates from the design

design §3 states the carve-out as "rules 6 and 7 apply only when the schedule
has never been completed **or is one-time**". The plan implements a slightly
different predicate:

```go
neverCompleted := m.lastCompletedDate.IsZero() && m.lastCompletedMileage == 0
if (m.oneTime && m.active) || (!m.oneTime && neverCompleted) { … }
```

Two deliberate differences, both closing holes in the literal wording:

1. **A completed one-time schedule is exempt.** Under the design's wording it is
   `oneTime`, so the anchor rule fires — but the completion flow deliberately
   cleared its anchor, making the row reject every PATCH except the conversion
   (which flips `oneTime` off and is therefore exempt). Gating on `active`
   instead exempts the terminal state without weakening anything: the row is
   deactivated, so nothing reads its due point.
2. **A live one-time schedule always needs an anchor, completed or not.** Under
   the design's "never completed" half, a PATCH setting `oneTime: true` on an
   already-completed recurring schedule would pass validation and produce a live
   row whose `NextDue` returns the zero value — permanently overdue. The PRD
   explicitly permits that PATCH at the API level ("no UI affordance is
   required"), so the validator is the only thing standing between it and a bad
   row.

If a reviewer objects, the fallback is the design's literal rule; the plan's
tests (`TestValidate_completedOneTimeIsInactiveAndValid`,
`TestValidate_reactivatedOneTimeNeedsAnAnchor`) are what encode the difference.

### The guard lives in `AdvanceTx`, not only in the handler

The handler pre-check gives the good error message and avoids opening a
transaction. But check-then-act across two statements is a race: two concurrent
completes of the same one-time schedule both pass the handler check, both open
transactions, and both write a maintenance record. Only the check *inside* the
transaction, on the row that transaction loaded, is authoritative — and its
failure rolls back the record insert and the mileage append with it. That is
exactly the acceptance criterion "verified by an integration test that fails the
last step and asserts nothing was written".

The guard also rejects completing an inactive *recurring* schedule. That is
correct and is what FR-COMPLETE-4 says, not a one-time-only rule.

### Clearing the anchor before computing `NextDue`

`AdvanceTx` calls `WithDuePoint(time.Time{}, 0)` **before** `NextDue`. Skipping
that ordering is the subtle bug: a recurring schedule's stale first-due anchor
would outrank the fresh completion point and `next_due_date` would never move.

### The completed one-time schedule's stored status is forced to `ok`

After the anchor is cleared, `NextDue` returns the zero value, and `DueState`
reads a zero due-date as `overdue`. The row is deactivated in the same update so
nothing live reads it — but the stored `status` is what the vehicle's schedule
list renders for an inactive row. Forcing `ok` keeps the card honest.

### Backfill in Go, not one `UPDATE ... FROM`

The date arithmetic needs `created_at + N months` where `N` is a column, which
in Postgres is `created_at + (interval_months || ' months')::interval` —
Postgres-specific syntax the package's sqlite-backed harness cannot execute.
Doing it in Go with `AddDate(0, n, 0)` keeps the backfill testable in the same
harness as everything else it must not break and reuses the exact call `NextDue`
uses. The cost is N updates instead of one; the table is small, it runs once at
boot, and the selection predicate makes every later boot a no-op after a single
count.

The plan's backfill also refreshes `next_due_*`, `status` and `severity` in the
same pass. That is a small addition beyond design §6: `TransformInternalDue`
reads the **stored** `next_due_*` columns, so a repaired row would otherwise
carry a stale reminder token until the next hourly recompute — one spurious
notification per repaired schedule.

Idempotence falls out of the predicate rather than a flag: a time-only schedule
ends with `due_date` non-null, a mileage-only one with `due_mileage != 0`, and
the selection requires **both** to still be at their defaults.

### One Zod object with a `superRefine`, not a discriminated union

A union would type more precisely, but `zodResolver` over a union fights
react-hook-form's single `defaultValues` object and its shared `categoryId` /
`recurrenceType` fields, and the form would have to remount on every kind change
to keep the resolver honest.

### `ScheduleCard` is extracted before it grows

The per-schedule card is a 30-line inline JSX block that this task would roughly
double (one-time badge, completed treatment, completion line, second action
button). Extracting first is the difference between one component with two
responsibilities and two with one each, and it gives the completed-state
rendering a test target that does not need a whole strip.

### Two entry points to the conversion

The post-completion toast (FR-CONV-1) and a `Set up recurrence` button on a
completed one-time card. The PRD deferred the second as open question 3; it is
one button and one piece of state once the dialog exists, and it removes the
only way to permanently lose the affordance — dismissing a toast.

---

## 5. What will bite

- **`RegisterInputHandler` decodes into the attrs struct directly.** A `*string`
  cannot express "explicit null". If `Nullable` is skipped, the conversion PATCH
  silently leaves the anchor in place and the converted schedule stays pinned to
  the due date it just completed. There is no error; it just doesn't work.
- **`dbAdministrator.Update` writes an explicit column map.** A column not named
  there is not written. This is the second silent-no-op failure mode, and
  `TestProcessorUpdate_conversionDerivesFromCompletionPoint` is the tripwire.
- **The sqlite harnesses use explicit DDL, not `AutoMigrate`.** Both
  `completion_db_test.go` and `admin/admintest/db.go` create
  `fleet.maintenance_schedules` by hand. Adding entity fields without adding the
  columns produces `no such column` on the first write. Task 2 step 6 does both.
- **Existing frontend schema tests all break in Task 10.** Every case in
  `lib/schemas/maintenanceSchedule.test.ts` now needs a `kind` and a due point.
  The plan rewrites the file wholesale rather than patching it.
- **`MaintenanceScheduleForm.tsx` and `AddScheduleDialog.tsx` stop type-checking
  between Tasks 10 and 11**, and `VehicleDetailPage.tsx` between Tasks 13 and
  15. That is expected — run `make fe-build` at the end of Task 11 and Task 15,
  not in between.
- **The anchor-defaulting `useEffect` must not call `setValue` with
  `shouldDirty`.** The whole guard is `form.formState.dirtyFields.dueDate`; if
  the effect dirties the field itself, the default stops tracking after the
  first write and the "recomputes when the interval changes" behaviour
  disappears.
- **`rankSchedule` is unchanged on purpose.** It reads `status`, which describes
  a finished cycle for a deactivated row. The active/inactive tier check has to
  come *first* in the comparator, not be folded into the rank.
- **`make ci` is the gate, and Node is not always on `PATH`.** Load nvm first:
  `export NVM_DIR="$HOME/.nvm" && . "$NVM_DIR/nvm.sh" && nvm use 22`.

---

## 6. Dependencies between tasks

```
1 (Nullable) ─────────────────────────────┐
                                          ▼
2 (columns) → 3 (NextDue) → 4 (validate) → 8 (transport) → 9 (transport tests)
                     │           │  ▲
                     │           ▼  │
                     │      5 (Update validates)
                     │           │
                     ▼           ▼
                    6 (AdvanceTx + Update column map)
                     │
                     ▼
                    7 (backfill + main.go)

10 (types + schema) → 11 (form) → 12 (ScheduleCard) → 13 (strip)
                            └───→ 14 (convert dialog) ──┐
                                                        ▼
                                             15 (toast + page wiring)
                                                        │
                                                        ▼
                                                16 (make ci + overlays + review)
```

Tasks 1–9 must all land before Task 10 begins: the frontend types mirror the
JSON keys Task 8 emits, and there is no value in a UI that talks to a server
that cannot store what it sends.

---

## 7. Acceptance-criteria map

| PRD §10 criterion | Where it is proved |
|---|---|
| Create with `oneTime` + a due point; rejected without one | Task 4 `TestBuild_validationMatrix` |
| One-time with a non-zero interval rejected | Task 4, same table |
| Recurring without a first-due point rejected | Task 4, same table |
| New recurring schedule reports `ok`, not `overdue` | Task 4 `TestBuild_newRecurringScheduleIsNotBornOverdue` |
| One-time `upcoming` / `overdue` / hybrid-either-axis | Task 3 `TestNextDue_duePointAndOneTime` + `TestNextDue_oneTimeWithoutAnchorIsZero`; `DueState` itself is unchanged and already covered |
| Appears in queues, banner, dashboard, internal feed | Inherited — every one of those reads `NextDue`; Task 9 pins the feed row |
| Completion writes record + mileage + `active:false` in one tx | Task 6 `TestCompleteInTransaction_oneTimeDeactivates` |
| Disappears from queues/banner/dashboard/feed after completion | Inherited — all four read active-only paths |
| Completing an inactive schedule returns a validation error, nothing written | Task 6 `TestCompleteInTransaction_inactiveScheduleWritesNothing`; Task 8 adds the handler pre-check |
| Recurring completion stays active, anchor cleared | Task 6 `TestCompleteInTransaction_recurringClearsAnchorAndStaysActive` |
| Toast offers "Set up recurrence" only for one-time | Task 15 `CompleteScheduleDialog.test.tsx` |
| Conversion submits one PATCH; result is recurring, active, unanchored | Task 14 test + Task 6 `TestProcessorUpdate_conversionDerivesFromCompletionPoint` |
| Failed conversion leaves the schedule completed, error toast, dialog open | Task 14 test |
| PATCH leaving a hybrid with a zero interval rejected | Task 5 `TestProcessorUpdate_rejectsZeroIntervalOnHybrid` |
| List badges one-time, de-emphasizes completed, sorts them below | Tasks 12 and 13 |
| Backfill anchors, exempts, and is a no-op on a second run | Task 7 |
| `make ci` passes | Task 16 |
| Both overlays render unchanged | Task 16 |

---

## 8. Out of scope

Restating the PRD's non-goals where implementation might drift into them:

- No notification-service change. The work is to *verify* the contract is
  unchanged, not to touch it.
- No filtering or collapsing of completed schedules in the vehicle list. If a
  vehicle accumulates enough one-offs to make the list noisy, that is a
  follow-up task with its own PRD.
- No UI for converting a *recurring* schedule to one-time. The PATCH accepts it;
  nothing offers it.
- No richer recurrence rules. `intervalMonths` / `intervalMiles` remain the
  entire vocabulary.
- No editing or reopening a completed one-time schedule beyond the conversion.
