# One-Time Scheduled Maintenance — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Give `fleet.maintenance_schedules` an absolute due point (`one_time`, `due_date`, `due_mileage`) so a schedule can be due exactly once at a fixed date/odometer, and so a newly created recurring schedule carries a first-due anchor instead of being born overdue.

**Architecture:** One primitive serves both features — an absolute due point stored on the row that `NextDue` prefers, per axis, over interval arithmetic. One-time schedules keep that point forever and are deactivated on completion; recurring schedules keep it for exactly one cycle and drop it at the first completion. `DueState`, `DueBreaches`, `Severity`, `AxisBreach`, `Thresholds`, `DueCycleToken`, the queue handlers, the internal due feed, the vehicle status banner and the dashboard counts all consume `NextDue` and the `recurrenceType` axis flags — neither changes meaning, so every one of those consumers inherits one-time support untouched.

**Tech Stack:** Go 1.x + chi + GORM (fleet-service), hand-rolled JSON:API in `packages/shared-go/server`, sqlite-in-memory test harness; React 18 + TypeScript + Vite, TanStack React Query, react-hook-form + Zod, shadcn/ui + Tailwind, sonner, Vitest + @testing-library/react (apps/web).

**Spec:** `docs/tasks/task-030-one-time-maintenance-schedules/design.md` (PRD alongside at `prd.md`). Executors read both.

## Global Constraints

- All work happens in the worktree `/home/tumidanski/source/MyFleet/.worktrees/task-030-one-time-maintenance-schedules` on branch `task-030-one-time-maintenance-schedules`. Every path below is relative to that root.
- **`recurrenceType` keeps meaning "which axes the schedule is judged on", not "how often it repeats."** `oneTime` is the orthogonal flag. Do **not** add a fourth `recurrenceType` value. (design §1.)
- **Do not modify** `DueState`, `DueBreaches`, `Severity`, `AxisBreach`, `Thresholds`, `wholeDays`, `overdueUrgency`, `upcomingUrgency`, `DueCycleToken`, `InternalDueSchedule`, `TransformInternalDue`, `queueHandler`, `Processor.Queue`, `Processor.DueAcrossAllFleets`, `Processor.RecomputeAll`, `Processor.ScheduleDueByVehicle`, or anything under `apps/fleet-service/internal/dashboard`, `apps/fleet-service/internal/vehicle`, or `apps/notification-service`. (design §1, §5, §10.)
- **No change to `deploy/k8s`.** Both `kustomize build` overlays must render byte-identical to `main`.
- The three new columns are **additive with safe defaults** (`one_time` not-null default `false`, `due_date` nullable, `due_mileage` not-null default `0`) and land via the existing `AutoMigrate` path. No column is dropped or retyped. (design §2.)
- `oneTime` is serialized **without** `omitempty` (a `false` must be distinguishable from a server predating the field); `dueDate` and `dueMileage` use `omitempty`. (design §5.)
- Validation errors are always `server.ErrValidation`, never a bespoke error type or per-field message. Per-field messages are the frontend Zod schema's job. (design §3.)
- Authorization is unchanged on every touched route: same-fleet for reads, same-fleet + `authz.RequireWrite` for create/update/delete/complete. No new endpoint, no new authorization surface. (PRD §8.)
- Go: run `make vet` and `make test` from the repo root. Package-scoped: `go test ./apps/fleet-service/internal/maintenanceschedule/... -run TestName -v`.
- Node is not always on `PATH`. Before any `npm`/`npx`/`make fe-*` command, run:
  `export NVM_DIR="$HOME/.nvm" && . "$NVM_DIR/nvm.sh" && nvm use 22`
- Frontend: **no hardcoded palette classes** (`bg-gray-*`, `text-white`, `text-red-*`, …). `apps/web/src/test/conventions.test.ts` fails the build on these repo-wide. Use semantic tokens (`text-muted-foreground`, `bg-danger-subtle`, …).
- jsdom cannot see CSS (`vite.config.ts` sets `css: false`). Every frontend assertion is on role, accessible name, rendered text, or request payload — **never** on visual styling.

---

## File Structure

### `packages/shared-go/server`

| File | Status | Responsibility |
|---|---|---|
| `nullable.go` | create | `Nullable[T]` — absent / explicit-null / value tri-state for PATCH fields |
| `nullable_test.go` | create | Table test over the three states plus a malformed value |

### `apps/fleet-service/internal/maintenanceschedule`

| File | Status | Responsibility |
|---|---|---|
| `entity.go` | modify | Three columns; `Make`/`ToEntity` mapping |
| `entity_test.go` | create | `Make`/`ToEntity` round-trip for the new fields |
| `model.go` | modify | Three fields, accessors, `WithOneTime`, `WithDuePoint`, extended `AsSchedule` |
| `recurrence.go` | modify | `Schedule` fields + the revised per-axis `NextDue` |
| `recurrence_test.go` | modify | `NextDue` matrix over kind × type × anchor state |
| `builder.go` | modify | `validate(Model) error`; `SetOneTime`, `SetDuePoint`, `SetCurrentMileage` |
| `builder_test.go` | create | The `validate` matrix |
| `processor.go` | modify | `Update` runs `validate` before recompute |
| `processor_update_test.go` | create | PATCH rejection + row-unchanged assertions |
| `administrator.go` | modify | `Update` column map; `AdvanceTx` anchor clear / deactivate / inactive guard |
| `completion_db_test.go` | modify | New sqlite columns; one-time + recurring + inactive completion cases |
| `backfill.go` | create | `Backfill(db)` — first-due anchor for uncompleted legacy rows |
| `backfill_test.go` | create | Anchors, exempts, idempotence |
| `resource.go` | modify | Create/PATCH attributes, RFC3339 parsing, inactive-complete pre-check |
| `rest.go` | modify | `oneTime` / `dueDate` / `dueMileage` in `Attributes` + `Transform` |
| `rest_test.go` | create | `Transform` emission rules; feed-token stability |

### `apps/fleet-service`

| File | Status | Responsibility |
|---|---|---|
| `cmd/main.go` | modify | Wire `maintenanceschedule.Backfill` after `maintenancecategory.Seed` |
| `internal/admin/admintest/db.go` | modify | Add the three columns to the sqlite fixture DDL |

### `apps/web/src`

| File | Status | Responsibility |
|---|---|---|
| `types/models/maintenanceSchedule.ts` | modify | New read/create/update attributes |
| `lib/schemas/maintenanceSchedule.ts` | modify | `kind` discriminant + extended `superRefine`; `convertToRecurrenceSchema` |
| `lib/schemas/maintenanceSchedule.test.ts` | modify | Rewritten for the four-rule matrix + the conversion schema |
| `components/features/vehicles/maintenance/MaintenanceScheduleForm.tsx` | modify | Kind selector, conditional field set, anchor defaulting |
| `components/features/vehicles/maintenance/MaintenanceScheduleForm.test.tsx` | create | Conditional fields + anchor-defaulting behaviour |
| `components/features/vehicles/dialogs/AddScheduleDialog.tsx` | modify | Map form values onto the new create attributes; take `currentMileage` |
| `components/features/vehicles/maintenance/ScheduleCard.tsx` | create | Per-schedule card: tone, badges, completed treatment, actions |
| `components/features/vehicles/maintenance/ScheduleCard.test.tsx` | create | Active one-time, completed one-time, active recurring |
| `components/features/vehicles/detail/UpcomingScheduleStrip.tsx` | modify | Two-tier sort, delegate the card, pass `onConvert` |
| `components/features/vehicles/detail/UpcomingScheduleStrip.test.tsx` | create | Two-tier ordering |
| `components/features/vehicles/dialogs/ConvertToRecurrenceDialog.tsx` | create | One PATCH that converts a completed one-time into a recurring schedule |
| `components/features/vehicles/dialogs/ConvertToRecurrenceDialog.test.tsx` | create | Payload shape; failure keeps the dialog open |
| `components/features/vehicles/dialogs/CompleteScheduleDialog.tsx` | modify | Success toast carries the conversion action for one-time schedules |
| `components/features/vehicles/dialogs/CompleteScheduleDialog.test.tsx` | create | Toast action present only for one-time |
| `pages/VehicleDetailPage.tsx` | modify | Own the conversion dialog + `convertingSchedule` state; pass `currentMileage` |

---

**Task order is dependency order.** Tasks 1–9 are backend and must land before Task 10; Tasks 10–15 are frontend; Task 16 is whole-repo verification.

---

## Task 1: `server.Nullable[T]` — the absent-vs-null tri-state

FR-UPD-3 requires `"dueDate": null` to clear the stored anchor. `encoding/json` cannot distinguish an absent key from an explicit `null` on a `*string` field: both leave the pointer nil. The distinction has to be carried by a field type implementing `json.Unmarshaler`.

**Files:**
- Create: `packages/shared-go/server/nullable.go`
- Test: `packages/shared-go/server/nullable_test.go`

**Interfaces:**
- Consumes: nothing.
- Produces: `type Nullable[T any] struct { Present bool; Valid bool; Value T }` with `func (n *Nullable[T]) UnmarshalJSON([]byte) error`. Task 8 uses it as `server.Nullable[string]` for the PATCH `dueDate` field.

- [x] **Step 1: Write the failing test**

Create `packages/shared-go/server/nullable_test.go`:

```go
package server

import (
	"encoding/json"
	"testing"
)

func TestNullableUnmarshal(t *testing.T) {
	type payload struct {
		Due Nullable[string] `json:"due"`
	}

	cases := []struct {
		name        string
		body        string
		wantPresent bool
		wantValid   bool
		wantValue   string
	}{
		{"absent", `{}`, false, false, ""},
		{"explicit null", `{"due":null}`, true, false, ""},
		{"value", `{"due":"2026-11-30T00:00:00Z"}`, true, true, "2026-11-30T00:00:00Z"},
		{"empty string is a value", `{"due":""}`, true, true, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var p payload
			if err := json.Unmarshal([]byte(c.body), &p); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if p.Due.Present != c.wantPresent {
				t.Errorf("Present = %v want %v", p.Due.Present, c.wantPresent)
			}
			if p.Due.Valid != c.wantValid {
				t.Errorf("Valid = %v want %v", p.Due.Valid, c.wantValid)
			}
			if p.Due.Value != c.wantValue {
				t.Errorf("Value = %q want %q", p.Due.Value, c.wantValue)
			}
		})
	}
}

// A malformed value must surface as an error rather than silently decoding to
// the zero value: RegisterInputHandler turns a decode error into a 400.
func TestNullableUnmarshal_malformed(t *testing.T) {
	type payload struct {
		Due Nullable[string] `json:"due"`
	}
	var p payload
	if err := json.Unmarshal([]byte(`{"due":42}`), &p); err == nil {
		t.Fatal("want an error decoding a number into Nullable[string]")
	}
}

// Nullable[int] proves the type parameter is not string-specific.
func TestNullableUnmarshal_int(t *testing.T) {
	type payload struct {
		N Nullable[int] `json:"n"`
	}
	var p payload
	if err := json.Unmarshal([]byte(`{"n":7}`), &p); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !p.N.Present || !p.N.Valid || p.N.Value != 7 {
		t.Fatalf("got %+v, want present/valid/7", p.N)
	}
}
```

- [x] **Step 2: Run the test to verify it fails**

Run: `go test ./packages/shared-go/server/... -run TestNullable -v`
Expected: FAIL — `undefined: Nullable`.

- [x] **Step 3: Write the implementation**

Create `packages/shared-go/server/nullable.go`:

```go
package server

import (
	"bytes"
	"encoding/json"
)

// Nullable distinguishes the three states a JSON PATCH field can be in:
// absent (Present == false), explicit null (Present && !Valid), and a value
// (Present && Valid).
//
// A *T cannot express the middle state: encoding/json sets a settable pointer
// to nil for an explicit null, making it byte-identical to an omitted key. Any
// PATCH field whose "cleared" state is a NULL column — as opposed to a zero
// value that already means "unset" — needs this instead of a pointer.
type Nullable[T any] struct {
	// Present reports whether the key appeared in the request body at all.
	Present bool
	// Valid reports whether the key carried a value rather than null. It is
	// meaningless unless Present.
	Valid bool
	// Value is the decoded value. It is the zero value unless Present && Valid.
	Value T
}

// UnmarshalJSON records that the key was present and decodes the value unless
// it was null. It is only ever called when the key IS present, which is what
// makes Present reliable.
func (n *Nullable[T]) UnmarshalJSON(b []byte) error {
	n.Present = true
	if bytes.Equal(b, []byte("null")) {
		return nil
	}
	if err := json.Unmarshal(b, &n.Value); err != nil {
		return err
	}
	n.Valid = true
	return nil
}
```

- [x] **Step 4: Run the test to verify it passes**

Run: `go test ./packages/shared-go/server/... -run TestNullable -v`
Expected: PASS (four subtests plus the two standalone tests).

- [x] **Step 5: Commit**

```bash
git add packages/shared-go/server/nullable.go packages/shared-go/server/nullable_test.go
git commit -m "feat(shared-go): add Nullable[T] for absent-vs-null PATCH fields"
```

---

## Task 2: Data model — three columns, three model fields

Purely structural: the columns, the model fields, and the `Schedule` fields exist and round-trip. `NextDue` still ignores them (Task 3 changes that), so no behaviour moves in this task.

**Files:**
- Modify: `apps/fleet-service/internal/maintenanceschedule/entity.go`
- Modify: `apps/fleet-service/internal/maintenanceschedule/model.go`
- Modify: `apps/fleet-service/internal/maintenanceschedule/recurrence.go` (the `Schedule` struct only)
- Modify: `apps/fleet-service/internal/maintenanceschedule/completion_db_test.go` (sqlite DDL)
- Modify: `apps/fleet-service/internal/admin/admintest/db.go` (sqlite DDL)
- Test: `apps/fleet-service/internal/maintenanceschedule/entity_test.go` (create)

**Interfaces:**
- Consumes: nothing.
- Produces:
  - `Entity` fields `OneTime bool`, `DueDate *time.Time`, `DueMileage int`.
  - `Model` accessors `OneTime() bool`, `DueDate() time.Time`, `DueMileage() int`; copy-withs `WithOneTime(bool) Model`, `WithDuePoint(time.Time, int) Model`.
  - `Schedule` fields `OneTime bool`, `DueDate time.Time`, `DueMileage int`.
  - Tasks 3–9 depend on every one of these names.

- [x] **Step 1: Write the failing test**

Create `apps/fleet-service/internal/maintenanceschedule/entity_test.go`:

```go
package maintenanceschedule

import (
	"testing"
	"time"
)

// The due point round-trips through the entity boundary in both directions.
// due_date is nullable (a zero time is NULL) while due_mileage is a plain int
// where 0 means unset — the same split the file already uses for
// last_completed_* and next_due_*.
func TestEntityRoundTrip_duePoint(t *testing.T) {
	due := time.Date(2026, 11, 30, 0, 0, 0, 0, time.UTC)

	m := Model{
		id:             "s1",
		vehicleID:      "v1",
		categoryID:     "c1",
		recurrenceType: "hybrid",
		oneTime:        true,
		dueDate:        due,
		dueMileage:     60000,
		active:         true,
	}

	e := m.ToEntity()
	if !e.OneTime {
		t.Error("one_time must survive ToEntity")
	}
	if e.DueDate == nil || !e.DueDate.Equal(due) {
		t.Errorf("due_date = %v want %v", e.DueDate, due)
	}
	if e.DueMileage != 60000 {
		t.Errorf("due_mileage = %d want 60000", e.DueMileage)
	}

	back := Make(e)
	if !back.OneTime() || !back.DueDate().Equal(due) || back.DueMileage() != 60000 {
		t.Fatalf("round-trip lost the due point: %+v", back)
	}
}

func TestEntityRoundTrip_zeroDueDateIsNull(t *testing.T) {
	e := Model{id: "s1", recurrenceType: "mileage", dueMileage: 60000}.ToEntity()
	if e.DueDate != nil {
		t.Fatalf("a zero due date must map to NULL, got %v", e.DueDate)
	}
	if got := Make(e).DueDate(); !got.IsZero() {
		t.Fatalf("NULL must map back to a zero time, got %v", got)
	}
}

func TestModelCopyWiths_duePoint(t *testing.T) {
	due := time.Date(2027, 3, 1, 0, 0, 0, 0, time.UTC)
	original := Model{recurrenceType: "hybrid"}

	withOneTime := original.WithOneTime(true)
	if original.OneTime() {
		t.Error("WithOneTime must not mutate the receiver")
	}
	if !withOneTime.OneTime() {
		t.Error("WithOneTime(true) must set the flag on the copy")
	}

	anchored := original.WithDuePoint(due, 60000)
	if !original.DueDate().IsZero() || original.DueMileage() != 0 {
		t.Error("WithDuePoint must not mutate the receiver")
	}
	if !anchored.DueDate().Equal(due) || anchored.DueMileage() != 60000 {
		t.Errorf("WithDuePoint lost values: %v / %d", anchored.DueDate(), anchored.DueMileage())
	}

	cleared := anchored.WithDuePoint(time.Time{}, 0)
	if !cleared.DueDate().IsZero() || cleared.DueMileage() != 0 {
		t.Error("WithDuePoint(zero, 0) must clear the anchor")
	}
}

// AsSchedule is the ONLY bridge from the model into the recurrence engine. A
// field that stops at this boundary is invisible to NextDue and every consumer
// downstream of it.
func TestAsSchedule_carriesDuePoint(t *testing.T) {
	due := time.Date(2026, 11, 30, 0, 0, 0, 0, time.UTC)
	s := Model{recurrenceType: "hybrid", oneTime: true, dueDate: due, dueMileage: 60000}.AsSchedule()
	if !s.OneTime || !s.DueDate.Equal(due) || s.DueMileage != 60000 {
		t.Fatalf("AsSchedule dropped the due point: %+v", s)
	}
}
```

- [x] **Step 2: Run the test to verify it fails**

Run: `go test ./apps/fleet-service/internal/maintenanceschedule/... -run 'TestEntityRoundTrip|TestModelCopyWiths|TestAsSchedule' -v`
Expected: FAIL to compile — `m.oneTime undefined`, `e.DueDate undefined`, `WithOneTime undefined`.

- [x] **Step 3: Add the entity columns and mapping**

In `apps/fleet-service/internal/maintenanceschedule/entity.go`, add three fields to `Entity` immediately after `IntervalMiles`:

```go
	IntervalMiles        int
	// OneTime marks a schedule that is due once and never repeats (FR-OT-1).
	// It is orthogonal to RecurrenceType, which continues to say which AXES
	// the schedule is judged on rather than how often it repeats.
	OneTime bool `gorm:"not null;default:false"`
	// DueDate / DueMileage are the absolute due point. For a one-time schedule
	// they are permanent; for a recurring schedule they are the first-due
	// anchor, cleared on the first completion (FR-ANCHOR-3).
	DueDate    *time.Time
	DueMileage int
```

In `Make`, add a `dueDate` local alongside `lastDate`/`nextDate`:

```go
	var lastDate, nextDate, dueDate time.Time
	if e.LastCompletedDate != nil {
		lastDate = *e.LastCompletedDate
	}
	if e.NextDueDate != nil {
		nextDate = *e.NextDueDate
	}
	if e.DueDate != nil {
		dueDate = *e.DueDate
	}
```

and three fields to the returned `Model` literal, after `intervalMiles`:

```go
		oneTime:              e.OneTime,
		dueDate:              dueDate,
		dueMileage:           e.DueMileage,
```

In `ToEntity`, add the pointer conversion alongside the existing two:

```go
	var lastDate, nextDate, dueDate *time.Time
	if !m.lastCompletedDate.IsZero() {
		d := m.lastCompletedDate
		lastDate = &d
	}
	if !m.nextDueDate.IsZero() {
		d := m.nextDueDate
		nextDate = &d
	}
	if !m.dueDate.IsZero() {
		d := m.dueDate
		dueDate = &d
	}
```

and three fields to the returned `Entity` literal, after `IntervalMiles`:

```go
		OneTime:              m.oneTime,
		DueDate:              dueDate,
		DueMileage:           m.dueMileage,
```

- [x] **Step 4: Add the model fields, accessors, copy-withs and `AsSchedule` carry**

In `apps/fleet-service/internal/maintenanceschedule/model.go`, add three fields to `Model` after `intervalMiles`:

```go
	intervalMiles        int
	oneTime              bool
	dueDate              time.Time
	dueMileage           int
```

Add three accessors after `IntervalMiles()`:

```go
func (m Model) OneTime() bool         { return m.oneTime }
func (m Model) DueDate() time.Time    { return m.dueDate }
func (m Model) DueMileage() int       { return m.dueMileage }
```

Extend `AsSchedule` to carry them:

```go
func (m Model) AsSchedule() Schedule {
	return Schedule{
		RecurrenceType:       m.recurrenceType,
		IntervalMonths:       m.intervalMonths,
		IntervalMiles:        m.intervalMiles,
		OneTime:              m.oneTime,
		DueDate:              m.dueDate,
		DueMileage:           m.dueMileage,
		LastCompletedDate:    m.lastCompletedDate,
		LastCompletedMileage: m.lastCompletedMileage,
	}
}
```

Add two copy-withs after `WithRecurrence`:

```go
// WithOneTime returns a copy with the one-time flag changed.
func (m Model) WithOneTime(v bool) Model { m.oneTime = v; return m }

// WithDuePoint returns a copy with the absolute due point set (or, with a zero
// date and 0 miles, cleared). Both axes move together because the completion
// flow clears both together (FR-ANCHOR-3).
func (m Model) WithDuePoint(date time.Time, miles int) Model {
	m.dueDate = date
	m.dueMileage = miles
	return m
}
```

- [x] **Step 5: Add the `Schedule` fields**

In `apps/fleet-service/internal/maintenanceschedule/recurrence.go`, extend the struct (leave `NextDue` alone for now):

