# One-Time Scheduled Maintenance — Product Requirements Document

Version: v1
Status: Draft
Created: 2026-08-25
---

## 1. Overview

Every maintenance schedule in MyFleet today is recurring. `recurrenceType` is one
of `time | mileage | hybrid`, and next-due is *derived* rather than stored:
`NextDue()` returns `lastCompletedDate + intervalMonths` and
`lastCompletedMileage + intervalMiles`
(`apps/fleet-service/internal/maintenanceschedule/recurrence.go:22`). There is no
way to express "state inspection due 2026-11-30" or "new tires at 60,000 miles" —
a one-off obligation with a fixed due point and no repeat.

This task adds **one-time schedules**: a schedule with an explicit due date
and/or due odometer and no interval. A one-time schedule flows through the exact
same status/severity, upcoming/overdue queue, vehicle status banner, dashboard
count, and reminder machinery as a recurring one — it differs only in how its
next-due point is computed and in what happens when it is completed. Completing a
one-time schedule creates the usual maintenance record and mileage entry, then
**deactivates** the schedule (`active = false`) so it stops appearing in queues,
banners, and the notification feed. The success toast offers a **"Set up
recurrence"** action that converts the just-completed item into a recurring
schedule anchored at the completion date and odometer — the "I didn't know this
would repeat until I did it" path.

The same absolute due-point columns fix an adjacent defect. A newly created
recurring schedule has a zero `lastCompletedDate`, so its computed next-due is
year 1 and it is born `overdue` (`builder.go:49`). Creation gains a required
**first due** date/odometer, which is what those columns hold until the first
completion.

## 2. Goals

Primary goals:

- Let a user create a maintenance schedule that is due once, at a fixed date, a
  fixed odometer reading, or both, and never repeats.
- Surface one-time schedules in every existing due-state surface (vehicle
  schedule list, upcoming/overdue queues, vehicle status banner, dashboard
  counts, reminder feed) with no consumer-side special-casing beyond a visual
  "One-time" badge.
- On completing a one-time schedule, offer a one-click path to convert it into a
  recurring schedule anchored at the completion point.
- Give recurring schedules an explicit first-due anchor at creation, so a new
  schedule is no longer born overdue.

Non-goals:

- Changing notification-service templates, copy, or delivery rules. The internal
  due-feed shape is unchanged and one-time schedules ride it as-is.
- Multi-step maintenance, checklists, or grouping several schedules into a
  service visit.
- Editing or reopening an already-completed one-time schedule (beyond the
  recurrence conversion described here).
- Retroactively converting a recurring schedule into a one-time schedule via the
  UI. The API permits it; no UI affordance is required.
