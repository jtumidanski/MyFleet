# One-Time Scheduled Maintenance — Design

Task: task-030-one-time-maintenance-schedules
PRD: `prd.md` (v1, approved)
Status: Draft
Created: 2026-08-25

---

## 1. The central idea

The PRD asks for two things that look separate and are not:

1. a schedule that is due once, at a fixed point, and never repeats;
2. a first-due anchor so a newly created recurring schedule is not born overdue.

Both are the same primitive: **an absolute due point stored on the row, which
`NextDue` prefers over interval arithmetic.** One-time schedules keep that point
forever; recurring schedules keep it for exactly one cycle and drop it at the
first completion.

That framing is what keeps the change small. `DueState`, `DueBreaches`,
`Severity`, `AxisBreach`, `Thresholds`, `DueCycleToken`, the queue handlers, the
internal due feed, the vehicle status banner, and the dashboard counts all
consume `NextDue` and the `recurrenceType` axis flags. Neither of those changes
meaning. Every one of those consumers therefore inherits one-time support
without being touched — which is also why the acceptance criteria can demand
"appears in the queues, banner, dashboard, and internal feed" without listing
work in those places.

`recurrenceType` keeps meaning *which axes the schedule is judged on*, not *how
often it repeats*. `oneTime` is the orthogonal flag that says whether the axes
recur. This is worth stating explicitly because the name `recurrenceType` invites
the opposite reading, and a reviewer who assumes `oneTime` should have been a
fourth `recurrenceType` value will keep asking why it isn't.

### Alternative considered: `recurrenceType: "once"`

Adding a fourth enum value instead of a boolean was the first thing to test. It
fails on the axis question: a one-time item can be due by date, by odometer, or
by either — the same three-way choice recurring schedules already make. Encoding
that as `once`, `once_mileage`, `once_hybrid` triples the enum and forces every
`type == "time" || type == "hybrid"` axis test in `recurrence.go` (there are six)
to grow a parallel one-time arm. The boolean keeps the axis vocabulary intact
and confines the branch to two places inside `NextDue`.

### Alternative considered: a separate `one_time_tasks` table

A distinct domain with its own model, provider, administrator, resource, and
transport would give one-time items a clean home. It would also require every
consumer named above to read and merge two sources: two queue queries, two
branches in the vehicle banner, a merged internal feed, a merged dashboard count.
The PRD's non-negotiable is that one-time items ride the existing machinery with
no consumer-side special-casing. A separate table contradicts that requirement
directly. Rejected.

---

## 2. Data model

`fleet.maintenance_schedules` gains three columns, all additive, all landing via
the existing `AutoMigrate` path (`entity.go:32`, wired at `cmd/main.go:55`):

| Column | Type | Null | Default |
| --- | --- | --- | --- |
| `one_time` | boolean | not null | `false` |
| `due_date` | timestamp | null | `NULL` |
| `due_mileage` | integer | not null | `0` |

`due_date` is nullable and `due_mileage` is not, matching how the file already
treats the two kinds: `LastCompletedDate`/`NextDueDate` are `*time.Time`,
`LastCompletedMileage`/`NextDueMileage` are plain `int` with `0` meaning unset.
Following that split rather than "correcting" it keeps `Make`/`ToEntity`
symmetric and avoids a `*int` that no other column in the table uses.

Because both columns are additive with safe defaults, a running previous version
of fleet-service is unaffected by the migration and the deploy needs no
coordination.

### Domain model

`Model` (`model.go`) gains `oneTime bool`, `dueDate time.Time`, `dueMileage int`
with accessors, plus two copy-withs in the existing immutable style:

```go
func (m Model) WithOneTime(v bool) Model            { m.oneTime = v; return m }
func (m Model) WithDuePoint(d time.Time, mi int) Model { m.dueDate = d; m.dueMileage = mi; return m }
```

`AsSchedule()` carries all three into `Schedule`, which gains `OneTime bool`,
`DueDate time.Time`, `DueMileage int`.