```go
// Schedule is the pure input to recurrence math (mirrors the entity's fields).
type Schedule struct {
	RecurrenceType       string // time | mileage | hybrid
	IntervalMonths       int
	IntervalMiles        int
	OneTime              bool
	DueDate              time.Time
	DueMileage           int
	LastCompletedDate    time.Time
	LastCompletedMileage int
}
```

- [x] **Step 6: Add the columns to both sqlite test harnesses**

The package's tests and the admin fixtures create `fleet.maintenance_schedules` with explicit DDL rather than `AutoMigrate`. A write that names a column the DDL lacks fails with `no such column`.

In `apps/fleet-service/internal/maintenanceschedule/completion_db_test.go`, replace the `fleet.maintenance_schedules` DDL string with:

```go
		`CREATE TABLE fleet.maintenance_schedules (
			id TEXT PRIMARY KEY, vehicle_id TEXT, category_id TEXT, recurrence_type TEXT,
			interval_months INTEGER, interval_miles INTEGER, one_time BOOLEAN DEFAULT 0,
			due_date DATETIME, due_mileage INTEGER DEFAULT 0, last_completed_date DATETIME,
			last_completed_mileage INTEGER, next_due_date DATETIME, next_due_mileage INTEGER,
			status TEXT, severity TEXT, active INTEGER, created_at DATETIME, updated_at DATETIME,
			deleted_at DATETIME, purge_operation_id TEXT)`,
```

In `apps/fleet-service/internal/admin/admintest/db.go`, replace the matching DDL string with:

```go
	`CREATE TABLE fleet.maintenance_schedules (
		id TEXT PRIMARY KEY, vehicle_id TEXT, category_id TEXT, recurrence_type TEXT,
		interval_months INTEGER, interval_miles INTEGER, one_time BOOLEAN DEFAULT 0,
		due_date DATETIME, due_mileage INTEGER DEFAULT 0, last_completed_date DATETIME,
		last_completed_mileage INTEGER, next_due_date DATETIME, next_due_mileage INTEGER,
		status TEXT, severity TEXT, active BOOLEAN, created_at DATETIME,
		updated_at DATETIME, deleted_at DATETIME, purge_operation_id TEXT)`,
```

- [x] **Step 7: Run the tests to verify they pass**

Run: `go test ./apps/fleet-service/internal/maintenanceschedule/... ./apps/fleet-service/internal/admin/... -v`
Expected: PASS, including every pre-existing test in both packages.

- [x] **Step 8: Commit**

```bash
git add apps/fleet-service/internal/maintenanceschedule/entity.go \
        apps/fleet-service/internal/maintenanceschedule/entity_test.go \
        apps/fleet-service/internal/maintenanceschedule/model.go \
        apps/fleet-service/internal/maintenanceschedule/recurrence.go \
        apps/fleet-service/internal/maintenanceschedule/completion_db_test.go \
        apps/fleet-service/internal/admin/admintest/db.go
git commit -m "feat(fleet): add one_time/due_date/due_mileage to maintenance schedules"
```

---

## Task 3: The revised `NextDue`

**Files:**
- Modify: `apps/fleet-service/internal/maintenanceschedule/recurrence.go` (the `NextDue` function)
- Test: `apps/fleet-service/internal/maintenanceschedule/recurrence_test.go`

**Interfaces:**
- Consumes: `Schedule.OneTime`, `Schedule.DueDate`, `Schedule.DueMileage` from Task 2.
- Produces: no new names — `NextDue(Schedule) (time.Time, int)` keeps its signature. Every downstream consumer (Tasks 4, 6, 7, 9 and the untouched queue/banner/dashboard/feed paths) inherits the new behaviour through it.

- [x] **Step 1: Write the failing test**

Append to `apps/fleet-service/internal/maintenanceschedule/recurrence_test.go`:

```go
// The due-point override is per AXIS, not per schedule: a hybrid row can hold a
// date anchor and no mileage anchor mid-migration, and each axis must resolve
// independently rather than one anchor suppressing the other's arithmetic.
//
// The !OneTime guard is what makes a one-time axis terminal. Without it, a
// completed one-time schedule whose anchor was cleared falls back to
// lastCompleted + 0 — "due again the instant it was completed".
func TestNextDue_duePointAndOneTime(t *testing.T) {
	anchorDate := time.Date(2026, 11, 30, 0, 0, 0, 0, time.UTC)

	cases := []struct {
		name      string
		s         Schedule
		wantDate  time.Time
		wantMiles int
	}{
		{
			name:      "one-time time: the anchor verbatim",
			s:         Schedule{RecurrenceType: "time", OneTime: true, DueDate: anchorDate},
			wantDate:  anchorDate,
			wantMiles: 0,
		},
		{
			name:      "one-time mileage: the anchor verbatim",
			s:         Schedule{RecurrenceType: "mileage", OneTime: true, DueMileage: 60000},
			wantDate:  time.Time{},
			wantMiles: 60000,
		},
		{
			name:      "one-time hybrid: both anchors verbatim",
			s:         Schedule{RecurrenceType: "hybrid", OneTime: true, DueDate: anchorDate, DueMileage: 60000},
			wantDate:  anchorDate,
			wantMiles: 60000,
		},
		{
			name: "completed one-time with a cleared anchor yields the zero due point",
			s: Schedule{
				RecurrenceType:       "hybrid",
				OneTime:              true,
				LastCompletedDate:    base,
				LastCompletedMileage: 42000,
			},
			wantDate:  time.Time{},
			wantMiles: 0,
		},
		{
			name: "recurring anchored: the anchor wins over interval arithmetic",
			s: Schedule{
				RecurrenceType: "hybrid",
				IntervalMonths: 12,
				IntervalMiles:  5000,
				DueDate:        anchorDate,
				DueMileage:     60000,
			},
			wantDate:  anchorDate,
			wantMiles: 60000,
		},
		{
			name: "recurring unanchored: interval arithmetic from the completion point",
			s: Schedule{
				RecurrenceType:       "hybrid",
				IntervalMonths:       12,
				IntervalMiles:        5000,
				LastCompletedDate:    base,
				LastCompletedMileage: 30000,
			},
			wantDate:  base.AddDate(0, 12, 0),
			wantMiles: 35000,
		},
		{
			name: "recurring half-anchored: each axis resolves independently",
			s: Schedule{
				RecurrenceType:       "hybrid",
				IntervalMonths:       12,
				IntervalMiles:        5000,
				DueDate:              anchorDate,
				LastCompletedDate:    base,
				LastCompletedMileage: 30000,
			},
			wantDate:  anchorDate,
			wantMiles: 35000,
		},
		{
			name: "a mileage anchor is ignored on a pure-time schedule",
			s: Schedule{
				RecurrenceType:    "time",
				IntervalMonths:    12,
				DueMileage:        60000,
				LastCompletedDate: base,
			},
			wantDate:  base.AddDate(0, 12, 0),
			wantMiles: 0,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			nd, nm := NextDue(c.s)
			if !nd.Equal(c.wantDate) {
				t.Errorf("next_due_date = %v want %v", nd, c.wantDate)
			}
			if nm != c.wantMiles {
				t.Errorf("next_due_mileage = %d want %d", nm, c.wantMiles)
			}
		})
	}
}

// A one-time time-axis schedule with no anchor resolves to the zero time, which
// DueState reads as overdue. Validation (Task 4) makes this unreachable on any
// row this service can create and the backfill (Task 7) repairs legacy rows;
// this test pins the behaviour so neither safeguard is quietly relied upon.
func TestNextDue_oneTimeWithoutAnchorIsZero(t *testing.T) {
	nd, nm := NextDue(Schedule{RecurrenceType: "hybrid", OneTime: true})
	if !nd.IsZero() || nm != 0 {
		t.Fatalf("want the zero due point, got %v / %d", nd, nm)
	}
	if got := DueState(Schedule{RecurrenceType: "time", OneTime: true}, base, 0, DefaultThresholds); got != "overdue" {
		t.Fatalf("an unanchored one-time time axis reads as %s, want overdue", got)
	}
}
```

- [x] **Step 2: Run the test to verify it fails**

Run: `go test ./apps/fleet-service/internal/maintenanceschedule/... -run TestNextDue -v`
Expected: FAIL — the anchored cases return interval arithmetic (or the zero-value fallback) instead of the anchor.

- [x] **Step 3: Rewrite `NextDue`**

Replace the whole `NextDue` function in `apps/fleet-service/internal/maintenanceschedule/recurrence.go` with:

```go
// NextDue resolves a schedule's next due point, one axis at a time.
//
// Each axis prefers the stored absolute due point over interval arithmetic. For
// a one-time schedule that point is permanent; for a recurring schedule it is
// the first-due anchor, which the completion flow clears so that arithmetic
// from the new completion point takes over for every subsequent cycle
// (FR-OT-4, FR-ANCHOR-2, FR-ANCHOR-3).
//
// The override is per AXIS rather than per schedule: a hybrid row may hold one
// anchor and not the other, and the presence of one must not suppress the
// other's arithmetic.
//
// The !s.OneTime guard is what makes a one-time axis terminal. Without it, a
// completed one-time schedule whose anchor was cleared would fall back to
// lastCompleted + 0 — "due again the instant it was completed".
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

- [x] **Step 4: Run the whole package to verify nothing regressed**

Run: `go test ./apps/fleet-service/internal/maintenanceschedule/... -v`
Expected: PASS. The pre-existing `TestNextDue` cases all carry a zero due point and `OneTime: false`, so they keep taking the arithmetic branch unchanged.

- [x] **Step 5: Commit**

```bash
git add apps/fleet-service/internal/maintenanceschedule/recurrence.go \
        apps/fleet-service/internal/maintenanceschedule/recurrence_test.go
git commit -m "feat(fleet): prefer the stored due point over interval arithmetic in NextDue"
```

---

## Task 4: One validator, plus the builder's new setters

Today the invariants live inside `Builder.Build` and `Processor.Update` does not consult them. Extract them into a package-level `validate(Model) error` that both paths run (Task 5 wires the second caller).

`SetCurrentMileage` is not in the PRD and is deliberate. `Build` currently derives the stored status with `DueState(..., b.m.lastCompletedMileage, ...)` — it passes the *last completed* mileage as the *current* mileage, which for a new schedule is `0`. A new mileage schedule due at 60,000 on a vehicle reading 59,900 is therefore stored as `ok`. Read paths compute state live so the user rarely sees it, but `RecomputeAll` compares against the stored value to detect the transition edge into `overdue`, so a wrong initial value can suppress or spuriously fire a `schedule.overdue` event on the first sweep. `resource.go` already holds `v.CurrentMileage()` at the call site.

**Files:**
- Modify: `apps/fleet-service/internal/maintenanceschedule/builder.go`
- Test: `apps/fleet-service/internal/maintenanceschedule/builder_test.go` (create)

**Interfaces:**
- Consumes: `Model.oneTime` / `dueDate` / `dueMileage` (Task 2), `NextDue` (Task 3).
- Produces:
  - `func validate(m Model) error` — package-private; Task 5's `Processor.Update` is the second caller.
  - `func (b *Builder) SetOneTime(v bool) *Builder`
  - `func (b *Builder) SetDuePoint(date time.Time, miles int) *Builder`
  - `func (b *Builder) SetCurrentMileage(miles int) *Builder`
  - Task 8's create handler calls all three.

- [x] **Step 1: Write the failing test**

Create `apps/fleet-service/internal/maintenanceschedule/builder_test.go`:

```go
package maintenanceschedule

import (
	"errors"
	"testing"
	"time"

	"github.com/jtumidanski/myfleet/packages/shared-go/server"
)

var anchor = time.Date(2026, 11, 30, 0, 0, 0, 0, time.UTC)

// build applies the common identity fields so each case only states what it is
// actually exercising.
func build(fn func(*Builder)) (Model, error) {
	b := NewBuilder().SetVehicleID("v1").SetCategoryID("c1")
	fn(b)
	return b.Build()
}