- Recurrence rules richer than the existing month/mile intervals (e.g. "every
  April", cron-style rules).

## 3. User Stories

- As a vehicle owner, I want to schedule my state inspection for a specific date
  so that it shows up in my overdue queue if I miss it.
- As a vehicle owner, I want to schedule "replace tires at 60,000 miles" so that
  it warns me as the odometer approaches, without committing to a repeat every
  60,000 miles.
- As a vehicle owner, I want a one-time item to disappear from my upcoming list
  once I have done it, without having to remember to delete it.
- As a vehicle owner, having just completed a one-time item, I want to say "this
  should actually repeat annually from today" without re-entering the category
  and re-deriving a start date.
- As a vehicle owner creating a recurring schedule, I want to say when it is
  first due so that a brand-new schedule does not immediately show as overdue.
- As a household member scanning the vehicle detail page, I want to tell at a
  glance which items repeat and which are one-offs.

## 4. Functional Requirements

### 4.1 One-time schedule definition

- **FR-OT-1** — A maintenance schedule has a boolean `oneTime` attribute,
  defaulting to `false`. `recurrenceType` (`time | mileage | hybrid`) is retained
  on one-time schedules and continues to mean *which axes the schedule is judged
  on*, not *how often it repeats*.
- **FR-OT-2** — A one-time schedule carries an explicit due point: `dueDate` (a
  timestamp) and/or `dueMileage` (an integer). `dueDate` is required when
  `recurrenceType` is `time` or `hybrid`; `dueMileage` must be greater than zero
  when `recurrenceType` is `mileage` or `hybrid`.
- **FR-OT-3** — A one-time schedule must not carry intervals. A create or update
  request that sets `oneTime = true` together with a non-zero `intervalMonths` or
  `intervalMiles` is rejected with a validation error.
- **FR-OT-4** — For a one-time schedule, `NextDue()` returns the stored
  `dueDate` / `dueMileage` verbatim. No interval arithmetic is performed.

### 4.2 First-due anchor for recurring schedules

- **FR-ANCHOR-1** — Creating a recurring schedule requires a first-due point on
  every axis its `recurrenceType` covers: `dueDate` for `time` and `hybrid`,
  `dueMileage > 0` for `mileage` and `hybrid`. This is in addition to the
  existing interval requirements, not a replacement for them.
- **FR-ANCHOR-2** — `NextDue()` returns the stored `dueDate` / `dueMileage`
  whenever they are set, on a per-axis basis, falling back to
  `lastCompleted* + interval*` when they are not. The per-axis fallback exists
  for rows that predate this task; FR-ANCHOR-1 makes a mixed state
  unreachable for newly created schedules.
- **FR-ANCHOR-3** — Completing a recurring schedule clears `dueDate` and
  `dueMileage`, so interval arithmetic from the new completion point takes over
  permanently. The first-due anchor governs exactly one cycle: the first.
- **FR-ANCHOR-4** — The create form defaults first-due to `today +
  intervalMonths` and `currentMileage + intervalMiles`, recomputed as the user
  edits the recurrence type or intervals, and overridable. A user who does not
  care about the anchor gets the intuitive "starts now" behavior for free.

### 4.3 Due-state evaluation

- **FR-STATE-1** — `DueState`, `DueBreaches`, `Severity`, and the due-soon
  thresholds (30 days / 500 miles) apply to one-time schedules unchanged. A
  one-time schedule is `ok`, `upcoming`, or `overdue` by the same rules.
- **FR-STATE-2** — A `hybrid` one-time schedule is `overdue` when *either* axis
  breaches, matching existing hybrid semantics
  (`recurrence.go:36`).
- **FR-STATE-3** — One-time schedules appear in
  `GET /fleets/{id}/maintenance/upcoming` and `/overdue`, in the vehicle status
  banner and its breach reasons, in dashboard status counts, and in the internal
  reminder feed, on the same terms as recurring schedules, for as long as they
  are `active`.
- **FR-STATE-4** — The hourly recompute job treats one-time schedules like any
  other active schedule, including emitting a single `schedule.overdue` activity
  event and outbox event on the transition edge into `overdue`.

### 4.4 Completion

- **FR-COMPLETE-1** — Completing a one-time schedule performs the existing
  three-step completion flow unchanged: create a pre-filled maintenance record,
  append a mileage record (`source=maintenance`, ref = record id), and advance
  the schedule's completion point (`completion.go:60`).
- **FR-COMPLETE-2** — After advancing, a one-time schedule is set to
  `active = false` in the same transaction. It therefore drops out of every
  active-only read path (queues, banner, dashboard, reminder feed) with no change
  to those consumers.
- **FR-COMPLETE-3** — A deactivated completed one-time schedule remains visible
  in the vehicle's schedule list, labelled as completed, showing its completion
  date and odometer. It is not deleted.
- **FR-COMPLETE-4** — Completing an already-inactive schedule is rejected with a
  validation error. This closes the double-completion path the deactivation
  opens.
- **FR-COMPLETE-5** — Completing a recurring schedule behaves exactly as it does
  today, except that `dueDate` / `dueMileage` are cleared per FR-ANCHOR-3. The
  schedule stays `active`.

### 4.5 Recurrence conversion

- **FR-CONV-1** — On successfully completing a one-time schedule, the web app
  shows a success toast carrying a **"Set up recurrence"** action.
- **FR-CONV-2** — Activating the toast action opens a dialog prefilled with the
  schedule's category (read-only) and the completion date and odometer, shown as
  the anchor the recurrence will run from. The user picks a recurrence type and
  the intervals it requires.
- **FR-CONV-3** — Submitting the dialog issues a single
  `PATCH /maintenance-schedules/{id}` that sets `oneTime = false`, the chosen
  `recurrenceType` and intervals, `active = true`, and clears `dueDate` /
  `dueMileage`. Next-due is then derived from the completion point that the
  completion flow already recorded in `lastCompletedDate` /
  `lastCompletedMileage`.
- **FR-CONV-4** — The conversion is a distinct, dismissible follow-up action. If
  the user dismisses the toast, ignores it, or the PATCH fails, the schedule
  remains a completed, deactivated one-time schedule — a valid terminal state.
  The completion is never rolled back on conversion failure, and a failed
  conversion surfaces an error toast without closing the dialog.
- **FR-CONV-5** — The toast action is offered only for one-time schedules.
  Completing a recurring schedule shows the existing plain success toast.

### 4.6 Update / validation

- **FR-UPD-1** — `PATCH /maintenance-schedules/{id}` accepts `oneTime`,
  `dueDate`, and `dueMileage` in addition to its existing fields. All fields
  remain optional; omitted fields are unchanged.
- **FR-UPD-2** — The update path validates the *resulting* model against the same
  invariants the builder enforces, and rejects an invalid result with a
  validation error. Today `Processor.Update` applies a mutation function and
  recomputes without re-validating (`processor.go:68`), so a PATCH can currently
  produce a `hybrid` schedule with a zero interval. The invariant checks must be
  extracted from `Builder.Build` into a shared validator used by both paths.
- **FR-UPD-3** — `dueDate` is clearable by sending explicit JSON `null`, and
  `dueMileage` by sending `0`. Conversion (FR-CONV-3) depends on this.

### 4.7 Presentation

- **FR-UI-1** — The add-schedule form offers a choice between a repeating
  schedule and a one-time schedule. Choosing one-time replaces the interval
  fields with due-date / due-odometer fields, keyed off the same
  `recurrenceType` axis selection.
- **FR-UI-2** — One-time schedules carry a distinguishing "One-time" badge in the
  vehicle schedule list and in the upcoming-schedule strip
  (`UpcomingScheduleStrip.tsx`).
- **FR-UI-3** — Completed (inactive) one-time schedules render visually distinct
  from active ones — de-emphasized, with the completion date shown — and sort
  below active schedules in the vehicle schedule list.
- **FR-UI-4** — The complete-schedule dialog is unchanged for one-time schedules;
  the conversion offer lives entirely in the post-success toast.

## 5. API Surface

All routes are as registered in
`apps/fleet-service/internal/maintenanceschedule/resource.go` and reached via the
`/api/fleet` prefix from the web app. Authorization is unchanged: same-fleet plus
write scope for mutations.

### 5.1 `POST /vehicles/{id}/maintenance-schedules`

Request attributes (additions marked):

```jsonc
{
  "categoryId": "uuid",
  "recurrenceType": "time | mileage | hybrid",
  "oneTime": false,              // NEW, optional, default false
  "intervalMonths": 6,           // required (>0) when !oneTime && type in {time, hybrid}
  "intervalMiles": 5000,         // required (>0) when !oneTime && type in {mileage, hybrid}
  "dueDate": "2026-11-30T00:00:00Z", // NEW, RFC3339; required when type in {time, hybrid}
  "dueMileage": 60000            // NEW, required (>0) when type in {mileage, hybrid}
}
```

Responses:

- `201 Created` with a `maintenanceSchedules` resource.
- `400` validation error when: category or recurrence type missing/invalid; a
  required interval is missing or non-positive for a recurring schedule; a
  required due-point axis is missing for either kind; or `oneTime` is true and a
  non-zero interval is supplied (FR-OT-3).
- `403` when the caller is not in the vehicle's fleet or lacks write scope.
- `404` when the vehicle does not exist.

### 5.2 `PATCH /maintenance-schedules/{id}`

Request attributes (all optional; additions marked):

```jsonc
{
  "recurrenceType": "time",
  "oneTime": false,      // NEW
  "intervalMonths": 12,
  "intervalMiles": 0,
  "dueDate": null,       // NEW; explicit null clears
  "dueMileage": 0,       // NEW; 0 clears
  "active": true
}
```

Responses: `200 OK` with the updated resource; `400` when the resulting model
violates any invariant (FR-UPD-2); `403`; `404`.

The recurrence conversion (FR-CONV-3) is exactly one such PATCH:
`{"oneTime": false, "recurrenceType": "time", "intervalMonths": 12,
"active": true, "dueDate": null, "dueMileage": 0}`.

### 5.3 `POST /maintenance-schedules/{id}/complete`

Request attributes are unchanged (`date`, `latestMileage`). Response is unchanged
— a `maintenanceCompletions` resource carrying `maintenanceRecordId`.

New behavior: when the target schedule has `oneTime = true`, the schedule is
deactivated inside the completion transaction (FR-COMPLETE-2).

New error: `400` validation error when the target schedule is already inactive
(FR-COMPLETE-4).

### 5.4 Resource representation

`maintenanceSchedules` attributes gain (`rest.go` `Attributes`):

```jsonc
{
  "oneTime": true,                        // always present
  "dueDate": "2026-11-30T00:00:00Z",      // omitempty
  "dueMileage": 60000                     // omitempty
}
```

`nextDueDate` / `nextDueMileage` continue to carry the effective due point, so
existing consumers that read only those keep working for one-time schedules
without change.

### 5.5 `GET /internal/maintenance/due`

Shape is unchanged (`InternalDueSchedule`). One-time schedules appear while
active and non-`ok`, and disappear once completed. `DueCycleToken` continues to
be built from `nextDueDate | nextDueMileage`, so the notification-service dedupe
key derivation stays byte-identical.

## 6. Data Model

Table `fleet.maintenance_schedules` (`entity.go`) gains three columns:

| Column | Type | Null | Default | Notes |
| --- | --- | --- | --- | --- |
| `one_time` | boolean | not null | `false` | Marks a non-repeating schedule. |
| `due_date` | timestamp | null | `NULL` | Explicit due instant. For a recurring schedule, the first-due anchor; cleared on first completion. |
| `due_mileage` | integer | not null | `0` | Explicit due odometer. Same lifecycle as `due_date`. `0` means unset. |

Domain model (`model.go`) gains `oneTime bool`, `dueDate time.Time`,
`dueMileage int` with accessors, a `WithDuePoint(date, miles)` copy-with, and a
`WithOneTime(bool)` copy-with, following the existing immutable-model style.
`Model.AsSchedule()` carries the three new fields into the pure `Schedule` input.

`Schedule` (`recurrence.go`) gains `OneTime bool`, `DueDate time.Time`,
`DueMileage int`.

`NextDue(s)` becomes, per axis:

- time axis (`type in {time, hybrid}`): `s.DueDate` when non-zero; otherwise, and
  only when `!s.OneTime`, `s.LastCompletedDate.AddDate(0, s.IntervalMonths, 0)`.
- mileage axis (`type in {mileage, hybrid}`): `s.DueMileage` when greater than
  zero; otherwise, and only when `!s.OneTime`,
  `s.LastCompletedMileage + s.IntervalMiles`.

`DueState`, `DueBreaches`, `Severity`, `AxisBreach`, and `Thresholds` are
untouched — they consume `NextDue` and the `recurrenceType` axis flags, both of
which retain their meaning.

Migration notes:

- Schema change is additive and lands via the existing `AutoMigrate` path
  (`entity.go:32`, wired at `cmd/main.go:55`). No column is dropped or retyped,
  so the deploy is backward-compatible with the running previous version.
- A one-shot idempotent backfill runs after migration, on the pattern of
  `maintenancecategory.Seed` (`cmd/main.go:67`): for every schedule with a zero
  `last_completed_date` and no `due_date` / `due_mileage`, set
  `due_date = created_at + interval_months` (when the type covers time) and
  `due_mileage = vehicle.current_mileage + interval_miles` (when it covers
  mileage). This repairs schedules currently born overdue against a year-1
  next-due. Rows that have been completed at least once are left alone and
  continue on pure interval arithmetic.
- The backfill must be safe to run repeatedly: it only touches rows where both
  new columns are still at their defaults.

## 7. Service Impact

**`apps/fleet-service`** — all backend work lands here, in
`internal/maintenanceschedule`:

- `entity.go` — three columns, `Make`/`ToEntity` mapping, backfill helper.
- `model.go` — three fields, accessors, `WithDuePoint`, `WithOneTime`, extended
  `AsSchedule`.
- `recurrence.go` — `Schedule` fields and the revised `NextDue`.
- `builder.go` — `SetOneTime`, `SetDuePoint`, and the extended validation from
  FR-OT-2, FR-OT-3, FR-ANCHOR-1, extracted into a shared `validate(Model) error`.
- `processor.go` — `Update` calls the shared validator (FR-UPD-2).
- `administrator.go` — `AdvanceTx` clears `due_date` / `due_mileage` and, for a
  one-time schedule, sets `active = false`; `RecomputeTx` is unchanged in
  structure and picks up the new `NextDue` behavior for free.
- `resource.go` — new create/update attributes, RFC3339 `dueDate` parsing, the
  already-inactive completion guard.
- `rest.go` — `oneTime`, `dueDate`, `dueMileage` in `Attributes` and `Transform`.

**`apps/web`**:

- `types/models/maintenanceSchedule.ts` — new attributes on the read model and on
  the create/update request types.
- `lib/schemas/maintenanceSchedule.ts` — the Zod schema gains the one-time branch
  and the first-due anchor, with `superRefine` conditional requirements per
  recurrence type and kind; plus a new schema for the conversion dialog.
- `components/features/vehicles/maintenance/MaintenanceScheduleForm.tsx` —
  repeating vs one-time selection and the conditional field set.
- `components/features/vehicles/dialogs/CompleteScheduleDialog.tsx` — success
  toast carries the conversion action for one-time schedules.
- New `components/features/vehicles/dialogs/ConvertToRecurrenceDialog.tsx`.
- `components/features/vehicles/detail/UpcomingScheduleStrip.tsx` and the vehicle
  maintenance section — "One-time" badge, completed-state rendering and ordering.
- `lib/hooks/api/maintenance.ts` / `services/api/MaintenanceScheduleService.ts` —
  carry the new attributes; the conversion reuses the existing update mutation.

**`apps/notification-service`** — no code change. The internal due-feed contract
and `DueCycleToken` derivation are unchanged; this must be verified, not assumed.

**Deployment manifests** — no change.

## 8. Non-Functional Requirements

- **Performance** — No new per-request queries. `NextDue` stays pure arithmetic
  over in-memory fields, so queue, banner, dashboard, and reminder-feed read
  paths keep their current cost. The backfill is a single bounded pass at
  startup, guarded to be a no-op on subsequent boots.
- **Security** — Authorization is unchanged and must remain enforced on every
  touched route: same-fleet check plus write scope for create, update, delete,
  and complete. The conversion PATCH inherits the existing update authorization;
  no new endpoint and no new authorization surface is introduced.
- **Data integrity** — Completion and deactivation occur in a single
  transaction, so a one-time schedule can never be recorded as completed while
  remaining active, nor deactivated without its maintenance record.
- **Backward compatibility** — Existing recurring schedules must behave
  identically after this change. `oneTime` defaults to false, and an uncompleted
  legacy row without a backfilled due point falls back to today's interval
  arithmetic.
- **Observability** — Completion, deactivation, and conversion reuse the existing
  activity-event and structured-logging paths. No new event type is introduced.
- **Testing** — Unit coverage for the revised `NextDue` across the one-time,
  first-due-anchor, and post-completion cases; for the builder/validator matrix
  (kind × recurrence type × missing field); for the completion deactivation and
  the already-inactive rejection; and for the backfill's idempotence. Frontend
  coverage for the Zod schema branches, the form's conditional fields, and the
  conversion toast action.

## 9. Open Questions

- Should a one-time schedule that is `overdue` for a very long time eventually
  stop generating reminders? Today the reminder safety-net dedupes per due-cycle
  token, and a one-time schedule's token never changes, so it should already
  settle to a single reminder — this needs verification against
  notification-service rather than assumption.
- Should completed one-time schedules be filterable or collapsible in the vehicle
  schedule list once a vehicle accumulates many of them? Deferred; revisit if the
  list becomes noisy.
- Should converting to a recurrence also be reachable from the schedule list for
  a completed one-time schedule the user dismissed the toast on? Not required for
  v1; the user can create a new recurring schedule. Worth considering during
  design if it is cheap.

## 10. Acceptance Criteria

- [ ] A schedule can be created with `oneTime: true` and a due date, a due
      odometer, or both, and is rejected when the axis its `recurrenceType`
      covers has no due point.
- [ ] Creating a one-time schedule with a non-zero interval is rejected with a
      validation error.
- [ ] Creating a recurring schedule without a first-due point for a covered axis
      is rejected with a validation error.
- [ ] A newly created recurring schedule with a first-due point in the future
      reports status `ok`, not `overdue`.
- [ ] A one-time schedule with a due date inside the 30-day window reports
      `upcoming` / `recommended`; past its due date it reports `overdue` /
      `urgent`; a hybrid one-time schedule reports `overdue` when either axis
      breaches.
- [ ] An active one-time schedule appears in the fleet upcoming/overdue queues,
      the vehicle status banner with correct breach detail, dashboard counts, and
      `GET /internal/maintenance/due`.
- [ ] Completing a one-time schedule creates a maintenance record, appends a
      mileage record, and leaves the schedule with `active: false` — all three
      in one transaction, verified by an integration test that fails the last
      step and asserts nothing was written.
- [ ] After completion, the one-time schedule no longer appears in the queues,
      banner, dashboard counts, or internal due feed.
- [ ] Completing an already-inactive schedule returns a validation error.
- [ ] Completing a recurring schedule leaves it active and clears its first-due
      anchor, with next-due derived from the new completion point.
- [ ] The success toast after completing a one-time schedule offers "Set up
      recurrence"; the toast after completing a recurring schedule does not.
- [ ] The conversion dialog submits one PATCH; afterwards the schedule is
      recurring, active, has no due point, and its next-due equals the completion
      point plus the chosen interval.
- [ ] A failed conversion PATCH leaves the schedule completed and deactivated,
      shows an error toast, and keeps the dialog open.
- [ ] A PATCH that would leave a `hybrid` schedule with a zero interval is
      rejected.
- [ ] The vehicle schedule list badges one-time schedules, renders completed ones
      de-emphasized with their completion date, and sorts them below active ones.
- [ ] The backfill sets a due point for uncompleted legacy schedules, leaves
      completed ones untouched, and is a no-op on a second run.
- [ ] `make ci` passes: `lint-check`, `vet`, `test`, `build`, `fe-test`,
      `fe-build`.
- [ ] Both `kustomize build` overlays render unchanged (no manifest changes
      expected in this task).