### The revised `NextDue`

```go
func NextDue(s Schedule) (nextDate time.Time, nextMiles int) {
    if s.RecurrenceType == "time" || s.RecurrenceType == "hybrid" {
        switch {
        case !s.DueDate.IsZero():
            nextDate = s.DueDate
        case !s.OneTime:
            nextDate = s.LastCompletedDate.AddDate(0, s.IntervalMonths, 0)
        }
    }
    if s.RecurrenceType == "mileage" || s.RecurrenceType == "hybrid" {
        switch {
        case s.DueMileage > 0:
            nextMiles = s.DueMileage
        case !s.OneTime:
            nextMiles = s.LastCompletedMileage + s.IntervalMiles
        }
    }
    return
}
```

Three properties of this shape matter:

- **The override is per axis, not per schedule.** A hybrid row can legitimately
  hold a date anchor and no mileage anchor mid-migration; each axis resolves
  independently rather than the presence of one anchor suppressing the other's
  arithmetic.
- **The `!s.OneTime` guard is what makes a one-time axis terminal.** Without it,
  a completed one-time schedule whose anchor was cleared would silently fall back
  to `lastCompleted + 0`, i.e. "due again the instant it was completed".
- **A one-time axis with no anchor yields the zero value**, which `DueState`
  reads as `overdue` for the time axis. Validation (§3) makes that state
  unreachable on any row this task can create, and the backfill (§6) repairs the
  legacy rows that could otherwise reach it.

`DueState`, `DueBreaches`, `Severity`, `AxisBreach`, `Thresholds`, and
`DueCycleToken` are not modified.

---

## 3. Validation: one validator, two callers

Today the invariants live inside `Builder.Build` (`builder.go:35-49`) and
`Processor.Update` (`processor.go:68`) does not consult them — it applies a
mutation function, recomputes next-due, and writes. A PATCH can therefore
currently produce a `hybrid` schedule with a zero interval, which `NextDue` turns
into "due at the last completion point" and the queues turn into a permanently
overdue row. FR-UPD-2 exists because of this, and the one-time work makes it
worse: `oneTime`, `dueDate`, and `dueMileage` all become PATCH-settable and each
one has invariants of its own.

Extract a single package-level function:

```go
// validate enforces every maintenance-schedule invariant on a fully-formed
// model. Both construction (Builder.Build) and mutation (Processor.Update) run
// it, so an invariant cannot be satisfied at creation and violated by a PATCH.
func validate(m Model) error
```

The rules, in the order they read most clearly:

| # | Rule | Source |
| --- | --- | --- |
| 1 | `vehicleID` and `categoryID` non-empty | existing |
| 2 | `recurrenceType` ∈ {time, mileage, hybrid} | existing |
| 3 | `!oneTime` ⇒ `intervalMonths > 0` when the type covers time | existing |
| 4 | `!oneTime` ⇒ `intervalMiles > 0` when the type covers mileage | existing |
| 5 | `oneTime` ⇒ `intervalMonths == 0` and `intervalMiles == 0` | FR-OT-3 |
| 6 | type covers time ⇒ `!dueDate.IsZero()` | FR-OT-2, FR-ANCHOR-1 |
| 7 | type covers mileage ⇒ `dueMileage > 0` | FR-OT-2, FR-ANCHOR-1 |

Rules 6 and 7 are deliberately *not* conditioned on `oneTime`: the PRD requires a
due point for one-time schedules (FR-OT-2) and a first-due anchor for recurring
ones (FR-ANCHOR-1), and both requirements land on the same columns with the same
per-axis shape. Collapsing them into one pair of rules is the whole reason the
two features fit in one task.

`validate` returns `server.ErrValidation` throughout, matching the existing
builder. It does *not* distinguish which rule failed. That is consistent with the
rest of the service and with `server.WriteError`'s mapping, and the frontend Zod
schema is the layer that produces per-field messages.