func TestBuild_validationMatrix(t *testing.T) {
	cases := []struct {
		name    string
		mutate  func(*Builder)
		wantErr bool
	}{
		// --- recurring: intervals AND a first-due anchor on every covered axis.
		{"recurring time: complete", func(b *Builder) {
			b.SetRecurrenceType("time").SetIntervalMonths(6).SetDuePoint(anchor, 0)
		}, false},
		{"recurring time: no interval", func(b *Builder) {
			b.SetRecurrenceType("time").SetDuePoint(anchor, 0)
		}, true},
		{"recurring time: no anchor", func(b *Builder) {
			b.SetRecurrenceType("time").SetIntervalMonths(6)
		}, true},
		{"recurring mileage: complete", func(b *Builder) {
			b.SetRecurrenceType("mileage").SetIntervalMiles(5000).SetDuePoint(time.Time{}, 60000)
		}, false},
		{"recurring mileage: no interval", func(b *Builder) {
			b.SetRecurrenceType("mileage").SetDuePoint(time.Time{}, 60000)
		}, true},
		{"recurring mileage: no anchor", func(b *Builder) {
			b.SetRecurrenceType("mileage").SetIntervalMiles(5000)
		}, true},
		{"recurring hybrid: complete", func(b *Builder) {
			b.SetRecurrenceType("hybrid").SetIntervalMonths(6).SetIntervalMiles(5000).SetDuePoint(anchor, 60000)
		}, false},
		{"recurring hybrid: missing the mileage anchor", func(b *Builder) {
			b.SetRecurrenceType("hybrid").SetIntervalMonths(6).SetIntervalMiles(5000).SetDuePoint(anchor, 0)
		}, true},
		{"recurring hybrid: missing the date anchor", func(b *Builder) {
			b.SetRecurrenceType("hybrid").SetIntervalMonths(6).SetIntervalMiles(5000).SetDuePoint(time.Time{}, 60000)
		}, true},

		// --- one-time: a due point on every covered axis and NO interval.
		{"one-time time: complete", func(b *Builder) {
			b.SetRecurrenceType("time").SetOneTime(true).SetDuePoint(anchor, 0)
		}, false},
		{"one-time time: no due date", func(b *Builder) {
			b.SetRecurrenceType("time").SetOneTime(true)
		}, true},
		{"one-time mileage: complete", func(b *Builder) {
			b.SetRecurrenceType("mileage").SetOneTime(true).SetDuePoint(time.Time{}, 60000)
		}, false},
		{"one-time mileage: zero due mileage", func(b *Builder) {
			b.SetRecurrenceType("mileage").SetOneTime(true).SetDuePoint(time.Time{}, 0)
		}, true},
		{"one-time hybrid: complete", func(b *Builder) {
			b.SetRecurrenceType("hybrid").SetOneTime(true).SetDuePoint(anchor, 60000)
		}, false},
		{"one-time with intervalMonths is rejected", func(b *Builder) {
			b.SetRecurrenceType("time").SetOneTime(true).SetDuePoint(anchor, 0).SetIntervalMonths(6)
		}, true},
		{"one-time with intervalMiles is rejected", func(b *Builder) {
			b.SetRecurrenceType("mileage").SetOneTime(true).SetDuePoint(time.Time{}, 60000).SetIntervalMiles(5000)
		}, true},

		// --- pre-existing invariants.
		{"unknown recurrence type", func(b *Builder) {
			b.SetRecurrenceType("once").SetDuePoint(anchor, 0)
		}, true},
		{"empty recurrence type", func(b *Builder) {
			b.SetDuePoint(anchor, 0)
		}, true},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := build(c.mutate)
			if c.wantErr {
				if !errors.Is(err, server.ErrValidation) {
					t.Fatalf("want server.ErrValidation, got %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestBuild_requiresIdentity(t *testing.T) {
	if _, err := NewBuilder().SetCategoryID("c1").SetRecurrenceType("time").
		SetIntervalMonths(6).SetDuePoint(anchor, 0).Build(); !errors.Is(err, server.ErrValidation) {
		t.Fatalf("missing vehicleID: want server.ErrValidation, got %v", err)
	}
	if _, err := NewBuilder().SetVehicleID("v1").SetRecurrenceType("time").
		SetIntervalMonths(6).SetDuePoint(anchor, 0).Build(); !errors.Is(err, server.ErrValidation) {
		t.Fatalf("missing categoryID: want server.ErrValidation, got %v", err)
	}
}

// The anchor requirement must NOT fire on a schedule that already has a
// completion point to derive from — FR-ANCHOR-3 clears the anchor on purpose,
// so without this carve-out the completion flow would write a row its own
// validator rejects.
func TestValidate_completedRecurringNeedsNoAnchor(t *testing.T) {
	m := Model{
		vehicleID:            "v1",
		categoryID:           "c1",
		recurrenceType:       "hybrid",
		intervalMonths:       12,
		intervalMiles:        5000,
		lastCompletedDate:    base,
		lastCompletedMileage: 42000,
		active:               true,
	}
	if err := validate(m); err != nil {
		t.Fatalf("a completed recurring schedule must validate without an anchor: %v", err)
	}
}

// A completed one-time schedule is deactivated with its anchor cleared. That is
// a valid terminal state and must validate, or the row becomes un-PATCHable.
func TestValidate_completedOneTimeIsInactiveAndValid(t *testing.T) {
	m := Model{
		vehicleID:            "v1",
		categoryID:           "c1",
		recurrenceType:       "mileage",
		oneTime:              true,
		lastCompletedMileage: 60000,
		active:               false,
	}
	if err := validate(m); err != nil {
		t.Fatalf("a completed, deactivated one-time schedule must validate: %v", err)
	}
}

// Reactivating a one-time schedule without giving it a due point would produce
// a live row with no resolvable due date — permanently overdue.
func TestValidate_reactivatedOneTimeNeedsAnAnchor(t *testing.T) {
	m := Model{
		vehicleID:            "v1",
		categoryID:           "c1",
		recurrenceType:       "mileage",
		oneTime:              true,
		lastCompletedMileage: 60000,
		active:               true,
	}
	if !errors.Is(validate(m), server.ErrValidation) {
		t.Fatal("an active one-time schedule with no due point must be rejected")
	}
}

// Build derives the stored status from the VEHICLE's current mileage, not from
// last_completed_mileage. RecomputeAll compares against this stored value to
// find the transition edge into overdue, so a wrong initial value suppresses or
// spuriously fires the first schedule.overdue event.
func TestBuild_usesCurrentMileageForInitialStatus(t *testing.T) {
	m, err := build(func(b *Builder) {
		b.SetRecurrenceType("mileage").SetOneTime(true).
			SetDuePoint(time.Time{}, 60000).SetCurrentMileage(59900)
	})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if m.Status() != "upcoming" {
		t.Fatalf("status = %q want upcoming (59900 is within 500 of 60000)", m.Status())
	}
	if m.Severity() != "recommended" {
		t.Fatalf("severity = %q want recommended", m.Severity())
	}
	if m.NextDueMileage() != 60000 {
		t.Fatalf("next_due_mileage = %d want 60000", m.NextDueMileage())
	}
}

// A brand-new recurring schedule anchored in the future is ok, not overdue —
// the defect FR-ANCHOR-1 exists to fix.
func TestBuild_newRecurringScheduleIsNotBornOverdue(t *testing.T) {
	future := time.Now().UTC().AddDate(0, 6, 0)
	m, err := build(func(b *Builder) {
		b.SetRecurrenceType("time").SetIntervalMonths(6).SetDuePoint(future, 0)
	})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if m.Status() != "ok" {
		t.Fatalf("status = %q want ok", m.Status())
	}
}
```

- [x] **Step 2: Run the test to verify it fails**

Run: `go test ./apps/fleet-service/internal/maintenanceschedule/... -run 'TestBuild|TestValidate' -v`
Expected: FAIL to compile — `SetOneTime`, `SetDuePoint`, `SetCurrentMileage`, `validate` undefined.

- [x] **Step 3: Extract `validate` and add the setters**

Rewrite `apps/fleet-service/internal/maintenanceschedule/builder.go` from the `Builder` type declaration down:

```go
// Builder constructs a valid maintenance schedule Model. Build() returns
// (Model, error) because vehicleID, categoryID, a valid recurrence type, and a
// sane interval and due point for that type are invariants enforced at
// construction.
//
// currentMileage is the VEHICLE's odometer, not part of the model. It exists so
// the initial stored status is derived against the same value the hourly
// recompute will use; without it a new mileage schedule is stored "ok"
// regardless of how close the vehicle already is to the due point.
type Builder struct {
	m              Model
	currentMileage int
}

func NewBuilder() *Builder {
	return &Builder{m: Model{id: uuid.NewString(), active: true}}
}

func (b *Builder) SetVehicleID(vehicleID string) *Builder    { b.m.vehicleID = vehicleID; return b }
func (b *Builder) SetCategoryID(categoryID string) *Builder  { b.m.categoryID = categoryID; return b }
func (b *Builder) SetRecurrenceType(t string) *Builder       { b.m.recurrenceType = t; return b }
func (b *Builder) SetIntervalMonths(months int) *Builder     { b.m.intervalMonths = months; return b }
func (b *Builder) SetIntervalMiles(miles int) *Builder       { b.m.intervalMiles = miles; return b }
func (b *Builder) SetActive(active bool) *Builder            { b.m.active = active; return b }
func (b *Builder) SetLastCompletedDate(t time.Time) *Builder { b.m.lastCompletedDate = t; return b }
func (b *Builder) SetLastCompletedMileage(m int) *Builder    { b.m.lastCompletedMileage = m; return b }
func (b *Builder) SetOneTime(v bool) *Builder                { b.m.oneTime = v; return b }

// SetDuePoint sets the absolute due point: the permanent due point of a
// one-time schedule, or the first-due anchor of a recurring one.
func (b *Builder) SetDuePoint(date time.Time, miles int) *Builder {
	b.m.dueDate = date
	b.m.dueMileage = miles
	return b
}

// SetCurrentMileage supplies the owning vehicle's odometer for the initial
// status derivation. It is not persisted on the model.
func (b *Builder) SetCurrentMileage(miles int) *Builder { b.currentMileage = miles; return b }

// validate enforces every maintenance-schedule invariant on a fully-formed
// model. Both construction (Builder.Build) and mutation (Processor.Update) run
// it, so an invariant cannot be satisfied at creation and violated by a PATCH.
//
// It returns server.ErrValidation for every failure and does not say which rule
// failed: that matches the rest of the service and server.WriteError's mapping,
// and the frontend Zod schema is the layer that produces per-field messages.
func validate(m Model) error {
	if m.vehicleID == "" || m.categoryID == "" {
		return server.ErrValidation
	}
	if !validRecurrence[m.recurrenceType] {
		return server.ErrValidation
	}
	timed := m.recurrenceType == "time" || m.recurrenceType == "hybrid"
	miled := m.recurrenceType == "mileage" || m.recurrenceType == "hybrid"

	// Intervals: required for a recurring schedule on every covered axis,
	// forbidden outright on a one-time one (FR-OT-3).
	if m.oneTime {
		if m.intervalMonths != 0 || m.intervalMiles != 0 {
			return server.ErrValidation
		}
	} else {
		if timed && m.intervalMonths <= 0 {
			return server.ErrValidation
		}
		if miled && m.intervalMiles <= 0 {
			return server.ErrValidation
		}
	}

	// The due point is required exactly where it is the ONLY thing that can
	// produce a due date: on every live one-time schedule (FR-OT-2), and on a
	// recurring schedule that has never been completed (FR-ANCHOR-1). It is not
	// required once there is a completion point to derive from — FR-ANCHOR-3
	// clears it on purpose — nor on a completed-and-deactivated one-time row,
	// whose anchor the completion flow deliberately cleared.
	neverCompleted := m.lastCompletedDate.IsZero() && m.lastCompletedMileage == 0
	if (m.oneTime && m.active) || (!m.oneTime && neverCompleted) {
		if timed && m.dueDate.IsZero() {
			return server.ErrValidation
		}
		if miled && m.dueMileage <= 0 {
			return server.ErrValidation
		}
	}
	return nil
}

// Build validates invariants and returns the model or a validation error.
func (b *Builder) Build() (Model, error) {
	if err := validate(b.m); err != nil {
		return Model{}, err
	}
	// Compute initial next-due + status from the due point / completion point.
	nd, nm := NextDue(b.m.AsSchedule())
	b.m.nextDueDate = nd
	b.m.nextDueMileage = nm
	state := DueState(b.m.AsSchedule(), time.Now().UTC(), b.currentMileage, DefaultThresholds)
	b.m.status = state
	b.m.severity = Severity(state)
	return b.m, nil
}
```

- [x] **Step 4: Run the package tests**

Run: `go test ./apps/fleet-service/internal/maintenanceschedule/... -v`
Expected: PASS. `TestCompleteInTransaction`'s fixture schedule carries `SetLastCompletedDate(base)` and `SetLastCompletedMileage(35000)`, so the never-completed carve-out exempts it from the anchor requirement and it still builds.

- [x] **Step 5: Commit**

```bash
git add apps/fleet-service/internal/maintenanceschedule/builder.go \
        apps/fleet-service/internal/maintenanceschedule/builder_test.go
git commit -m "feat(fleet): extract schedule invariants into a shared validate()"
```

---

## Task 5: `Processor.Update` validates the resulting model

FR-UPD-2. Today `Update` applies a mutation function, recomputes next-due, and writes — so a PATCH can produce a `hybrid` schedule with a zero interval, which `NextDue` turns into "due at the last completion point" and the queues turn into a permanently overdue row.

**Files:**
- Modify: `apps/fleet-service/internal/maintenanceschedule/processor.go` (the `Update` method)
- Test: `apps/fleet-service/internal/maintenanceschedule/processor_update_test.go` (create)

**Interfaces:**
- Consumes: `validate` (Task 4), `newCompletionDB(t)` from `completion_db_test.go` (same package).
- Produces: no new names — `Processor.Update(id string, apply func(Model) Model) (Model, error)` keeps its signature and gains a `server.ErrValidation` failure mode.

- [x] **Step 1: Write the failing test**

Create `apps/fleet-service/internal/maintenanceschedule/processor_update_test.go`:

```go
package maintenanceschedule

import (
	"errors"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"gorm.io/gorm"

	"github.com/jtumidanski/myfleet/packages/shared-go/server"
)

// seedHybrid inserts a valid, anchored, never-completed hybrid schedule and
// returns the processor bound to the same db plus the created model.
func seedHybrid(t *testing.T, db *gorm.DB) (*Processor, Model) {
	t.Helper()
	m, err := NewBuilder().SetVehicleID("v1").SetCategoryID("c1").
		SetRecurrenceType("hybrid").SetIntervalMonths(12).SetIntervalMiles(5000).
		SetDuePoint(anchor, 60000).Build()
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	created, err := NewAdministrator(db).Insert(m)
	if err != nil {
		t.Fatalf("insert: %v", err)
	}
	return NewProcessor(logrus.New(), NewProvider(db), NewAdministrator(db)), created
}

// A PATCH that would leave a hybrid schedule with a zero interval is rejected,
// and the stored row is untouched.
func TestProcessorUpdate_rejectsZeroIntervalOnHybrid(t *testing.T) {
	db := newCompletionDB(t)
	proc, created := seedHybrid(t, db)

	_, err := proc.Update(created.ID(), func(m Model) Model {
		return m.WithRecurrence("hybrid", 0, m.IntervalMiles())
	})
	if !errors.Is(err, server.ErrValidation) {
		t.Fatalf("want server.ErrValidation, got %v", err)
	}

	after, err := NewProvider(db).GetByID(created.ID())
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if after.IntervalMonths() != 12 {
		t.Fatalf("a rejected PATCH must not write: interval_months = %d want 12", after.IntervalMonths())
	}
}

// A PATCH that sets oneTime while leaving the intervals in place is rejected
// (FR-OT-3): a one-time schedule must not carry intervals.
func TestProcessorUpdate_rejectsOneTimeWithIntervals(t *testing.T) {
	db := newCompletionDB(t)
	proc, created := seedHybrid(t, db)

	_, err := proc.Update(created.ID(), func(m Model) Model { return m.WithOneTime(true) })
	if !errors.Is(err, server.ErrValidation) {
		t.Fatalf("want server.ErrValidation, got %v", err)
	}
}
```

Drop the `"time"` import if the file does not otherwise use it — the two tests above do not.


- [x] **Step 2: Run the test to verify it fails**

Run: `go test ./apps/fleet-service/internal/maintenanceschedule/... -run TestProcessorUpdate -v`
Expected: FAIL — the first two cases return `nil` and write the invalid row.

- [x] **Step 3: Add the validation call**

In `apps/fleet-service/internal/maintenanceschedule/processor.go`, replace the body of `Update` with:

```go
// Update applies a partial update to an existing schedule, recomputing next-due.
//
// The RESULTING model is validated before anything is recomputed or written
// (FR-UPD-2). Validation precedes the recompute so an invalid intermediate
// never reaches NextDue, and precedes the write so a rejected PATCH leaves the
// stored row exactly as it was.
func (pr *Processor) Update(id string, apply func(Model) Model) (Model, error) {
	m, err := pr.GetByID(id)
	if err != nil {
		return Model{}, err
	}
	updated := apply(m)
	if err := validate(updated); err != nil {
		return Model{}, err
	}
	// Recompute next-due + status from the (possibly changed) recurrence params.
	nd, nm := NextDue(updated.AsSchedule())
	updated = updated.WithNextDue(nd, nm)
	state := DueState(updated.AsSchedule(), time.Now().UTC(), updated.LastCompletedMileage(), DefaultThresholds)
	updated = updated.WithStatus(state, Severity(state))
	return pr.a.Update(updated)
}
```

- [x] **Step 4: Run the tests to verify they pass**

Run: `go test ./apps/fleet-service/internal/maintenanceschedule/... -v`
Expected: PASS, including every pre-existing test in the package.

- [x] **Step 5: Commit**

```bash
git add apps/fleet-service/internal/maintenanceschedule/processor.go \
        apps/fleet-service/internal/maintenanceschedule/processor_update_test.go
git commit -m "fix(fleet): validate the resulting model on schedule PATCH"
```

---

## Task 6: Persist the new columns; clear the anchor and deactivate on completion

Two writes are missing today. `dbAdministrator.Update` writes an explicit column map that does not name the new columns — the failure mode where the conversion PATCH appears to succeed and changes nothing. And `AdvanceTx` neither clears the anchor (FR-ANCHOR-3) nor deactivates a completed one-time schedule (FR-COMPLETE-2).

The already-inactive guard lives **inside** `AdvanceTx`, on the row the transaction has loaded. A handler pre-check (Task 8) gives the good error message, but check-then-act across two statements is a race: two concurrent completes of the same one-time schedule both pass the handler check and both write a maintenance record. Only the in-transaction check is authoritative, and its failure rolls back the record insert and the mileage append with it.

**Files:**
- Modify: `apps/fleet-service/internal/maintenanceschedule/administrator.go`
- Test: `apps/fleet-service/internal/maintenanceschedule/completion_db_test.go`
- Test: `apps/fleet-service/internal/maintenanceschedule/processor_update_test.go`

**Interfaces:**
- Consumes: `Model.OneTime()`, `WithDuePoint` (Task 2), `NextDue` (Task 3).
- Produces: no signature changes. `AdvanceTx` gains a `server.ErrValidation` failure mode on an inactive target; `Administrator.Update` now persists `one_time`, `due_date`, `due_mileage`.

- [x] **Step 1: Write the failing tests**

Append to `apps/fleet-service/internal/maintenanceschedule/completion_db_test.go`:

```go
// seedVehicle inserts a vehicle at the given odometer and returns its id.
func seedVehicle(t *testing.T, db *gorm.DB, miles int) string {
	t.Helper()
	v, err := vehicle.NewBuilder().SetFleetID("f1").SetMake("Honda").SetModel("Civic").
		SetYear(2020).SetCurrentMileage(miles).Build()
	if err != nil {
		t.Fatalf("build vehicle: %v", err)
	}
	if _, err := vehicle.NewAdministrator(db).Insert(v); err != nil {
		t.Fatalf("insert vehicle: %v", err)
	}
	return v.ID()
}

// countRows is a small helper so each assertion reads as one line.
func countRows(t *testing.T, db *gorm.DB, table, where string, args ...any) int64 {
	t.Helper()
	var n int64
	if err := db.Table(table).Where(where, args...).Count(&n).Error; err != nil {
		t.Fatalf("count %s: %v", table, err)
	}
	return n
}

// FR-COMPLETE-1 + FR-COMPLETE-2: completing a one-time schedule writes the
// record and the mileage row AND deactivates the schedule with its anchor
// cleared, all in one transaction.
func TestCompleteInTransaction_oneTimeDeactivates(t *testing.T) {
	db := newCompletionDB(t)
	vehicleID := seedVehicle(t, db, 40000)

	s, err := NewBuilder().SetVehicleID(vehicleID).SetCategoryID("c1").
		SetRecurrenceType("mileage").SetOneTime(true).
		SetDuePoint(time.Time{}, 60000).SetCurrentMileage(40000).Build()
	if err != nil {
		t.Fatalf("build schedule: %v", err)
	}
	created, err := NewAdministrator(db).Insert(s)
	if err != nil {
		t.Fatalf("insert schedule: %v", err)
	}

	deps := NewCompletionDeps(db, maintenancerecord.NewAdministrator(db), NewAdministrator(db))
	at := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	out, err := deps.CompleteInTransaction(logrus.New(), CompletionInput{
		ScheduleID: created.ID(), VehicleID: vehicleID, CategoryID: "c1",
		Date: at, LatestMileage: 60100,
	})
	if err != nil {
		t.Fatalf("complete: %v", err)
	}

	if n := countRows(t, db, "fleet.maintenance_records", "id = ?", out.MaintenanceRecordID); n != 1 {
		t.Fatalf("want 1 maintenance record, got %d", n)
	}
	if n := countRows(t, db, "fleet.mileage_records",
		"vehicle_id = ? AND source = ? AND source_ref_id = ?", vehicleID, "maintenance", out.MaintenanceRecordID); n != 1 {
		t.Fatalf("want 1 mileage record, got %d", n)
	}

	after, err := NewProvider(db).GetByID(created.ID())
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if after.Active() {
		t.Error("a completed one-time schedule must be deactivated")
	}
	if !after.DueDate().IsZero() || after.DueMileage() != 0 {
		t.Errorf("the due point must be cleared, got %v / %d", after.DueDate(), after.DueMileage())
	}
	if after.LastCompletedMileage() != 60100 || !after.LastCompletedDate().Equal(at) {
		t.Errorf("completion point not recorded: %v / %d", after.LastCompletedDate(), after.LastCompletedMileage())
	}
	// A completed one-time schedule has no next due point; the generic DueState
	// would read its zero due-date as overdue, which is what the vehicle
	// schedule list would then render for the inactive row.
	if after.Status() != "ok" {
		t.Errorf("status = %q want ok", after.Status())
	}
}

// FR-COMPLETE-5 + FR-ANCHOR-3: a recurring schedule stays active, and its
// first-due anchor is cleared so interval arithmetic from the new completion
// point takes over permanently.
func TestCompleteInTransaction_recurringClearsAnchorAndStaysActive(t *testing.T) {
	db := newCompletionDB(t)
	vehicleID := seedVehicle(t, db, 40000)

	// Anchored two years out: if the anchor survived, next-due would still be
	// the anchor rather than completion + 12 months.
	farAnchor := time.Date(2028, 1, 1, 0, 0, 0, 0, time.UTC)
	s, err := NewBuilder().SetVehicleID(vehicleID).SetCategoryID("c1").
		SetRecurrenceType("hybrid").SetIntervalMonths(12).SetIntervalMiles(5000).
		SetDuePoint(farAnchor, 90000).SetCurrentMileage(40000).Build()
	if err != nil {
		t.Fatalf("build schedule: %v", err)
	}
	created, err := NewAdministrator(db).Insert(s)
	if err != nil {
		t.Fatalf("insert schedule: %v", err)
	}

	deps := NewCompletionDeps(db, maintenancerecord.NewAdministrator(db), NewAdministrator(db))
	at := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	if _, err := deps.CompleteInTransaction(logrus.New(), CompletionInput{
		ScheduleID: created.ID(), VehicleID: vehicleID, CategoryID: "c1",
		Date: at, LatestMileage: 42000,
	}); err != nil {
		t.Fatalf("complete: %v", err)
	}

	after, err := NewProvider(db).GetByID(created.ID())
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if !after.Active() {
		t.Error("a completed recurring schedule must stay active")
	}
	if !after.DueDate().IsZero() || after.DueMileage() != 0 {
		t.Errorf("the first-due anchor must be cleared, got %v / %d", after.DueDate(), after.DueMileage())
	}
	if want := at.AddDate(0, 12, 0); !after.NextDueDate().Equal(want) {
		t.Errorf("next_due_date = %v want %v (completion point + interval)", after.NextDueDate(), want)
	}
	if after.NextDueMileage() != 47000 {
		t.Errorf("next_due_mileage = %d want 47000", after.NextDueMileage())
	}
}

// FR-COMPLETE-4: completing an already-inactive schedule is rejected, and the
// rejection rolls back the record insert and the mileage append with it. This
// is the acceptance criterion "an integration test that fails the last step and
// asserts nothing was written".
func TestCompleteInTransaction_inactiveScheduleWritesNothing(t *testing.T) {
	db := newCompletionDB(t)
	vehicleID := seedVehicle(t, db, 40000)

	s, err := NewBuilder().SetVehicleID(vehicleID).SetCategoryID("c1").
		SetRecurrenceType("mileage").SetOneTime(true).
		SetDuePoint(time.Time{}, 60000).SetCurrentMileage(40000).Build()
	if err != nil {
		t.Fatalf("build schedule: %v", err)
	}
	created, err := NewAdministrator(db).Insert(s.WithActive(false))
	if err != nil {
		t.Fatalf("insert schedule: %v", err)
	}

	deps := NewCompletionDeps(db, maintenancerecord.NewAdministrator(db), NewAdministrator(db))
	_, err = deps.CompleteInTransaction(logrus.New(), CompletionInput{
		ScheduleID: created.ID(), VehicleID: vehicleID, CategoryID: "c1",
		Date: time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC), LatestMileage: 60100,
	})
	if !errors.Is(err, server.ErrValidation) {
		t.Fatalf("want server.ErrValidation, got %v", err)
	}

	if n := countRows(t, db, "fleet.maintenance_records", "vehicle_id = ?", vehicleID); n != 0 {
		t.Errorf("a rejected completion wrote %d maintenance records", n)
	}
	if n := countRows(t, db, "fleet.mileage_records", "vehicle_id = ?", vehicleID); n != 0 {
		t.Errorf("a rejected completion wrote %d mileage records", n)
	}
	var current int
	if err := db.Table("fleet.vehicles").Select("current_mileage").Where("id = ?", vehicleID).Scan(&current).Error; err != nil {
		t.Fatalf("read current_mileage: %v", err)
	}
	if current != 40000 {
		t.Errorf("current_mileage = %d want 40000 (unchanged)", current)
	}
}
```

Add `"errors"` and `"github.com/jtumidanski/myfleet/packages/shared-go/server"` to that file's imports.

Append to `apps/fleet-service/internal/maintenanceschedule/processor_update_test.go` (and restore its `"time"` import):

```go
// The conversion PATCH: oneTime=false, a recurrence type + interval, active,
// and a cleared due point. Next-due then derives from the completion point the
// completion flow already recorded. This fails outright unless
// Administrator.Update names one_time / due_date / due_mileage in its column
// map — the failure mode where the PATCH appears to succeed and changes nothing.
func TestProcessorUpdate_conversionDerivesFromCompletionPoint(t *testing.T) {
	db := newCompletionDB(t)
	completedAt := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)

	m, err := NewBuilder().SetVehicleID("v1").SetCategoryID("c1").
		SetRecurrenceType("time").SetOneTime(true).SetDuePoint(anchor, 0).Build()
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	created, err := NewAdministrator(db).Insert(
		m.WithLastCompleted(completedAt, 42000).WithDuePoint(time.Time{}, 0).WithActive(false))
	if err != nil {
		t.Fatalf("insert: %v", err)
	}

	proc := NewProcessor(logrus.New(), NewProvider(db), NewAdministrator(db))
	updated, err := proc.Update(created.ID(), func(m Model) Model {
		return m.WithOneTime(false).WithRecurrence("time", 12, 0).
			WithDuePoint(time.Time{}, 0).WithActive(true)
	})
	if err != nil {
		t.Fatalf("conversion PATCH: %v", err)
	}
	if updated.OneTime() || !updated.Active() {
		t.Fatalf("want a recurring, active schedule, got oneTime=%v active=%v", updated.OneTime(), updated.Active())
	}
	if !updated.DueDate().IsZero() || updated.DueMileage() != 0 {
		t.Fatalf("the anchor must be cleared, got %v / %d", updated.DueDate(), updated.DueMileage())
	}
	if want := completedAt.AddDate(0, 12, 0); !updated.NextDueDate().Equal(want) {
		t.Fatalf("next_due_date = %v want %v", updated.NextDueDate(), want)
	}
}

// The inverse: a PATCH that sets a one-time schedule's due point must persist
// it rather than silently dropping it at the column map.
func TestProcessorUpdate_persistsDuePoint(t *testing.T) {
	db := newCompletionDB(t)
	proc, created := seedHybrid(t, db)

	moved := anchor.AddDate(0, 1, 0)
	updated, err := proc.Update(created.ID(), func(m Model) Model {
		return m.WithDuePoint(moved, 70000)
	})
	if err != nil {
		t.Fatalf("PATCH: %v", err)
	}
	if !updated.DueDate().Equal(moved) || updated.DueMileage() != 70000 {
		t.Fatalf("due point not persisted: %v / %d", updated.DueDate(), updated.DueMileage())
	}
}
```

- [x] **Step 2: Run the tests to verify they fail**

Run: `go test ./apps/fleet-service/internal/maintenanceschedule/... -run 'TestCompleteInTransaction|TestProcessorUpdate' -v`
Expected: FAIL — the one-time schedule stays active with its anchor intact, the inactive completion succeeds, and the conversion PATCH leaves `one_time` at `true`.

- [x] **Step 3: Name the new columns in `Administrator.Update`**

In `apps/fleet-service/internal/maintenanceschedule/administrator.go`, extend the `Updates` map inside `Update`:

```go
		Updates(map[string]any{
			"recurrence_type":  e.RecurrenceType,
			"interval_months":  e.IntervalMonths,
			"interval_miles":   e.IntervalMiles,
			"one_time":         e.OneTime,
			// e.DueDate is a *time.Time that ToEntity leaves nil for a zero
			// time, so a cleared anchor writes SQL NULL rather than year 1.
			"due_date":         e.DueDate,
			"due_mileage":      e.DueMileage,
			"active":           e.Active,
			"next_due_date":    e.NextDueDate,
			"next_due_mileage": e.NextDueMileage,
			"status":           e.Status,
			"severity":         e.Severity,
		}).Error; err != nil {
```

- [x] **Step 4: Add the guard, the anchor clearing and the deactivation to `AdvanceTx`**

Replace the whole `AdvanceTx` method with:

```go
func (a *dbAdministrator) AdvanceTx(tx *gorm.DB, id string, date time.Time, miles int) error {
	var e Entity
	if err := tx.First(&e, "id = ?", id).Error; err != nil {
		return err
	}
	// FR-COMPLETE-4. The handler pre-checks this too, for the better error
	// message, but check-then-act across two statements is a race: two
	// concurrent completes of the same one-time schedule would both pass a
	// handler check and both write a maintenance record. Only the check on the
	// row THIS transaction loaded is authoritative, and failing here rolls back
	// the record insert and the mileage append with it.
	if !e.Active {
		return server.ErrValidation
	}

	// Clearing the anchor happens for BOTH kinds (FR-ANCHOR-3, FR-COMPLETE-2).
	// For a recurring schedule it hands next-due permanently to interval
	// arithmetic from the new completion point; for a one-time schedule the row
	// is deactivated in the same update, so the cleared anchor is never read
	// again. Clearing before NextDue is what stops a stale anchor from
	// outranking the completion point.
	m := Make(e).WithLastCompleted(date, miles).WithDuePoint(time.Time{}, 0)
	nd, nm := NextDue(m.AsSchedule())
	m = m.WithNextDue(nd, nm)
	state := DueState(m.AsSchedule(), time.Now().UTC(), miles, DefaultThresholds)
	if m.OneTime() {
		// A completed one-time schedule has no next due point at all, and the
		// generic DueState reads a zero due-date as overdue. The row is being
		// deactivated in this same update, but the stored status is what the
		// vehicle's schedule list renders for an inactive row.
		state = "ok"
	}
	m = m.WithStatus(state, Severity(state))

	updates := map[string]any{
		"last_completed_date":    date,
		"last_completed_mileage": miles,
		"due_date":               nil,
		"due_mileage":            0,
		"next_due_mileage":       m.NextDueMileage(),
		"status":                 m.Status(),
		"severity":               m.Severity(),
	}
	if !m.NextDueDate().IsZero() {
		updates["next_due_date"] = m.NextDueDate()
	} else {
		updates["next_due_date"] = nil
	}
	if m.OneTime() {
		updates["active"] = false
	}
	return tx.Model(&Entity{}).Where("id = ?", id).Updates(updates).Error
}
```

Add `"github.com/jtumidanski/myfleet/packages/shared-go/server"` to the file's imports.

- [x] **Step 5: Run the whole package**

Run: `go test ./apps/fleet-service/internal/maintenanceschedule/... -v`
Expected: PASS, including the pre-existing `TestCompleteInTransaction`.

- [x] **Step 6: Commit**

```bash
git add apps/fleet-service/internal/maintenanceschedule/administrator.go \
        apps/fleet-service/internal/maintenanceschedule/completion_db_test.go \
        apps/fleet-service/internal/maintenanceschedule/processor_update_test.go
git commit -m "feat(fleet): clear the due anchor and deactivate one-time schedules on completion"
```

---

## Task 7: The legacy backfill

Uncompleted legacy schedules have a zero `last_completed_date` and therefore a year-1 next-due; they read as permanently `overdue`. Now that the anchor requirement applies to `Update` (Task 5), they would also reject any PATCH. A one-shot idempotent backfill fixes both, on the pattern of `maintenancecategory.Seed`.

It is implemented in Go, not as a single `UPDATE ... FROM`. The date arithmetic needs `created_at + N months` where `N` is a column, which in Postgres is `created_at + (interval_months || ' months')::interval` — syntax the package's sqlite-backed test harness cannot execute. Doing it in Go reuses the exact `AddDate(0, n, 0)` call `NextDue` uses, so an anchored row and a completed row land on the same date math.

**Files:**
- Create: `apps/fleet-service/internal/maintenanceschedule/backfill.go`
- Create: `apps/fleet-service/internal/maintenanceschedule/backfill_test.go`
- Modify: `apps/fleet-service/cmd/main.go`

**Interfaces:**
- Consumes: `NextDue`, `DueState`, `Severity`, `Thresholds` (Tasks 2–3).
- Produces: `func Backfill(db *gorm.DB) error` — called once from `cmd/main.go`. `func backfill(db *gorm.DB, now time.Time) error` is the package-private, time-injectable body the tests drive.

- [x] **Step 1: Write the failing test**

Create `apps/fleet-service/internal/maintenanceschedule/backfill_test.go`:

```go
package maintenanceschedule

import (
	"testing"
	"time"

	"gorm.io/gorm"
)

// insertRaw writes a schedule row directly, bypassing the builder, so the test
// can create exactly the pre-task-030 shapes the backfill exists to repair.
func insertRaw(t *testing.T, db *gorm.DB, cols map[string]any) {
	t.Helper()
	if err := db.Table("fleet.maintenance_schedules").Create(cols).Error; err != nil {
		t.Fatalf("insert raw schedule: %v", err)
	}
}

func readSchedule(t *testing.T, db *gorm.DB, id string) Model {
	t.Helper()
	m, err := NewProvider(db).GetByID(id)
	if err != nil {
		t.Fatalf("read schedule %s: %v", id, err)
	}
	return m
}

func TestBackfill_anchorsUncompletedSchedules(t *testing.T) {
	db := newCompletionDB(t)
	vehicleID := seedVehicle(t, db, 40000)
	createdAt := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	now := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)

	insertRaw(t, db, map[string]any{
		"id": "s-time", "vehicle_id": vehicleID, "category_id": "c1",
		"recurrence_type": "time", "interval_months": 6, "interval_miles": 0,
		"one_time": false, "due_mileage": 0, "last_completed_mileage": 0,
		"active": true, "status": "overdue", "severity": "urgent", "created_at": createdAt,
	})
	insertRaw(t, db, map[string]any{
		"id": "s-mileage", "vehicle_id": vehicleID, "category_id": "c1",
		"recurrence_type": "mileage", "interval_months": 0, "interval_miles": 5000,
		"one_time": false, "due_mileage": 0, "last_completed_mileage": 0,
		"active": true, "status": "overdue", "severity": "urgent", "created_at": createdAt,
	})
	insertRaw(t, db, map[string]any{
		"id": "s-hybrid", "vehicle_id": vehicleID, "category_id": "c1",
		"recurrence_type": "hybrid", "interval_months": 6, "interval_miles": 5000,
		"one_time": false, "due_mileage": 0, "last_completed_mileage": 0,
		"active": true, "status": "overdue", "severity": "urgent", "created_at": createdAt,
	})

	if err := backfill(db, now); err != nil {
		t.Fatalf("backfill: %v", err)
	}

	wantDate := createdAt.AddDate(0, 6, 0)

	timed := readSchedule(t, db, "s-time")
	if !timed.DueDate().Equal(wantDate) {
		t.Errorf("s-time due_date = %v want %v", timed.DueDate(), wantDate)
	}
	if timed.DueMileage() != 0 {
		t.Errorf("s-time must get no mileage anchor, got %d", timed.DueMileage())
	}
	// The stored next-due and status are refreshed in the same pass, so the
	// internal reminder feed (which reads the stored columns) does not carry a
	// stale token until the next hourly recompute.
	if !timed.NextDueDate().Equal(wantDate) {
		t.Errorf("s-time next_due_date = %v want %v", timed.NextDueDate(), wantDate)
	}
	if timed.Status() != "ok" {
		t.Errorf("s-time status = %q want ok", timed.Status())
	}

	miled := readSchedule(t, db, "s-mileage")
	if miled.DueMileage() != 45000 {
		t.Errorf("s-mileage due_mileage = %d want 45000 (40000 + 5000)", miled.DueMileage())
	}
	if !miled.DueDate().IsZero() {
		t.Errorf("s-mileage must get no date anchor, got %v", miled.DueDate())
	}
	if miled.NextDueMileage() != 45000 {
		t.Errorf("s-mileage next_due_mileage = %d want 45000", miled.NextDueMileage())
	}

	hybrid := readSchedule(t, db, "s-hybrid")
	if !hybrid.DueDate().Equal(wantDate) || hybrid.DueMileage() != 45000 {
		t.Errorf("s-hybrid anchors = %v / %d want %v / 45000", hybrid.DueDate(), hybrid.DueMileage(), wantDate)
	}
}

// Rows that have been completed at least once derive next-due from interval
// arithmetic and legitimately hold no anchor — giving them one would pin them
// to a due point they already passed.
func TestBackfill_leavesCompletedAndAnchoredRowsAlone(t *testing.T) {
	db := newCompletionDB(t)
	vehicleID := seedVehicle(t, db, 40000)
	createdAt := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	completedAt := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	existing := time.Date(2027, 5, 5, 0, 0, 0, 0, time.UTC)

	insertRaw(t, db, map[string]any{
		"id": "s-completed", "vehicle_id": vehicleID, "category_id": "c1",
		"recurrence_type": "hybrid", "interval_months": 6, "interval_miles": 5000,
		"one_time": false, "due_mileage": 0, "last_completed_date": completedAt,
		"last_completed_mileage": 38000, "active": true, "created_at": createdAt,
	})
	insertRaw(t, db, map[string]any{
		"id": "s-anchored", "vehicle_id": vehicleID, "category_id": "c1",
		"recurrence_type": "time", "interval_months": 6, "interval_miles": 0,
		"one_time": false, "due_date": existing, "due_mileage": 0,
		"last_completed_mileage": 0, "active": true, "created_at": createdAt,
	})
	insertRaw(t, db, map[string]any{
		"id": "s-onetime", "vehicle_id": vehicleID, "category_id": "c1",
		"recurrence_type": "mileage", "interval_months": 0, "interval_miles": 0,
		"one_time": true, "due_mileage": 60000, "last_completed_mileage": 0,
		"active": true, "created_at": createdAt,
	})

	if err := backfill(db, time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)); err != nil {
		t.Fatalf("backfill: %v", err)
	}

	completed := readSchedule(t, db, "s-completed")
	if !completed.DueDate().IsZero() || completed.DueMileage() != 0 {
		t.Errorf("a completed row must keep no anchor, got %v / %d", completed.DueDate(), completed.DueMileage())
	}
	if got := readSchedule(t, db, "s-anchored").DueDate(); !got.Equal(existing) {
		t.Errorf("an already-anchored row must be untouched, got %v want %v", got, existing)
	}
	if got := readSchedule(t, db, "s-onetime").DueMileage(); got != 60000 {
		t.Errorf("a one-time row must be untouched, got %d want 60000", got)
	}
}

// The selection predicate must make a second run a no-op: after the first pass
// every repaired row has at least one anchor set and no longer matches.
func TestBackfill_isIdempotent(t *testing.T) {
	db := newCompletionDB(t)
	vehicleID := seedVehicle(t, db, 40000)
	createdAt := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	for _, id := range []string{"s-time", "s-mileage", "s-hybrid"} {
		recur := map[string]string{"s-time": "time", "s-mileage": "mileage", "s-hybrid": "hybrid"}[id]
		insertRaw(t, db, map[string]any{
			"id": id, "vehicle_id": vehicleID, "category_id": "c1",
			"recurrence_type": recur, "interval_months": 6, "interval_miles": 5000,
			"one_time": false, "due_mileage": 0, "last_completed_mileage": 0,
			"active": true, "created_at": createdAt,
		})
	}

	first := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	if err := backfill(db, first); err != nil {
		t.Fatalf("first backfill: %v", err)
	}
	before := map[string]Model{}
	for _, id := range []string{"s-time", "s-mileage", "s-hybrid"} {
		before[id] = readSchedule(t, db, id)
	}

	// A LATER "now" and a moved odometer would both change the result if the
	// second run touched anything.
	if err := db.Table("fleet.vehicles").Where("id = ?", vehicleID).
		Update("current_mileage", 55000).Error; err != nil {
		t.Fatalf("move odometer: %v", err)
	}
	if err := backfill(db, first.AddDate(1, 0, 0)); err != nil {
		t.Fatalf("second backfill: %v", err)
	}

	for id, was := range before {
		now := readSchedule(t, db, id)
		if !now.DueDate().Equal(was.DueDate()) || now.DueMileage() != was.DueMileage() {
			t.Errorf("%s changed on the second run: %v/%d -> %v/%d",
				id, was.DueDate(), was.DueMileage(), now.DueDate(), now.DueMileage())
		}
	}
}

// The exported entry point exists and is safe to call against a schema with no
// matching rows — the shape every boot after the first one sees.
func TestBackfill_noMatchingRows(t *testing.T) {
	db := newCompletionDB(t)
	if err := Backfill(db); err != nil {
		t.Fatalf("Backfill on an empty table: %v", err)
	}
}
```

- [x] **Step 2: Run the test to verify it fails**

Run: `go test ./apps/fleet-service/internal/maintenanceschedule/... -run TestBackfill -v`
Expected: FAIL to compile — `backfill` and `Backfill` undefined.

- [x] **Step 3: Write the backfill**

Create `apps/fleet-service/internal/maintenanceschedule/backfill.go`:

```go
package maintenanceschedule

import (
	"time"

	"gorm.io/gorm"
)

// backfillRow is one candidate schedule joined to its vehicle's odometer.
type backfillRow struct {
	ID             string
	RecurrenceType string
	IntervalMonths int
	IntervalMiles  int
	CreatedAt      time.Time
	CurrentMileage int
}

// Backfill assigns a first-due anchor to schedules that have never been
// completed, repairing rows created before task-030 that would otherwise read
// as permanently overdue (their next-due derives from a zero completion date,
// putting it in year 1) and that would now also fail validation on any PATCH.
//
// Idempotent: it only touches rows where BOTH new columns are still at their
// defaults, and every row it touches ends up with at least one of them set, so
// a second run selects nothing. Wired at startup on the pattern of
// maintenancecategory.Seed and fatal on error for the same reason — a service
// whose schedules are all falsely overdue is worse than one that refuses to
// start.
func Backfill(db *gorm.DB) error { return backfill(db, time.Now().UTC()) }

func backfill(db *gorm.DB, now time.Time) error {
	var rows []backfillRow
	if err := db.Table("fleet.maintenance_schedules AS s").
		Select("s.id, s.recurrence_type, s.interval_months, s.interval_miles, s.created_at, v.current_mileage").
		Joins("JOIN fleet.vehicles v ON v.id = s.vehicle_id").
		Where("s.one_time = ?", false).
		Where("s.due_date IS NULL AND s.due_mileage = 0").
		Where("s.last_completed_date IS NULL AND s.last_completed_mileage = 0").
		Where("s.deleted_at IS NULL").
		Scan(&rows).Error; err != nil {
		return err
	}

	for _, r := range rows {
		// The anchor is computed through the same Schedule/NextDue path every
		// other caller uses, so an anchored row and a completed row land on
		// identical date math.
		s := Schedule{
			RecurrenceType: r.RecurrenceType,
			IntervalMonths: r.IntervalMonths,
			IntervalMiles:  r.IntervalMiles,
		}
		if r.RecurrenceType == "time" || r.RecurrenceType == "hybrid" {
			s.DueDate = r.CreatedAt.AddDate(0, r.IntervalMonths, 0)
		}
		if r.RecurrenceType == "mileage" || r.RecurrenceType == "hybrid" {
			s.DueMileage = r.CurrentMileage + r.IntervalMiles
		}
		if s.DueDate.IsZero() && s.DueMileage == 0 {
			// An unrecognized recurrence type: nothing to anchor, and writing
			// nothing keeps the row selectable if the type is later corrected.
			continue
		}

		nd, nm := NextDue(s)
		state := DueState(s, now, r.CurrentMileage, DefaultThresholds)
		updates := map[string]any{
			"due_mileage":      s.DueMileage,
			"next_due_mileage": nm,
			"status":           state,
			"severity":         Severity(state),
		}
		if s.DueDate.IsZero() {
			updates["due_date"] = nil
		} else {
			updates["due_date"] = s.DueDate
		}
		if nd.IsZero() {
			updates["next_due_date"] = nil
		} else {
			updates["next_due_date"] = nd
		}
		if err := db.Table("fleet.maintenance_schedules").
			Where("id = ?", r.ID).Updates(updates).Error; err != nil {
			return err
		}
	}
	return nil
}
```

- [x] **Step 4: Run the test to verify it passes**

Run: `go test ./apps/fleet-service/internal/maintenanceschedule/... -v`
Expected: PASS.

- [x] **Step 5: Wire it into startup**

In `apps/fleet-service/cmd/main.go`, immediately after the `maintenancecategory.Seed` block, insert:

```go
	// Assign a first-due anchor to maintenance schedules created before
	// task-030 (idempotent; FR-ANCHOR-2). Without it those rows read as
	// permanently overdue and reject every PATCH.
	if err := maintenanceschedule.Backfill(db); err != nil {
		log.WithError(err).Fatal("backfill maintenance schedule due points")
	}
```

- [x] **Step 6: Verify the service still builds**

Run: `make vet && make build`
Expected: both succeed.

- [x] **Step 7: Commit**

```bash
git add apps/fleet-service/internal/maintenanceschedule/backfill.go \
        apps/fleet-service/internal/maintenanceschedule/backfill_test.go \
        apps/fleet-service/cmd/main.go
git commit -m "feat(fleet): backfill a first-due anchor for uncompleted legacy schedules"
```

---

## Task 8: Transport — create, PATCH, complete

**Files:**
- Modify: `apps/fleet-service/internal/maintenanceschedule/resource.go`
- Modify: `apps/fleet-service/internal/maintenanceschedule/rest.go`

**Interfaces:**
- Consumes: `server.Nullable[string]` (Task 1), the builder setters (Task 4), `Model.OneTime()/DueDate()/DueMileage()` (Task 2).
- Produces: `Attributes` gains `OneTime bool \`json:"oneTime"\``, `DueDate string \`json:"dueDate,omitempty"\``, `DueMileage int \`json:"dueMileage,omitempty"\``. Task 10's TypeScript read model mirrors these keys exactly.

- [x] **Step 1: Extend the resource representation**

In `apps/fleet-service/internal/maintenanceschedule/rest.go`, add three fields to `Attributes` after `IntervalMiles`:

```go
	IntervalMiles        int    `json:"intervalMiles,omitempty"`
	// oneTime carries no omitempty on purpose: with it, a false would be
	// indistinguishable from a server that predates the field, and the
	// frontend keys the whole one-time treatment off this value.
	OneTime    bool   `json:"oneTime"`
	DueDate    string `json:"dueDate,omitempty"`
	DueMileage int    `json:"dueMileage,omitempty"`
```

In `Transform`, add to the `Attributes` literal:

```go
		OneTime:              m.OneTime(),
		DueMileage:           m.DueMileage(),
```

and, beside the existing date formatting:

```go
	if !m.DueDate().IsZero() {
		a.DueDate = m.DueDate().Format(timeFormat)
	}
```

`nextDueDate` / `nextDueMileage` continue to carry the *effective* due point, which for a one-time schedule is now the stored anchor — that is what lets the frontend's `deriveNextService`, `rankSchedule` and the vehicle status banner go untouched. Do not change them.

- [x] **Step 2: Extend the create route**

In `apps/fleet-service/internal/maintenanceschedule/resource.go`, replace the `POST /vehicles/{id}/maintenance-schedules` attrs struct and builder call. The attrs struct becomes:

```go
		r.Post("/vehicles/{id}/maintenance-schedules", server.RegisterInputHandler(func(w http.ResponseWriter, req *http.Request, attrs struct {
			CategoryID     string `json:"categoryId"`
			RecurrenceType string `json:"recurrenceType"`
			OneTime        bool   `json:"oneTime"`
			IntervalMonths int    `json:"intervalMonths"`
			IntervalMiles  int    `json:"intervalMiles"`
			DueDate        string `json:"dueDate"`
			DueMileage     int    `json:"dueMileage"`
		},
		) {
```

and, between the `authz.RequireWrite` check and the `NewBuilder()` call, insert the date parsing:

```go
			// An empty dueDate is passed through as the zero time and left to
			// validate; an unparseable one is a 400 here, mirroring the
			// complete route's date handling.
			var dueDate time.Time
			if attrs.DueDate != "" {
				parsed, perr := time.Parse(time.RFC3339, attrs.DueDate)
				if perr != nil {
					server.WriteError(w, server.ErrValidation)
					return
				}
				dueDate = parsed
			}
```

Then extend the builder chain:

```go
			m, err := NewBuilder().
				SetVehicleID(vehicleID).
				SetCategoryID(attrs.CategoryID).
				SetRecurrenceType(attrs.RecurrenceType).
				SetOneTime(attrs.OneTime).
				SetIntervalMonths(attrs.IntervalMonths).
				SetIntervalMiles(attrs.IntervalMiles).
				SetDuePoint(dueDate, attrs.DueMileage).
				SetCurrentMileage(v.CurrentMileage()).
				Build()
```

- [x] **Step 3: Extend the PATCH route**

Replace the PATCH attrs struct with:

```go
		r.Patch("/maintenance-schedules/{id}", server.RegisterInputHandler(func(w http.ResponseWriter, req *http.Request, attrs struct {
			RecurrenceType *string                 `json:"recurrenceType"`
			OneTime        *bool                   `json:"oneTime"`
			IntervalMonths *int                    `json:"intervalMonths"`
			IntervalMiles  *int                    `json:"intervalMiles"`
			// dueDate needs Nullable because its cleared state is a NULL
			// column, which a *string cannot tell apart from an absent key.
			// dueMileage stays a *int: 0 is already its "unset" encoding at the
			// column, the model and NextDue, so {"dueMileage": 0} is
			// unambiguous without new machinery (FR-UPD-3).
			DueDate    server.Nullable[string] `json:"dueDate"`
			DueMileage *int                    `json:"dueMileage"`
			Active     *bool                   `json:"active"`
		},
		) {
```

After the `authz.RequireWrite` check and before `proc.Update`, parse the value form:

```go
			var parsedDueDate time.Time
			if attrs.DueDate.Present && attrs.DueDate.Valid {
				parsed, perr := time.Parse(time.RFC3339, attrs.DueDate.Value)
				if perr != nil {
					server.WriteError(w, server.ErrValidation)
					return
				}
				parsedDueDate = parsed
			}
```

Extend the mutation closure — everything before `if attrs.Active != nil` stays as it is, with these lines added after `m = m.WithRecurrence(rt, months, miles)`:

```go
				if attrs.OneTime != nil {
					m = m.WithOneTime(*attrs.OneTime)
				}
				dueDate := m.DueDate()
				dueMileage := m.DueMileage()
				if attrs.DueDate.Present {
					// Present-but-not-valid is an explicit null: clear it.
					dueDate = time.Time{}
					if attrs.DueDate.Valid {
						dueDate = parsedDueDate
					}
				}
				if attrs.DueMileage != nil {
					dueMileage = *attrs.DueMileage
				}
				m = m.WithDuePoint(dueDate, dueMileage)
```

- [x] **Step 4: Add the inactive pre-check to the complete route**

In the `POST /maintenance-schedules/{id}/complete` handler, immediately after the `authz.RequireWrite` check and before the date parsing, insert:

```go
			// FR-COMPLETE-4. AdvanceTx re-checks this inside the transaction,
			// which is the authoritative check; this one exists so a caller
			// gets a 400 without a transaction ever being opened. Placed after
			// the authz checks so a non-member still gets 403/404, not 400.
			if !sched.Active() {
				server.WriteError(w, server.ErrValidation)
				return
			}
```

- [x] **Step 5: Verify it builds and the package still passes**

Run: `make vet && go test ./apps/fleet-service/... -v`
Expected: PASS.

- [x] **Step 6: Commit**

```bash
git add apps/fleet-service/internal/maintenanceschedule/resource.go \
        apps/fleet-service/internal/maintenanceschedule/rest.go
git commit -m "feat(fleet): accept oneTime/dueDate/dueMileage on schedule create and PATCH"
```

---

## Task 9: Transport tests — emission rules and feed-token stability

The PRD's open question about runaway reminders resolves against the source, not assumption. `reminder.DueCycleToken` builds `"<next_due_date>|<next_due_mileage>"` from the feed (`apps/notification-service/internal/reminder/job.go:107`) and `consumer.OverdueDedupeKey` composes `"<user>:overdue:<schedule>:<cycle>"` (`consume.go:164`), which `notification.Processor.Generate` dedupes on. For a one-time schedule the anchor never moves, so the token is constant for the row's whole life and every user receives exactly one overdue notification however long it stays overdue. **No notification-service change is needed** — but that guarantee must be pinned by a fleet-service test, so a future change to `NextDue` cannot quietly break the dedupe.

**Files:**
- Create: `apps/fleet-service/internal/maintenanceschedule/rest_test.go`

**Interfaces:**
- Consumes: `Transform`, `TransformInternalDue`, `DueCycleToken` (unchanged), `Administrator.Recompute` (unchanged).
- Produces: nothing.

- [x] **Step 1: Write the failing test**

Create `apps/fleet-service/internal/maintenanceschedule/rest_test.go`:

```go
package maintenanceschedule

import (
	"encoding/json"
	"testing"
	"time"
)

// attrsJSON renders a model's attributes to a generic map so the test can
// assert on KEY PRESENCE, which is the actual contract with the frontend —
// `oneTime` must always be emitted, the due fields only when set.
func attrsJSON(t *testing.T, m Model) map[string]any {
	t.Helper()
	b, err := json.Marshal(Transform(m).Attributes)
	if err != nil {
		t.Fatalf("marshal attributes: %v", err)
	}
	var out map[string]any
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("unmarshal attributes: %v", err)
	}
	return out
}

func TestTransform_alwaysEmitsOneTime(t *testing.T) {
	recurring := attrsJSON(t, Model{recurrenceType: "time", intervalMonths: 12})
	v, ok := recurring["oneTime"]
	if !ok {
		t.Fatal("oneTime must be emitted even when false: an omitted key is " +
			"indistinguishable from a server predating the field")
	}
	if v != false {
		t.Fatalf("oneTime = %v want false", v)
	}

	oneTime := attrsJSON(t, Model{recurrenceType: "mileage", oneTime: true, dueMileage: 60000})
	if oneTime["oneTime"] != true {
		t.Fatalf("oneTime = %v want true", oneTime["oneTime"])
	}
}

func TestTransform_emitsDueFieldsOnlyWhenSet(t *testing.T) {
	bare := attrsJSON(t, Model{recurrenceType: "time", intervalMonths: 12})
	if _, ok := bare["dueDate"]; ok {
		t.Error("dueDate must be omitted when unset")
	}
	if _, ok := bare["dueMileage"]; ok {
		t.Error("dueMileage must be omitted when unset")
	}

	due := time.Date(2026, 11, 30, 0, 0, 0, 0, time.UTC)
	anchored := attrsJSON(t, Model{
		recurrenceType: "hybrid", oneTime: true, dueDate: due, dueMileage: 60000,
	})
	if got := anchored["dueDate"]; got != due.Format(timeFormat) {
		t.Errorf("dueDate = %v want %s", got, due.Format(timeFormat))
	}
	if got := anchored["dueMileage"]; got != float64(60000) {
		t.Errorf("dueMileage = %v want 60000", got)
	}
}

// A one-time schedule's due point never moves, so its feed row — and therefore
// the DueCycleToken notification-service derives from it — must be identical
// across repeated recomputes. If this ever drifts, every recompute mints a new
// dedupe key and the user gets a fresh overdue notification every hour.
func TestTransformInternalDue_oneTimeTokenIsStableAcrossRecomputes(t *testing.T) {
	db := newCompletionDB(t)
	vehicleID := seedVehicle(t, db, 59000)

	due := time.Date(2026, 11, 30, 0, 0, 0, 0, time.UTC)
	s, err := NewBuilder().SetVehicleID(vehicleID).SetCategoryID("c1").
		SetRecurrenceType("hybrid").SetOneTime(true).
		SetDuePoint(due, 60000).SetCurrentMileage(59000).Build()
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	created, err := NewAdministrator(db).Insert(s)
	if err != nil {
		t.Fatalf("insert: %v", err)
	}

	admin := NewAdministrator(db)
	feedRow := func(now time.Time, currentMileage int) InternalDueSchedule {
		t.Helper()
		if err := admin.Recompute(created.ID(), currentMileage, now); err != nil {
			t.Fatalf("recompute: %v", err)
		}
		m, err := NewProvider(db).GetByID(created.ID())
		if err != nil {
			t.Fatalf("reload: %v", err)
		}
		rows := TransformInternalDue([]DueEntry{{Schedule: m, FleetID: "f1", State: "overdue"}})
		if len(rows) != 1 {
			t.Fatalf("want 1 feed row, got %d", len(rows))
		}
		return rows[0]
	}

	first := feedRow(time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC), 60500)
	// A later sweep, a moved odometer, a deeper overdue — none of it may move
	// the due point.
	second := feedRow(time.Date(2027, 6, 1, 0, 0, 0, 0, time.UTC), 71000)

	if first.NextDueDate != second.NextDueDate || first.NextDueMileage != second.NextDueMileage {
		t.Fatalf("feed due point drifted: %+v -> %+v", first, second)
	}
	if first.NextDueDate != due.Format(timeFormat) || first.NextDueMileage != 60000 {
		t.Fatalf("feed row does not carry the stored anchor: %+v", first)
	}

	m, err := NewProvider(db).GetByID(created.ID())
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if got, want := DueCycleToken(m), due.Format(timeFormat)+"|60000"; got != want {
		t.Fatalf("DueCycleToken = %q want %q", got, want)
	}
}
```

- [x] **Step 2: Run the test to verify it passes**

Run: `go test ./apps/fleet-service/internal/maintenanceschedule/... -run 'TestTransform' -v`
Expected: PASS — these tests pin behaviour Tasks 3 and 8 already delivered. If any fail, the defect is in Task 3 or Task 8, not here.

- [x] **Step 3: Confirm notification-service is genuinely untouched**

Run: `git status --short apps/notification-service`
Expected: empty output. The contract is unchanged; the work was to *verify* that, not to touch it.

- [x] **Step 4: Run the whole Go suite**

Run: `make vet && make test`
Expected: PASS.

- [x] **Step 5: Commit**

```bash
git add apps/fleet-service/internal/maintenanceschedule/rest_test.go
git commit -m "test(fleet): pin schedule attribute emission and one-time feed-token stability"
```

---

## Task 10: Frontend types and the Zod schema

The schema stays **one object with an extended `superRefine`**, gaining a `kind: 'recurring' | 'oneTime'` discriminant, rather than becoming a discriminated union. A union would type more precisely, but `zodResolver` over a union fights react-hook-form's single `defaultValues` object and its shared `categoryId` / `recurrenceType` fields, and the form would have to remount on kind change to keep the resolver honest.

**Files:**
- Modify: `apps/web/src/types/models/maintenanceSchedule.ts`
- Modify: `apps/web/src/lib/schemas/maintenanceSchedule.ts`
- Test: `apps/web/src/lib/schemas/maintenanceSchedule.test.ts` (rewritten — every existing case now needs a `kind` and a due point)

**Interfaces:**
- Consumes: the JSON keys Task 8 emits and accepts.
- Produces:
  - `MaintenanceScheduleAttributes` gains `oneTime: boolean`, `dueDate?: string`, `dueMileage?: number`.
  - `CreateMaintenanceScheduleAttributes` gains `oneTime?: boolean`, `dueDate?: string`, `dueMileage?: number`.
  - `UpdateMaintenanceScheduleAttributes` gains `oneTime?: boolean`, `dueDate?: string | null`, `dueMileage?: number`.
  - `MaintenanceScheduleFormInput` gains `kind: 'recurring' | 'oneTime'`, `dueDate?: string`, `dueMileage?: number`.
  - `convertToRecurrenceSchema` / `ConvertToRecurrenceFormInput` — Task 15 consumes these.

- [x] **Step 1: Write the failing test**

Replace the entire contents of `apps/web/src/lib/schemas/maintenanceSchedule.test.ts` with:

```ts
import { describe, it, expect } from 'vitest';
import {
  maintenanceScheduleSchema,
  convertToRecurrenceSchema,
} from './maintenanceSchedule';

const DUE_DATE = '2026-11-30';
const DUE_MILEAGE = 60000;

/**
 * The four-rule matrix from design §7:
 *
 * | kind      | covers time                                | covers mileage                                 |
 * | recurring | intervalMonths > 0 AND dueDate required    | intervalMiles > 0 AND dueMileage > 0 required  |
 * | oneTime   | dueDate required, intervalMonths forbidden | dueMileage > 0 required, intervalMiles forbidden |
 */
const recurringTime = {
  categoryId: 'cat-1',
  kind: 'recurring' as const,
  recurrenceType: 'time' as const,
  intervalMonths: 6,
  dueDate: DUE_DATE,
};

const oneTimeMileage = {
  categoryId: 'cat-1',
  kind: 'oneTime' as const,
  recurrenceType: 'mileage' as const,
  dueMileage: DUE_MILEAGE,
};

/** The first issue path, so a test can assert WHICH field was blamed. */
function firstIssuePath(input: unknown): string | undefined {
  const result = maintenanceScheduleSchema.safeParse(input);
  if (result.success) return undefined;
  return result.error.issues[0]?.path.join('.');
}

describe('maintenanceScheduleSchema — recurring', () => {
  it('accepts a complete time schedule', () => {
    expect(maintenanceScheduleSchema.safeParse(recurringTime).success).toBe(true);
  });

  it('requires intervalMonths for a time schedule', () => {
    const { intervalMonths, ...rest } = recurringTime;
    void intervalMonths;
    expect(firstIssuePath(rest)).toBe('intervalMonths');
  });

  it('requires a first-due date for a time schedule', () => {
    const { dueDate, ...rest } = recurringTime;
    void dueDate;
    expect(firstIssuePath(rest)).toBe('dueDate');
  });

  it('rejects a zero intervalMonths', () => {
    expect(maintenanceScheduleSchema.safeParse({ ...recurringTime, intervalMonths: 0 }).success).toBe(false);
  });

  it('accepts a complete mileage schedule', () => {
    const result = maintenanceScheduleSchema.safeParse({
      categoryId: 'cat-1',
      kind: 'recurring',
      recurrenceType: 'mileage',
      intervalMiles: 5000,
      dueMileage: DUE_MILEAGE,
    });
    expect(result.success).toBe(true);
  });

  it('requires a first-due odometer for a mileage schedule', () => {
    expect(
      firstIssuePath({
        categoryId: 'cat-1',
        kind: 'recurring',
        recurrenceType: 'mileage',
        intervalMiles: 5000,
      }),
    ).toBe('dueMileage');
  });

  it('accepts a complete hybrid schedule', () => {
    const result = maintenanceScheduleSchema.safeParse({
      categoryId: 'cat-1',
      kind: 'recurring',
      recurrenceType: 'hybrid',
      intervalMonths: 6,
      intervalMiles: 5000,
      dueDate: DUE_DATE,
      dueMileage: DUE_MILEAGE,
    });
    expect(result.success).toBe(true);
  });

  it('rejects a hybrid schedule missing the mileage axis entirely', () => {
    const result = maintenanceScheduleSchema.safeParse({
      categoryId: 'cat-1',
      kind: 'recurring',
      recurrenceType: 'hybrid',
      intervalMonths: 6,
      dueDate: DUE_DATE,
    });
    expect(result.success).toBe(false);
  });

  it('does not require the mileage axis on a time schedule', () => {
    const result = maintenanceScheduleSchema.safeParse({
      ...recurringTime,
      intervalMiles: undefined,
      dueMileage: undefined,
    });
    expect(result.success).toBe(true);
  });
});

describe('maintenanceScheduleSchema — one-time', () => {
  it('accepts a mileage one-off with no interval', () => {
    expect(maintenanceScheduleSchema.safeParse(oneTimeMileage).success).toBe(true);
  });

  it('accepts a date one-off with no interval', () => {
    const result = maintenanceScheduleSchema.safeParse({
      categoryId: 'cat-1',
      kind: 'oneTime',
      recurrenceType: 'time',
      dueDate: DUE_DATE,
    });
    expect(result.success).toBe(true);
  });

  it('requires a due date on the time axis', () => {
    expect(
      firstIssuePath({ categoryId: 'cat-1', kind: 'oneTime', recurrenceType: 'time' }),
    ).toBe('dueDate');
  });

  it('requires a due odometer on the mileage axis', () => {
    expect(
      firstIssuePath({ categoryId: 'cat-1', kind: 'oneTime', recurrenceType: 'mileage' }),
    ).toBe('dueMileage');
  });

  it('rejects an intervalMonths on a one-time schedule', () => {
    const result = maintenanceScheduleSchema.safeParse({
      categoryId: 'cat-1',
      kind: 'oneTime',
      recurrenceType: 'time',
      dueDate: DUE_DATE,
      intervalMonths: 6,
    });
    expect(result.success).toBe(false);
  });

  it('rejects an intervalMiles on a one-time schedule', () => {
    const result = maintenanceScheduleSchema.safeParse({
      ...oneTimeMileage,
      intervalMiles: 5000,
    });
    expect(result.success).toBe(false);
  });

  it('accepts a hybrid one-off carrying both due points', () => {
    const result = maintenanceScheduleSchema.safeParse({
      categoryId: 'cat-1',
      kind: 'oneTime',
      recurrenceType: 'hybrid',
      dueDate: DUE_DATE,
      dueMileage: DUE_MILEAGE,
    });
    expect(result.success).toBe(true);
  });
});

describe('maintenanceScheduleSchema — shared fields', () => {
  it('requires categoryId', () => {
    const { categoryId, ...rest } = recurringTime;
    void categoryId;
    expect(maintenanceScheduleSchema.safeParse(rest).success).toBe(false);
  });

  it('requires kind', () => {
    const { kind, ...rest } = recurringTime;
    void kind;
    expect(maintenanceScheduleSchema.safeParse(rest).success).toBe(false);
  });

  it('rejects an unknown recurrenceType', () => {
    expect(
      maintenanceScheduleSchema.safeParse({ ...recurringTime, recurrenceType: 'once' }).success,
    ).toBe(false);
  });
});

describe('convertToRecurrenceSchema', () => {
  it('accepts a time recurrence with an interval', () => {
    expect(convertToRecurrenceSchema.safeParse({ recurrenceType: 'time', intervalMonths: 12 }).success).toBe(true);
  });

  it('requires intervalMonths for a time recurrence', () => {
    expect(convertToRecurrenceSchema.safeParse({ recurrenceType: 'time' }).success).toBe(false);
  });

  it('requires intervalMiles for a mileage recurrence', () => {
    expect(convertToRecurrenceSchema.safeParse({ recurrenceType: 'mileage' }).success).toBe(false);
  });

  it('requires both intervals for a hybrid recurrence', () => {
    expect(
      convertToRecurrenceSchema.safeParse({ recurrenceType: 'hybrid', intervalMonths: 12 }).success,
    ).toBe(false);
    expect(
      convertToRecurrenceSchema.safeParse({
        recurrenceType: 'hybrid',
        intervalMonths: 12,
        intervalMiles: 5000,
      }).success,
    ).toBe(true);
  });

  // The conversion dialog carries no category or due-point fields: the category
  // is fixed and read-only, and the anchor is being cleared, not set.
  it('does not require a categoryId or a due point', () => {
    expect(convertToRecurrenceSchema.safeParse({ recurrenceType: 'mileage', intervalMiles: 5000 }).success).toBe(true);
  });
});
```

- [x] **Step 2: Run the test to verify it fails**

Run: `export NVM_DIR="$HOME/.nvm" && . "$NVM_DIR/nvm.sh" && nvm use 22 && npx vitest run src/lib/schemas/maintenanceSchedule.test.ts --root apps/web`
Expected: FAIL — `convertToRecurrenceSchema` is not exported, and the `kind`/due-point cases do not behave as asserted.

- [x] **Step 3: Extend the types**

In `apps/web/src/types/models/maintenanceSchedule.ts`, add to `MaintenanceScheduleAttributes` after `intervalMiles`:

```ts
  /** True when the schedule is due once and never repeats. Always present. */
  oneTime: boolean;
  /** RFC3339. The permanent due point of a one-time schedule, or a recurring schedule's first-due anchor. */
  dueDate?: string;
  dueMileage?: number;
```

Replace `CreateMaintenanceScheduleAttributes` and `UpdateMaintenanceScheduleAttributes` with:

```ts
/** POST /api/fleet/vehicles/{id}/maintenance-schedules body attributes */
export interface CreateMaintenanceScheduleAttributes {
  categoryId: string;
  recurrenceType: string;
  oneTime?: boolean;
  intervalMonths?: number;
  intervalMiles?: number;
  /** RFC3339 */
  dueDate?: string;
  dueMileage?: number;
}

/** PATCH /api/fleet/maintenance-schedules/{id} body attributes */
export interface UpdateMaintenanceScheduleAttributes {
  recurrenceType?: string;
  oneTime?: boolean;
  intervalMonths?: number;
  intervalMiles?: number;
  /**
   * RFC3339, or an explicit `null` to clear the stored anchor. The server
   * distinguishes an absent key from a null one (server.Nullable), so the two
   * are NOT interchangeable — omitting the key leaves the anchor in place.
   */
  dueDate?: string | null;
  /** 0 clears the stored odometer anchor. */
  dueMileage?: number;
  active?: boolean;
}
```

- [x] **Step 4: Extend the schema**

Replace `maintenanceScheduleSchema` in `apps/web/src/lib/schemas/maintenanceSchedule.ts` and add the conversion schema (leave `completeScheduleSchema` alone):

```ts
import { z } from 'zod';

const recurrenceTypes = ['time', 'mileage', 'hybrid'] as const;
const scheduleKinds = ['recurring', 'oneTime'] as const;

const intervalMonths = z
  .number({ error: 'Interval months must be a number' })
  .int('Interval months must be a whole number')
  .positive('Interval months must be greater than 0')
  .optional();

const intervalMiles = z
  .number({ error: 'Interval miles must be a number' })
  .int('Interval miles must be a whole number')
  .positive('Interval miles must be greater than 0')
  .optional();

/**
 * Maintenance schedule form schema.
 *
 * Two independent axes, because `recurrenceType` says which axes the schedule
 * is JUDGED ON, not how often it repeats:
 *  - `kind`: 'recurring' | 'oneTime'
 *  - `recurrenceType`: 'time' | 'mileage' | 'hybrid'
 *
 * | kind      | covers time                                  | covers mileage                                   |
 * | recurring | intervalMonths > 0 AND dueDate required      | intervalMiles > 0 AND dueMileage > 0 required    |
 * | oneTime   | dueDate required, intervalMonths forbidden   | dueMileage > 0 required, intervalMiles forbidden |
 *
 * One object with a superRefine rather than a discriminated union: zodResolver
 * over a union fights react-hook-form's single defaultValues object and the
 * shared categoryId / recurrenceType fields, and the form would have to remount
 * on every kind change to keep the resolver honest.
 */
export const maintenanceScheduleSchema = z
  .object({
    categoryId: z.string().min(1, 'Category is required'),
    kind: z.enum(scheduleKinds, { error: 'Schedule kind is required' }),
    recurrenceType: z.enum(recurrenceTypes, { error: 'Recurrence type is required' }),
    intervalMonths,
    intervalMiles,
    /** YYYY-MM-DD from the date input; converted to RFC3339 at the call site. */
    dueDate: z.string().optional(),
    dueMileage: z
      .number({ error: 'Due odometer must be a number' })
      .int('Due odometer must be a whole number')
      .positive('Due odometer must be greater than 0')
      .optional(),
  })
  .superRefine((data, ctx) => {
    const coversTime = data.recurrenceType === 'time' || data.recurrenceType === 'hybrid';
    const coversMileage = data.recurrenceType === 'mileage' || data.recurrenceType === 'hybrid';
    const recurring = data.kind === 'recurring';

    if (coversTime) {
      if (recurring && !data.intervalMonths) {
        ctx.addIssue({
          code: z.ZodIssueCode.custom,
          path: ['intervalMonths'],
          message: 'Interval months is required for this recurrence type',
        });
      }
      if (!data.dueDate) {
        ctx.addIssue({
          code: z.ZodIssueCode.custom,
          path: ['dueDate'],
          message: recurring ? 'First due date is required' : 'Due date is required',
        });
      }
    }

    if (coversMileage) {
      if (recurring && !data.intervalMiles) {
        ctx.addIssue({
          code: z.ZodIssueCode.custom,
          path: ['intervalMiles'],
          message: 'Interval miles is required for this recurrence type',
        });
      }
      if (!data.dueMileage) {
        ctx.addIssue({
          code: z.ZodIssueCode.custom,
          path: ['dueMileage'],
          message: recurring ? 'First due odometer is required' : 'Due odometer is required',
        });
      }
    }

    if (!recurring) {
      if (data.intervalMonths) {
        ctx.addIssue({
          code: z.ZodIssueCode.custom,
          path: ['intervalMonths'],
          message: 'A one-time schedule cannot repeat',
        });
      }
      if (data.intervalMiles) {
        ctx.addIssue({
          code: z.ZodIssueCode.custom,
          path: ['intervalMiles'],
          message: 'A one-time schedule cannot repeat',
        });
      }
    }
  });

export type MaintenanceScheduleFormInput = z.infer<typeof maintenanceScheduleSchema>;

/**
 * Converting a completed one-time schedule into a recurring one. The category
 * is fixed and read-only, and the recurrence runs from the completion point the
 * completion flow already recorded — so there is no category field and no due
 * point to collect, only the recurrence type and its intervals.
 */
export const convertToRecurrenceSchema = z
  .object({
    recurrenceType: z.enum(recurrenceTypes, { error: 'Recurrence type is required' }),
    intervalMonths,
    intervalMiles,
  })
  .superRefine((data, ctx) => {
    const coversTime = data.recurrenceType === 'time' || data.recurrenceType === 'hybrid';
    const coversMileage = data.recurrenceType === 'mileage' || data.recurrenceType === 'hybrid';

    if (coversTime && !data.intervalMonths) {
      ctx.addIssue({
        code: z.ZodIssueCode.custom,
        path: ['intervalMonths'],
        message: 'Interval months is required for this recurrence type',
      });
    }
    if (coversMileage && !data.intervalMiles) {
      ctx.addIssue({
        code: z.ZodIssueCode.custom,
        path: ['intervalMiles'],
        message: 'Interval miles is required for this recurrence type',
      });
    }
  });

export type ConvertToRecurrenceFormInput = z.infer<typeof convertToRecurrenceSchema>;
```

- [x] **Step 5: Run the test to verify it passes**

Run: `export NVM_DIR="$HOME/.nvm" && . "$NVM_DIR/nvm.sh" && nvm use 22 && npx vitest run src/lib/schemas/maintenanceSchedule.test.ts --root apps/web`
Expected: PASS.

Note: `MaintenanceScheduleForm.tsx` and `AddScheduleDialog.tsx` will not type-check until Task 11 — `kind` is now a required field on `MaintenanceScheduleFormInput`. That is expected; do not patch them here.

- [x] **Step 6: Commit**

```bash
git add apps/web/src/types/models/maintenanceSchedule.ts \
        apps/web/src/lib/schemas/maintenanceSchedule.ts \
        apps/web/src/lib/schemas/maintenanceSchedule.test.ts
git commit -m "feat(web): add the one-time kind and due-point fields to the schedule schema"
```

---

## Task 11: The schedule form — kind selector, conditional fields, anchor defaulting

FR-UI-1 and FR-ANCHOR-4. The recurrence-type select stays visible for both kinds and keeps its current labels — it answers "judged by time, mileage, or both", which is the same question either way. Below it the field set swaps, keyed off the same `showMonths` / `showMiles` axis booleans the component already derives.

For a recurring schedule the first-due fields appear *alongside* the intervals, defaulted to `today + intervalMonths` and `currentMileage + intervalMiles` and recomputed as the user edits — but **only while the user has not touched the anchor field**, tracked with `form.formState.dirtyFields`. Without that condition, editing an interval would silently stomp a deliberately chosen anchor.

**Files:**
- Modify: `apps/web/src/components/features/vehicles/maintenance/MaintenanceScheduleForm.tsx`
- Modify: `apps/web/src/components/features/vehicles/dialogs/AddScheduleDialog.tsx`
- Modify: `apps/web/src/pages/VehicleDetailPage.tsx` (pass `currentMileage`)
- Test: `apps/web/src/components/features/vehicles/maintenance/MaintenanceScheduleForm.test.tsx` (create)

**Interfaces:**
- Consumes: `maintenanceScheduleSchema`, `MaintenanceScheduleFormInput` (Task 10).
- Produces: `MaintenanceScheduleForm` gains a `currentMileage?: number` prop; `AddScheduleDialog` gains a `currentMileage?: number` prop.

- [x] **Step 1: Write the failing test**

Create `apps/web/src/components/features/vehicles/maintenance/MaintenanceScheduleForm.test.tsx`:

```tsx
import { describe, it, expect, vi, beforeAll } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { MaintenanceScheduleForm } from './MaintenanceScheduleForm';
import type { MaintenanceCategory } from '../../../../types/models/maintenanceCategory';

// cmdk scrolls the selected item into view; jsdom does not implement it. Same
// stub MaintenanceRecordForm.test.tsx installs.
beforeAll(() => {
  Element.prototype.scrollIntoView = vi.fn();
});

const categories: MaintenanceCategory[] = [
  {
    id: 'c1',
    type: 'maintenanceCategories',
    attributes: { name: 'Oil Change', systemDefined: true, kind: 'maintenance' },
  },
];

function renderForm(props: Partial<React.ComponentProps<typeof MaintenanceScheduleForm>> = {}) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const onSubmit = props.onSubmit ?? vi.fn();
  render(
    <QueryClientProvider client={client}>
      <MaintenanceScheduleForm categories={categories} {...props} onSubmit={onSubmit} />
    </QueryClientProvider>,
  );
  return { onSubmit };
}

/** Pick an option from a shadcn/ui Select by its trigger's accessible name. */
async function selectOption(triggerName: RegExp, optionName: RegExp) {
  const user = userEvent.setup();
  await user.click(screen.getByRole('combobox', { name: triggerName }));
  await user.click(await screen.findByRole('option', { name: optionName }));
}

describe('MaintenanceScheduleForm — kind', () => {
  it('defaults to a repeating schedule and shows the interval field', () => {
    renderForm();
    expect(screen.getByLabelText(/every \(months\)/i)).toBeInTheDocument();
  });

  it('replaces the interval field with a due date when one-time is chosen', async () => {
    renderForm();
    await selectOption(/schedule type/i, /one-time/i);

    await waitFor(() => {
      expect(screen.queryByLabelText(/every \(months\)/i)).not.toBeInTheDocument();
    });
    expect(screen.getByLabelText(/due date/i)).toBeInTheDocument();
  });

  it('shows the due odometer instead of the mileage interval for a one-time mileage schedule', async () => {
    renderForm();
    await selectOption(/schedule type/i, /one-time/i);
    await selectOption(/recurrence type/i, /mileage-based/i);

    await waitFor(() => {
      expect(screen.getByLabelText(/due odometer/i)).toBeInTheDocument();
    });
    expect(screen.queryByLabelText(/every \(miles\)/i)).not.toBeInTheDocument();
  });

  it('keeps both the interval and the first-due field for a repeating schedule', () => {
    renderForm();
    expect(screen.getByLabelText(/every \(months\)/i)).toBeInTheDocument();
    expect(screen.getByLabelText(/first due/i)).toBeInTheDocument();
  });
});

describe('MaintenanceScheduleForm — first-due defaulting', () => {
  it('defaults the first-due odometer to currentMileage + intervalMiles', async () => {
    const user = userEvent.setup();
    renderForm({ currentMileage: 40000 });
    await selectOption(/recurrence type/i, /mileage-based/i);

    const interval = await screen.findByLabelText(/every \(miles\)/i);
    await user.clear(interval);
    await user.type(interval, '5000');

    await waitFor(() => {
      expect(screen.getByLabelText(/first due odometer/i)).toHaveValue(45000);
    });
  });

  it('recomputes the default when the interval changes', async () => {
    const user = userEvent.setup();
    renderForm({ currentMileage: 40000 });
    await selectOption(/recurrence type/i, /mileage-based/i);

    const interval = await screen.findByLabelText(/every \(miles\)/i);
    await user.clear(interval);
    await user.type(interval, '5000');
    await waitFor(() => {
      expect(screen.getByLabelText(/first due odometer/i)).toHaveValue(45000);
    });

    await user.clear(interval);
    await user.type(interval, '10000');
    await waitFor(() => {
      expect(screen.getByLabelText(/first due odometer/i)).toHaveValue(50000);
    });
  });

  // The whole point of the dirty-field guard: a deliberately chosen anchor must
  // survive a later edit to the interval.
  it('does not stomp a first-due odometer the user has edited', async () => {
    const user = userEvent.setup();
    renderForm({ currentMileage: 40000 });
    await selectOption(/recurrence type/i, /mileage-based/i);

    const interval = await screen.findByLabelText(/every \(miles\)/i);
    await user.clear(interval);
    await user.type(interval, '5000');
    await waitFor(() => {
      expect(screen.getByLabelText(/first due odometer/i)).toHaveValue(45000);
    });

    const anchorField = screen.getByLabelText(/first due odometer/i);
    await user.clear(anchorField);
    await user.type(anchorField, '99000');

    await user.clear(interval);
    await user.type(interval, '10000');

    await waitFor(() => {
      expect(screen.getByLabelText(/every \(miles\)/i)).toHaveValue(10000);
    });
    expect(screen.getByLabelText(/first due odometer/i)).toHaveValue(99000);
  });

  // A one-time schedule has no interval to derive an anchor from.
  it('does not default the due odometer for a one-time schedule', async () => {
    renderForm({ currentMileage: 40000 });
    await selectOption(/schedule type/i, /one-time/i);
    await selectOption(/recurrence type/i, /mileage-based/i);

    const due = await screen.findByLabelText(/due odometer/i);
    expect(due).toHaveValue(null);
  });
});

describe('MaintenanceScheduleForm — submission', () => {
  it('submits kind and the due point alongside the interval', async () => {
    const user = userEvent.setup();
    const { onSubmit } = renderForm({ currentMileage: 40000 });

    await user.click(screen.getByRole('combobox', { name: /category/i }));
    await user.click(await screen.findByRole('option', { name: /oil change/i }));

    const months = screen.getByLabelText(/every \(months\)/i);
    await user.clear(months);
    await user.type(months, '6');

    const firstDue = screen.getByLabelText(/first due date/i);
    await user.clear(firstDue);
    await user.type(firstDue, '2027-01-15');

    await user.click(screen.getByRole('button', { name: /save schedule/i }));

    await waitFor(() => expect(onSubmit).toHaveBeenCalled());
    expect(onSubmit).toHaveBeenCalledWith(
      expect.objectContaining({
        categoryId: 'c1',
        kind: 'recurring',
        recurrenceType: 'time',
        intervalMonths: 6,
        dueDate: '2027-01-15',
      }),
    );
  });
});
```

- [x] **Step 2: Run the test to verify it fails**

Run: `export NVM_DIR="$HOME/.nvm" && . "$NVM_DIR/nvm.sh" && nvm use 22 && npx vitest run src/components/features/vehicles/maintenance/MaintenanceScheduleForm.test.tsx --root apps/web`
Expected: FAIL — there is no "Schedule type" select, no due-date field, and no `currentMileage` prop.

- [x] **Step 3: Rewrite the form**

Replace `apps/web/src/components/features/vehicles/maintenance/MaintenanceScheduleForm.tsx` with:

```tsx
import { useEffect } from 'react';
import { useForm, useWatch } from 'react-hook-form';
import { zodResolver } from '@hookform/resolvers/zod';
import { Loader2 } from 'lucide-react';
import {
  maintenanceScheduleSchema,
  type MaintenanceScheduleFormInput,
} from '../../../../lib/schemas/maintenanceSchedule';
import { Button } from '../../../ui/button';
import { Input } from '../../../ui/input';
import { Form, FormControl, FormField, FormItem, FormLabel, FormMessage } from '../../../ui/form';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '../../../ui/select';
import { CategoryCombobox } from '../CategoryCombobox';
import type { MaintenanceCategory } from '../../../../types/models/maintenanceCategory';

interface MaintenanceScheduleFormProps {
  categories: MaintenanceCategory[];
  onSubmit: (values: MaintenanceScheduleFormInput) => Promise<void> | void;
  onCancel?: () => void;
  submitting?: boolean;
  /** The vehicle's odometer, used to default a recurring schedule's first-due odometer. */
  currentMileage?: number;
}

/** YYYY-MM-DD, the value format a native date input uses. */
function toDateInputValue(d: Date): string {
  return d.toISOString().slice(0, 10);
}

/**
 * Form for defining a maintenance schedule.
 *
 * Two independent choices. `kind` says whether the schedule repeats;
 * `recurrenceType` says which axes it is judged on — time, mileage, or both —
 * which is the same question for a one-off as for a repeating item, so the
 * select stays visible for both kinds. The field set below it swaps:
 *  - recurring: intervals PLUS a first-due anchor on each covered axis
 *  - one-time:  a due date / due odometer on each covered axis, no intervals
 */
export function MaintenanceScheduleForm({
  categories,
  onSubmit,
  onCancel,
  submitting,
  currentMileage,
}: MaintenanceScheduleFormProps) {
  const form = useForm<MaintenanceScheduleFormInput>({
    resolver: zodResolver(maintenanceScheduleSchema),
    defaultValues: {
      categoryId: '',
      kind: 'recurring',
      recurrenceType: 'time',
      intervalMonths: undefined,
      intervalMiles: undefined,
      dueDate: undefined,
      dueMileage: undefined,
    },
  });

  const kind = useWatch({ control: form.control, name: 'kind' });
  const recurrenceType = useWatch({ control: form.control, name: 'recurrenceType' });
  const intervalMonths = useWatch({ control: form.control, name: 'intervalMonths' });
  const intervalMiles = useWatch({ control: form.control, name: 'intervalMiles' });

  const recurring = kind === 'recurring';
  const showMonths = recurrenceType === 'time' || recurrenceType === 'hybrid';
  const showMiles = recurrenceType === 'mileage' || recurrenceType === 'hybrid';

  const { setValue } = form;
  const dueDateDirty = Boolean(form.formState.dirtyFields.dueDate);
  const dueMileageDirty = Boolean(form.formState.dirtyFields.dueMileage);

  // FR-ANCHOR-4: a user who does not care about the anchor gets the intuitive
  // "starts now" behaviour for free. The dirty-field guard is what keeps a
  // later interval edit from stomping an anchor the user chose deliberately —
  // setValue without shouldDirty deliberately leaves the field clean, so the
  // default keeps tracking until the user actually types in it.
  useEffect(() => {
    if (!recurring) return;
    if (showMonths && intervalMonths && !dueDateDirty) {
      const next = new Date();
      next.setMonth(next.getMonth() + intervalMonths);
      setValue('dueDate', toDateInputValue(next));
    }
    if (showMiles && intervalMiles && currentMileage !== undefined && !dueMileageDirty) {
      setValue('dueMileage', currentMileage + intervalMiles);
    }
  }, [
    recurring,
    showMonths,
    showMiles,
    intervalMonths,
    intervalMiles,
    currentMileage,
    dueDateDirty,
    dueMileageDirty,
    setValue,
  ]);

  const dueDateLabel = recurring ? 'First due date' : 'Due date';
  const dueMileageLabel = recurring ? 'First due odometer (miles)' : 'Due odometer (miles)';

  return (
    <Form {...form}>
      <form onSubmit={form.handleSubmit((values) => onSubmit(values))} className="space-y-4">
        <FormField
          control={form.control}
          name="categoryId"
          render={({ field }) => (
            <FormItem>
              <FormLabel>Category</FormLabel>
              <FormControl>
                <CategoryCombobox
                  categories={categories}
                  kind="maintenance"
                  value={field.value}
                  onChange={field.onChange}
                  ariaLabel="Category"
                />
              </FormControl>
              <FormMessage />
            </FormItem>
          )}
        />

        <FormField
          control={form.control}
          name="kind"
          render={({ field }) => (
            <FormItem>
              <FormLabel>Schedule Type</FormLabel>
              <Select onValueChange={field.onChange} value={field.value}>
                <FormControl>
                  <SelectTrigger>
                    <SelectValue placeholder="Select schedule type" />
                  </SelectTrigger>
                </FormControl>
                <SelectContent>
                  <SelectItem value="recurring">Repeating</SelectItem>
                  <SelectItem value="oneTime">One-time</SelectItem>
                </SelectContent>
              </Select>
              <FormMessage />
            </FormItem>
          )}
        />

        <FormField
          control={form.control}
          name="recurrenceType"
          render={({ field }) => (
            <FormItem>
              <FormLabel>Recurrence Type</FormLabel>
              <Select onValueChange={field.onChange} value={field.value}>
                <FormControl>
                  <SelectTrigger>
                    <SelectValue placeholder="Select recurrence type" />
                  </SelectTrigger>
                </FormControl>
                <SelectContent>
                  <SelectItem value="time">Time-based</SelectItem>
                  <SelectItem value="mileage">Mileage-based</SelectItem>
                  <SelectItem value="hybrid">Hybrid (time + mileage)</SelectItem>
                </SelectContent>
              </Select>
              <FormMessage />
            </FormItem>
          )}
        />

        {recurring && showMonths && (
          <FormField
            control={form.control}
            name="intervalMonths"
            render={({ field }) => (
              <FormItem>
                <FormLabel>Every (months)</FormLabel>
                <FormControl>
                  <Input
                    type="number"
                    min={1}
                    value={field.value ?? ''}
                    onChange={(e) =>
                      field.onChange(e.target.value === '' ? undefined : e.target.valueAsNumber)
                    }
                    onBlur={field.onBlur}
                    name={field.name}
                    ref={field.ref}
                  />
                </FormControl>
                <FormMessage />
              </FormItem>
            )}
          />
        )}

        {recurring && showMiles && (
          <FormField
            control={form.control}
            name="intervalMiles"
            render={({ field }) => (
              <FormItem>
                <FormLabel>Every (miles)</FormLabel>
                <FormControl>
                  <Input
                    type="number"
                    min={1}
                    value={field.value ?? ''}
                    onChange={(e) =>
                      field.onChange(e.target.value === '' ? undefined : e.target.valueAsNumber)
                    }
                    onBlur={field.onBlur}
                    name={field.name}
                    ref={field.ref}
                  />
                </FormControl>
                <FormMessage />
              </FormItem>
            )}
          />
        )}

        {showMonths && (
          <FormField
            control={form.control}
            name="dueDate"
            render={({ field }) => (
              <FormItem>
                <FormLabel>{dueDateLabel}</FormLabel>
                <FormControl>
                  <Input
                    type="date"
                    value={field.value ?? ''}
                    onChange={field.onChange}
                    onBlur={field.onBlur}
                    name={field.name}
                    ref={field.ref}
                  />
                </FormControl>
                <FormMessage />
              </FormItem>
            )}
          />
        )}

        {showMiles && (
          <FormField
            control={form.control}
            name="dueMileage"
            render={({ field }) => (
              <FormItem>
                <FormLabel>{dueMileageLabel}</FormLabel>
                <FormControl>
                  <Input
                    type="number"
                    min={1}
                    value={field.value ?? ''}
                    onChange={(e) =>
                      field.onChange(e.target.value === '' ? undefined : e.target.valueAsNumber)
                    }
                    onBlur={field.onBlur}
                    name={field.name}
                    ref={field.ref}
                  />
                </FormControl>
                <FormMessage />
              </FormItem>
            )}
          />
        )}

        <div className="flex justify-end gap-2">
          {onCancel && (
            <Button type="button" variant="outline" onClick={onCancel}>
              Cancel
            </Button>
          )}
          <Button type="submit" disabled={submitting}>
            {submitting && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
            Save Schedule
          </Button>
        </div>
      </form>
    </Form>
  );
}
```

- [x] **Step 4: Map the form values onto the create attributes**

In `apps/web/src/components/features/vehicles/dialogs/AddScheduleDialog.tsx`, add `currentMileage` to the props interface and the destructuring:

```tsx
interface AddScheduleDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  vehicleId: string;
  /** The vehicle's odometer, used to default a recurring schedule's first-due odometer. */
  currentMileage?: number;
}

export function AddScheduleDialog({
  open,
  onOpenChange,
  vehicleId,
  currentMileage,
}: AddScheduleDialogProps) {
```

Replace the mutation call inside `handleCreateSchedule` with:

```tsx
      const oneTime = values.kind === 'oneTime';
      await createSchedule.mutateAsync({
        categoryId: values.categoryId,
        recurrenceType: values.recurrenceType,
        oneTime,
        // A one-time schedule must carry no interval at all (FR-OT-3); the
        // schema already blocks a value here, and undefined keeps a stale
        // field from riding along if the user switched kinds.
        intervalMonths: oneTime ? undefined : values.intervalMonths,
        intervalMiles: oneTime ? undefined : values.intervalMiles,
        // The date input yields YYYY-MM-DD; the API takes RFC3339.
        dueDate: values.dueDate ? new Date(values.dueDate).toISOString() : undefined,
        dueMileage: values.dueMileage,
      });
```

and pass the prop through to the form:

```tsx
          <MaintenanceScheduleForm
            categories={maintenanceCategories}
            currentMileage={currentMileage}
            onSubmit={handleCreateSchedule}
            onCancel={() => onOpenChange(false)}
            submitting={createSchedule.isPending}
          />
```

- [x] **Step 5: Pass the odometer from the page**

In `apps/web/src/pages/VehicleDetailPage.tsx`, add the prop to the `AddScheduleDialog` element:

```tsx
      <AddScheduleDialog
        open={openDialog === 'schedule'}
        onOpenChange={(open) => !open && closeDialog()}
        vehicleId={vehicle.id}
        currentMileage={odometer}
      />
```

- [x] **Step 6: Run the tests and the type check**

Run: `export NVM_DIR="$HOME/.nvm" && . "$NVM_DIR/nvm.sh" && nvm use 22 && make fe-test && make fe-build`
Expected: PASS for both.

- [x] **Step 7: Commit**

```bash
git add apps/web/src/components/features/vehicles/maintenance/MaintenanceScheduleForm.tsx \
        apps/web/src/components/features/vehicles/maintenance/MaintenanceScheduleForm.test.tsx \
        apps/web/src/components/features/vehicles/dialogs/AddScheduleDialog.tsx \
        apps/web/src/pages/VehicleDetailPage.tsx
git commit -m "feat(web): offer one-time schedules and a first-due anchor in the schedule form"
```

---

## Task 12: Extract `ScheduleCard`

`UpcomingScheduleStrip` is, despite its name, the vehicle's whole schedule list — `VehicleDetailPage.tsx:192` feeds it `schedulesQuery.data`, which is `ListByVehicle` and includes inactive rows. FR-UI-2 and FR-UI-3 both land here.

The per-schedule card is currently a 30-line inline JSX block that this task would grow by roughly as much again (a one-time badge, a completed treatment, a completion-date line, a second action button). Extracting before growing is the difference between one component with two responsibilities and two with one each, and it gives the completed-state rendering a test target that does not require constructing a whole strip.

**Files:**
- Create: `apps/web/src/components/features/vehicles/maintenance/ScheduleCard.tsx`
- Test: `apps/web/src/components/features/vehicles/maintenance/ScheduleCard.test.tsx`

**Interfaces:**
- Consumes: `MaintenanceSchedule` (Task 10), `SeverityChip`, `Button`, `cn`.
- Produces:
```tsx
interface ScheduleCardProps {
  schedule: MaintenanceSchedule;
  categoryName: string;
  canWrite: boolean;
  onComplete: (schedule: MaintenanceSchedule) => void;
  onConvert: (schedule: MaintenanceSchedule) => void;
}
export function ScheduleCard(props: ScheduleCardProps): JSX.Element;
```
Task 13 renders it; Task 15 supplies `onConvert` from the page.

- [x] **Step 1: Write the failing test**

Create `apps/web/src/components/features/vehicles/maintenance/ScheduleCard.test.tsx`:

```tsx
import { describe, it, expect, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { ScheduleCard } from './ScheduleCard';
import type {
  MaintenanceSchedule,
  MaintenanceScheduleAttributes,
} from '../../../../types/models/maintenanceSchedule';

function schedule(overrides: Partial<MaintenanceScheduleAttributes> = {}): MaintenanceSchedule {
  return {
    id: 's1',
    type: 'maintenanceSchedules',
    attributes: {
      vehicleId: 'v1',
      categoryId: 'c1',
      recurrenceType: 'time',
      intervalMonths: 6,
      oneTime: false,
      status: 'ok',
      severity: 'informational',
      active: true,
      ...overrides,
    },
  };
}

function renderCard(
  s: MaintenanceSchedule,
  props: Partial<React.ComponentProps<typeof ScheduleCard>> = {},
) {
  const onComplete = props.onComplete ?? vi.fn();
  const onConvert = props.onConvert ?? vi.fn();
  render(
    <ScheduleCard
      schedule={s}
      categoryName="Oil Change"
      canWrite
      {...props}
      onComplete={onComplete}
      onConvert={onConvert}
    />,
  );
  return { onComplete, onConvert };
}

describe('ScheduleCard — recurring', () => {
  it('shows the interval and no one-time badge', () => {
    renderCard(schedule());
    expect(screen.getByText(/every 6 months/i)).toBeInTheDocument();
    expect(screen.queryByText(/one-time/i)).not.toBeInTheDocument();
  });

  it('offers Complete when the user can write', async () => {
    const user = userEvent.setup();
    const { onComplete } = renderCard(schedule());
    await user.click(screen.getByRole('button', { name: /complete/i }));
    expect(onComplete).toHaveBeenCalledTimes(1);
  });

  it('hides the actions for a read-only user', () => {
    renderCard(schedule(), { canWrite: false });
    expect(screen.queryByRole('button', { name: /complete/i })).not.toBeInTheDocument();
  });
});

describe('ScheduleCard — active one-time', () => {
  it('badges the card and states the due point instead of an interval', () => {
    renderCard(
      schedule({
        oneTime: true,
        intervalMonths: undefined,
        dueDate: '2026-11-30T00:00:00Z',
        recurrenceType: 'time',
      }),
    );
    expect(screen.getByText(/one-time/i)).toBeInTheDocument();
    expect(screen.getByText(/^due /i)).toBeInTheDocument();
    expect(screen.queryByText(/every/i)).not.toBeInTheDocument();
  });

  it('states a mileage due point', () => {
    renderCard(
      schedule({
        oneTime: true,
        intervalMonths: undefined,
        recurrenceType: 'mileage',
        dueMileage: 60000,
      }),
    );
    expect(screen.getByText(/at 60,000 miles/i)).toBeInTheDocument();
  });

  it('still offers Complete', () => {
    renderCard(schedule({ oneTime: true, intervalMonths: undefined, dueMileage: 60000, recurrenceType: 'mileage' }));
    expect(screen.getByRole('button', { name: /complete/i })).toBeInTheDocument();
  });
});

describe('ScheduleCard — completed one-time', () => {
  const completed = schedule({
    oneTime: true,
    intervalMonths: undefined,
    recurrenceType: 'mileage',
    active: false,
    lastCompletedDate: '2026-06-01T00:00:00Z',
    lastCompletedMileage: 60100,
  });

  // FR-COMPLETE-3: a deactivated schedule keeps showing BOTH its completion
  // date and its completion odometer.
  it('shows the completion date and odometer and drops the Complete action', () => {
    renderCard(completed);
    const line = screen.getByText(/^completed /i);
    expect(line).toBeInTheDocument();
    expect(line.textContent).toMatch(/60,100 miles/);
    expect(screen.queryByRole('button', { name: /^complete$/i })).not.toBeInTheDocument();
  });

  it('offers Set up recurrence to a writer', async () => {
    const user = userEvent.setup();
    const { onConvert } = renderCard(completed);
    await user.click(screen.getByRole('button', { name: /set up recurrence/i }));
    expect(onConvert).toHaveBeenCalledTimes(1);
  });

  it('does not offer Set up recurrence to a read-only user', () => {
    renderCard(completed, { canWrite: false });
    expect(screen.queryByRole('button', { name: /set up recurrence/i })).not.toBeInTheDocument();
  });

  // The conversion path exists only for one-time schedules; a deactivated
  // recurring schedule has nothing to convert to.
  it('does not offer Set up recurrence on a deactivated recurring schedule', () => {
    renderCard(schedule({ active: false, lastCompletedDate: '2026-06-01T00:00:00Z' }));
    expect(screen.queryByRole('button', { name: /set up recurrence/i })).not.toBeInTheDocument();
  });
});
```

- [x] **Step 2: Run the test to verify it fails**

Run: `export NVM_DIR="$HOME/.nvm" && . "$NVM_DIR/nvm.sh" && nvm use 22 && npx vitest run src/components/features/vehicles/maintenance/ScheduleCard.test.tsx --root apps/web`
Expected: FAIL — `ScheduleCard` does not exist.

- [x] **Step 3: Write the component**

Create `apps/web/src/components/features/vehicles/maintenance/ScheduleCard.tsx`:

```tsx
import { Button } from '../../../ui/button';
import { SeverityChip } from './SeverityChip';
import { cn } from '../../../../lib/utils';
import type { MaintenanceSchedule } from '../../../../types/models/maintenanceSchedule';

interface ScheduleCardProps {
  schedule: MaintenanceSchedule;
  categoryName: string;
  canWrite: boolean;
  onComplete: (schedule: MaintenanceSchedule) => void;
  onConvert: (schedule: MaintenanceSchedule) => void;
}

/**
 * The recurrence line answers "when is this due, and does it repeat".
 *
 * A one-time schedule has no interval to state, so it states its due point
 * instead — which is also the only place in the card where `dueDate` /
 * `dueMileage` are read directly rather than through `nextDue*`. That is
 * deliberate: `nextDue*` is refreshed by the hourly recompute and can lag the
 * anchor the user just chose.
 */
function recurrenceLine(schedule: MaintenanceSchedule): string {
  const { recurrenceType, intervalMonths, intervalMiles, oneTime, dueDate, dueMileage } =
    schedule.attributes;

  const parts: string[] = [];
  if (oneTime) {
    if (dueDate) parts.push(`due ${new Date(dueDate).toLocaleDateString()}`);
    if (dueMileage) parts.push(`at ${dueMileage.toLocaleString()} miles`);
  } else {
    if (intervalMonths) {
      parts.push(`every ${intervalMonths} month${intervalMonths === 1 ? '' : 's'}`);
    }
    if (intervalMiles) {
      parts.push(`every ${intervalMiles.toLocaleString()} miles`);
    }
  }
  return parts.length > 0 ? parts.join(' · ') : recurrenceType;
}

/**
 * The completed line: date and odometer, both of which FR-COMPLETE-3 requires a
 * deactivated schedule to keep showing. Either may be absent — a schedule can
 * be completed with no odometer reading — so each is included only when set.
 */
function completedLine(schedule: MaintenanceSchedule): string {
  const { lastCompletedDate, lastCompletedMileage } = schedule.attributes;
  const parts: string[] = [];
  if (lastCompletedDate) parts.push(new Date(lastCompletedDate).toLocaleDateString());
  if (lastCompletedMileage) parts.push(`${lastCompletedMileage.toLocaleString()} miles`);
  return parts.length > 0 ? `Completed ${parts.join(' · ')}` : 'Completed';
}

/**
 * One maintenance schedule in the vehicle's schedule list.
 *
 * Three states, in the order the checks read:
 *  - inactive: de-emphasized, no tone border, a completion line, no Complete
 *    button, and — for a one-time schedule — a Set up recurrence action
 *    (FR-UI-3, FR-CONV-4's second entry point).
 *  - active one-time: a One-time badge and a stated due point (FR-UI-2).
 *  - active recurring: unchanged from before this task.
 *
 * Tone comes from `status`, never `severity`: they are different vocabularies
 * on the same resource — `status` is how due it is, `severity` is how much it
 * matters. Only `status` answers "is this urgent right now".
 */
export function ScheduleCard({
  schedule,
  categoryName,
  canWrite,
  onComplete,
  onConvert,
}: ScheduleCardProps) {
  const { status, severity, active, oneTime } = schedule.attributes;

  // A deactivated row's status describes a cycle that is over, so it gets no
  // tone at all rather than a stale one.
  const toneClass = !active
    ? 'opacity-70'
    : status === 'overdue'
      ? 'border-danger-border bg-danger-subtle/45'
      : status === 'upcoming'
        ? 'border-warning-border bg-warning-subtle/45'
        : '';

  return (
    <div className={cn('space-y-2 rounded-lg border p-3', toneClass)}>
      <div className="flex items-center justify-between gap-2">
        <span className={cn('truncate text-sm font-medium', !active && 'text-muted-foreground')}>
          {categoryName}
        </span>
        {active && <SeverityChip severity={severity} />}
      </div>

      {oneTime && (
        <span className="inline-flex items-center rounded-full border border-border px-2 py-0.5 text-xs font-medium text-muted-foreground">
          One-time
        </span>
      )}

      {active ? (
        <p className="text-xs text-muted-foreground">{recurrenceLine(schedule)}</p>
      ) : (
        <p className="text-xs text-muted-foreground">{completedLine(schedule)}</p>
      )}

      {canWrite && active && (
        <Button type="button" size="sm" variant="outline" onClick={() => onComplete(schedule)}>
          Complete
        </Button>
      )}

      {canWrite && !active && oneTime && (
        <Button type="button" size="sm" variant="outline" onClick={() => onConvert(schedule)}>
          Set up recurrence
        </Button>
      )}
    </div>
  );
}
```

- [x] **Step 4: Run the test to verify it passes**

Run: `export NVM_DIR="$HOME/.nvm" && . "$NVM_DIR/nvm.sh" && nvm use 22 && npx vitest run src/components/features/vehicles/maintenance/ScheduleCard.test.tsx --root apps/web`
Expected: PASS.

- [x] **Step 5: Commit**

```bash
git add apps/web/src/components/features/vehicles/maintenance/ScheduleCard.tsx \
        apps/web/src/components/features/vehicles/maintenance/ScheduleCard.test.tsx
git commit -m "feat(web): extract ScheduleCard with one-time and completed states"
```

---

## Task 13: Two-tier sorting in the schedule strip

`rankSchedule` reads `status`, which is meaningless for a deactivated row, so the active/inactive tier check must come **first** rather than being folded into it. `rankSchedule` itself is unchanged.

**Files:**
- Modify: `apps/web/src/components/features/vehicles/detail/UpcomingScheduleStrip.tsx`
- Test: `apps/web/src/components/features/vehicles/detail/UpcomingScheduleStrip.test.tsx` (create)

**Interfaces:**
- Consumes: `ScheduleCard` (Task 12), `rankSchedule` (unchanged).
- Produces: `UpcomingScheduleStripProps` gains `onConvert: (schedule: MaintenanceSchedule) => void`. Task 15 supplies it.

- [x] **Step 1: Write the failing test**

Create `apps/web/src/components/features/vehicles/detail/UpcomingScheduleStrip.test.tsx`:

```tsx
import { describe, it, expect, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import { UpcomingScheduleStrip } from './UpcomingScheduleStrip';
import type {
  MaintenanceSchedule,
  MaintenanceScheduleAttributes,
} from '../../../../types/models/maintenanceSchedule';

function schedule(
  id: string,
  overrides: Partial<MaintenanceScheduleAttributes>,
): MaintenanceSchedule {
  return {
    id,
    type: 'maintenanceSchedules',
    attributes: {
      vehicleId: 'v1',
      categoryId: id,
      recurrenceType: 'time',
      intervalMonths: 6,
      oneTime: false,
      status: 'ok',
      severity: 'informational',
      active: true,
      ...overrides,
    },
  };
}

const names = new Map([
  ['overdue', 'Overdue Item'],
  ['upcoming', 'Upcoming Item'],
  ['ok', 'Ok Item'],
  ['done-old', 'Old Done Item'],
  ['done-new', 'New Done Item'],
]);

function renderStrip(schedules: MaintenanceSchedule[]) {
  render(
    <UpcomingScheduleStrip
      schedules={schedules}
      categoryNames={names}
      canWrite
      onAddSchedule={vi.fn()}
      onComplete={vi.fn()}
      onConvert={vi.fn()}
    />,
  );
}

/**
 * The rendered category names in DOM order. getAllByText returns matches in
 * document order, which is exactly the ordering under test — asserting on the
 * rendered sequence rather than on the comparator keeps the test honest about
 * what the user sees.
 */
function renderedOrder(): (string | null)[] {
  return screen.getAllByText(/ Item$/).map((node) => node.textContent);
}

describe('UpcomingScheduleStrip ordering', () => {
  it('ranks active schedules overdue, then upcoming, then ok', () => {
    renderStrip([
      schedule('ok', { status: 'ok' }),
      schedule('overdue', { status: 'overdue', severity: 'urgent' }),
      schedule('upcoming', { status: 'upcoming', severity: 'recommended' }),
    ]);
    expect(renderedOrder()).toEqual(['Overdue Item', 'Upcoming Item', 'Ok Item']);
  });

  // A deactivated row's status describes a cycle that is over, so the tier
  // check has to come before rankSchedule rather than inside it — otherwise a
  // completed schedule whose last stored status was 'overdue' would sort to
  // the top of the list.
  it('sorts inactive schedules below every active one regardless of stored status', () => {
    renderStrip([
      schedule('done-old', {
        active: false,
        oneTime: true,
        status: 'overdue',
        lastCompletedDate: '2026-01-01T00:00:00Z',
      }),
      schedule('ok', { status: 'ok' }),
    ]);
    expect(renderedOrder()).toEqual(['Ok Item', 'Old Done Item']);
  });

  it('orders inactive schedules most-recently-completed first', () => {
    renderStrip([
      schedule('done-old', {
        active: false,
        oneTime: true,
        lastCompletedDate: '2026-01-01T00:00:00Z',
      }),
      schedule('done-new', {
        active: false,
        oneTime: true,
        lastCompletedDate: '2026-08-01T00:00:00Z',
      }),
    ]);
    expect(renderedOrder()).toEqual(['New Done Item', 'Old Done Item']);
  });

  it('shows the empty state when there are no schedules', () => {
    renderStrip([]);
    expect(screen.getByText(/no maintenance schedules defined/i)).toBeInTheDocument();
  });
});
```

- [x] **Step 2: Run the test to verify it fails**

Run: `export NVM_DIR="$HOME/.nvm" && . "$NVM_DIR/nvm.sh" && nvm use 22 && npx vitest run src/components/features/vehicles/detail/UpcomingScheduleStrip.test.tsx --root apps/web`
Expected: FAIL — `onConvert` is not a prop and the inactive rows are not tiered.

- [x] **Step 3: Rewrite the strip**

Replace `apps/web/src/components/features/vehicles/detail/UpcomingScheduleStrip.tsx` with:

```tsx
import { Card, CardContent, CardHeader, CardTitle } from '../../../ui/card';
import { Button } from '../../../ui/button';
import { ScheduleCard } from '../maintenance/ScheduleCard';
import { rankSchedule } from '../../../../lib/vehicleStats';
import type { MaintenanceSchedule } from '../../../../types/models/maintenanceSchedule';

interface UpcomingScheduleStripProps {
  schedules: MaintenanceSchedule[];
  /**
   * categoryId -> display name. Resolved by the caller (which already holds
   * the category list for the rest of the page) so this component stays
   * presentational and doesn't issue its own fetch.
   */
  categoryNames: Map<string, string>;
  canWrite: boolean;
  onAddSchedule: () => void;
  onComplete: (schedule: MaintenanceSchedule) => void;
  onConvert: (schedule: MaintenanceSchedule) => void;
}

/** Epoch millis of a completion, or 0 when there is none. */
function completedAt(schedule: MaintenanceSchedule): number {
  const value = schedule.attributes.lastCompletedDate;
  return value ? new Date(value).getTime() : 0;
}

/**
 * Two tiers, in this order:
 *  1. active schedules, ranked by `rankSchedule` (overdue → upcoming → ok)
 *  2. inactive (completed) schedules, most recently completed first
 *
 * The tier check comes FIRST and is not folded into `rankSchedule`, because
 * `rankSchedule` reads `status` — which describes a cycle that is over for a
 * deactivated row. A completed one-time schedule whose last stored status was
 * 'overdue' would otherwise sort above every live schedule on the page.
 */
function compareSchedules(a: MaintenanceSchedule, b: MaintenanceSchedule): number {
  const aActive = a.attributes.active;
  const bActive = b.attributes.active;
  if (aActive !== bActive) return aActive ? -1 : 1;
  if (!aActive) return completedAt(b) - completedAt(a);
  return rankSchedule(a) - rankSchedule(b);
}

/**
 * The vehicle's maintenance schedules — active ones first, ranked by how due
 * they are, with completed one-offs settling underneath. Despite the name this
 * is the whole schedule list, not just the upcoming slice: the caller feeds it
 * `ListByVehicle`, which includes inactive rows.
 */
export function UpcomingScheduleStrip({
  schedules,
  categoryNames,
  canWrite,
  onAddSchedule,
  onComplete,
  onConvert,
}: UpcomingScheduleStripProps) {
  const sorted = [...schedules].sort(compareSchedules);

  return (
    <Card>
      <CardHeader className="flex-row items-center justify-between space-y-0">
        <CardTitle className="text-base font-semibold">Upcoming maintenance</CardTitle>
        {canWrite && (
          <Button type="button" size="sm" onClick={onAddSchedule}>
            Add schedule
          </Button>
        )}
      </CardHeader>
      <CardContent>
        {sorted.length === 0 ? (
          <p className="text-sm text-muted-foreground">No maintenance schedules defined.</p>
        ) : (
          <div className="grid grid-cols-[repeat(auto-fit,minmax(210px,1fr))] gap-3">
            {sorted.map((schedule) => (
              <ScheduleCard
                key={schedule.id}
                schedule={schedule}
                categoryName={
                  categoryNames.get(schedule.attributes.categoryId) ??
                  schedule.attributes.categoryId
                }
                canWrite={canWrite}
                onComplete={onComplete}
                onConvert={onConvert}
              />
            ))}
          </div>
        )}
      </CardContent>
    </Card>
  );
}
```

- [x] **Step 4: Run the test to verify it passes**

Run: `export NVM_DIR="$HOME/.nvm" && . "$NVM_DIR/nvm.sh" && nvm use 22 && npx vitest run src/components/features/vehicles/detail/UpcomingScheduleStrip.test.tsx --root apps/web`
Expected: PASS. `VehicleDetailPage.tsx` will not type-check until Task 15 supplies `onConvert`; that is expected.

- [x] **Step 5: Commit**

```bash
git add apps/web/src/components/features/vehicles/detail/UpcomingScheduleStrip.tsx \
        apps/web/src/components/features/vehicles/detail/UpcomingScheduleStrip.test.tsx
git commit -m "feat(web): sort completed schedules below active ones in the schedule list"
```

---

## Task 14: `ConvertToRecurrenceDialog`

FR-CONV-2, FR-CONV-3, FR-CONV-4. Submitting issues exactly one `PATCH /maintenance-schedules/{id}` through the existing update mutation. Next-due then derives from `lastCompletedDate` / `lastCompletedMileage`, which the completion flow already wrote. No new endpoint, no new authorization surface.

`dueDate: null` is load-bearing. The server distinguishes an absent key from an explicit null, so omitting it would leave the anchor in place and the converted schedule would still be pinned to its old due date.

**Files:**
- Create: `apps/web/src/components/features/vehicles/dialogs/ConvertToRecurrenceDialog.tsx`
- Test: `apps/web/src/components/features/vehicles/dialogs/ConvertToRecurrenceDialog.test.tsx`

**Interfaces:**
- Consumes: `convertToRecurrenceSchema` / `ConvertToRecurrenceFormInput` (Task 10), `useUpdateMaintenanceSchedule` (unchanged).
- Produces:
```tsx
interface ConvertToRecurrenceDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  schedule: MaintenanceSchedule;
  categoryName: string;
}
export function ConvertToRecurrenceDialog(props: ConvertToRecurrenceDialogProps): JSX.Element;
```
Task 15 mounts it from the page.

- [x] **Step 1: Write the failing test**

Create `apps/web/src/components/features/vehicles/dialogs/ConvertToRecurrenceDialog.test.tsx`:

```tsx
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { ConvertToRecurrenceDialog } from './ConvertToRecurrenceDialog';
import { maintenanceScheduleService } from '../../../../services/api/MaintenanceScheduleService';
import type { MaintenanceSchedule } from '../../../../types/models/maintenanceSchedule';

vi.mock('../../../../services/api/MaintenanceScheduleService', () => ({
  maintenanceScheduleService: { patch: vi.fn() },
}));

const toastError = vi.fn();
vi.mock('sonner', () => ({
  toast: {
    success: vi.fn(),
    error: (...args: unknown[]) => toastError(...args),
  },
}));

const completedOneTime: MaintenanceSchedule = {
  id: 's1',
  type: 'maintenanceSchedules',
  attributes: {
    vehicleId: 'v1',
    categoryId: 'c1',
    recurrenceType: 'time',
    oneTime: true,
    status: 'ok',
    severity: 'informational',
    active: false,
    lastCompletedDate: '2026-06-01T00:00:00Z',
    lastCompletedMileage: 42000,
  },
};

function renderDialog(onOpenChange = vi.fn()) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  render(
    <QueryClientProvider client={client}>
      <ConvertToRecurrenceDialog
        open
        onOpenChange={onOpenChange}
        schedule={completedOneTime}
        categoryName="Oil Change"
      />
    </QueryClientProvider>,
  );
  return { onOpenChange };
}

beforeEach(() => {
  vi.mocked(maintenanceScheduleService.patch).mockReset();
  toastError.mockReset();
});

describe('ConvertToRecurrenceDialog', () => {
  it('shows the category read-only and states the completion anchor', () => {
    renderDialog();
    expect(screen.getByText('Oil Change')).toBeInTheDocument();
    // The anchor the recurrence will run from — 42,000 miles on the recorded date.
    expect(screen.getByText(/42,000/)).toBeInTheDocument();
    // No category input: the category is fixed.
    expect(screen.queryByRole('combobox', { name: /category/i })).not.toBeInTheDocument();
  });

  it('issues exactly one PATCH clearing the anchor and reactivating the schedule', async () => {
    const user = userEvent.setup();
    vi.mocked(maintenanceScheduleService.patch).mockResolvedValue({
      ...completedOneTime,
      attributes: { ...completedOneTime.attributes, oneTime: false, active: true },
    });
    const { onOpenChange } = renderDialog();

    const months = screen.getByLabelText(/every \(months\)/i);
    await user.clear(months);
    await user.type(months, '12');
    await user.click(screen.getByRole('button', { name: /set up recurrence/i }));

    await waitFor(() => expect(maintenanceScheduleService.patch).toHaveBeenCalledTimes(1));
    expect(maintenanceScheduleService.patch).toHaveBeenCalledWith('s1', {
      oneTime: false,
      recurrenceType: 'time',
      intervalMonths: 12,
      intervalMiles: 0,
      active: true,
      // An explicit null, not an omitted key: the server tells them apart, and
      // omitting it would leave the schedule pinned to its old due date.
      dueDate: null,
      dueMileage: 0,
    });
    await waitFor(() => expect(onOpenChange).toHaveBeenCalledWith(false));
  });

  it('keeps the dialog open and shows an error toast when the PATCH fails', async () => {
    const user = userEvent.setup();
    vi.mocked(maintenanceScheduleService.patch).mockRejectedValue(new Error('nope'));
    const { onOpenChange } = renderDialog();

    const months = screen.getByLabelText(/every \(months\)/i);
    await user.clear(months);
    await user.type(months, '12');
    await user.click(screen.getByRole('button', { name: /set up recurrence/i }));

    await waitFor(() => expect(toastError).toHaveBeenCalled());
    expect(onOpenChange).not.toHaveBeenCalledWith(false);
  });

  it('does not submit without the interval its recurrence type needs', async () => {
    const user = userEvent.setup();
    renderDialog();
    await user.click(screen.getByRole('button', { name: /set up recurrence/i }));
    await waitFor(() => expect(screen.getByText(/interval months is required/i)).toBeInTheDocument());
    expect(maintenanceScheduleService.patch).not.toHaveBeenCalled();
  });
});
```

- [x] **Step 2: Run the test to verify it fails**

Run: `export NVM_DIR="$HOME/.nvm" && . "$NVM_DIR/nvm.sh" && nvm use 22 && npx vitest run src/components/features/vehicles/dialogs/ConvertToRecurrenceDialog.test.tsx --root apps/web`
Expected: FAIL — the component does not exist.

- [x] **Step 3: Write the component**

Create `apps/web/src/components/features/vehicles/dialogs/ConvertToRecurrenceDialog.tsx`:

```tsx
import { useForm, useWatch } from 'react-hook-form';
import { zodResolver } from '@hookform/resolvers/zod';
import { toast } from 'sonner';
import { Loader2 } from 'lucide-react';
import { createErrorFromUnknown } from '@myfleet/shared-ts';
import { Dialog, DialogContent, DialogHeader, DialogTitle } from '../../../ui/dialog';
import { Button } from '../../../ui/button';
import { Input } from '../../../ui/input';
import { Form, FormControl, FormField, FormItem, FormLabel, FormMessage } from '../../../ui/form';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '../../../ui/select';
import { useUpdateMaintenanceSchedule } from '../../../../lib/hooks/api/maintenance';
import {
  convertToRecurrenceSchema,
  type ConvertToRecurrenceFormInput,
} from '../../../../lib/schemas/maintenanceSchedule';
import type { MaintenanceSchedule } from '../../../../types/models/maintenanceSchedule';

interface ConvertToRecurrenceDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  schedule: MaintenanceSchedule;
  categoryName: string;
}

/**
 * Turns a just-completed one-time schedule into a recurring one — the
 * "I didn't know this would repeat until I did it" path (FR-CONV-2, FR-CONV-3).
 *
 * Submitting issues exactly ONE PATCH. Next-due is then derived server-side
 * from `lastCompletedDate` / `lastCompletedMileage`, which the completion flow
 * already recorded, so the dialog collects no anchor — only the recurrence type
 * and the intervals it needs.
 *
 * The conversion is strictly additive and never rolls the completion back. If
 * the PATCH fails the dialog stays open with an error toast, and the schedule
 * remains a completed, deactivated one-time schedule — a valid terminal state
 * (FR-CONV-4).
 */
export function ConvertToRecurrenceDialog({
  open,
  onOpenChange,
  schedule,
  categoryName,
}: ConvertToRecurrenceDialogProps) {
  const update = useUpdateMaintenanceSchedule();

  const form = useForm<ConvertToRecurrenceFormInput>({
    resolver: zodResolver(convertToRecurrenceSchema),
    defaultValues: {
      recurrenceType: 'time',
      intervalMonths: undefined,
      intervalMiles: undefined,
    },
  });

  const recurrenceType = useWatch({ control: form.control, name: 'recurrenceType' });
  const showMonths = recurrenceType === 'time' || recurrenceType === 'hybrid';
  const showMiles = recurrenceType === 'mileage' || recurrenceType === 'hybrid';

  const { lastCompletedDate, lastCompletedMileage } = schedule.attributes;
  const anchorParts = [
    lastCompletedDate ? new Date(lastCompletedDate).toLocaleDateString() : null,
    lastCompletedMileage ? `${lastCompletedMileage.toLocaleString()} miles` : null,
  ].filter(Boolean);

  const handleSubmit = async (values: ConvertToRecurrenceFormInput) => {
    try {
      await update.mutateAsync({
        id: schedule.id,
        attributes: {
          oneTime: false,
          recurrenceType: values.recurrenceType,
          intervalMonths: values.intervalMonths ?? 0,
          intervalMiles: values.intervalMiles ?? 0,
          active: true,
          // An explicit null, not an omitted key. The server distinguishes the
          // two (server.Nullable), and omitting it would leave the converted
          // schedule pinned to the one-time due date it just completed.
          dueDate: null,
          dueMileage: 0,
        },
      });
      toast.success('Recurrence set up');
      onOpenChange(false);
    } catch (err) {
      const apiError = createErrorFromUnknown(err);
      toast.error(apiError.message || 'Could not set up recurrence');
    }
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Set Up Recurrence</DialogTitle>
        </DialogHeader>

        <div className="space-y-1">
          <p className="text-sm font-medium">{categoryName}</p>
          {anchorParts.length > 0 && (
            <p className="text-xs text-muted-foreground">
              Repeats from the completion you just recorded: {anchorParts.join(' · ')}
            </p>
          )}
        </div>

        <Form {...form}>
          <form onSubmit={form.handleSubmit(handleSubmit)} className="space-y-4">
            <FormField
              control={form.control}
              name="recurrenceType"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>Recurrence Type</FormLabel>
                  <Select onValueChange={field.onChange} value={field.value}>
                    <FormControl>
                      <SelectTrigger>
                        <SelectValue placeholder="Select recurrence type" />
                      </SelectTrigger>
                    </FormControl>
                    <SelectContent>
                      <SelectItem value="time">Time-based</SelectItem>
                      <SelectItem value="mileage">Mileage-based</SelectItem>
                      <SelectItem value="hybrid">Hybrid (time + mileage)</SelectItem>
                    </SelectContent>
                  </Select>
                  <FormMessage />
                </FormItem>
              )}
            />

            {showMonths && (
              <FormField
                control={form.control}
                name="intervalMonths"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>Every (months)</FormLabel>
                    <FormControl>
                      <Input
                        type="number"
                        min={1}
                        value={field.value ?? ''}
                        onChange={(e) =>
                          field.onChange(e.target.value === '' ? undefined : e.target.valueAsNumber)
                        }
                        onBlur={field.onBlur}
                        name={field.name}
                        ref={field.ref}
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
            )}

            {showMiles && (
              <FormField
                control={form.control}
                name="intervalMiles"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>Every (miles)</FormLabel>
                    <FormControl>
                      <Input
                        type="number"
                        min={1}
                        value={field.value ?? ''}
                        onChange={(e) =>
                          field.onChange(e.target.value === '' ? undefined : e.target.valueAsNumber)
                        }
                        onBlur={field.onBlur}
                        name={field.name}
                        ref={field.ref}
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
            )}

            <div className="flex justify-end gap-2">
              <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
                Cancel
              </Button>
              <Button type="submit" disabled={update.isPending}>
                {update.isPending && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
                Set up recurrence
              </Button>
            </div>
          </form>
        </Form>
      </DialogContent>
    </Dialog>
  );
}
```

- [x] **Step 4: Run the test to verify it passes**

Run: `export NVM_DIR="$HOME/.nvm" && . "$NVM_DIR/nvm.sh" && nvm use 22 && npx vitest run src/components/features/vehicles/dialogs/ConvertToRecurrenceDialog.test.tsx --root apps/web`
Expected: PASS.

- [x] **Step 5: Commit**

```bash
git add apps/web/src/components/features/vehicles/dialogs/ConvertToRecurrenceDialog.tsx \
        apps/web/src/components/features/vehicles/dialogs/ConvertToRecurrenceDialog.test.tsx
git commit -m "feat(web): add the convert-to-recurrence dialog"
```

---

## Task 15: The completion toast action and the page wiring

FR-CONV-1, FR-CONV-5, FR-UI-4. The complete-schedule dialog is otherwise unchanged for one-time schedules; the conversion offer lives entirely in the post-success toast. The conversion dialog is opened by the **page**, not nested inside the completion dialog, so dismissing one does not affect the other.

**Files:**
- Modify: `apps/web/src/components/features/vehicles/dialogs/CompleteScheduleDialog.tsx`
- Modify: `apps/web/src/pages/VehicleDetailPage.tsx`
- Test: `apps/web/src/components/features/vehicles/dialogs/CompleteScheduleDialog.test.tsx` (create)

**Interfaces:**
- Consumes: `ConvertToRecurrenceDialog` (Task 14), `UpcomingScheduleStrip`'s `onConvert` (Task 13).
- Produces: `CompleteScheduleDialogProps` gains `onRequestConvert: (schedule: MaintenanceSchedule) => void`.

- [x] **Step 1: Write the failing test**

Create `apps/web/src/components/features/vehicles/dialogs/CompleteScheduleDialog.test.tsx`:

```tsx
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { CompleteScheduleDialog } from './CompleteScheduleDialog';
import { maintenanceScheduleService } from '../../../../services/api/MaintenanceScheduleService';
import type {
  MaintenanceSchedule,
  MaintenanceScheduleAttributes,
} from '../../../../types/models/maintenanceSchedule';

vi.mock('../../../../services/api/MaintenanceScheduleService', () => ({
  maintenanceScheduleService: { complete: vi.fn() },
}));

// The vehicle and mileage queries are incidental to what is under test; stub
// the hooks rather than the transport so the dialog mounts without a network.
vi.mock('../../../../lib/hooks/api/vehicles', () => ({
  useVehicle: () => ({ data: { id: 'v1', type: 'vehicles', attributes: { currentMileage: 40000 } } }),
}));
vi.mock('../../../../lib/hooks/api/mileage', () => ({
  useMileageRecords: () => ({ data: { rows: [] } }),
  getLatestMileage: () => undefined,
}));

const toastSuccess = vi.fn();
vi.mock('sonner', () => ({
  toast: {
    success: (...args: unknown[]) => toastSuccess(...args),
    error: vi.fn(),
  },
}));

function schedule(overrides: Partial<MaintenanceScheduleAttributes>): MaintenanceSchedule {
  return {
    id: 's1',
    type: 'maintenanceSchedules',
    attributes: {
      vehicleId: 'v1',
      categoryId: 'c1',
      recurrenceType: 'time',
      intervalMonths: 6,
      oneTime: false,
      status: 'ok',
      severity: 'informational',
      active: true,
      ...overrides,
    },
  };
}

async function completeVia(s: MaintenanceSchedule) {
  const user = userEvent.setup();
  const onRequestConvert = vi.fn();
  const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  render(
    <QueryClientProvider client={client}>
      <CompleteScheduleDialog
        open
        onOpenChange={vi.fn()}
        schedule={s}
        onRequestConvert={onRequestConvert}
      />
    </QueryClientProvider>,
  );
  await user.click(screen.getByRole('button', { name: /mark complete/i }));
  return { onRequestConvert };
}

beforeEach(() => {
  vi.mocked(maintenanceScheduleService.complete).mockReset();
  vi.mocked(maintenanceScheduleService.complete).mockResolvedValue({
    id: 's1',
    type: 'maintenanceCompletions',
    attributes: { maintenanceRecordId: 'r1' },
  });
  toastSuccess.mockReset();
});

describe('CompleteScheduleDialog success toast', () => {
  it('offers a Set up recurrence action after completing a one-time schedule', async () => {
    await completeVia(schedule({ oneTime: true, intervalMonths: undefined, dueDate: '2026-11-30T00:00:00Z' }));

    await waitFor(() => expect(toastSuccess).toHaveBeenCalled());
    const [, options] = toastSuccess.mock.calls[0] as [string, { action?: { label: string } }];
    expect(options?.action?.label).toBe('Set up recurrence');
  });

  it('invokes onRequestConvert when the toast action is activated', async () => {
    const { onRequestConvert } = await completeVia(
      schedule({ oneTime: true, intervalMonths: undefined, dueDate: '2026-11-30T00:00:00Z' }),
    );

    await waitFor(() => expect(toastSuccess).toHaveBeenCalled());
    const [, options] = toastSuccess.mock.calls[0] as [
      string,
      { action?: { onClick: () => void } },
    ];
    options?.action?.onClick();
    expect(onRequestConvert).toHaveBeenCalledTimes(1);
  });

  // FR-CONV-5: a recurring completion keeps today's plain success toast.
  it('offers no action after completing a recurring schedule', async () => {
    await completeVia(schedule({}));

    await waitFor(() => expect(toastSuccess).toHaveBeenCalled());
    const [, options] = toastSuccess.mock.calls[0] as [string, { action?: unknown } | undefined];
    expect(options?.action).toBeUndefined();
  });
});
```

- [x] **Step 2: Run the test to verify it fails**

Run: `export NVM_DIR="$HOME/.nvm" && . "$NVM_DIR/nvm.sh" && nvm use 22 && npx vitest run src/components/features/vehicles/dialogs/CompleteScheduleDialog.test.tsx --root apps/web`
Expected: FAIL — `onRequestConvert` is not a prop and the toast carries no action.

- [x] **Step 3: Add the toast action**

In `apps/web/src/components/features/vehicles/dialogs/CompleteScheduleDialog.tsx`, extend the props interface and destructuring:

```tsx
interface CompleteScheduleDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  schedule: MaintenanceSchedule;
  /**
   * Opens the conversion dialog. Owned by the PAGE, not nested here: the two
   * dialogs are independent, so dismissing one must not affect the other.
   */
  onRequestConvert: (schedule: MaintenanceSchedule) => void;
}

export function CompleteScheduleDialog({
  open,
  onOpenChange,
  schedule,
  onRequestConvert,
}: CompleteScheduleDialogProps) {
```

Replace the success branch of `handleSubmit`:

```tsx
      // FR-CONV-1 / FR-CONV-5: the conversion offer rides the success toast,
      // and only for a one-time schedule. Completing a recurring one keeps the
      // plain toast — there is nothing to convert.
      if (schedule.attributes.oneTime) {
        toast.success('Maintenance marked as complete', {
          action: {
            label: 'Set up recurrence',
            onClick: () => onRequestConvert(schedule),
          },
        });
      } else {
        toast.success('Maintenance marked as complete');
      }
      onOpenChange(false);
```

- [x] **Step 4: Wire the page**

In `apps/web/src/pages/VehicleDetailPage.tsx`:

Import the dialog beside the others:

```tsx
import { ConvertToRecurrenceDialog } from '../components/features/vehicles/dialogs/ConvertToRecurrenceDialog';
```

Extend the dialog union and add the state:

```tsx
type OpenDialog = QuickAction | 'edit' | 'gallery' | 'complete' | 'convert' | null;
```

```tsx
  const [convertingSchedule, setConvertingSchedule] = useState<MaintenanceSchedule | null>(null);
```

Add the handler beside `handleComplete`:

```tsx
  const handleConvert = (schedule: MaintenanceSchedule) => {
    setConvertingSchedule(schedule);
    setOpenDialog('convert');
  };
```

Pass `onConvert` to the strip:

```tsx
          <UpcomingScheduleStrip
            schedules={schedulesQuery.data ?? []}
            categoryNames={categoryNames}
            canWrite={canWrite}
            onAddSchedule={() => setOpenDialog('schedule')}
            onComplete={handleComplete}
            onConvert={handleConvert}
          />
```

Pass `onRequestConvert` to the completion dialog:

```tsx
        <CompleteScheduleDialog
          open={openDialog === 'complete'}
          onOpenChange={(open) => {
            if (!open) {
              closeDialog();
              setCompletingSchedule(null);
            }
          }}
          schedule={completingSchedule}
          onRequestConvert={handleConvert}
        />
```

And mount the conversion dialog immediately after that block:

```tsx
      {convertingSchedule && (
        <ConvertToRecurrenceDialog
          open={openDialog === 'convert'}
          onOpenChange={(open) => {
            if (!open) {
              closeDialog();
              setConvertingSchedule(null);
            }
          }}
          schedule={convertingSchedule}
          categoryName={
            categoryNames.get(convertingSchedule.attributes.categoryId) ??
            convertingSchedule.attributes.categoryId
          }
        />
      )}
```

- [x] **Step 5: Run the frontend suite and the type check**

Run: `export NVM_DIR="$HOME/.nvm" && . "$NVM_DIR/nvm.sh" && nvm use 22 && make fe-test && make fe-build`
Expected: PASS for both.

- [x] **Step 6: Commit**

```bash
git add apps/web/src/components/features/vehicles/dialogs/CompleteScheduleDialog.tsx \
        apps/web/src/components/features/vehicles/dialogs/CompleteScheduleDialog.test.tsx \
        apps/web/src/pages/VehicleDetailPage.tsx
git commit -m "feat(web): offer recurrence setup after completing a one-time schedule"
```

---

## Task 16: Whole-repo verification

**Files:** none created or modified unless a check fails.

**Interfaces:** none.

- [ ] **Step 1: Run the full CI target**

Run: `export NVM_DIR="$HOME/.nvm" && . "$NVM_DIR/nvm.sh" && nvm use 22 && make ci`
Expected: PASS for `lint-check`, `vet`, `test`, `build`, `fe-test`, `fe-build`. Fix anything that fails and re-run the whole target — a partial pass is not a pass.

Left unticked deliberately. `vet`, `test`, `build`, `fe-test` and `fe-build` all
pass and were run individually. `lint-check` is red: six Go modules fail on a
golangci-lint go1.27-vs-go1.26 toolchain mismatch, reproduced identically on a
pristine merge-base checkout. It is pre-existing and belongs to
`task-029-go-127-migration`, so this branch cannot make the whole target green.

- [x] **Step 2: Render both deployment overlays**

Run:

```bash
kustomize build deploy/k8s/overlays/local > /tmp/task030-local.yaml
kustomize build deploy/k8s/overlays/main  > /tmp/task030-main.yaml
git stash && kustomize build deploy/k8s/overlays/local > /tmp/base-local.yaml && \
  kustomize build deploy/k8s/overlays/main > /tmp/base-main.yaml && git stash pop
diff /tmp/base-local.yaml /tmp/task030-local.yaml
diff /tmp/base-main.yaml /tmp/task030-main.yaml
```

Expected: both diffs empty. No manifest change is expected in this task, and a diff means something leaked. If the working tree is clean at this point (everything committed), compare against `main` with `git worktree`/`git show` instead of `git stash`.

- [x] **Step 3: Confirm the untouched-service claim**

Run: `git diff --name-only main... | sort`
Expected: no path under `apps/notification-service/` and no path under `deploy/`.

- [x] **Step 4: Run the code review**

Per `CLAUDE.md`, run `/audit-plan` or `superpowers:requesting-code-review` before opening a PR. It dispatches `plan-adherence-reviewer`, `backend-guidelines-reviewer` (Go changed) and `frontend-guidelines-reviewer` (TS changed); findings land in `docs/tasks/task-030-one-time-maintenance-schedules/audit.md`.

- [x] **Step 5: Commit any review fixes**

```bash
git add -A
git commit -m "fix(task-030): address code review findings"
```

(Skip if the review found nothing to change.)