### The `validate`-on-`Update` consequence

Rules 6 and 7 apply to the *resulting* model on every PATCH, including PATCHes
that predate this task and touch only `active`. A legacy row with no due point
would fail such a PATCH. The backfill (§6) closes that gap for uncompleted rows;
completed rows are exempt because rule 6/7 must not fire on them.

This is the one place where the "same rules for both kinds" symmetry breaks, and
it needs an explicit carve-out: **rules 6 and 7 apply only when the schedule has
never been completed** (`lastCompletedDate.IsZero() && lastCompletedMileage == 0`)
**or is one-time.** A recurring schedule that has been completed at least once
derives next-due from interval arithmetic and legitimately holds no anchor —
FR-ANCHOR-3 clears it on purpose. Without the carve-out, the completion flow
would write a row that its own validator rejects.

### `Builder` additions

`SetOneTime(bool)`, `SetDuePoint(time.Time, int)`, and `SetCurrentMileage(int)`.

The third is not in the PRD and is worth justifying. `Build` currently derives
the initial stored status with `DueState(..., b.m.lastCompletedMileage, ...)`
(`builder.go:47`) — it passes the *last completed* mileage as the *current*
mileage, which for a new schedule is `0`. A new mileage-based schedule due at
60,000 on a vehicle reading 59,900 is therefore stored as `ok` until the hourly
recompute corrects it. Read paths compute state live so the user rarely sees it,
but the stored value is what `RecomputeAll` compares against to detect the
transition edge into `overdue`, so a wrong initial value can suppress or spuriously
fire a `schedule.overdue` event on the first sweep. `resource.go` already holds
`v.CurrentMileage()` at the creation call site. Passing it in is a two-line fix to
a defect this task's acceptance criteria ("a newly created schedule reports `ok`,
not `overdue`") are otherwise only half-fixing.

---

## 4. Completion and deactivation

The three-step orchestration in `CompletionProcessor.Complete` (`completion.go`)
is unchanged: create record → append mileage → advance schedule. Deactivation
belongs inside step three, in `Administrator.AdvanceTx`, because that method
already loads the entity and issues the single `Updates` call that must be atomic
with the record and mileage writes.

`AdvanceTx` gains, after loading the entity:

```go
if !e.Active {
    return server.ErrValidation  // FR-COMPLETE-4
}
```

and, in the update map:

```go
updates["due_date"]    = nil   // FR-ANCHOR-3 / FR-COMPLETE-2
updates["due_mileage"] = 0
if m.OneTime() {
    updates["active"] = false
}
```

Clearing the anchor happens for both kinds. For a recurring schedule it hands
next-due permanently to interval arithmetic; for a one-time schedule the row is
simultaneously deactivated, so the cleared anchor is never read again. One code
path, no `if` on the clearing itself.

### Why the guard lives in `AdvanceTx` and not only the handler

The handler already loads the schedule (`resource.go`, complete route) and can
return a 400 before starting the transaction — that is the good error message and
it should exist. But check-then-act across two statements is a race: two
concurrent completes of the same one-time schedule both pass the handler check,
both open transactions, and both write a maintenance record. Only the check
*inside* the transaction, on the row the transaction has loaded, is authoritative;
its failure rolls back the record insert and the mileage append with it.

So: **handler pre-check for the error message, `AdvanceTx` check for
correctness.** The acceptance criterion "verified by an integration test that
fails the last step and asserts nothing was written" is exactly this guard's
test — `completion_db_test.go` already has the sqlite harness for it.

Note that the guard also rejects completing an inactive *recurring* schedule.
That is correct and is what FR-COMPLETE-4 says ("an already-inactive schedule"),
not a one-time-only rule.

### `Administrator.Update`

`dbAdministrator.Update` writes an explicit column map (`administrator.go:47`).
It must gain `one_time`, `due_date`, and `due_mileage`, with `due_date` mapped
through the same zero-time → `nil` treatment `AdvanceTx` uses. Omitting them is
the failure mode where the conversion PATCH appears to succeed and changes
nothing.

---

## 5. Transport

### Create — `POST /vehicles/{id}/maintenance-schedules`

Attributes gain `oneTime bool`, `dueDate string` (RFC3339), `dueMileage int`.
`dueDate` parses with `time.Parse(time.RFC3339, ...)` and a parse failure returns
`server.ErrValidation`, mirroring the complete route's existing date handling.
An empty `dueDate` is passed through as the zero time and left to `validate`.

### Update — `PATCH /maintenance-schedules/{id}` and the null problem

FR-UPD-3 requires `"dueDate": null` to clear the anchor. `encoding/json` cannot
distinguish an absent key from an explicit `null` on a `*string` field: both
leave the pointer nil. `server.RegisterInputHandler` decodes straight into the
attrs struct, so the distinction has to be carried by a field type that
implements `json.Unmarshaler` on a non-pointer receiver.

Add to `packages/shared-go/server`:

```go
// Nullable distinguishes the three states a JSON PATCH field can be in:
// absent (Present == false), explicit null (Present && !Valid), and a value
// (Present && Valid). A *T cannot express the middle state: encoding/json
// sets a settable pointer to nil for an explicit null, making it identical
// to an omitted key.
type Nullable[T any] struct {
    Present bool
    Valid   bool
    Value   T
}

func (n *Nullable[T]) UnmarshalJSON(b []byte) error {
    n.Present = true
    if string(b) == "null" {
        return nil
    }
    n.Valid = true
    return json.Unmarshal(b, &n.Value)
}
```

The PATCH attrs then read:

```go
RecurrenceType *string                  `json:"recurrenceType"`
OneTime        *bool                    `json:"oneTime"`
IntervalMonths *int                     `json:"intervalMonths"`
IntervalMiles  *int                     `json:"intervalMiles"`
DueDate        server.Nullable[string]  `json:"dueDate"`
DueMileage     *int                     `json:"dueMileage"`
Active         *bool                    `json:"active"`
```

`dueMileage` stays a `*int` because `0` is already its "unset" encoding at every
other layer — the column, the model, and `NextDue` all read `0` as absent, so
`{"dueMileage": 0}` is unambiguous without new machinery. Only `dueDate`, whose
absence is a `NULL` column, needs `Nullable`.

Two alternatives were weighed. An **empty-string sentinel** (`"dueDate": ""`
means clear) needs no shared-go change and would work, but it deviates from the
PRD's literal wording and bakes a magic value into the public API. **Deriving the
clear server-side** (clear whenever `oneTime` flips false) removes the client's
explicit control and couples two independent fields. `Nullable[T]` costs about
thirty lines plus a table test and is reusable by every future PATCH in the repo,
including the several that already carry the absent-vs-null ambiguity silently.

The mutation closure inside `proc.Update` applies each present field, then
`Processor.Update` runs `validate` on the result before recomputing and writing.
Validation precedes the recompute so an invalid intermediate never reaches
`NextDue`.

### Complete — `POST /maintenance-schedules/{id}/complete`

Request and response shapes unchanged. New `400` on an inactive target (§4).

### Resource representation

`Attributes` gains `OneTime bool \`json:"oneTime"\`` (always present — a
`omitempty` bool would make `false` indistinguishable from a server that predates
the field), `DueDate string \`json:"dueDate,omitempty"\``, and
`DueMileage int \`json:"dueMileage,omitempty"\``.

`nextDueDate` / `nextDueMileage` continue to carry the *effective* due point,
which for a one-time schedule is now the stored anchor. Existing consumers that
read only those two keys work unchanged — this is the property that lets the
frontend's `deriveNextService`, `rankSchedule`, and the vehicle status banner go
untouched.

### Internal due feed

`InternalDueSchedule` and `TransformInternalDue` are unchanged.

The PRD's open question about runaway reminders resolves against the source
rather than assumption. `reminder.DueCycleToken` builds
`"<next_due_date>|<next_due_mileage>"` from the feed
(`apps/notification-service/internal/reminder/job.go:107`), and
`consumer.OverdueDedupeKey` composes `"<user>:overdue:<schedule>:<cycle>"`
(`consume.go:164`), which `notification.Processor.Generate` dedupes on. For a
one-time schedule the anchor never moves, so the token is constant for the row's
whole life and every user receives **exactly one** overdue notification no matter
how long it stays overdue. No notification-service change is needed. This should
be locked down by a fleet-service test asserting the feed row for a one-time
schedule carries a stable `next_due_date`/`next_due_mileage` across repeated
recomputes, so a future change to `NextDue` cannot quietly break the dedupe.

---

## 6. Backfill

Uncompleted legacy schedules have a zero `last_completed_date` and therefore a
year-1 next-due; they read as permanently `overdue`. Once rules 6 and 7 apply to
`Update`, they would also reject any PATCH against those rows. A one-shot
idempotent backfill fixes both, on the pattern of `maintenancecategory.Seed`
(`entity.go:103`, called from `cmd/main.go:67`):

```go
// Backfill assigns a first-due anchor to schedules that have never been
// completed, repairing rows created before task-030 that would otherwise read
// as permanently overdue. Idempotent: it only touches rows where both new
// columns are still at their defaults.
func Backfill(db *gorm.DB) error
```

Selection: `one_time = false AND due_date IS NULL AND due_mileage = 0 AND
last_completed_date IS NULL AND last_completed_mileage = 0 AND deleted_at IS
NULL`, joined to `fleet.vehicles` for `current_mileage`. For each row, when the
type covers time set `due_date = created_at + interval_months months`; when it
covers mileage set `due_mileage = vehicle.current_mileage + interval_miles`.

**Implemented in Go, not as a single `UPDATE ... FROM`.** The date arithmetic
needs `created_at + N months` where `N` is a column, which in Postgres is
`created_at + (interval_months || ' months')::interval` — Postgres-specific
syntax that the package's sqlite-backed test harness
(`completion_db_test.go:7`) cannot execute. Doing the arithmetic in Go with
`AddDate(0, n, 0)` keeps the backfill testable in the same harness as everything
else it must not break, and reuses the exact function `NextDue` uses so an
anchored row and a completed row land on the same date math. The cost is N
updates instead of one; the table is small, this runs once at boot, and the
selection predicate makes every subsequent boot a no-op after a single indexed
count.

Wired in `cmd/main.go` immediately after `maintenancecategory.Seed`, fatal on
error, for the same reason `Seed` is: a service whose schedules are all falsely
overdue is worse than one that refuses to start.

---

## 7. Frontend

### Types and schema

`maintenanceSchedule.ts` gains `oneTime: boolean`, `dueDate?: string`,
`dueMileage?: number` on the read attributes, on
`CreateMaintenanceScheduleAttributes`, and — with `dueDate?: string | null` —
on `UpdateMaintenanceScheduleAttributes`, so the conversion PATCH can send an
explicit `null`.

The Zod schema stays **one object with an extended `superRefine`**, gaining a
`kind: 'recurring' | 'oneTime'` discriminant field, rather than becoming a
discriminated union. A union would type more precisely, but `zodResolver` over a
union fights react-hook-form's single `defaultValues` object and its shared
`categoryId` / `recurrenceType` fields, and the form would have to remount on
kind change to keep the resolver honest. The refinement matrix is four rules:

| kind | axis covers time | axis covers mileage |
| --- | --- | --- |
| recurring | `intervalMonths > 0` **and** `dueDate` required | `intervalMiles > 0` **and** `dueMileage > 0` required |
| oneTime | `dueDate` required, `intervalMonths` must be empty | `dueMileage > 0` required, `intervalMiles` must be empty |

A second small schema, `convertToRecurrenceSchema`, covers the conversion dialog:
`recurrenceType` plus the interval fields, with the same per-axis requirement and
no category or due-point fields.

### `MaintenanceScheduleForm`

A kind selector (repeating vs one-time) above the recurrence-type select. The
recurrence-type select stays visible for both kinds and keeps its current labels
— it is answering "judged by time, mileage, or both", which is the same question
either way. Below it, the field set swaps: intervals for recurring, due-date /
due-odometer for one-time, both keyed off the same `showMonths` / `showMiles`
axis booleans the component already derives.

For recurring, the first-due fields appear *alongside* the intervals, defaulted
per FR-ANCHOR-4 to `today + intervalMonths` and `currentMileage + intervalMiles`.
The defaulting is a `useEffect` on `[kind, recurrenceType, intervalMonths,
intervalMiles]` that writes through `form.setValue` **only while the user has not
touched the anchor field** — tracked with `form.formState.dirtyFields.dueDate`.
Without that condition, editing the interval would silently stomp a deliberately
chosen anchor. The form needs `currentMileage` passed in; `AddScheduleDialog`'s
caller (`VehicleDetailPage`) already holds the vehicle.

### Schedule list: extracting `ScheduleCard`

`UpcomingScheduleStrip` is, despite its name, the vehicle's whole schedule list —
`VehicleDetailPage:192` feeds it `schedulesQuery.data`, which is
`ListByVehicle` and includes inactive rows. FR-UI-2 and FR-UI-3 therefore both
land here.

Extract the per-schedule card into
`components/features/vehicles/maintenance/ScheduleCard.tsx`, owning the tone
class, category name, badges, and action buttons. `UpcomingScheduleStrip` keeps
sorting, layout, and the empty state. The card is currently a 30-line inline JSX
block that this task would grow by roughly as much again (a one-time badge, a
completed treatment, a completion date line, a second action button) — extracting
before growing is the difference between one component with two responsibilities
and two with one each, and it gives the completed-state rendering a test target
that does not require constructing a whole strip.

Card behavior:

- `oneTime && active` → a `One-time` badge; the recurrence line reads
  `due <date>` / `at <mileage> miles` instead of `every N months`.
- `!active` → muted foreground, no tone border, a `Completed <date>` line from
  `lastCompletedDate`, no Complete button.
- `!active && oneTime && canWrite` → a `Set up recurrence` button (see below).

Sorting in the strip becomes a two-tier comparator: active rows first, ranked by
the existing `rankSchedule`; inactive rows last, ordered by `lastCompletedDate`
descending. `rankSchedule` itself is unchanged — it reads `status`, which is
meaningless for a deactivated row, so the tier check must come first rather than
being folded into it.

### Conversion

`ConvertToRecurrenceDialog.tsx` takes the schedule, shows the category read-only
and the completion date/odometer as the stated anchor, and collects recurrence
type plus intervals. Submit issues one `PATCH` through the existing update
mutation:

```json
{"oneTime": false, "recurrenceType": "time", "intervalMonths": 12,
 "intervalMiles": 0, "active": true, "dueDate": null, "dueMileage": 0}
```

Next-due then derives from `lastCompletedDate` / `lastCompletedMileage`, which
the completion flow already wrote. No new endpoint, no new authorization surface.

On failure the dialog stays open and shows an error toast (FR-CONV-4). The
completion is never rolled back — a completed, deactivated one-time schedule is a
valid terminal state, and the conversion is strictly additive.

**Two entry points.** The post-completion toast action (FR-CONV-1) and the
`Set up recurrence` button on a completed one-time card. The PRD deferred the
second as open question 3; it is cheap once the dialog exists (one button, one
piece of state) and it removes the only way to permanently lose the affordance —
dismissing a toast. Both open the same dialog with the same props.

Toast wiring in `CompleteScheduleDialog`: the success branch checks
`schedule.attributes.oneTime` and, when true, passes
`{ label: 'Set up recurrence', onClick: … }` as sonner's `action`. Recurring
completions keep today's plain success toast (FR-CONV-5). The dialog closes
either way; the conversion dialog is opened by the page, not nested inside the
completion dialog, so dismissing one does not affect the other.

---

## 8. Error handling

| Condition | Where | Result |
| --- | --- | --- |
| Missing/invalid category, recurrence type, interval, or due point | `validate` via `Build` or `Update` | `400` `server.ErrValidation` |
| `oneTime` with a non-zero interval | `validate` rule 5 | `400` |
| PATCH result violates any invariant | `Processor.Update` → `validate` | `400`, nothing written |
| Unparseable `dueDate` | `resource.go` | `400` |
| Complete an inactive schedule | handler pre-check | `400`, no transaction opened |
| Complete an inactive schedule, concurrently | `AdvanceTx` | `400`, whole transaction rolled back |
| Not in the vehicle's fleet / no write scope | `authz` (unchanged) | `403` / `404` |
| Conversion PATCH fails | `ConvertToRecurrenceDialog` | error toast, dialog stays open, schedule unchanged |

---

## 9. Testing

**Go — `recurrence_test.go`:** a `NextDue` table over the cross product of
`{oneTime, recurring} × {time, mileage, hybrid} × {anchored, unanchored,
completed}`, asserting in particular that a completed-and-cleared one-time
schedule yields the zero due point rather than falling back to the completion
instant. Plus `DueState` cases pinning one-time `ok` / `upcoming` / `overdue`
against the 30-day and 500-mile thresholds, and a hybrid one-time row overdue on
one axis only.

**Go — `builder_test.go` / `processor_test.go`:** the `validate` matrix, kind ×
recurrence type × missing field, asserting `server.ErrValidation`; the
never-completed carve-out on rules 6/7; and a PATCH that would leave a hybrid
schedule with a zero interval, asserting rejection *and* that the row is
unchanged.

**Go — `completion_db_test.go`:** completing a one-time schedule writes the
record, the mileage row, and `active = false` in one transaction; the anchor is
cleared for both kinds; a recurring completion leaves `active = true`; completing
an inactive schedule returns a validation error with nothing written.

**Go — backfill:** anchors an uncompleted row, leaves a completed row untouched,
leaves an already-anchored row untouched, and is a no-op on a second run
(assert by comparing the full row set before and after).

**Go — `rest_test.go`:** `Transform` emits `oneTime` unconditionally and the due
fields only when set; `TransformInternalDue` produces a stable
`next_due_date`/`next_due_mileage` for a one-time schedule across repeated
recomputes (the `DueCycleToken` stability guarantee from §5).

**Go — `packages/shared-go/server`:** a `Nullable[T]` table covering absent,
`null`, and a value, plus a malformed value.

**Frontend:** Zod branch coverage for the four-rule matrix; the form's
conditional field set and the anchor-defaulting effect including the
don't-stomp-a-dirty-field case; `ScheduleCard` rendering for active one-time,
completed one-time, and active recurring; the strip's two-tier sort; and the
completion toast carrying the action only for one-time schedules.

Verification per `CLAUDE.md`: `make ci`. Both `kustomize build` overlays should
render byte-identical to `main` — no manifest changes are expected, and a diff
there means something leaked.

---

## 10. Scope boundaries

Explicitly not in this task, restating the PRD's non-goals where design work
might otherwise drift into them:

- No notification-service change. §5 establishes the contract is unchanged; the
  work is to *verify* that, not to touch it.
- No filtering or collapsing of completed schedules in the vehicle list. If a
  vehicle accumulates enough one-time items to make the list noisy, that is a
  follow-up task with its own PRD.
- No UI for converting a *recurring* schedule to one-time. The PATCH accepts it;
  nothing offers it.
- No richer recurrence rules. `intervalMonths` / `intervalMiles` remain the
  entire vocabulary.
