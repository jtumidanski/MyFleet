# Vehicle Card Status Banner — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Rebuild the vehicles-list card around a 16:9 hero photo and a conditional status banner that states *why* a vehicle needs attention, fed by two new read-only vehicle attributes derived from data the backend already computes and discards.

**Architecture:** Three layers, in dependency order. (1) `maintenanceschedule` gains a pure `DueBreaches` function beside `DueState` that turns a schedule into per-axis breaches carrying a threshold-normalized `Urgency`, and a widened `ScheduleDueByVehicle` that returns them — no new query. (2) `vehicle` declares its own port type for that shape (it must not import `maintenanceschedule`), selects the single governing breach with a pure `selectNextDue`, and returns status + last-activity + that breach from one `StatusDeps.Derive` call. (3) `apps/web` maps `(status, nextDue, lastActivityAt, now)` to `{tone, icon, text}` in a pure module and rebuilds `VehicleCard` as a vertical stack with an overlay-link root.

**Tech Stack:** Go 1.x (chi, GORM, logrus), React 18 + TypeScript, Vite, Vitest + Testing Library, Tailwind CSS 3.4, lucide-react, TanStack React Query, react-router-dom.

## Global Constraints

- **No new database queries.** The vehicles list issues exactly 2 gathers per vehicle before and after this change (NFR-1). Task 2's test measures this rather than asserting it by inspection.
- **No schema changes, no migrations, no new persisted columns** (PRD §6).
- **`vehicle` must not import `maintenanceschedule`** (FR-9.5). The port type is duplicated in `vehicle`; the mapping lives in `cmd/main.go`.
- **`status.Derive` is untouched** — its `Input`, its rule, and its output are unchanged (FR-9.3). Status values must be byte-for-byte identical to today's.
- **Both new attributes are read-only** — never accepted on create or update (FR-8.3, NFR-7).
- **Both new attributes are omitted, never zero-valued**, when their source gather fails (FR-8.4).
- **Semantic colour tokens are used as-is.** `danger-subtle` / `danger-subtle-foreground` / `danger-border` and the `warning-*` equivalents are already measured for AA contrast in `docs/tasks/task-003-dark-mode-branding/contrast.md`. No new colour combinations (NFR-11).
- **Colour is never the only signal.** Every banner carries an icon plus text (FR-3.4, NFR-10).
- **Healthy and Inactive get no tint** (FR-3.3). This deliberately diverges from `ux-prototype.html`'s always-on grid, which tints Healthy with `success-subtle`; the chosen "Conditional" grid uses its `.quietrow`.
- **Every card region is present on every card** — no region is conditionally omitted in a way that changes card height (FR-1.2).
- **Task-005's Carfax protections carry over unchanged**: rendered only when a VIN is present and the template contains `{vin}`; `target="_blank"` with `rel="noopener noreferrer"`; vehicle-identifying `aria-label`; VIN URL-encoded; template read through `useRuntimeConfig`, not build-time env; nothing contacts Carfax until an explicit click (FR-6.7, NFR-8).
- **Thresholds:** `DefaultThresholds` = 30 days / 500 miles (`maintenanceschedule/recurrence.go:20`). Inactivity threshold = 365 days (`vehicle/status.go:11`). Neither is changed by this task.
- **Urgency formula** (design D2), one monotone scale spanning both states, higher = more urgent:
  - overdue: `1 + breach/threshold` (always `> 1`)
  - upcoming: `1 - remaining/threshold` (always in `[0, 1]`)
- **Sequencing:** land task-006 (`created_at` wipe) before merging this branch. Both touch `vehicle/status.go`. This task degrades safely without it (design D6), so it is a merge-order preference, not a blocker.
- **Gate:** `make ci` must pass on the branch (NFR-17). Node may not be on `PATH`; if `npm` is missing run `export NVM_DIR="$HOME/.nvm" && . "$NVM_DIR/nvm.sh" && nvm use 22` first.
- **Formatting:** `make lint-check` (part of `make ci`) fails on any gofmt or prettier deviation. Run `make lint` before each commit — the Go snippets below are written for readability, and gofmt may re-align struct fields or expression spacing.

## File Structure

**`apps/fleet-service` — backend**

| File | Responsibility |
| --- | --- |
| `internal/maintenanceschedule/recurrence.go` (modify) | Add `AxisBreach` + `DueBreaches` beside `DueState`, sharing its branch conditions |
| `internal/maintenanceschedule/recurrence_test.go` (modify) | `DueBreaches` cases + the `DueState`/`DueBreaches` agreement invariant |
| `internal/maintenanceschedule/processor.go` (modify) | Add `ScheduleDue` + `ScheduleDueByVehicle`; later delete `ScheduleStatesByVehicle` |
| `internal/maintenanceschedule/processor_test.go` (create) | Fake `Provider` that counts `ListActiveByVehicle` calls |
| `internal/vehicle/nextdue.go` (create) | Port types (`ScheduleDue`, `Breach`, `ScheduleDueGatherer`), the wire type `NextDue`, and `selectNextDue` |
| `internal/vehicle/nextdue_test.go` (create) | `selectNextDue` table tests |
| `internal/vehicle/status.go` (modify) | `Derived` + `StatusDeps.Derive`; `ScheduleStateGatherer` and `DeriveStatus` deleted |
| `internal/vehicle/status_test.go` (create) | `Derive` with fake gatherers; the Inactive-implies-no-NextDue invariant; status parity |
| `internal/vehicle/rest.go` (modify) | Two new attributes, `TransformDerived`, named `createAttributes`/`patchAttributes` binding types |
| `internal/vehicle/rest_test.go` (create) | JSON shape of the derived attributes; read-only binding assertions |
| `internal/vehicle/resource.go` (modify) | Two read call sites use `TransformDerived`; POST/PATCH bind the named types |
| `cmd/main.go` (modify) | `scheduleDueAdapter` mapping `maintenanceschedule.ScheduleDue` → `vehicle.ScheduleDue` |

**`packages/ui-components` — shared formatters**

| File | Responsibility |
| --- | --- |
| `src/formatters.ts` (modify) | Add `formatRelativeTime` beside `formatMileage` |
| `src/formatters.test.ts` (modify) | `formatRelativeTime` ladder with an injected `now` |

**`apps/web` — the card**

| File | Responsibility |
| --- | --- |
| `src/types/models/vehicle.ts` (modify) | `VehicleNextDue`; optional `lastActivityAt` / `nextDue` on `VehicleAttributes` |
| `src/components/features/vehicles/vehicleBanner.ts` (create) | Pure `(attributes, now) → {tone, icon, text}`. No React import |
| `src/components/features/vehicles/vehicleBanner.test.ts` (create) | Every copy rule as plain data assertions |
| `src/components/features/vehicles/VehiclePhotoThumbnail.tsx` (modify) | `boxClassName` override threaded through all four states |
| `src/components/features/vehicles/VehiclePhotoThumbnail.test.tsx` (modify) | The override reaches all four states |
| `src/components/features/vehicles/VehicleCard.tsx` (modify) | Rebuilt stack + overlay link; also exports `VehicleCardSkeleton` |
| `src/components/features/vehicles/VehicleCard.test.tsx` (modify) | Layout, banner, strip, navigation, Carfax regression |
| `src/components/features/vehicles/VehicleList.tsx` (modify) | Renders `VehicleCardSkeleton`; the `h-40` magic number deleted |

**Repo**

| File | Responsibility |
| --- | --- |
| `Makefile` (modify) | `fe-test` runs `packages/ui-components` too — it currently runs in no gate |
| `docs/tasks/task-007-vehicle-card-status-banner/manual-verification.md` (create) | The checks jsdom structurally cannot make |

---

## Task 1: `DueBreaches` — per-axis breach detail with normalized urgency

**Files:**
- Modify: `apps/fleet-service/internal/maintenanceschedule/recurrence.go`
- Test: `apps/fleet-service/internal/maintenanceschedule/recurrence_test.go` (append)

**Interfaces:**
- Consumes: existing `Schedule`, `Thresholds`, `DefaultThresholds`, `NextDue`, `DueState` in the same file.
- Produces:
  - `type AxisBreach struct { Axis string; Days int; Miles int; Urgency float64 }`
  - `func DueBreaches(s Schedule, today time.Time, currentMileage int, th Thresholds) []AxisBreach`

Purely additive — nothing else in the repo changes, so the build and every existing test stay green.

- [ ] **Step 1: Write the failing tests**

Append to `apps/fleet-service/internal/maintenanceschedule/recurrence_test.go`. The file already declares `var base = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)` at package scope — reuse it, do not redeclare.

```go
// mileageOnly is due at 35000: last completed at 30000 with a 5000-mile interval.
func mileageOnly() Schedule {
	return Schedule{RecurrenceType: "mileage", IntervalMiles: 5000, LastCompletedMileage: 30000}
}

// timeOnly is due 12 months after base.
func timeOnly() Schedule {
	return Schedule{RecurrenceType: "time", IntervalMonths: 12, LastCompletedDate: base}
}

// hybridBoth is due at 35000 miles AND 12 months after base.
func hybridBoth() Schedule {
	return Schedule{
		RecurrenceType:       "hybrid",
		IntervalMonths:       12,
		IntervalMiles:        5000,
		LastCompletedDate:    base,
		LastCompletedMileage: 30000,
	}
}

func TestDueBreaches_okScheduleHasNoBreaches(t *testing.T) {
	// 31000 is well under 35000 and base+1mo is well before base+12mo.
	if got := DueBreaches(hybridBoth(), base.AddDate(0, 1, 0), 31000, DefaultThresholds); got != nil {
		t.Fatalf("an ok schedule must breach on no axis, got %+v", got)
	}
}

func TestDueBreaches_mileageOverdue(t *testing.T) {
	got := DueBreaches(mileageOnly(), base, 36120, DefaultThresholds)
	if len(got) != 1 {
		t.Fatalf("want 1 breach, got %+v", got)
	}
	b := got[0]
	if b.Axis != "mileage" || b.Miles != 1120 || b.Days != 0 {
		t.Fatalf("want mileage/1120mi/0d, got %+v", b)
	}
	// 1 + 1120/500
	if want := 1 + 1120.0/500.0; b.Urgency != want {
		t.Fatalf("urgency = %v want %v", b.Urgency, want)
	}
}

func TestDueBreaches_mileageUpcoming(t *testing.T) {
	got := DueBreaches(mileageOnly(), base, 34690, DefaultThresholds)
	if len(got) != 1 {
		t.Fatalf("want 1 breach, got %+v", got)
	}
	b := got[0]
	if b.Axis != "mileage" || b.Miles != 310 {
		t.Fatalf("want mileage/310mi remaining, got %+v", b)
	}
	// 1 - 310/500
	if want := 1 - 310.0/500.0; b.Urgency != want {
		t.Fatalf("urgency = %v want %v", b.Urgency, want)
	}
}

func TestDueBreaches_timeOverdue(t *testing.T) {
	due := base.AddDate(0, 12, 0)
	got := DueBreaches(timeOnly(), due.AddDate(0, 0, 12), 0, DefaultThresholds)
	if len(got) != 1 {
		t.Fatalf("want 1 breach, got %+v", got)
	}
	b := got[0]
	if b.Axis != "time" || b.Days != 12 || b.Miles != 0 {
		t.Fatalf("want time/12d/0mi, got %+v", b)
	}
	if want := 1 + 12.0/30.0; b.Urgency != want {
		t.Fatalf("urgency = %v want %v", b.Urgency, want)
	}
}

func TestDueBreaches_timeUpcoming(t *testing.T) {
	due := base.AddDate(0, 12, 0)
	got := DueBreaches(timeOnly(), due.AddDate(0, 0, -20), 0, DefaultThresholds)
	if len(got) != 1 || got[0].Axis != "time" || got[0].Days != 20 {
		t.Fatalf("want time/20d remaining, got %+v", got)
	}
	if want := 1 - 20.0/30.0; got[0].Urgency != want {
		t.Fatalf("urgency = %v want %v", got[0].Urgency, want)
	}
}

func TestDueBreaches_overdueByPartOfADayFloorsToOne(t *testing.T) {
	// "overdue by 0 days" is nonsense in the card copy.
	due := base.AddDate(0, 12, 0)
	got := DueBreaches(timeOnly(), due.Add(2*time.Hour), 0, DefaultThresholds)
	if len(got) != 1 || got[0].Days != 1 {
		t.Fatalf("want a 1-day floor, got %+v", got)
	}
}

func TestDueBreaches_upcomingDueTodayIsZeroDays(t *testing.T) {
	// 0 is legal on the upcoming branch and means "due today".
	due := base.AddDate(0, 12, 0)
	got := DueBreaches(timeOnly(), due, 0, DefaultThresholds)
	if len(got) != 1 || got[0].Axis != "time" || got[0].Days != 0 {
		t.Fatalf("want time/0d, got %+v", got)
	}
}

func TestDueBreaches_hybridBreachingOnBothAxes(t *testing.T) {
	due := base.AddDate(0, 12, 0)
	got := DueBreaches(hybridBoth(), due.AddDate(0, 0, 5), 35600, DefaultThresholds)
	if len(got) != 2 {
		t.Fatalf("want 2 breaches, got %+v", got)
	}
	byAxis := map[string]AxisBreach{}
	for _, b := range got {
		byAxis[b.Axis] = b
	}
	if byAxis["time"].Days != 5 {
		t.Fatalf("time axis = %+v", byAxis["time"])
	}
	if byAxis["mileage"].Miles != 600 {
		t.Fatalf("mileage axis = %+v", byAxis["mileage"])
	}
	// Design D2's worked example: a 600-mile overrun outranks a 5-day overrun.
	if byAxis["mileage"].Urgency <= byAxis["time"].Urgency {
		t.Fatalf("600mi (%v) must outrank 5d (%v)", byAxis["mileage"].Urgency, byAxis["time"].Urgency)
	}
}

func TestDueBreaches_hybridOverdueOnMileageWithHealthyTimeAxisReportsOneBreach(t *testing.T) {
	// The time axis is nowhere near due, so it must not appear at all — and
	// certainly not as an "upcoming in -40 days" second entry.
	got := DueBreaches(hybridBoth(), base.AddDate(0, 1, 0), 36000, DefaultThresholds)
	if len(got) != 1 || got[0].Axis != "mileage" {
		t.Fatalf("want a single mileage breach, got %+v", got)
	}
}

func TestDueBreaches_hybridOverdueOnTimeWithHealthyMileageAxis(t *testing.T) {
	due := base.AddDate(0, 12, 0)
	got := DueBreaches(hybridBoth(), due.AddDate(0, 0, 3), 31000, DefaultThresholds)
	if len(got) != 1 || got[0].Axis != "time" {
		t.Fatalf("want a single time breach, got %+v", got)
	}
}

func TestDueBreaches_upcomingMoreImminentScoresHigher(t *testing.T) {
	// The reason upcoming inverts the ratio: 20mi remaining is more urgent than
	// 490mi remaining, and a naive "largest normalized value" would say the
	// opposite.
	near := DueBreaches(mileageOnly(), base, 34980, DefaultThresholds)
	far := DueBreaches(mileageOnly(), base, 34510, DefaultThresholds)
	if near[0].Urgency <= far[0].Urgency {
		t.Fatalf("20mi (%v) must outrank 490mi (%v)", near[0].Urgency, far[0].Urgency)
	}
}

func TestDueBreaches_agreesWithDueState(t *testing.T) {
	// The invariant the whole design rests on: DueBreaches is non-empty for
	// exactly the schedules DueState calls non-ok. If this ever fails, a card
	// can show a tinted banner with no detail, or a quiet banner on an overdue
	// vehicle.
	sched := hybridBoth()
	for _, days := range []int{0, 30, 300, 335, 360, 366, 400} {
		for _, miles := range []int{0, 30000, 34400, 34600, 35000, 35001, 40000} {
			today := base.AddDate(0, 0, days)
			state := DueState(sched, today, miles, DefaultThresholds)
			breaches := DueBreaches(sched, today, miles, DefaultThresholds)
			if (state == "ok") != (len(breaches) == 0) {
				t.Fatalf("day %d mileage %d: state=%q but %d breaches", days, miles, state, len(breaches))
			}
		}
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

```sh
go test ./apps/fleet-service/internal/maintenanceschedule/ -run TestDueBreaches -v
```

Expected: compile failure — `undefined: DueBreaches`, `undefined: AxisBreach`.

- [ ] **Step 3: Implement `AxisBreach` and `DueBreaches`**

Append to `apps/fleet-service/internal/maintenanceschedule/recurrence.go`, after `DueState`:

```go
// AxisBreach describes one axis of one schedule that is currently breaching,
// with its magnitude and a threshold-normalized urgency.
//
// Urgency is a single monotone scale spanning both due-states, so a max over a
// mixed set always picks the more pressing item:
//
//	overdue:  1 + breach/threshold     (always > 1)
//	upcoming: 1 - remaining/threshold  (always within [0, 1])
//
// "remaining" can never exceed the threshold, because an axis only qualifies as
// upcoming inside the due-soon window, so the two ranges never overlap. Ranking
// upcoming schedules by raw normalized magnitude would surface the LEAST
// imminent one; the inversion is what makes "max urgency" correct in both
// directions.
type AxisBreach struct {
	Axis    string // "time" | "mileage"
	Days    int    // set when Axis == "time"
	Miles   int    // set when Axis == "mileage"
	Urgency float64
}

// DueBreaches returns one entry per axis that is itself breaching. It returns
// nil for a schedule whose DueState is "ok": an ok schedule breaches on no axis,
// so the correspondence holds by construction rather than by an explicit check.
//
// The per-axis conditions are re-tested here rather than inferred from the
// schedule's state. A hybrid schedule is "overdue" when EITHER axis is overdue,
// so the time axis of a mileage-overdue hybrid may be perfectly fine.
//
// The upcoming branches carry an upper bound (today not after nd; mileage not
// above nm) that the matching DueState branches omit. DueState only reaches its
// upcoming branches after the overdue branches have already returned, so the
// bound is implicit there. Here each axis is judged independently, so the bound
// has to be explicit — otherwise a hybrid overdue on mileage would also report
// its time axis as "upcoming in -40 days".
func DueBreaches(s Schedule, today time.Time, currentMileage int, th Thresholds) []AxisBreach {
	nd, nm := NextDue(s)
	timed := s.RecurrenceType == "time" || s.RecurrenceType == "hybrid"
	miled := s.RecurrenceType == "mileage" || s.RecurrenceType == "hybrid"

	var out []AxisBreach

	if timed && today.After(nd) {
		// Anything past the due instant reads as at least a day late in the
		// card copy; "overdue by 0 days" is nonsense.
		days := wholeDays(today.Sub(nd))
		if days < 1 {
			days = 1
		}
		out = append(out, AxisBreach{Axis: "time", Days: days, Urgency: overdueUrgency(days, th.DueSoonDays)})
	} else if timed && !today.Before(nd.AddDate(0, 0, -th.DueSoonDays)) && !today.After(nd) {
		// 0 is legal here and means "due today".
		days := wholeDays(nd.Sub(today))
		out = append(out, AxisBreach{Axis: "time", Days: days, Urgency: upcomingUrgency(days, th.DueSoonDays)})
	}

	if miled && currentMileage > nm {
		miles := currentMileage - nm
		out = append(out, AxisBreach{Axis: "mileage", Miles: miles, Urgency: overdueUrgency(miles, th.DueSoonMiles)})
	} else if miled && currentMileage >= nm-th.DueSoonMiles && currentMileage <= nm {
		miles := nm - currentMileage
		out = append(out, AxisBreach{Axis: "mileage", Miles: miles, Urgency: upcomingUrgency(miles, th.DueSoonMiles)})
	}

	return out
}

// wholeDays truncates a duration to whole days, matching the day granularity the
// card copy speaks in.
func wholeDays(d time.Duration) int { return int(d.Hours() / 24) }

// overdueUrgency and upcomingUrgency are computed from the same integer
// magnitude that is reported, not from the underlying duration, so a test can
// predict the score exactly from the value the user will see.
func overdueUrgency(breach, threshold int) float64 {
	if threshold <= 0 {
		return 1
	}
	return 1 + float64(breach)/float64(threshold)
}

func upcomingUrgency(remaining, threshold int) float64 {
	if threshold <= 0 {
		return 0
	}
	return 1 - float64(remaining)/float64(threshold)
}
```

- [ ] **Step 4: Run the tests to verify they pass**

```sh
go test ./apps/fleet-service/internal/maintenanceschedule/ -v
```

Expected: PASS, including the pre-existing `TestNextDue`, `TestDueState`, `TestSeverity`.

- [ ] **Step 5: Commit**

```sh
git add apps/fleet-service/internal/maintenanceschedule/recurrence.go apps/fleet-service/internal/maintenanceschedule/recurrence_test.go
git commit -m "feat(fleet-service): compute per-axis due breaches with normalized urgency"
```

---

## Task 2: `ScheduleDueByVehicle` — widen the gatherer without adding a query

**Files:**
- Modify: `apps/fleet-service/internal/maintenanceschedule/processor.go`
- Test: `apps/fleet-service/internal/maintenanceschedule/processor_test.go` (create)

**Interfaces:**
- Consumes: `AxisBreach`, `DueBreaches` (Task 1); existing `Provider.ListActiveByVehicle`, `QueueRow`, `DueState`.
- Produces:
  - `type ScheduleDue struct { ScheduleID string; State string; Breaches []AxisBreach }`
  - `func (pr *Processor) ScheduleDueByVehicle(vehicleID string) ([]ScheduleDue, error)`

`ScheduleStatesByVehicle` is **kept** in this task and deleted in Task 4, so the build stays green at every commit. `ListActiveByFleet`, `Queue`, `DueAcrossAllFleets`, and `RecomputeAll` are untouched — the dashboard (FR-9.6) and the reminder feed are unaffected.

- [ ] **Step 1: Write the failing test**

Create `apps/fleet-service/internal/maintenanceschedule/processor_test.go`. The fixture is mileage-only on purpose: `ScheduleDueByVehicle` reads `time.Now()` internally, and a mileage-only schedule makes the assertion independent of when the test runs.

```go
package maintenanceschedule

import (
	"testing"

	"github.com/sirupsen/logrus"

	"github.com/jtumidanski/myfleet/packages/shared-go/server"
)

// countingProvider records how many times each read is issued, so the
// "no additional queries" requirement is measured rather than eyeballed.
type countingProvider struct {
	rows            []QueueRow
	byVehicleCalls  int
	byFleetCalls    int
	listActiveCalls int
}

func (p *countingProvider) GetByID(string) (Model, error) { return Model{}, ErrNotFound }

func (p *countingProvider) ListByVehicle(string, server.Page) ([]Model, int, error) {
	return nil, 0, nil
}

func (p *countingProvider) ListActiveByFleet(string) ([]QueueRow, error) {
	p.byFleetCalls++
	return p.rows, nil
}

func (p *countingProvider) ListActiveByVehicle(string) ([]QueueRow, error) {
	p.byVehicleCalls++
	return p.rows, nil
}

func (p *countingProvider) ListActive() ([]QueueRow, error) {
	p.listActiveCalls++
	return p.rows, nil
}

// mileageSchedule is due at lastMiles+5000 and carries no time axis, so its
// due-state does not depend on the wall clock.
func mileageSchedule(id string, lastMiles int) Model {
	return Model{
		id:                   id,
		vehicleID:            "v1",
		recurrenceType:       "mileage",
		intervalMiles:        5000,
		lastCompletedMileage: lastMiles,
		active:               true,
	}
}

func TestScheduleDueByVehicle_returnsStateAndBreachesPerSchedule(t *testing.T) {
	// All three share a current mileage of 36120; each schedule's own
	// last-completed value is what puts it in a different state.
	p := &countingProvider{rows: []QueueRow{
		{Schedule: mileageSchedule("s-overdue", 30000), CurrentMileage: 36120, FleetID: "f1"},
		{Schedule: mileageSchedule("s-upcoming", 31500), CurrentMileage: 36120, FleetID: "f1"},
		{Schedule: mileageSchedule("s-ok", 36000), CurrentMileage: 36120, FleetID: "f1"},
	}}
	pr := NewProcessor(logrus.New(), p, nil)

	dues, err := pr.ScheduleDueByVehicle("v1")
	if err != nil {
		t.Fatalf("ScheduleDueByVehicle: %v", err)
	}
	if len(dues) != 3 {
		t.Fatalf("want 3 dues, got %d", len(dues))
	}

	// s-overdue: due at 35000, currently 36120 -> 1120mi over.
	if dues[0].ScheduleID != "s-overdue" || dues[0].State != "overdue" {
		t.Fatalf("dues[0] = %+v", dues[0])
	}
	if len(dues[0].Breaches) != 1 || dues[0].Breaches[0].Miles != 1120 {
		t.Fatalf("dues[0].Breaches = %+v", dues[0].Breaches)
	}

	// s-upcoming: due at 36500, currently 36120 -> 380mi remaining, inside the
	// 500-mile window.
	if dues[1].ScheduleID != "s-upcoming" || dues[1].State != "upcoming" {
		t.Fatalf("dues[1] = %+v", dues[1])
	}
	if len(dues[1].Breaches) != 1 || dues[1].Breaches[0].Miles != 380 {
		t.Fatalf("dues[1].Breaches = %+v", dues[1].Breaches)
	}

	// s-ok: due at 41000, currently 36120 -> 4880 remaining, well outside it.
	if dues[2].State != "ok" || len(dues[2].Breaches) != 0 {
		t.Fatalf("an ok schedule must carry no breaches, got %+v", dues[2])
	}
}

func TestScheduleDueByVehicle_upcomingCarriesRemainingDistance(t *testing.T) {
	p := &countingProvider{rows: []QueueRow{
		// due at 35000, currently 34690 -> 310 remaining, inside the 500 window.
		{Schedule: mileageSchedule("s1", 30000), CurrentMileage: 34690, FleetID: "f1"},
	}}
	pr := NewProcessor(logrus.New(), p, nil)

	dues, err := pr.ScheduleDueByVehicle("v1")
	if err != nil {
		t.Fatalf("ScheduleDueByVehicle: %v", err)
	}
	if dues[0].State != "upcoming" {
		t.Fatalf("state = %q want upcoming", dues[0].State)
	}
	if len(dues[0].Breaches) != 1 || dues[0].Breaches[0].Axis != "mileage" || dues[0].Breaches[0].Miles != 310 {
		t.Fatalf("breaches = %+v", dues[0].Breaches)
	}
}

func TestScheduleDueByVehicle_issuesExactlyOneQuery(t *testing.T) {
	// NFR-1 / the acceptance criterion that says "verified by test, not by
	// inspection": widening the return must not widen the read.
	p := &countingProvider{rows: []QueueRow{
		{Schedule: mileageSchedule("s1", 30000), CurrentMileage: 36120, FleetID: "f1"},
		{Schedule: mileageSchedule("s2", 31000), CurrentMileage: 36120, FleetID: "f1"},
	}}
	pr := NewProcessor(logrus.New(), p, nil)

	if _, err := pr.ScheduleDueByVehicle("v1"); err != nil {
		t.Fatalf("ScheduleDueByVehicle: %v", err)
	}
	if p.byVehicleCalls != 1 {
		t.Fatalf("ListActiveByVehicle called %d times, want exactly 1", p.byVehicleCalls)
	}
	if p.byFleetCalls != 0 || p.listActiveCalls != 0 {
		t.Fatalf("no other read may be issued: byFleet=%d listActive=%d", p.byFleetCalls, p.listActiveCalls)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

```sh
go test ./apps/fleet-service/internal/maintenanceschedule/ -run TestScheduleDueByVehicle -v
```

Expected: compile failure — `pr.ScheduleDueByVehicle undefined`.

- [ ] **Step 3: Implement `ScheduleDue` and `ScheduleDueByVehicle`**

In `apps/fleet-service/internal/maintenanceschedule/processor.go`, immediately **above** the existing `ScheduleStatesByVehicle` (around line 163), add:

```go
// ScheduleDue is one active schedule's live due-state plus, when the state is
// non-ok, the per-axis breach detail behind it.
type ScheduleDue struct {
	ScheduleID string
	State      string // ok | upcoming | overdue
	Breaches   []AxisBreach
}

// ScheduleDueByVehicle returns the live DueState of every active schedule for a
// vehicle together with the breach detail behind a non-ok state, computed on
// read from the vehicle's current mileage and now. Used by the vehicle layer to
// derive a vehicle's status and the reason behind it.
//
// This is the same single ListActiveByVehicle read the narrower
// ScheduleStatesByVehicle issued: the breach detail was already in hand and was
// discarded at the return boundary.
//
// State and breach magnitude are both computed from AsSchedule(), never from the
// stored next_due_* columns. Those columns are refreshed by the hourly recompute
// job, so reading one beside a freshly computed state can contradict — a
// schedule completed twenty minutes ago reports "ok" from fresh math while the
// stored next_due_mileage still describes the previous cycle.
func (pr *Processor) ScheduleDueByVehicle(vehicleID string) ([]ScheduleDue, error) {
	rows, err := pr.p.ListActiveByVehicle(vehicleID)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	out := make([]ScheduleDue, 0, len(rows))
	for _, r := range rows {
		s := r.Schedule.AsSchedule()
		out = append(out, ScheduleDue{
			ScheduleID: r.Schedule.ID(),
			State:      DueState(s, now, r.CurrentMileage, DefaultThresholds),
			Breaches:   DueBreaches(s, now, r.CurrentMileage, DefaultThresholds),
		})
	}
	return out, nil
}
```

- [ ] **Step 4: Run the tests to verify they pass**

```sh
go test ./apps/fleet-service/internal/maintenanceschedule/ -v
go build ./...
```

Expected: PASS and a clean build.

- [ ] **Step 5: Commit**

```sh
git add apps/fleet-service/internal/maintenanceschedule/processor.go apps/fleet-service/internal/maintenanceschedule/processor_test.go
git commit -m "feat(fleet-service): return per-schedule due detail from ScheduleDueByVehicle"
```

---

## Task 3: `vehicle` port types and `selectNextDue`

**Files:**
- Create: `apps/fleet-service/internal/vehicle/nextdue.go`
- Test: `apps/fleet-service/internal/vehicle/nextdue_test.go` (create)

**Interfaces:**
- Consumes: nothing from earlier tasks at compile time — the port shape is declared independently, on purpose.
- Produces (all in package `vehicle`):
  - `type ScheduleDue struct { ScheduleID string; State string; Breaches []Breach }`
  - `type Breach struct { Axis string; Days int; Miles int; Urgency float64 }`
  - `type NextDue struct { State string; Axis string; Miles *int; Days *int }` with JSON tags `state`, `axis`, `miles,omitempty`, `days,omitempty`
  - `type ScheduleDueGatherer interface { ScheduleDueByVehicle(vehicleID string) ([]ScheduleDue, error) }`
  - `func selectNextDue(dues []ScheduleDue) *NextDue` (unexported)

Purely additive to the `vehicle` package — `StatusDeps` is not touched until Task 4, so the build stays green.

- [ ] **Step 1: Write the failing test**

Create `apps/fleet-service/internal/vehicle/nextdue_test.go`:

```go
package vehicle

import "testing"

func mileageBreach(miles int, urgency float64) Breach {
	return Breach{Axis: "mileage", Miles: miles, Urgency: urgency}
}

func timeBreach(days int, urgency float64) Breach {
	return Breach{Axis: "time", Days: days, Urgency: urgency}
}

func TestSelectNextDue(t *testing.T) {
	cases := []struct {
		name      string
		dues      []ScheduleDue
		wantNil   bool
		wantState string
		wantAxis  string
		wantMiles *int
		wantDays  *int
	}{
		{
			name:    "no schedules at all",
			dues:    nil,
			wantNil: true,
		},
		{
			name: "every schedule ok",
			dues: []ScheduleDue{
				{ScheduleID: "s1", State: "ok"},
				{ScheduleID: "s2", State: "ok"},
			},
			wantNil: true,
		},
		{
			name: "single mileage overdue",
			dues: []ScheduleDue{
				{ScheduleID: "s1", State: "overdue", Breaches: []Breach{mileageBreach(1120, 3.24)}},
			},
			wantState: "overdue",
			wantAxis:  "mileage",
			wantMiles: intPtr(1120),
		},
		{
			name: "single time upcoming due today keeps a zero day count",
			dues: []ScheduleDue{
				{ScheduleID: "s1", State: "upcoming", Breaches: []Breach{timeBreach(0, 1.0)}},
			},
			wantState: "upcoming",
			wantAxis:  "time",
			wantDays:  intPtr(0),
		},
		{
			name: "overdue governs even when an upcoming schedule scores high on its own scale",
			dues: []ScheduleDue{
				{ScheduleID: "s-up", State: "upcoming", Breaches: []Breach{mileageBreach(5, 0.99)}},
				{ScheduleID: "s-od", State: "overdue", Breaches: []Breach{timeBreach(2, 1.0667)}},
			},
			wantState: "overdue",
			wantAxis:  "time",
			wantDays:  intPtr(2),
		},
		{
			name: "max urgency wins across several overdue schedules",
			dues: []ScheduleDue{
				{ScheduleID: "s1", State: "overdue", Breaches: []Breach{timeBreach(5, 1 + 5.0/30.0)}},
				{ScheduleID: "s2", State: "overdue", Breaches: []Breach{mileageBreach(600, 1 + 600.0/500.0)}},
			},
			wantState: "overdue",
			wantAxis:  "mileage",
			wantMiles: intPtr(600),
		},
		{
			name: "a hybrid overdue on mileage also carrying an upcoming time breach reports the overdue axis",
			dues: []ScheduleDue{
				{ScheduleID: "s1", State: "overdue", Breaches: []Breach{
					mileageBreach(600, 1 + 600.0/500.0),
					timeBreach(9, 1 - 9.0/30.0),
				}},
			},
			wantState: "overdue",
			wantAxis:  "mileage",
			wantMiles: intPtr(600),
		},
		{
			name: "mileage wins the tiebreak on equal urgency",
			dues: []ScheduleDue{
				{ScheduleID: "s1", State: "overdue", Breaches: []Breach{timeBreach(30, 2.0)}},
				{ScheduleID: "s2", State: "overdue", Breaches: []Breach{mileageBreach(500, 2.0)}},
			},
			wantState: "overdue",
			wantAxis:  "mileage",
			wantMiles: intPtr(500),
		},
		{
			name: "lowest schedule id breaks a tie on equal urgency and axis",
			dues: []ScheduleDue{
				{ScheduleID: "s-b", State: "overdue", Breaches: []Breach{mileageBreach(700, 2.4)}},
				{ScheduleID: "s-a", State: "overdue", Breaches: []Breach{mileageBreach(700, 2.4)}},
			},
			wantState: "overdue",
			wantAxis:  "mileage",
			wantMiles: intPtr(700),
		},
		{
			name: "most imminent upcoming wins",
			dues: []ScheduleDue{
				{ScheduleID: "s1", State: "upcoming", Breaches: []Breach{mileageBreach(490, 1 - 490.0/500.0)}},
				{ScheduleID: "s2", State: "upcoming", Breaches: []Breach{mileageBreach(20, 1 - 20.0/500.0)}},
			},
			wantState: "upcoming",
			wantAxis:  "mileage",
			wantMiles: intPtr(20),
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := selectNextDue(c.dues)
			if c.wantNil {
				if got != nil {
					t.Fatalf("want nil, got %+v", got)
				}
				return
			}
			if got == nil {
				t.Fatal("want a NextDue, got nil")
			}
			if got.State != c.wantState || got.Axis != c.wantAxis {
				t.Fatalf("state/axis = %q/%q want %q/%q", got.State, got.Axis, c.wantState, c.wantAxis)
			}
			assertIntPtr(t, "miles", got.Miles, c.wantMiles)
			assertIntPtr(t, "days", got.Days, c.wantDays)
		})
	}
}

func TestSelectNextDue_setsExactlyOneMagnitudePointer(t *testing.T) {
	// The invariant the nested-object encoding exists to protect: an axis:"time"
	// object must never carry a miles value, and vice versa.
	mileage := selectNextDue([]ScheduleDue{
		{ScheduleID: "s1", State: "overdue", Breaches: []Breach{mileageBreach(100, 1.2)}},
	})
	if mileage.Miles == nil || mileage.Days != nil {
		t.Fatalf("mileage axis must set Miles and leave Days nil, got %+v", mileage)
	}

	timed := selectNextDue([]ScheduleDue{
		{ScheduleID: "s1", State: "overdue", Breaches: []Breach{timeBreach(10, 1.3)}},
	})
	if timed.Days == nil || timed.Miles != nil {
		t.Fatalf("time axis must set Days and leave Miles nil, got %+v", timed)
	}
}

func TestSelectNextDue_nonOkStateWithNoBreachesYieldsNil(t *testing.T) {
	// Defensive: a state/breach disagreement must not produce a NextDue with no
	// magnitude at all. It cannot happen through the real gatherer (Task 1's
	// agreement test proves it), but selectNextDue is fed across a port.
	if got := selectNextDue([]ScheduleDue{{ScheduleID: "s1", State: "overdue"}}); got != nil {
		t.Fatalf("want nil, got %+v", got)
	}
}

func intPtr(n int) *int { return &n }

func assertIntPtr(t *testing.T, name string, got, want *int) {
	t.Helper()
	switch {
	case want == nil && got != nil:
		t.Fatalf("%s = %d, want nil", name, *got)
	case want != nil && got == nil:
		t.Fatalf("%s = nil, want %d", name, *want)
	case want != nil && *got != *want:
		t.Fatalf("%s = %d, want %d", name, *got, *want)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

```sh
go test ./apps/fleet-service/internal/vehicle/ -run TestSelectNextDue -v
```

Expected: compile failure — `undefined: ScheduleDue`, `undefined: Breach`, `undefined: selectNextDue`.

- [ ] **Step 3: Implement the port types and `selectNextDue`**

Create `apps/fleet-service/internal/vehicle/nextdue.go`:

```go
package vehicle

// ScheduleDue mirrors maintenanceschedule.ScheduleDue across the domain
// boundary. The shape is duplicated rather than aliased on purpose: an alias
// would make this package import maintenanceschedule transitively, which is the
// exact coupling the port exists to prevent. The mapping lives in the
// composition root (cmd/main.go), field for field, so a change on either side is
// a compile error rather than a silent drop.
type ScheduleDue struct {
	ScheduleID string
	State      string // ok | upcoming | overdue
	Breaches   []Breach
}

// Breach mirrors maintenanceschedule.AxisBreach. Urgency arrives already
// normalized against the schedule domain's own thresholds, so this package
// selects a maximum without ever learning what a threshold is.
type Breach struct {
	Axis    string // "time" | "mileage"
	Days    int
	Miles   int
	Urgency float64
}

// NextDue is the single governing due detail exposed on the vehicle resource.
// Exactly one of Miles/Days is non-nil, chosen by Axis.
//
// Pointers rather than plain ints with omitempty: an upcoming time-axis schedule
// due today has Days == 0, and omitempty on a plain int would drop the key and
// hand the client an axis:"time" object carrying no magnitude at all.
type NextDue struct {
	State string `json:"state"`           // upcoming | overdue
	Axis  string `json:"axis"`            // time | mileage
	Miles *int   `json:"miles,omitempty"` // non-nil iff Axis == "mileage"
	Days  *int   `json:"days,omitempty"`  // non-nil iff Axis == "time"
}

// ScheduleDueGatherer returns the live due-state and per-axis breach detail of
// every active maintenance schedule for a vehicle. Injected (read-only) so the
// vehicle layer can derive on read without owning schedule internals. Satisfied
// by an adapter over *maintenanceschedule.Processor, wired in cmd/main.go.
type ScheduleDueGatherer interface {
	ScheduleDueByVehicle(vehicleID string) ([]ScheduleDue, error)
}

// selectNextDue picks the single breach that explains the vehicle's status: the
// governing state first (overdue beats upcoming — the same priority
// status.Derive applies), then the highest-urgency breach among the schedules in
// that state.
//
// Because Urgency is monotone across the state boundary (overdue is always above
// 1, upcoming always at or below it), an overdue hybrid that also carries an
// upcoming breach on its healthy axis reports the overdue axis with no extra
// filtering.
//
// Ties resolve to the mileage axis, then to the lowest schedule ID. The second
// tiebreak exists because the first is not total: two mileage schedules with
// identical breaches would otherwise be ordered by slice iteration.
//
// Returns nil when no schedule is non-ok, and also when a non-ok schedule
// carries no breach at all — reporting a state with no magnitude would give the
// card a tinted banner it cannot caption.
func selectNextDue(dues []ScheduleDue) *NextDue {
	governing := ""
	for _, d := range dues {
		if d.State == "overdue" {
			governing = "overdue"
			break
		}
		if d.State == "upcoming" {
			governing = "upcoming"
		}
	}
	if governing == "" {
		return nil
	}

	var best Breach
	bestID := ""
	found := false
	for _, d := range dues {
		if d.State != governing {
			continue
		}
		for _, b := range d.Breaches {
			if !found || outranks(b, d.ScheduleID, best, bestID) {
				best, bestID, found = b, d.ScheduleID, true
			}
		}
	}
	if !found {
		return nil
	}

	out := &NextDue{State: governing, Axis: best.Axis}
	switch best.Axis {
	case "mileage":
		miles := best.Miles
		out.Miles = &miles
	case "time":
		days := best.Days
		out.Days = &days
	}
	return out
}

// outranks reports whether candidate c, from schedule cID, beats the incumbent b
// from schedule bID: higher urgency, then the mileage axis, then the lower ID.
func outranks(c Breach, cID string, b Breach, bID string) bool {
	if c.Urgency != b.Urgency {
		return c.Urgency > b.Urgency
	}
	if (c.Axis == "mileage") != (b.Axis == "mileage") {
		return c.Axis == "mileage"
	}
	return cID < bID
}
```

- [ ] **Step 4: Run the tests to verify they pass**

```sh
go test ./apps/fleet-service/internal/vehicle/ -v
go build ./...
```

Expected: PASS, clean build.

- [ ] **Step 5: Commit**

```sh
git add apps/fleet-service/internal/vehicle/nextdue.go apps/fleet-service/internal/vehicle/nextdue_test.go
git commit -m "feat(fleet-service): select the governing due breach in the vehicle domain"
```

---

## Task 4: `StatusDeps.Derive` — one call, three values

**Files:**
- Modify: `apps/fleet-service/internal/vehicle/status.go`
- Modify: `apps/fleet-service/internal/vehicle/resource.go:52` and `:120`
- Modify: `apps/fleet-service/internal/maintenanceschedule/processor.go` (delete `ScheduleStatesByVehicle`)
- Modify: `apps/fleet-service/cmd/main.go`
- Test: `apps/fleet-service/internal/vehicle/status_test.go` (create)

**Interfaces:**
- Consumes: `ScheduleDue`, `Breach`, `NextDue`, `ScheduleDueGatherer`, `selectNextDue` (Task 3); `maintenanceschedule.ScheduleDueByVehicle` (Task 2).
- Produces:
  - `type Derived struct { Status string; LastActivityAt time.Time; NextDue *NextDue }`
  - `func (d StatusDeps) Derive(m Model, now time.Time) Derived`
  - `StatusDeps.Schedules` retyped from `ScheduleStateGatherer` to `ScheduleDueGatherer`
- Deleted: `vehicle.ScheduleStateGatherer`, `vehicle.StatusDeps.DeriveStatus`, `maintenanceschedule.Processor.ScheduleStatesByVehicle`

These move together because they are one compile unit: retyping the port breaks `main.go`'s structural binding and `resource.go`'s call sites simultaneously. `resource.go` gets the minimal edit here (`.Status` off the new struct); Task 5 rewrites those lines properly.

- [ ] **Step 1: Write the failing test**

Create `apps/fleet-service/internal/vehicle/status_test.go`:

```go
package vehicle

import (
	"errors"
	"testing"
	"time"
)

var statusNow = time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC)

type fakeSchedules struct {
	dues []ScheduleDue
	err  error
}

func (f fakeSchedules) ScheduleDueByVehicle(string) ([]ScheduleDue, error) { return f.dues, f.err }

type fakeActivity struct {
	at  time.Time
	err error
}

func (f fakeActivity) LastActivityByVehicle(string) (time.Time, error) { return f.at, f.err }

// testVehicle builds a model directly; the builder has no created-at setter and
// these tests are in-package.
func testVehicle(createdAt time.Time) Model {
	return Model{id: "v1", fleetID: "f1", make: "Honda", model: "Civic", year: 2019, createdAt: createdAt}
}

func TestDerive_scheduleGatherErrorYieldsZeroDerived(t *testing.T) {
	d := StatusDeps{
		Schedules: fakeSchedules{err: errors.New("boom")},
		Activity:  fakeActivity{at: statusNow},
	}
	got := d.Derive(testVehicle(statusNow), statusNow)
	if got.Status != "" || !got.LastActivityAt.IsZero() || got.NextDue != nil {
		t.Fatalf("a gather error must omit every derived value, got %+v", got)
	}
}

func TestDerive_activityGatherErrorYieldsZeroDerived(t *testing.T) {
	d := StatusDeps{
		Schedules: fakeSchedules{dues: []ScheduleDue{{ScheduleID: "s1", State: "overdue", Breaches: []Breach{{Axis: "mileage", Miles: 100, Urgency: 1.2}}}}},
		Activity:  fakeActivity{err: errors.New("boom")},
	}
	got := d.Derive(testVehicle(statusNow), statusNow)
	if got.Status != "" || !got.LastActivityAt.IsZero() || got.NextDue != nil {
		t.Fatalf("a gather error must omit every derived value, got %+v", got)
	}
}

func TestDerive_reportsStatusLastActivityAndGoverningBreach(t *testing.T) {
	last := statusNow.AddDate(0, 0, -6)
	d := StatusDeps{
		Schedules: fakeSchedules{dues: []ScheduleDue{
			{ScheduleID: "s1", State: "ok"},
			{ScheduleID: "s2", State: "overdue", Breaches: []Breach{{Axis: "mileage", Miles: 1120, Urgency: 3.24}}},
		}},
		Activity: fakeActivity{at: last},
	}
	got := d.Derive(testVehicle(statusNow.AddDate(-2, 0, 0)), statusNow)

	if got.Status != "Overdue" {
		t.Fatalf("status = %q want Overdue", got.Status)
	}
	if !got.LastActivityAt.Equal(last) {
		t.Fatalf("lastActivityAt = %v want %v", got.LastActivityAt, last)
	}
	if got.NextDue == nil || got.NextDue.Axis != "mileage" || got.NextDue.Miles == nil || *got.NextDue.Miles != 1120 {
		t.Fatalf("nextDue = %+v", got.NextDue)
	}
}

func TestDerive_fallsBackToCreatedAtWhenNoActivityRecorded(t *testing.T) {
	// A brand-new vehicle with no activity must be Healthy, not Inactive, and
	// the exposed timestamp is the post-fallback value — so the card can never
	// show an em-dash beside a status computed from a real timestamp.
	created := statusNow.AddDate(0, 0, -3)
	d := StatusDeps{
		Schedules: fakeSchedules{dues: nil},
		Activity:  fakeActivity{at: time.Time{}},
	}
	got := d.Derive(testVehicle(created), statusNow)

	if got.Status != "Healthy" {
		t.Fatalf("status = %q want Healthy", got.Status)
	}
	if !got.LastActivityAt.Equal(created) {
		t.Fatalf("lastActivityAt = %v want the created-at fallback %v", got.LastActivityAt, created)
	}
}

func TestDerive_zeroCreatedAtLeavesLastActivityZero(t *testing.T) {
	// The task-006 case: a wiped created_at must surface as an omitted attribute
	// (an em-dash on the card), never as an absurd year-1 date.
	d := StatusDeps{
		Schedules: fakeSchedules{dues: nil},
		Activity:  fakeActivity{at: time.Time{}},
	}
	got := d.Derive(testVehicle(time.Time{}), statusNow)
	if !got.LastActivityAt.IsZero() {
		t.Fatalf("lastActivityAt = %v want zero", got.LastActivityAt)
	}
}

func TestDerive_inactiveNeverCarriesDueDetail(t *testing.T) {
	// Design D7: status.Derive only reaches Inactive after both the overdue and
	// upcoming scans fall through, so an Inactive vehicle has no non-ok schedule
	// by construction. Asserted rather than defended against, so a future change
	// to Derive that breaks the invariant fails loudly instead of producing an
	// Inactive card with a tinted banner.
	d := StatusDeps{
		Schedules: fakeSchedules{dues: []ScheduleDue{{ScheduleID: "s1", State: "ok"}}},
		Activity:  fakeActivity{at: statusNow.AddDate(0, 0, -400)},
	}
	got := d.Derive(testVehicle(statusNow.AddDate(-3, 0, 0)), statusNow)
	if got.Status != "Inactive" {
		t.Fatalf("status = %q want Inactive", got.Status)
	}
	if got.NextDue != nil {
		t.Fatalf("Inactive must carry no due detail, got %+v", got.NextDue)
	}
}

func TestDerive_statusValuesAreUnchanged(t *testing.T) {
	// NFR-16: widening the gatherer must not move a single status value. Each row
	// is the status today's DeriveStatus produced for the same inputs.
	cases := []struct {
		name       string
		states     []string
		lastOffset int // days before statusNow
		want       string
	}{
		{"overdue beats everything", []string{"ok", "upcoming", "overdue"}, 1, "Overdue"},
		{"upcoming beats healthy", []string{"ok", "upcoming"}, 1, "Upcoming Maintenance"},
		{"no schedules, recent activity", nil, 10, "Healthy"},
		{"no schedules, stale activity", nil, 400, "Inactive"},
		{"all ok, recent activity", []string{"ok", "ok"}, 10, "Healthy"},
		{"all ok, stale activity", []string{"ok"}, 400, "Inactive"},
		{"upcoming outranks inactivity", []string{"upcoming"}, 400, "Upcoming Maintenance"},
		{"overdue outranks inactivity", []string{"overdue"}, 400, "Overdue"},
		{"exactly at the 365-day boundary is still Healthy", nil, 365, "Healthy"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dues := make([]ScheduleDue, 0, len(c.states))
			for i, s := range c.states {
				dues = append(dues, ScheduleDue{ScheduleID: string(rune('a' + i)), State: s})
			}
			d := StatusDeps{
				Schedules: fakeSchedules{dues: dues},
				Activity:  fakeActivity{at: statusNow.AddDate(0, 0, -c.lastOffset)},
			}
			if got := d.Derive(testVehicle(statusNow), statusNow).Status; got != c.want {
				t.Fatalf("status = %q want %q", got, c.want)
			}
		})
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

```sh
go test ./apps/fleet-service/internal/vehicle/ -run TestDerive -v
```

Expected: compile failure — `fakeSchedules does not implement ScheduleStateGatherer`, `d.Derive undefined`.

- [ ] **Step 3: Rewrite `status.go`**

Replace the body of `apps/fleet-service/internal/vehicle/status.go` from the `ScheduleStateGatherer` declaration onward. The `inactivityDays` constant and the `LastActivityGatherer` interface are unchanged. The file becomes:

```go
package vehicle

import (
	"time"

	"github.com/jtumidanski/myfleet/apps/fleet-service/internal/status"
)

// inactivityDays is the threshold past which a vehicle with no recent activity
// is considered "Inactive" (design §10.2).
const inactivityDays = 365

// LastActivityGatherer returns the most-recent activity timestamp for a vehicle
// (design §8.2). Satisfied by an adapter over *activity.Processor; falls back to
// the vehicle's created_at when there is no recorded activity.
type LastActivityGatherer interface {
	LastActivityByVehicle(vehicleID string) (time.Time, error)
}

// StatusDeps bundles the read-only accessors needed to derive a vehicle's
// read-time values. Both are injected from main.go (cross-domain) so the vehicle
// resource never calls another domain's internals directly.
type StatusDeps struct {
	Schedules ScheduleDueGatherer
	Activity  LastActivityGatherer
}

// Derived is everything the vehicle resource exposes that is computed on read
// rather than stored. A zero Derived means a gather failed; every derived
// attribute is then omitted from the response and the read still succeeds.
type Derived struct {
	Status         string    // "" when a gather failed
	LastActivityAt time.Time // zero when unavailable
	NextDue        *NextDue  // nil when no schedule is non-ok
}

// Derive computes a vehicle's read-only values in one pass: it gathers the
// vehicle's active schedules' due detail and its last activity time, applies the
// priority-ordered rule in status.Derive, and selects the single breach that
// explains the resulting status.
//
// On any gather error it returns the zero Derived, so the caller omits the
// attributes rather than failing the read. That is unchanged behaviour, applied
// to three values instead of one.
func (d StatusDeps) Derive(m Model, now time.Time) Derived {
	dues, err := d.Schedules.ScheduleDueByVehicle(m.ID())
	if err != nil {
		return Derived{}
	}
	last, err := d.Activity.LastActivityByVehicle(m.ID())
	if err != nil {
		return Derived{}
	}
	// Guard against a missing activity record: fall back to the vehicle's
	// creation time so a brand-new vehicle is "Healthy", not "Inactive". The
	// exposed timestamp is this post-fallback value — the same one status
	// derivation used — so the card can never show an em-dash beside a status
	// computed from a real timestamp. A vehicle whose created_at is also zero
	// leaves the field zero, and the attribute is omitted.
	if last.IsZero() {
		last = m.CreatedAt()
	}
	return Derived{
		Status: status.Derive(status.Input{
			ScheduleStates: scheduleStates(dues),
			LastActivityAt: last,
			Now:            now,
			InactivityDays: inactivityDays,
		}),
		LastActivityAt: last,
		NextDue:        selectNextDue(dues),
	}
}

// scheduleStates projects the widened due detail back down to the plain state
// strings status.Derive consumes. status.Derive's input, rule, and output are
// deliberately untouched: this projection is the only new code on the status
// path, which is what makes "status values are unchanged" a testable claim
// rather than a hope.
func scheduleStates(dues []ScheduleDue) []string {
	out := make([]string, 0, len(dues))
	for _, d := range dues {
		out = append(out, d.State)
	}
	return out
}
```

- [ ] **Step 4: Update the two `resource.go` call sites (minimal edit)**

In `apps/fleet-service/internal/vehicle/resource.go`, line 52:

```go
				resources = append(resources, TransformWithStatus(m, statusDeps.Derive(m, now).Status))
```

and line 120:

```go
			st := statusDeps.Derive(m, time.Now().UTC()).Status
```

Task 5 replaces both with `TransformDerived`; this keeps the tree compiling in between.

- [ ] **Step 5: Delete `ScheduleStatesByVehicle`**

In `apps/fleet-service/internal/maintenanceschedule/processor.go`, delete the whole `ScheduleStatesByVehicle` method and its doc comment (the block starting `// ScheduleStatesByVehicle returns the live DueState`). It had exactly one consumer; leaving both would create two paths to the same rows.

- [ ] **Step 6: Add the composition-root adapter**

In `apps/fleet-service/cmd/main.go`, change the `vehicleStatusDeps` binding (around line 122) to use the adapter:

```go
	// Read-only accessors for deriving a vehicle's status, last activity, and
	// governing due detail on read (design §10.2). Schedule detail comes from the
	// schedule processor through an adapter; last activity comes from the
	// activity domain (falls back to vehicle created_at).
	vehicleStatusDeps := vehicle.StatusDeps{
		Schedules: scheduleDueAdapter{p: scheduleProc},
		Activity:  activityProc,
	}
```

and add at the **bottom of the file**:

```go
// scheduleDueAdapter maps maintenanceschedule's due detail onto the vehicle
// domain's port type. The mapping lives here, in the composition root, so
// neither domain imports the other; a field added on one side becomes a compile
// error here rather than a silently dropped value.
//
// The previous binding worked by structural typing because the gatherer returned
// a []string. With named struct types on both sides it cannot, which is the
// point: the boundary is now explicit.
type scheduleDueAdapter struct{ p *maintenanceschedule.Processor }

func (a scheduleDueAdapter) ScheduleDueByVehicle(vehicleID string) ([]vehicle.ScheduleDue, error) {
	dues, err := a.p.ScheduleDueByVehicle(vehicleID)
	if err != nil {
		return nil, err
	}
	out := make([]vehicle.ScheduleDue, 0, len(dues))
	for _, d := range dues {
		breaches := make([]vehicle.Breach, 0, len(d.Breaches))
		for _, b := range d.Breaches {
			breaches = append(breaches, vehicle.Breach{
				Axis:    b.Axis,
				Days:    b.Days,
				Miles:   b.Miles,
				Urgency: b.Urgency,
			})
		}
		out = append(out, vehicle.ScheduleDue{
			ScheduleID: d.ScheduleID,
			State:      d.State,
			Breaches:   breaches,
		})
	}
	return out, nil
}
```

`maintenanceschedule` and `vehicle` are already imported in `main.go`; no import changes are needed.

- [ ] **Step 7: Verify the boundary is intact**

```sh
go list -deps github.com/jtumidanski/myfleet/apps/fleet-service/internal/vehicle | grep maintenanceschedule
```

Expected: **no output**. If this prints anything, FR-9.5 is violated — find the import and remove it.

- [ ] **Step 8: Run the tests to verify they pass**

```sh
go build ./...
go vet ./...
go test -race ./apps/fleet-service/...
```

Expected: PASS across the whole service, including `dashboard` (which uses `ListActiveByFleet` and is untouched).

- [ ] **Step 9: Commit**

```sh
git add apps/fleet-service/internal/vehicle/status.go apps/fleet-service/internal/vehicle/status_test.go apps/fleet-service/internal/vehicle/resource.go apps/fleet-service/internal/maintenanceschedule/processor.go apps/fleet-service/cmd/main.go
git commit -m "feat(fleet-service): derive status, last activity, and due detail in one pass"
```

---

## Task 5: Expose `lastActivityAt` and `nextDue` on the vehicle resource

**Files:**
- Modify: `apps/fleet-service/internal/vehicle/rest.go`
- Modify: `apps/fleet-service/internal/vehicle/resource.go` (two read sites; POST and PATCH bind named types)
- Test: `apps/fleet-service/internal/vehicle/rest_test.go` (create)

**Interfaces:**
- Consumes: `Derived`, `NextDue` (Tasks 3–4).
- Produces:
  - `Attributes.LastActivityAt string` (`json:"lastActivityAt,omitempty"`), `Attributes.NextDue *NextDue` (`json:"nextDue,omitempty"`)
  - `func TransformDerived(m Model, d Derived) server.Resource`
  - `type createAttributes struct { … }` and `type patchAttributes struct { … }` — the named binding types the handlers use
- Deleted: `TransformWithStatus`

- [ ] **Step 1: Write the failing test**

Create `apps/fleet-service/internal/vehicle/rest_test.go`:

```go
package vehicle

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"
)

func restVehicle() Model {
	return Model{
		id: "v1", fleetID: "f1", make: "Honda", model: "Civic", year: 2019,
		currentMileage: 42000,
	}
}

// attributesJSON marshals a resource and returns its attributes object as a map,
// so absence of a key is distinguishable from a zero value.
func attributesJSON(t *testing.T, r interface{}) map[string]any {
	t.Helper()
	raw, err := json.Marshal(r)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var doc struct {
		Attributes map[string]any `json:"attributes"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return doc.Attributes
}

func TestTransformDerived_omitsBothAttributesWhenDerivedIsZero(t *testing.T) {
	attrs := attributesJSON(t, TransformDerived(restVehicle(), Derived{}))
	if _, ok := attrs["lastActivityAt"]; ok {
		t.Fatalf("lastActivityAt must be omitted, got %v", attrs["lastActivityAt"])
	}
	if _, ok := attrs["nextDue"]; ok {
		t.Fatalf("nextDue must be omitted, got %v", attrs["nextDue"])
	}
	if _, ok := attrs["status"]; ok {
		t.Fatalf("status must be omitted, got %v", attrs["status"])
	}
}

func TestTransformDerived_lastActivityIsRFC3339UTC(t *testing.T) {
	last := time.Date(2026, 4, 2, 14, 31, 7, 0, time.UTC)
	attrs := attributesJSON(t, TransformDerived(restVehicle(), Derived{Status: "Healthy", LastActivityAt: last}))
	if attrs["lastActivityAt"] != "2026-04-02T14:31:07Z" {
		t.Fatalf("lastActivityAt = %v", attrs["lastActivityAt"])
	}
}

func TestTransformDerived_lastActivityIsNormalisedToUTC(t *testing.T) {
	// A non-UTC timestamp reaching the transport must still serialise as UTC, so
	// the client never has to reason about a server's local zone.
	zone := time.FixedZone("EST", -5*60*60)
	last := time.Date(2026, 4, 2, 9, 31, 7, 0, zone)
	attrs := attributesJSON(t, TransformDerived(restVehicle(), Derived{LastActivityAt: last}))
	if attrs["lastActivityAt"] != "2026-04-02T14:31:07Z" {
		t.Fatalf("lastActivityAt = %v", attrs["lastActivityAt"])
	}
}

func TestTransformDerived_nextDueEmitsMilesXorDays(t *testing.T) {
	miles := 1120
	attrs := attributesJSON(t, TransformDerived(restVehicle(), Derived{
		Status:  "Overdue",
		NextDue: &NextDue{State: "overdue", Axis: "mileage", Miles: &miles},
	}))
	nd, ok := attrs["nextDue"].(map[string]any)
	if !ok {
		t.Fatalf("nextDue = %v", attrs["nextDue"])
	}
	if nd["state"] != "overdue" || nd["axis"] != "mileage" || nd["miles"] != float64(1120) {
		t.Fatalf("nextDue = %v", nd)
	}
	if _, present := nd["days"]; present {
		t.Fatalf("a mileage axis must not emit days, got %v", nd)
	}
}

func TestTransformDerived_nextDueKeepsAZeroDayCount(t *testing.T) {
	// The reason Days is a pointer: "due today" is days == 0, and omitempty on a
	// plain int would drop the key and leave an axis:"time" object with no
	// magnitude at all.
	days := 0
	attrs := attributesJSON(t, TransformDerived(restVehicle(), Derived{
		Status:  "Upcoming Maintenance",
		NextDue: &NextDue{State: "upcoming", Axis: "time", Days: &days},
	}))
	nd := attrs["nextDue"].(map[string]any)
	if nd["days"] != float64(0) {
		t.Fatalf("days must be present and zero, got %v", nd)
	}
	if _, present := nd["miles"]; present {
		t.Fatalf("a time axis must not emit miles, got %v", nd)
	}
}

func TestTransform_writePathEmitsNoDerivedAttributes(t *testing.T) {
	// Create/update/restore/primary-image responses echo a write, and none of the
	// derived values is a property of the write.
	attrs := attributesJSON(t, Transform(restVehicle()))
	for _, k := range []string{"status", "lastActivityAt", "nextDue"} {
		if _, ok := attrs[k]; ok {
			t.Fatalf("write path must not emit %q, got %v", k, attrs[k])
		}
	}
}

func TestCreateAndPatchBindingsRejectDerivedAttributes(t *testing.T) {
	// FR-8.3 / NFR-7: a client must not be able to write a derived value. The
	// handlers bind these narrow types, which simply have nowhere to put the
	// derived keys — asserted against the real types rather than trusted.
	body := `{"nickname":"Mine","currentMileage":1,"status":"Healthy",` +
		`"lastActivityAt":"2020-01-01T00:00:00Z","nextDue":{"state":"overdue","axis":"mileage","miles":9}}`

	var create createAttributes
	if err := json.Unmarshal([]byte(body), &create); err != nil {
		t.Fatalf("unmarshal create: %v", err)
	}
	var patch patchAttributes
	if err := json.Unmarshal([]byte(body), &patch); err != nil {
		t.Fatalf("unmarshal patch: %v", err)
	}

	for _, typ := range []reflect.Type{reflect.TypeOf(create), reflect.TypeOf(patch)} {
		for i := 0; i < typ.NumField(); i++ {
			tag := strings.Split(typ.Field(i).Tag.Get("json"), ",")[0]
			switch tag {
			case "status", "lastActivityAt", "nextDue":
				t.Fatalf("%s must not accept a derived attribute %q", typ.Name(), tag)
			}
		}
	}

	// And the fields that DO exist still bind, so the assertion above is not
	// passing because the types are empty.
	if create.Nickname != "Mine" || create.CurrentMileage != 1 {
		t.Fatalf("create bindings broke: %+v", create)
	}
	if patch.Nickname == nil || *patch.Nickname != "Mine" {
		t.Fatalf("patch bindings broke: %+v", patch)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

```sh
go test ./apps/fleet-service/internal/vehicle/ -run 'TestTransform|TestCreateAndPatch' -v
```

Expected: compile failure — `undefined: TransformDerived`, `undefined: createAttributes`, `undefined: patchAttributes`.

- [ ] **Step 3: Rewrite `rest.go`**

Replace `apps/fleet-service/internal/vehicle/rest.go` with:

```go
package vehicle

import (
	"time"

	"github.com/jtumidanski/myfleet/packages/shared-go/server"
)

// Attributes is the JSON:API attributes payload for a vehicle.
//
// Status, LastActivityAt, and NextDue are all DERIVED ON READ (design §10.2) and
// never stored on the entity. They are computed from the vehicle's active
// maintenance-schedule due detail and its last activity time, and are exposed
// read-only here.
type Attributes struct {
	FleetID             string   `json:"fleetId"`
	Nickname            string   `json:"nickname,omitempty"`
	Make                string   `json:"make"`
	Model               string   `json:"model"`
	Trim                string   `json:"trim,omitempty"`
	Year                int      `json:"year"`
	VIN                 string   `json:"vin,omitempty"`
	CurrentMileage      int      `json:"currentMileage,omitempty"`
	PrimaryImageMediaID string   `json:"primaryImageMediaId,omitempty"`
	Notes               string   `json:"notes,omitempty"`
	Status              string   `json:"status,omitempty"`
	LastActivityAt      string   `json:"lastActivityAt,omitempty"` // RFC 3339, UTC
	NextDue             *NextDue `json:"nextDue,omitempty"`
}

// createAttributes is the exact set of fields POST /fleets/{id}/vehicles accepts.
// Named rather than anonymous so a test can assert that no derived attribute is
// bindable — this narrow shape IS the read-only enforcement (FR-8.3, NFR-7): an
// unknown lastActivityAt or nextDue in a request body has nowhere to land.
type createAttributes struct {
	Nickname       string `json:"nickname"`
	Make           string `json:"make"`
	Model          string `json:"model"`
	Trim           string `json:"trim"`
	Year           int    `json:"year"`
	VIN            string `json:"vin"`
	CurrentMileage int    `json:"currentMileage"`
	Notes          string `json:"notes"`
}

// patchAttributes is the exact set of fields PATCH /vehicles/{id} accepts.
// Pointers distinguish "absent" from "set to zero" on a partial update.
type patchAttributes struct {
	Nickname       *string `json:"nickname"`
	CurrentMileage *int    `json:"currentMileage"`
	Notes          *string `json:"notes"`
}

// Transform converts a Model to a JSON:API Resource carrying no derived
// attributes. Used by the write paths (create, update, restore, primary-image):
// those responses echo a write, and none of the derived values is a property of
// the write.
func Transform(m Model) server.Resource {
	return TransformDerived(m, Derived{})
}

// TransformDerived converts a Model to a JSON:API Resource, attaching the
// read-only values derived on read.
//
// LastActivityAt is carried as a string rather than a time.Time because
// encoding/json's omitempty has no effect on a struct: a time.Time field would
// emit "0001-01-01T00:00:00Z" for the absent case and defeat FR-8.4's
// "omitted, not zero-valued" contract.
func TransformDerived(m Model, d Derived) server.Resource {
	lastActivity := ""
	if !d.LastActivityAt.IsZero() {
		lastActivity = d.LastActivityAt.UTC().Format(time.RFC3339)
	}
	return server.Resource{
		Type: "vehicles",
		ID:   m.ID(),
		Attributes: Attributes{
			FleetID:             m.FleetID(),
			Nickname:            m.Nickname(),
			Make:                m.Make(),
			Model:               m.Model(),
			Trim:                m.Trim(),
			Year:                m.Year(),
			VIN:                 m.VIN(),
			CurrentMileage:      m.CurrentMileage(),
			PrimaryImageMediaID: m.PrimaryImageMediaID(),
			Notes:               m.Notes(),
			Status:              d.Status,
			LastActivityAt:      lastActivity,
			NextDue:             d.NextDue,
		},
	}
}

// TransformSlice converts a slice of Models to JSON:API Resources (no derived
// attributes).
func TransformSlice(ms []Model) []server.Resource {
	out := make([]server.Resource, 0, len(ms))
	for _, m := range ms {
		out = append(out, Transform(m))
	}
	return out
}
```

- [ ] **Step 4: Update `resource.go` to the final shape**

Four edits in `apps/fleet-service/internal/vehicle/resource.go`.

List handler, replacing the line changed in Task 4 (around `:52`):

```go
				resources = append(resources, TransformDerived(m, statusDeps.Derive(m, now)))
```

Single-vehicle handler (around `:120-121`):

```go
			// Status, last activity, and due detail are derived on read
			// (design §10.2), never stored.
			server.WriteJSON(w, http.StatusOK, server.Document{
				Data: TransformDerived(m, statusDeps.Derive(m, time.Now().UTC())),
			})
```

POST handler — replace the inline anonymous struct with the named type:

```go
		r.Post("/fleets/{id}/vehicles", server.RegisterInputHandler(func(w http.ResponseWriter, req *http.Request, attrs createAttributes) {
```

(delete the anonymous struct literal and the `,\n\t\t)` that closed it; the closing of the handler becomes `}))` as before).

PATCH handler — same substitution:

```go
		r.Patch("/vehicles/{id}", server.RegisterInputHandler(func(w http.ResponseWriter, req *http.Request, attrs patchAttributes) {
```

Nothing inside either handler body changes — the field names are identical.

- [ ] **Step 5: Run the tests to verify they pass**

```sh
go build ./...
go vet ./...
go test -race ./apps/fleet-service/...
```

Expected: PASS.

- [ ] **Step 6: Verify the JSON shape end to end**

```sh
go test ./apps/fleet-service/internal/vehicle/ -run 'TestTransformDerived' -v
```

Expected: all four `TransformDerived` cases PASS. This is the backend acceptance criteria block in the PRD: `axis:"mileage"` emits `miles` and no `days`; `axis:"time"` emits `days` and no `miles`; `nextDue` absent when no schedule is non-ok; `lastActivityAt` absent when the gather failed.

- [ ] **Step 7: Commit**

```sh
git add apps/fleet-service/internal/vehicle/rest.go apps/fleet-service/internal/vehicle/rest_test.go apps/fleet-service/internal/vehicle/resource.go
git commit -m "feat(fleet-service): expose lastActivityAt and nextDue on the vehicle resource"
```

---

## Task 6: `formatRelativeTime` in `packages/ui-components`

**Files:**
- Modify: `packages/ui-components/src/formatters.ts`
- Test: `packages/ui-components/src/formatters.test.ts` (append)
- Modify: `Makefile`

**Interfaces:**
- Consumes: nothing.
- Produces: `export function formatRelativeTime(iso: string, now?: Date): string` — exported from `@myfleet/ui-components` via the existing `export * from './formatters'` in `src/index.ts`.

There is no relative-time helper and no date library anywhere in the repo (`date-fns`, `dayjs`, `Intl.RelativeTimeFormat` all return zero hits), so this goes beside `formatMileage` rather than adding a third dependency.

**`packages/ui-components` currently runs in no automated gate.** `make fe-test` runs `apps/web` and `packages/shared-ts` only, even though `formatters.test.ts` already exists — the same gap the Makefile's own comment says was closed for `shared-ts`. This task closes it.

- [ ] **Step 1: Write the failing test**

Append to `packages/ui-components/src/formatters.test.ts` (add `formatRelativeTime` to the existing import from `./formatters`):

```ts
describe('formatRelativeTime', () => {
  const now = new Date('2026-08-02T12:00:00Z');
  const iso = (daysAgo: number) =>
    new Date(now.getTime() - daysAgo * 24 * 60 * 60 * 1000).toISOString();

  it('says "today" for anything inside the last day', () => {
    expect(formatRelativeTime(now.toISOString(), now)).toBe('today');
    expect(formatRelativeTime(iso(0.5), now)).toBe('today');
  });

  it('says "yesterday" one day back', () => {
    expect(formatRelativeTime(iso(1), now)).toBe('yesterday');
  });

  it('counts days up to a week', () => {
    expect(formatRelativeTime(iso(6), now)).toBe('6 days ago');
  });

  it('switches to weeks at seven days', () => {
    expect(formatRelativeTime(iso(7), now)).toBe('last week');
    expect(formatRelativeTime(iso(21), now)).toBe('3 weeks ago');
  });

  it('switches to months at five weeks', () => {
    expect(formatRelativeTime(iso(35), now)).toBe('last month');
    expect(formatRelativeTime(iso(120), now)).toBe('4 months ago');
  });

  it('switches to years at a full year', () => {
    expect(formatRelativeTime(iso(365), now)).toBe('last year');
    expect(formatRelativeTime(iso(800), now)).toBe('2 years ago');
  });

  it('clamps a future timestamp to "today" rather than counting forwards', () => {
    // lastActivityAt should never be in the future, but clock skew is real and
    // "in 3 days" on a Last activity slot would read as a bug.
    expect(formatRelativeTime(iso(-3), now)).toBe('today');
  });

  it('returns an empty string for an unparseable input', () => {
    // The card renders an em-dash for this, same as an absent value.
    expect(formatRelativeTime('not-a-date', now)).toBe('');
    expect(formatRelativeTime('', now)).toBe('');
  });
});
```

- [ ] **Step 2: Run the test to verify it fails**

```sh
export NVM_DIR="$HOME/.nvm" && . "$NVM_DIR/nvm.sh" && nvm use 22
npm run -w packages/ui-components test
```

Expected: FAIL — `formatRelativeTime is not a function` / no matching export.

- [ ] **Step 3: Implement `formatRelativeTime`**

Append to `packages/ui-components/src/formatters.ts`:

```ts
const DAY_MS = 24 * 60 * 60 * 1000;

// numeric: 'auto' is what yields "yesterday", "last week", and "last month"
// instead of "1 day ago", "1 week ago", "1 month ago".
const RELATIVE = new Intl.RelativeTimeFormat('en-US', { numeric: 'auto' });

/**
 * Formats a timestamp as coarse relative time ("6 days ago", "3 weeks ago",
 * "yesterday"), for slots where the exact instant is noise.
 *
 * `now` is injectable so tests can pin it; a helper whose output depends on the
 * wall clock cannot be asserted on.
 *
 * Returns '' for an unparseable input, which callers render as an em-dash — the
 * same treatment an absent value gets.
 */
export function formatRelativeTime(iso: string, now: Date = new Date()): string {
  const then = Date.parse(iso);
  if (Number.isNaN(then)) return '';

  // A future timestamp means clock skew, not a scheduled event. Clamping to 0
  // keeps "in 3 days" off a slot labelled "Last activity".
  const days = Math.max(0, Math.floor((now.getTime() - then) / DAY_MS));

  if (days < 1) return RELATIVE.format(0, 'day');
  if (days < 7) return RELATIVE.format(-days, 'day');
  if (days < 35) return RELATIVE.format(-Math.floor(days / 7), 'week');
  if (days < 365) return RELATIVE.format(-Math.floor(days / 30), 'month');
  return RELATIVE.format(-Math.floor(days / 365), 'year');
}
```

- [ ] **Step 4: Wire `ui-components` into the frontend test gate**

In `Makefile`, extend the `fe-test` target and its comment:

```make
# Every JS workspace that has tests, not just apps/web. packages/shared-ts owns
# fetchAuthenticated — the single 401-refresh path every SPA call goes through —
# and its tests previously ran in no automated gate at all. packages/ui-components
# was in the same position: it had a test script and a test file, and nothing ran
# them.
fe-test:
	npm run -w apps/web test
	npm run -w packages/shared-ts test
	npm run -w packages/ui-components test
```

- [ ] **Step 5: Run the tests to verify they pass**

```sh
npm run -w packages/ui-components test
make fe-test
```

Expected: PASS, and `make fe-test` now runs three workspaces.

- [ ] **Step 6: Commit**

```sh
git add packages/ui-components/src/formatters.ts packages/ui-components/src/formatters.test.ts Makefile
git commit -m "feat(ui-components): add formatRelativeTime and gate the package in fe-test"
```

---

## Task 7: Vehicle types and the pure `vehicleBanner` module

**Files:**
- Modify: `apps/web/src/types/models/vehicle.ts`
- Create: `apps/web/src/components/features/vehicles/vehicleBanner.ts`
- Test: `apps/web/src/components/features/vehicles/vehicleBanner.test.ts` (create)

**Interfaces:**
- Consumes: `VehicleAttributes` (extended in this task).
- Produces:
  - `export interface VehicleNextDue { state: 'upcoming' | 'overdue'; axis: 'time' | 'mileage'; miles?: number; days?: number }`
  - `VehicleAttributes.lastActivityAt?: string` and `VehicleAttributes.nextDue?: VehicleNextDue`
  - `export type BannerTone = 'danger' | 'warning' | 'quiet'`
  - `export type BannerIcon = 'overdue' | 'upcoming' | 'healthy' | 'inactive' | 'unknown'`
  - `export interface BannerContent { tone: BannerTone; icon: BannerIcon; text: string }`
  - `export function vehicleBanner(attributes: Pick<VehicleAttributes, 'status' | 'nextDue' | 'lastActivityAt'>, now: Date): BannerContent`
  - `export function asVehicleStatus(value: string | undefined): VehicleStatus | null` — moved here from `VehicleCard.tsx`

`CreateVehicleAttributes` and `UpdateVehicleAttributes` are **not** extended (FR-8.3).

- [ ] **Step 1: Extend the vehicle types**

In `apps/web/src/types/models/vehicle.ts`, add above `VehicleAttributes`:

```ts
// The single governing due detail behind a vehicle's status. Nested rather than
// four flat fields because axis determines which magnitude is present: flattening
// would make illegal combinations (axis 'time' with a miles value) representable
// and turn "is there any due detail?" into a multi-field presence test.
export interface VehicleNextDue {
  state: 'upcoming' | 'overdue';
  axis: 'time' | 'mileage';
  miles?: number; // present iff axis === 'mileage'
  days?: number; // present iff axis === 'time'
}
```

and add two fields to `VehicleAttributes`, after `status`:

```ts
  status?: string;
  /** RFC 3339. Derived read-only on the server; omitted when unavailable. */
  lastActivityAt?: string;
  /** Derived read-only on the server; omitted when no schedule is non-ok. */
  nextDue?: VehicleNextDue;
```

Update the file's leading comment to say all three of `status`, `lastActivityAt`, and `nextDue` are derived read-only on the server and never written by the client.

- [ ] **Step 2: Write the failing test**

Create `apps/web/src/components/features/vehicles/vehicleBanner.test.ts`:

```ts
import { describe, it, expect } from 'vitest';
import { vehicleBanner, asVehicleStatus } from './vehicleBanner';

const NOW = new Date('2026-08-02T12:00:00Z');

/** An ISO timestamp `days` before NOW. */
function daysAgo(days: number): string {
  return new Date(NOW.getTime() - days * 24 * 60 * 60 * 1000).toISOString();
}

describe('vehicleBanner — overdue', () => {
  it('states the mileage overrun with a thousands separator', () => {
    expect(
      vehicleBanner(
        { status: 'Overdue', nextDue: { state: 'overdue', axis: 'mileage', miles: 1120 } },
        NOW,
      ),
    ).toEqual({ tone: 'danger', icon: 'overdue', text: 'Service overdue by 1,120 mi' });
  });

  it('states a day count for a time-axis schedule', () => {
    expect(
      vehicleBanner(
        { status: 'Overdue', nextDue: { state: 'overdue', axis: 'time', days: 12 } },
        NOW,
      ).text,
    ).toBe('Service overdue by 12 days');
  });

  it('uses the singular for one day', () => {
    expect(
      vehicleBanner({ status: 'Overdue', nextDue: { state: 'overdue', axis: 'time', days: 1 } }, NOW)
        .text,
    ).toBe('Service overdue by 1 day');
  });

  it('stays tinted with generic copy when no due detail arrived', () => {
    // Urgency must survive missing detail.
    expect(vehicleBanner({ status: 'Overdue' }, NOW)).toEqual({
      tone: 'danger',
      icon: 'overdue',
      text: 'Maintenance overdue',
    });
  });
});

describe('vehicleBanner — upcoming', () => {
  it('states the remaining distance', () => {
    expect(
      vehicleBanner(
        { status: 'Upcoming Maintenance', nextDue: { state: 'upcoming', axis: 'mileage', miles: 310 } },
        NOW,
      ),
    ).toEqual({ tone: 'warning', icon: 'upcoming', text: 'Service due in 310 mi' });
  });

  it('says "due today" for a zero-day time axis', () => {
    expect(
      vehicleBanner(
        { status: 'Upcoming Maintenance', nextDue: { state: 'upcoming', axis: 'time', days: 0 } },
        NOW,
      ).text,
    ).toBe('Service due today');
  });

  it('stays tinted with generic copy when no due detail arrived', () => {
    expect(vehicleBanner({ status: 'Upcoming Maintenance' }, NOW)).toEqual({
      tone: 'warning',
      icon: 'upcoming',
      text: 'Maintenance due soon',
    });
  });
});

describe('vehicleBanner — axis discipline', () => {
  it('never renders a mileage figure for a time-axis schedule', () => {
    const text = vehicleBanner(
      // A miles value alongside axis 'time' is a server bug; the axis wins.
      { status: 'Overdue', nextDue: { state: 'overdue', axis: 'time', days: 4, miles: 900 } },
      NOW,
    ).text;
    expect(text).toBe('Service overdue by 4 days');
    expect(text).not.toContain('900');
  });

  it('never renders a day count for a mileage-axis schedule', () => {
    const text = vehicleBanner(
      { status: 'Overdue', nextDue: { state: 'overdue', axis: 'mileage', miles: 900, days: 4 } },
      NOW,
    ).text;
    expect(text).toBe('Service overdue by 900 mi');
    expect(text).not.toContain('4 days');
  });

  it('counts days below sixty and months at sixty and above', () => {
    const at = (days: number) =>
      vehicleBanner({ status: 'Overdue', nextDue: { state: 'overdue', axis: 'time', days } }, NOW)
        .text;
    expect(at(59)).toBe('Service overdue by 59 days');
    expect(at(60)).toBe('Service overdue by 2 months');
    expect(at(100)).toBe('Service overdue by 3 months');
  });

  it('falls back to generic tinted copy when the magnitude for the axis is missing', () => {
    // A hand-rolled fixture or a server bug must not render "Service overdue by
    // undefined mi".
    expect(vehicleBanner({ status: 'Overdue', nextDue: { state: 'overdue', axis: 'mileage' } }, NOW))
      .toEqual({ tone: 'danger', icon: 'overdue', text: 'Maintenance overdue' });
    expect(vehicleBanner({ status: 'Overdue', nextDue: { state: 'overdue', axis: 'time' } }, NOW).text)
      .toBe('Maintenance overdue');
  });

  it('falls back when the axis itself is unrecognised', () => {
    const nextDue = { state: 'overdue', axis: 'torque', miles: 5 } as unknown as never;
    expect(vehicleBanner({ status: 'Overdue', nextDue }, NOW).text).toBe('Maintenance overdue');
  });
});

describe('vehicleBanner — quiet statuses', () => {
  it('reads "Up to date" for a healthy vehicle', () => {
    expect(vehicleBanner({ status: 'Healthy' }, NOW)).toEqual({
      tone: 'quiet',
      icon: 'healthy',
      text: 'Up to date',
    });
  });

  it('states the dormancy in whole months for an inactive vehicle', () => {
    expect(vehicleBanner({ status: 'Inactive', lastActivityAt: daysAgo(400) }, NOW)).toEqual({
      tone: 'quiet',
      icon: 'inactive',
      text: 'No activity in 13 months',
    });
  });

  it('never reports fewer than twelve months for an inactive vehicle', () => {
    // The Inactive threshold is 365 days, so anything reaching this branch is at
    // least a year stale. A rounding artefact reading "11 months" would be wrong
    // in a way the user can see.
    expect(vehicleBanner({ status: 'Inactive', lastActivityAt: daysAgo(366) }, NOW).text).toBe(
      'No activity in 12 months',
    );
  });

  it('says "No activity recorded" when the timestamp is missing', () => {
    expect(vehicleBanner({ status: 'Inactive' }, NOW).text).toBe('No activity recorded');
  });

  it('says "No activity recorded" when the timestamp is unparseable', () => {
    expect(vehicleBanner({ status: 'Inactive', lastActivityAt: 'nope' }, NOW).text).toBe(
      'No activity recorded',
    );
  });
});

describe('vehicleBanner — unknown status', () => {
  it('renders quietly when status is absent', () => {
    expect(vehicleBanner({}, NOW)).toEqual({
      tone: 'quiet',
      icon: 'unknown',
      text: 'Status unavailable',
    });
  });

  it('renders quietly for an unrecognised status rather than tinting a raw string', () => {
    expect(vehicleBanner({ status: 'Exploded' }, NOW)).toEqual({
      tone: 'quiet',
      icon: 'unknown',
      text: 'Status unavailable',
    });
  });

  it('ignores due detail attached to an unrecognised status', () => {
    expect(
      vehicleBanner(
        { status: 'Exploded', nextDue: { state: 'overdue', axis: 'mileage', miles: 500 } },
        NOW,
      ).tone,
    ).toBe('quiet');
  });
});

describe('asVehicleStatus', () => {
  it('passes through the four known statuses', () => {
    for (const s of ['Healthy', 'Upcoming Maintenance', 'Overdue', 'Inactive']) {
      expect(asVehicleStatus(s)).toBe(s);
    }
  });

  it('rejects undefined and anything unrecognised', () => {
    expect(asVehicleStatus(undefined)).toBeNull();
    expect(asVehicleStatus('')).toBeNull();
    expect(asVehicleStatus('healthy')).toBeNull();
  });
});
```

- [ ] **Step 3: Run the test to verify it fails**

```sh
npm run -w apps/web test -- vehicleBanner
```

Expected: FAIL — cannot resolve `./vehicleBanner`.

- [ ] **Step 4: Implement `vehicleBanner.ts`**

Create `apps/web/src/components/features/vehicles/vehicleBanner.ts`. No React import — that is what makes this testable as plain data:

```ts
import type { VehicleStatus } from '@myfleet/ui-components';
import type { VehicleAttributes, VehicleNextDue } from '../../../types/models/vehicle';

const KNOWN_STATUSES: readonly VehicleStatus[] = [
  'Healthy',
  'Upcoming Maintenance',
  'Overdue',
  'Inactive',
];

/**
 * Narrows a raw server string to a status the UI recognises, or null.
 *
 * The single definition of "is this a status I know": the card, the banner tone,
 * and the icon all key off this, so an unrecognised value can never reach a
 * tinted band as a raw string.
 */
export function asVehicleStatus(value: string | undefined): VehicleStatus | null {
  return value && (KNOWN_STATUSES as readonly string[]).includes(value)
    ? (value as VehicleStatus)
    : null;
}

export type BannerTone = 'danger' | 'warning' | 'quiet';
export type BannerIcon = 'overdue' | 'upcoming' | 'healthy' | 'inactive' | 'unknown';

export interface BannerContent {
  tone: BannerTone;
  icon: BannerIcon;
  text: string;
}

const DAY_MS = 24 * 60 * 60 * 1000;
/** Past this many days, a duration reads better in months than in days. */
const MONTHS_THRESHOLD_DAYS = 60;
/** The server's Inactive threshold, so nothing reaching that branch is younger. */
const MIN_INACTIVE_MONTHS = 12;

type BannerInput = Pick<VehicleAttributes, 'status' | 'nextDue' | 'lastActivityAt'>;

/**
 * Maps a vehicle's derived attributes onto the card banner's tone, icon, and
 * copy.
 *
 * The icon is a string token rather than a component so callers can assert on
 * plain data; VehicleCard owns the single token -> lucide icon map. `now` is
 * injected because the Inactive duration depends on it, and a test that cannot
 * pin "now" cannot assert "13 months".
 *
 * Tone is spent only where action is required: Healthy and Inactive are quiet,
 * so colour anywhere in the grid always means "look here".
 */
export function vehicleBanner(attributes: BannerInput, now: Date): BannerContent {
  const status = asVehicleStatus(attributes.status);

  switch (status) {
    case 'Overdue': {
      const amount = formatAmount(attributes.nextDue);
      return {
        tone: 'danger',
        icon: 'overdue',
        text: amount ? `Service overdue by ${amount}` : 'Maintenance overdue',
      };
    }
    case 'Upcoming Maintenance': {
      const nextDue = attributes.nextDue;
      if (nextDue?.axis === 'time' && nextDue.days === 0) {
        return { tone: 'warning', icon: 'upcoming', text: 'Service due today' };
      }
      const amount = formatAmount(nextDue);
      return {
        tone: 'warning',
        icon: 'upcoming',
        text: amount ? `Service due in ${amount}` : 'Maintenance due soon',
      };
    }
    case 'Healthy':
      return { tone: 'quiet', icon: 'healthy', text: 'Up to date' };
    case 'Inactive': {
      const months = monthsSince(attributes.lastActivityAt, now);
      return {
        tone: 'quiet',
        icon: 'inactive',
        text: months === null ? 'No activity recorded' : `No activity in ${months} months`,
      };
    }
    default:
      // Absent or unrecognised. Quiet, with no attempt to caption a status we
      // cannot name, and never a raw unknown string in a tinted band.
      return { tone: 'quiet', icon: 'unknown', text: 'Status unavailable' };
  }
}

/**
 * Renders the magnitude for a due detail, driven entirely by `axis` — never by
 * which magnitude field happens to be populated. Returns null when the axis is
 * unrecognised or carries no magnitude, which pushes the caller onto its
 * generic-but-still-tinted copy: urgency has to survive bad data.
 */
function formatAmount(nextDue: VehicleNextDue | undefined): string | null {
  if (!nextDue) return null;

  if (nextDue.axis === 'mileage') {
    if (typeof nextDue.miles !== 'number') return null;
    return `${nextDue.miles.toLocaleString('en-US')} mi`;
  }
  if (nextDue.axis === 'time') {
    const days = nextDue.days;
    if (typeof days !== 'number') return null;
    if (days >= MONTHS_THRESHOLD_DAYS) return `${Math.round(days / 30)} months`;
    return `${days} ${days === 1 ? 'day' : 'days'}`;
  }
  return null;
}

/**
 * Whole months since a timestamp, floored at the server's 365-day Inactive
 * threshold. Returns null when the timestamp is absent or unparseable.
 */
function monthsSince(iso: string | undefined, now: Date): number | null {
  if (!iso) return null;
  const then = Date.parse(iso);
  if (Number.isNaN(then)) return null;
  const days = Math.max(0, Math.floor((now.getTime() - then) / DAY_MS));
  return Math.max(MIN_INACTIVE_MONTHS, Math.floor(days / 30));
}
```

- [ ] **Step 5: Run the tests to verify they pass**

```sh
npm run -w apps/web test -- vehicleBanner
```

Expected: PASS, all cases.

- [ ] **Step 6: Commit**

```sh
git add apps/web/src/types/models/vehicle.ts apps/web/src/components/features/vehicles/vehicleBanner.ts apps/web/src/components/features/vehicles/vehicleBanner.test.ts
git commit -m "feat(web): add the pure vehicle banner copy module and derived attribute types"
```

---

## Task 8: `VehiclePhotoThumbnail` box override

**Files:**
- Modify: `apps/web/src/components/features/vehicles/VehiclePhotoThumbnail.tsx`
- Test: `apps/web/src/components/features/vehicles/VehiclePhotoThumbnail.test.tsx` (append)

**Interfaces:**
- Consumes: nothing new.
- Produces: `VehiclePhotoThumbnailProps` gains `boxClassName?: string`, defaulting to today's `h-20 w-20 shrink-0 rounded-md`.

The only consumers are `VehicleCard` and this component's own test, so widening the prop is safe. The override must reach **all four** states (image, skeleton, no-photo placeholder, failed placeholder) or FR-2.3/FR-2.4's "identical dimensions in every state" breaks.

- [ ] **Step 1: Write the failing test**

Append to `apps/web/src/components/features/vehicles/VehiclePhotoThumbnail.test.tsx` (inside the existing top-level `describe`, or as a new `describe` block at the end):

```ts
describe('boxClassName override', () => {
  const BOX = 'aspect-[16/9] w-full rounded-none';

  it('reaches the no-photo placeholder', () => {
    renderWithProviders(<VehiclePhotoThumbnail vehicleLabel="Civic" boxClassName={BOX} />);
    expect(screen.getByRole('img', { name: 'No photo' })).toHaveClass('aspect-[16/9]', 'w-full');
  });

  it('reaches the loading skeleton', () => {
    // The blob promise is left pending, so the component stays in isLoading.
    vi.mocked(mediaService.getContentBlob).mockReturnValue(new Promise(() => {}));
    const { container } = renderWithProviders(
      <VehiclePhotoThumbnail mediaId="m1" vehicleLabel="Civic" boxClassName={BOX} />,
    );
    expect(container.querySelector('.aspect-\\[16\\/9\\]')).toBeInTheDocument();
  });

  it('reaches the failed-photo placeholder', async () => {
    vi.mocked(mediaService.getContentBlob).mockRejectedValue(new Error('nope'));
    renderWithProviders(
      <VehiclePhotoThumbnail mediaId="m1" vehicleLabel="Civic" boxClassName={BOX} />,
    );
    const placeholder = await screen.findByRole('img', { name: 'Photo unavailable' });
    expect(placeholder).toHaveClass('aspect-[16/9]', 'w-full');
  });

  it('reaches the loaded image', async () => {
    vi.mocked(mediaService.getContentBlob).mockResolvedValue(new Blob(['x']));
    renderWithProviders(
      <VehiclePhotoThumbnail mediaId="m1" vehicleLabel="Civic" boxClassName={BOX} />,
    );
    const img = await screen.findByAltText('Photo of Civic');
    expect(img).toHaveClass('aspect-[16/9]', 'w-full', 'object-cover');
  });

  it('keeps the 80x80 square when no override is given', () => {
    renderWithProviders(<VehiclePhotoThumbnail vehicleLabel="Civic" />);
    expect(screen.getByRole('img', { name: 'No photo' })).toHaveClass('h-20', 'w-20');
  });
});
```

If the existing file's imports do not already include `vi`, `screen`, `renderWithProviders`, and `mediaService`, add them — the head of the file already imports all four.

- [ ] **Step 2: Run the test to verify it fails**

```sh
npm run -w apps/web test -- VehiclePhotoThumbnail
```

Expected: FAIL — the placeholder still carries `h-20 w-20`; `boxClassName` is not a known prop (a TS error under `fe-build`, and a runtime class mismatch under vitest).

- [ ] **Step 3: Thread `boxClassName` through all four states**

Edit `apps/web/src/components/features/vehicles/VehiclePhotoThumbnail.tsx`:

Replace the `BOX` constant's doc comment and the props interface:

```ts
/**
 * The default box: an 80x80 square. Every state occupies exactly the box in
 * force, so cards with and without photos align within a grid row and the card
 * does not reflow when an image arrives. `shrink-0` is what stops a long title
 * from squeezing it.
 */
const DEFAULT_BOX = 'h-20 w-20 shrink-0 rounded-md';

interface VehiclePhotoThumbnailProps {
  mediaId?: string;
  /** Used for the image's alt text — the card already knows it, so no metadata request is needed. */
  vehicleLabel: string;
  /**
   * Overrides the default 80x80 square — the list card passes a 16:9 hero box.
   * Applied to ALL four states (image, skeleton, and both placeholders), so
   * "identical dimensions in every state" is structurally guaranteed here rather
   * than restated at four call sites.
   */
  boxClassName?: string;
  className?: string;
}
```

Give `PhotoPlaceholder` the box as a prop:

```ts
function PhotoPlaceholder({
  label,
  boxClassName,
  className,
}: {
  label: string;
  boxClassName: string;
  className?: string;
}) {
  return (
    <div
      role="img"
      aria-label={label}
      className={cn(
        boxClassName,
        'flex items-center justify-center bg-muted text-muted-foreground',
        className,
      )}
    >
      <Car className="h-8 w-8" aria-hidden="true" />
    </div>
  );
}
```

and rewrite the component body to thread it:

```ts
export function VehiclePhotoThumbnail({
  mediaId,
  vehicleLabel,
  boxClassName = DEFAULT_BOX,
  className,
}: VehiclePhotoThumbnailProps) {
  const { url, isLoading, isError } = useMediaContentUrl(mediaId, 'thumbnail');

  if (!mediaId) {
    return <PhotoPlaceholder label="No photo" boxClassName={boxClassName} className={className} />;
  }
  if (isLoading) {
    return <Skeleton className={cn(boxClassName, className)} />;
  }
  if (isError || !url) {
    // "No photo" is reserved for the `!mediaId` branch above. Reaching here
    // means the vehicle DOES have a photo we could not show — a real error, or
    // React Query pausing the query offline (isLoading false, isError false,
    // data undefined), which would otherwise tell the user their vehicle has no
    // photo at all.
    return (
      <PhotoPlaceholder label="Photo unavailable" boxClassName={boxClassName} className={className} />
    );
  }
  return (
    <img
      src={url}
      alt={`Photo of ${vehicleLabel}`}
      className={cn(boxClassName, 'object-cover', className)}
    />
  );
}
```

The `useMediaContentUrl(mediaId, 'thumbnail')` call, the no-toast behaviour, and both accessible labels are unchanged (FR-2.2, FR-2.5).

- [ ] **Step 4: Run the tests to verify they pass**

```sh
npm run -w apps/web test -- VehiclePhotoThumbnail
```

Expected: PASS, including every pre-existing case in the file.

- [ ] **Step 5: Commit**

```sh
git add apps/web/src/components/features/vehicles/VehiclePhotoThumbnail.tsx apps/web/src/components/features/vehicles/VehiclePhotoThumbnail.test.tsx
git commit -m "feat(web): let VehiclePhotoThumbnail take a box override across all states"
```

---

## Task 9: Rebuild `VehicleCard`

**Files:**
- Modify: `apps/web/src/components/features/vehicles/VehicleCard.tsx`
- Test: `apps/web/src/components/features/vehicles/VehicleCard.test.tsx` (rewrite the assertions that no longer apply, keep the rest)

**Interfaces:**
- Consumes: `vehicleBanner`, `BannerIcon` (Task 7); `VehiclePhotoThumbnail`'s `boxClassName` (Task 8); `formatRelativeTime`, `formatMileage` (Task 6 / existing); `VehicleNextDue` (Task 7).
- Produces: `export function VehicleCard({ vehicle }: { vehicle: Vehicle })` — same signature. `VehicleCardSkeleton` is added in Task 10, in this same file.

**What changes for existing tests.** The chevron icon button is gone (FR-6.5), so the assertion `getByRole('link', { name: 'Open details for 2019 Honda Civic' })` no longer matches — the card link's accessible name is now the vehicle title itself (FR-6.3), so it becomes `getByRole('link', { name: '2019 Honda Civic' })`. `'does not make the card body or the thumbnail clickable'` inverts: the card body IS clickable now, via an overlay that is not a DOM ancestor of anything. `StatusBadge` is no longer rendered, so `getByText('Healthy')` becomes the banner's `'Up to date'`.

**Honest scope note on the Carfax regression test.** jsdom performs no layout, so a click on the Carfax anchor never dispatches through a CSS pseudo-element overlay regardless of z-index — a jsdom test **cannot** catch a missing `z-10`. The tests below assert the structural invariants that make the pattern correct (the Carfax anchor has no ancestor anchor; the overlay class and the `z-10` class are both present; a click does not change the router location). Genuine click-through verification is a browser check, recorded in Task 10's `manual-verification.md`.

- [ ] **Step 1: Write the failing tests**

Rewrite `apps/web/src/components/features/vehicles/VehicleCard.test.tsx`. Keep the existing imports, `latchConfig`, `makeVehicle`, `beforeEach`, and `afterEach` blocks verbatim, and add `userEvent` plus a location probe. Replace the whole `describe('VehicleCard', …)` block with:

```tsx
import userEvent from '@testing-library/user-event';
import { useLocation } from 'react-router-dom';

/**
 * Renders the router's current pathname so a test can assert navigation.
 *
 * Assert on `.textContent` with `toBe`, never `toHaveTextContent('/')` — that
 * matcher does a substring match, and '/vehicles/v1' contains '/', so the
 * "did NOT navigate" assertion would pass even when the bug is present.
 */
function LocationProbe() {
  return <span data-testid="pathname">{useLocation().pathname}</span>;
}

describe('VehicleCard — photo', () => {
  it('renders the vehicle photo when one is set', async () => {
    renderWithProviders(<VehicleCard vehicle={makeVehicle({ primaryImageMediaId: 'm1' })} />);
    expect(await screen.findByAltText('Photo of 2019 Honda Civic')).toBeInTheDocument();
  });

  it('renders the hero at a 16:9 aspect ratio', async () => {
    renderWithProviders(<VehicleCard vehicle={makeVehicle({ primaryImageMediaId: 'm1' })} />);
    expect(await screen.findByAltText('Photo of 2019 Honda Civic')).toHaveClass(
      'aspect-[16/9]',
      'w-full',
    );
  });

  it('renders the placeholder at identical dimensions when no photo is set', () => {
    renderWithProviders(<VehicleCard vehicle={makeVehicle()} />);
    const placeholder = screen.getByRole('img', { name: 'No photo' });
    expect(placeholder).toHaveClass('aspect-[16/9]', 'w-full');
    expect(mediaService.getContentBlob).not.toHaveBeenCalled();
  });

  it('renders the placeholder with no error tile and no toast when the photo fails', async () => {
    vi.mocked(mediaService.getContentBlob).mockRejectedValue(new Error('nope'));
    renderWithProviders(<VehicleCard vehicle={makeVehicle({ primaryImageMediaId: 'm1' })} />);

    expect(await screen.findByRole('img', { name: 'Photo unavailable' })).toBeInTheDocument();
    expect(screen.queryByText(/failed/i)).not.toBeInTheDocument();
  });
});

describe('VehicleCard — banner', () => {
  const banner = () => screen.getByTestId('vehicle-card-banner');

  it('tints an overdue vehicle in danger and states the overrun', () => {
    renderWithProviders(
      <VehicleCard
        vehicle={makeVehicle({
          status: 'Overdue',
          nextDue: { state: 'overdue', axis: 'mileage', miles: 1120 },
        })}
      />,
    );
    expect(screen.getByText('Service overdue by 1,120 mi')).toBeInTheDocument();
    expect(banner()).toHaveClass('bg-danger-subtle', 'text-danger-subtle-foreground');
  });

  it('tints an upcoming vehicle in warning and states the remaining distance', () => {
    renderWithProviders(
      <VehicleCard
        vehicle={makeVehicle({
          status: 'Upcoming Maintenance',
          nextDue: { state: 'upcoming', axis: 'mileage', miles: 310 },
        })}
      />,
    );
    expect(screen.getByText('Service due in 310 mi')).toBeInTheDocument();
    expect(banner()).toHaveClass('bg-warning-subtle', 'text-warning-subtle-foreground');
  });

  it('leaves a healthy vehicle untinted', () => {
    renderWithProviders(<VehicleCard vehicle={makeVehicle({ status: 'Healthy' })} />);
    expect(screen.getByText('Up to date')).toBeInTheDocument();
    expect(banner()).toHaveClass('text-muted-foreground');
    expect(banner().className).not.toMatch(/bg-(danger|warning|success)-subtle/);
  });

  it('leaves an inactive vehicle untinted and states the dormancy', () => {
    const longAgo = new Date(Date.now() - 400 * 24 * 60 * 60 * 1000).toISOString();
    renderWithProviders(
      <VehicleCard vehicle={makeVehicle({ status: 'Inactive', lastActivityAt: longAgo })} />,
    );
    expect(screen.getByText(/No activity in \d+ months/)).toBeInTheDocument();
    expect(banner().className).not.toMatch(/bg-(danger|warning|success)-subtle/);
  });

  it('renders a time-axis schedule as a day count, never as mileage', () => {
    renderWithProviders(
      <VehicleCard
        vehicle={makeVehicle({
          status: 'Overdue',
          nextDue: { state: 'overdue', axis: 'time', days: 12 },
        })}
      />,
    );
    expect(screen.getByText('Service overdue by 12 days')).toBeInTheDocument();
    expect(screen.queryByText(/mi$/)).not.toBeInTheDocument();
  });

  it('renders the quiet banner when status is absent', () => {
    renderWithProviders(<VehicleCard vehicle={makeVehicle()} />);
    expect(screen.getByText('Status unavailable')).toBeInTheDocument();
    expect(banner().className).not.toMatch(/bg-(danger|warning)-subtle/);
  });

  it('renders the quiet banner for an unrecognised status rather than crashing', () => {
    renderWithProviders(<VehicleCard vehicle={makeVehicle({ status: 'Exploded' })} />);
    expect(screen.getByText('Status unavailable')).toBeInTheDocument();
    expect(screen.queryByText('Exploded')).not.toBeInTheDocument();
  });

  it('stays tinted with generic copy when an overdue vehicle has no due detail', () => {
    // Urgency has to survive missing detail — this is the case a naive
    // "render nothing without nextDue" would get silently wrong.
    renderWithProviders(<VehicleCard vehicle={makeVehicle({ status: 'Overdue' })} />);
    expect(screen.getByText('Maintenance overdue')).toBeInTheDocument();
    expect(banner()).toHaveClass('bg-danger-subtle');
  });

  it('carries an icon alongside the text so colour is never the only signal', () => {
    renderWithProviders(<VehicleCard vehicle={makeVehicle({ status: 'Overdue' })} />);
    expect(banner().querySelector('svg')).toBeInTheDocument();
  });

  it('truncates long banner text instead of wrapping', () => {
    renderWithProviders(<VehicleCard vehicle={makeVehicle({ status: 'Healthy' })} />);
    expect(screen.getByText('Up to date')).toHaveClass('truncate');
  });

  it('does not render a StatusBadge anywhere on the card', () => {
    // The banner replaces it. A badge alongside would restate the status without
    // the reason and reintroduce colour on healthy cards.
    renderWithProviders(<VehicleCard vehicle={makeVehicle({ status: 'Healthy' })} />);
    expect(screen.queryByText('Healthy')).not.toBeInTheDocument();
  });
});

describe('VehicleCard — stat strip', () => {
  it('shows odometer and last activity with tabular figures', () => {
    const sixDaysAgo = new Date(Date.now() - 6 * 24 * 60 * 60 * 1000).toISOString();
    renderWithProviders(
      <VehicleCard
        vehicle={makeVehicle({ currentMileage: 42000, lastActivityAt: sixDaysAgo })}
      />,
    );
    expect(screen.getByText('Odometer')).toBeInTheDocument();
    expect(screen.getByText('Last activity')).toBeInTheDocument();

    const odometer = screen.getByText('42,000 mi');
    expect(odometer).toHaveClass('tabular-nums');
    expect(screen.getByText('6 days ago')).toBeInTheDocument();
  });

  it('renders an em-dash and keeps the slot when mileage is missing', () => {
    renderWithProviders(<VehicleCard vehicle={makeVehicle({ lastActivityAt: undefined })} />);
    expect(screen.getByText('Odometer')).toBeInTheDocument();
    expect(screen.getAllByText('—')).toHaveLength(2);
  });

  it('renders an em-dash when last activity is missing', () => {
    renderWithProviders(<VehicleCard vehicle={makeVehicle({ currentMileage: 42000 })} />);
    expect(screen.getByText('42,000 mi')).toBeInTheDocument();
    expect(screen.getAllByText('—')).toHaveLength(1);
  });

  it('renders an em-dash when last activity is unparseable', () => {
    renderWithProviders(
      <VehicleCard vehicle={makeVehicle({ currentMileage: 1, lastActivityAt: 'nope' })} />,
    );
    expect(screen.getAllByText('—')).toHaveLength(1);
  });

  it('does not show a next-service figure in the strip', () => {
    // The banner already states it where it matters; a third slot would read
    // "—" on every healthy card.
    renderWithProviders(
      <VehicleCard
        vehicle={makeVehicle({
          status: 'Overdue',
          nextDue: { state: 'overdue', axis: 'mileage', miles: 1120 },
        })}
      />,
    );
    expect(screen.queryByText('Next service')).not.toBeInTheDocument();
  });
});

describe('VehicleCard — navigation', () => {
  it('makes the title a real anchor to the detail page', () => {
    // A real <a href> is what preserves middle-click, cmd/ctrl-click, and the
    // link context menu; an onClick handler on a div would not.
    renderWithProviders(<VehicleCard vehicle={makeVehicle()} />);
    const link = screen.getByRole('link', { name: '2019 Honda Civic' });
    expect(link).toHaveAttribute('href', '/vehicles/v1');
  });

  it('names the card link with the nickname when the vehicle has one', () => {
    renderWithProviders(<VehicleCard vehicle={makeVehicle({ nickname: 'Daily Driver' })} />);
    expect(screen.getByRole('link', { name: 'Daily Driver' })).toBeInTheDocument();
  });

  it('spans the whole card with an overlay pseudo-element rather than wrapping it', () => {
    // FR-6.1/6.2: the anchor must never be a DOM ancestor of another interactive
    // element. The overlay is what makes the whole card clickable without that.
    renderWithProviders(<VehicleCard vehicle={makeVehicle()} />);
    const link = screen.getByRole('link', { name: '2019 Honda Civic' });
    expect(link.className).toContain('after:absolute');
    expect(link.className).toContain('after:inset-0');
    expect(screen.getByRole('img', { name: 'No photo' }).closest('a')).toBeNull();
  });

  it('navigates to the detail page when the card link is activated', async () => {
    renderWithProviders(
      <>
        <VehicleCard vehicle={makeVehicle()} />
        <LocationProbe />
      </>,
    );
    expect(screen.getByTestId('pathname').textContent).toBe('/');

    await userEvent.click(screen.getByRole('link', { name: '2019 Honda Civic' }));
    expect(screen.getByTestId('pathname').textContent).toBe('/vehicles/v1');
  });

  it('no longer renders a separate chevron detail button', () => {
    renderWithProviders(<VehicleCard vehicle={makeVehicle()} />);
    expect(screen.queryByRole('link', { name: /Open details/ })).not.toBeInTheDocument();
    expect(screen.getAllByRole('link')).toHaveLength(1);
  });

  it('puts the card link before Carfax in DOM order', () => {
    renderWithProviders(<VehicleCard vehicle={makeVehicle({ vin: '1HGCM82633A004352' })} />);
    const links = screen.getAllByRole('link');
    expect(links).toHaveLength(2);
    expect(links[0]).toHaveAttribute('href', '/vehicles/v1');
    expect(links[1]).toHaveAttribute('href', expect.stringContaining('carfax.com'));
  });
});

describe('VehicleCard — Carfax', () => {
  it('renders a Carfax link with the VIN substituted when a VIN is present', () => {
    renderWithProviders(<VehicleCard vehicle={makeVehicle({ vin: '1HGCM82633A004352' })} />);

    const carfax = screen.getByRole('link', {
      name: 'View Carfax report for 2019 Honda Civic (opens in a new tab)',
    });
    expect(carfax).toHaveAttribute(
      'href',
      'https://www.carfax.com/VehicleHistory/p/Report.cfx?vin=1HGCM82633A004352',
    );
    expect(carfax).toHaveAttribute('target', '_blank');
    // noopener blocks window.opener access from the opened page; noreferrer
    // stops MyFleet being sent as the referrer alongside the VIN.
    expect(carfax).toHaveAttribute('rel', 'noopener noreferrer');
  });

  it('omits the Carfax button entirely when there is no VIN', () => {
    renderWithProviders(<VehicleCard vehicle={makeVehicle({ vin: '   ' })} />);
    expect(screen.queryByRole('link', { name: /Carfax/ })).not.toBeInTheDocument();
    expect(screen.getAllByRole('link')).toHaveLength(1);
  });

  it('omits the Carfax button when the configured template ignores the VIN', async () => {
    await latchConfig('https://www.carfax.com/');
    renderWithProviders(<VehicleCard vehicle={makeVehicle({ vin: '1HGCM82633A004352' })} />);
    expect(screen.queryByRole('link', { name: /Carfax/ })).not.toBeInTheDocument();
  });

  it('picks up a runtime config that arrives AFTER the card has mounted', async () => {
    // main.tsx does not block the mount on the config fetch, so this is the
    // ordering that actually happens in production. If the card read the module
    // getter directly instead of subscribing, a ConfigMap override would
    // silently never take effect.
    renderWithProviders(<VehicleCard vehicle={makeVehicle({ vin: '1HGCM82633A004352' })} />);
    expect(screen.getByRole('link', { name: /Carfax/ })).toHaveAttribute(
      'href',
      'https://www.carfax.com/VehicleHistory/p/Report.cfx?vin=1HGCM82633A004352',
    );

    await act(async () => {
      await latchConfig('https://example.test/report?vin={vin}');
    });

    expect(screen.getByRole('link', { name: /Carfax/ })).toHaveAttribute(
      'href',
      'https://example.test/report?vin=1HGCM82633A004352',
    );
  });

  it('drops the Carfax button when a late config makes the template unusable', async () => {
    renderWithProviders(<VehicleCard vehicle={makeVehicle({ vin: '1HGCM82633A004352' })} />);
    expect(screen.getByRole('link', { name: /Carfax/ })).toBeInTheDocument();

    await act(async () => {
      await latchConfig('https://www.carfax.com/');
    });

    expect(screen.queryByRole('link', { name: /Carfax/ })).not.toBeInTheDocument();
  });

  it('is not nested inside the card link and sits above the overlay', () => {
    // The FR-6.6 regression this layout risks. Two structural guards: the
    // Carfax anchor has no ancestor anchor (so a click can never be the card
    // link's), and its wrapper is lifted above the overlay pseudo-element.
    //
    // jsdom does no layout, so it cannot prove the stacking actually works —
    // that check lives in manual-verification.md. What it CAN prove is that the
    // two structural preconditions are in place.
    renderWithProviders(<VehicleCard vehicle={makeVehicle({ vin: '1HGCM82633A004352' })} />);
    const carfax = screen.getByRole('link', { name: /Carfax/ });

    expect(carfax.parentElement?.closest('a')).toBeNull();
    expect(carfax.className).toContain('z-10');
  });

  it('does not navigate to the detail page when Carfax is clicked', async () => {
    renderWithProviders(
      <>
        <VehicleCard vehicle={makeVehicle({ vin: '1HGCM82633A004352' })} />
        <LocationProbe />
      </>,
    );

    await userEvent.click(screen.getByRole('link', { name: /Carfax/ }));
    expect(screen.getByTestId('pathname').textContent).toBe('/');
  });
});
```

- [ ] **Step 2: Run the tests to verify they fail**

```sh
npm run -w apps/web test -- VehicleCard
```

Expected: FAIL — no `vehicle-card-banner` test id, the card link is still named "Open details for …", the strip does not exist.

- [ ] **Step 3: Rebuild `VehicleCard.tsx`**

Replace `apps/web/src/components/features/vehicles/VehicleCard.tsx` with:

```tsx
import { Link } from 'react-router-dom';
import {
  AlertTriangle,
  CheckCircle,
  Clock,
  HelpCircle,
  History,
  Moon,
  type LucideIcon,
} from 'lucide-react';
import { formatMileage, formatRelativeTime } from '@myfleet/ui-components';
import { Button } from '../../ui/button';
import { Card } from '../../ui/card';
import { VehiclePhotoThumbnail } from './VehiclePhotoThumbnail';
import { vehicleBanner, type BannerIcon, type BannerTone } from './vehicleBanner';
import { buildCarfaxUrl } from '../../../lib/carfax';
import { useRuntimeConfig } from '../../../lib/hooks/useRuntimeConfig';
import { cn } from '../../../lib/utils';
import type { Vehicle } from '../../../types/models/vehicle';

/**
 * The banner's colour treatment. Only the two states that need action are
 * tinted, so colour anywhere in the grid always means "look here" — a per-card
 * chip cannot achieve that.
 *
 * These are the semantic token pairs task-003 measured for AA contrast in both
 * themes (docs/tasks/task-003-dark-mode-branding/contrast.md); no new colour
 * combination is introduced here.
 */
const TONE_CLASSES: Record<BannerTone, string> = {
  danger: 'bg-danger-subtle text-danger-subtle-foreground border-danger-border',
  warning: 'bg-warning-subtle text-warning-subtle-foreground border-warning-border',
  quiet: 'bg-card text-muted-foreground border-border',
};

/**
 * The one place a banner icon token becomes a component. Keeping lucide out of
 * vehicleBanner.ts is what lets its tests assert on plain data.
 */
const BANNER_ICONS: Record<BannerIcon, LucideIcon> = {
  overdue: AlertTriangle,
  upcoming: Clock,
  healthy: CheckCircle,
  inactive: Moon,
  unknown: HelpCircle,
};

const EM_DASH = '—';

export function VehicleCard({ vehicle }: { vehicle: Vehicle }) {
  const { attributes } = vehicle;
  const title =
    attributes.nickname?.trim() ||
    `${attributes.year} ${attributes.make} ${attributes.model}`.trim();

  const banner = vehicleBanner(attributes, new Date());
  const BannerIconComponent = BANNER_ICONS[banner.icon];

  // Read through the hook, not the module getter: the tree mounts before the
  // runtime config fetch resolves, so a card rendered in that window has to
  // re-render when the real template lands or a ConfigMap override would never
  // reach the user.
  const { carfaxUrlTemplate } = useRuntimeConfig();
  // null means "render no button": no VIN, a template that ignores {vin}, or a
  // template whose scheme is not https:. Nothing contacts Carfax until a click.
  const carfaxUrl = buildCarfaxUrl(carfaxUrlTemplate, attributes.vin);

  const lastActivity = attributes.lastActivityAt
    ? formatRelativeTime(attributes.lastActivityAt)
    : '';

  return (
    // `relative` anchors the card link's overlay to the whole card. `isolate`
    // creates a local stacking context so the z-indices inside cannot leak in or
    // out. `group` drives the hover treatment.
    //
    // Note there is no `overflow-hidden` here: on the root it would clip the
    // card link's focus ring. It sits on the photo wrapper instead, where it
    // does the one job it is needed for.
    <Card
      className={cn(
        'group relative isolate flex flex-col p-0 transition-shadow hover:shadow-md',
        // The overlay anchor has no visible box of its own, so its own ring is
        // suppressed and the card takes it. The data attribute scopes the
        // selector, so focusing Carfax does not also ring the whole card.
        'has-[a[data-card-link]:focus-visible]:ring-2',
        'has-[a[data-card-link]:focus-visible]:ring-ring',
        'has-[a[data-card-link]:focus-visible]:ring-offset-2',
      )}
    >
      {/* Clips the hero's top corners to the card's radius. */}
      <div className="overflow-hidden rounded-t-lg">
        <VehiclePhotoThumbnail
          mediaId={attributes.primaryImageMediaId}
          vehicleLabel={title}
          boxClassName="aspect-[16/9] w-full rounded-none"
        />
      </div>

      {/* Fixed height on every card regardless of tone, so cards in a grid row
          align whatever their status. */}
      <div
        data-testid="vehicle-card-banner"
        className={cn(
          'flex h-9 shrink-0 items-center gap-2 border-b px-4 text-sm',
          TONE_CLASSES[banner.tone],
        )}
      >
        <BannerIconComponent className="h-4 w-4 shrink-0" aria-hidden="true" />
        <span className="truncate">{banner.text}</span>
      </div>

      <div className="min-w-0 px-4 pt-3">
        {/* The ::after overlay is what makes the whole card clickable without the
            anchor ever becoming a DOM ancestor of the Carfax link — the nesting
            that made task-005 remove whole-card navigation in the first place.
            Being a real <a href>, it keeps middle-click, cmd/ctrl-click, and the
            link context menu. Its text is the vehicle title, so a grid of cards
            is a list of distinctly-named links with no aria-label needed. */}
        <Link
          to={`/vehicles/${vehicle.id}`}
          data-card-link
          className="block truncate font-medium text-foreground after:absolute after:inset-0 after:content-[''] focus-visible:outline-none"
        >
          {title}
        </Link>
        <div className="truncate text-sm text-muted-foreground">
          {attributes.year} {attributes.make} {attributes.model}
          {attributes.trim ? ` ${attributes.trim}` : ''}
        </div>
      </div>

      {/* Both slots always render; an absent value is an em-dash, never a
          missing column, so values line up across cards. */}
      <div className="mt-3 grid grid-cols-2 gap-4 border-t border-border px-4 py-3">
        <Stat
          label="Odometer"
          value={
            typeof attributes.currentMileage === 'number'
              ? formatMileage(attributes.currentMileage)
              : EM_DASH
          }
        />
        <Stat label="Last activity" value={lastActivity || EM_DASH} />
      </div>

      {/* Fixed height so a VIN-less card is exactly as tall as its neighbours. */}
      <div className="flex h-12 items-center justify-end px-4 pb-2">
        {carfaxUrl && (
          // A plain <a>, not a react-router <Link> — this leaves the SPA.
          // rel="noopener noreferrer" stops the opened page reaching back
          // through window.opener and suppresses the referrer, which matters
          // because the URL carries the VIN.
          //
          // `relative z-10` lifts it above the card link's overlay so it
          // receives its own clicks. Without it, clicking Carfax silently
          // navigates to the detail page instead — it looks fine and behaves
          // wrong.
          <Button asChild variant="ghost" size="icon" className="relative z-10">
            <a
              href={carfaxUrl}
              target="_blank"
              rel="noopener noreferrer"
              aria-label={`View Carfax report for ${title} (opens in a new tab)`}
            >
              <History className="h-4 w-4" aria-hidden="true" />
            </a>
          </Button>
        )}
      </div>
    </Card>
  );
}

function Stat({ label, value }: { label: string; value: string }) {
  return (
    <div className="min-w-0">
      <div className="text-xs uppercase tracking-wide text-muted-foreground">{label}</div>
      <div className="truncate tabular-nums text-sm text-foreground">{value}</div>
    </div>
  );
}
```

Two notes for the implementer:

- The `z-10` assertion in the test reads `carfax.className`. `Button asChild` renders through a Radix `Slot`, which **merges** its className onto the child `<a>`, so `relative z-10` lands on the anchor itself. If the assertion fails, check that `Slot` is merging rather than replacing before changing the test.
- `has-[…]` is a Tailwind 3.4 variant (`apps/web/package.json:45` pins `^3.4.0`), so no config change is needed.

- [ ] **Step 4: Run the tests to verify they pass**

```sh
npm run -w apps/web test -- VehicleCard
```

Expected: PASS across all five describe blocks.

- [ ] **Step 5: Type-check**

```sh
npm run -w apps/web build
```

Expected: clean. `StatusBadge` and `ChevronRight` are no longer imported by this file — if either import lingers, the lint step will flag it.

- [ ] **Step 6: Commit**

```sh
git add apps/web/src/components/features/vehicles/VehicleCard.tsx apps/web/src/components/features/vehicles/VehicleCard.test.tsx
git commit -m "feat(web): rebuild the vehicle card around a hero photo and status banner"
```

---

## Task 10: Structural skeleton, and gate the branch

**Files:**
- Modify: `apps/web/src/components/features/vehicles/VehicleCard.tsx` (add `VehicleCardSkeleton`)
- Modify: `apps/web/src/components/features/vehicles/VehicleList.tsx`
- Test: `apps/web/src/components/features/vehicles/VehicleCard.test.tsx` (append)
- Create: `docs/tasks/task-007-vehicle-card-status-banner/manual-verification.md`

**Interfaces:**
- Consumes: everything above.
- Produces: `export function VehicleCardSkeleton()`, exported from `VehicleCard.tsx`.

The skeleton is co-located with the card deliberately: a region added to one is visibly missing from the other in the same file. The current `h-40` magic number and the comment deriving 160px arithmetically — which concedes it is "computed, not measured" — are deleted.

- [ ] **Step 1: Write the failing test**

Append to `apps/web/src/components/features/vehicles/VehicleCard.test.tsx` (and add `VehicleCardSkeleton` to the import from `./VehicleCard`):

```tsx
describe('VehicleCardSkeleton', () => {
  it('mirrors the card structure rather than a single fixed-height block', () => {
    // A structural skeleton cannot drift from the card the way a magic number
    // does — the two live in the same file.
    const { container } = renderWithProviders(<VehicleCardSkeleton />);
    expect(container.querySelector('.aspect-\\[16\\/9\\]')).toBeInTheDocument();
    // Photo, banner, title, subtitle, two stat slots, footer.
    expect(container.querySelectorAll('.animate-pulse').length).toBeGreaterThanOrEqual(6);
  });
});
```

`.animate-pulse` is the class the shared `Skeleton` component applies; confirm against `apps/web/src/components/ui/skeleton.tsx` and adjust the selector if it differs.

- [ ] **Step 2: Run the test to verify it fails**

```sh
npm run -w apps/web test -- VehicleCard
```

Expected: FAIL — `VehicleCardSkeleton` is not exported.

- [ ] **Step 3: Add `VehicleCardSkeleton`**

Add the `Skeleton` import to `VehicleCard.tsx`:

```tsx
import { Skeleton } from '../../ui/skeleton';
```

and append to the same file, below `Stat`:

```tsx
/**
 * The loading placeholder for a VehicleCard.
 *
 * Deliberately built from the same region structure as the card above — hero,
 * banner, title, subtitle, stat strip, footer — rather than one fixed-height
 * block. It lives in this file so that adding a region to the card and
 * forgetting the skeleton is visible in the same diff.
 */
export function VehicleCardSkeleton() {
  return (
    <Card className="flex flex-col p-0">
      <Skeleton className="aspect-[16/9] w-full rounded-none rounded-t-lg" />
      <div className="flex h-9 items-center border-b border-border px-4">
        <Skeleton className="h-3 w-40" />
      </div>
      <div className="space-y-2 px-4 pt-3">
        <Skeleton className="h-4 w-2/3" />
        <Skeleton className="h-3 w-1/2" />
      </div>
      <div className="mt-3 grid grid-cols-2 gap-4 border-t border-border px-4 py-3">
        <Skeleton className="h-8 w-full" />
        <Skeleton className="h-8 w-full" />
      </div>
      <div className="flex h-12 items-center justify-end px-4 pb-2">
        <Skeleton className="h-8 w-8 rounded-md" />
      </div>
    </Card>
  );
}
```

- [ ] **Step 4: Update `VehicleList.tsx`**

Replace the loading branch of `apps/web/src/components/features/vehicles/VehicleList.tsx`, deleting the `h-40` block and its justification comment entirely, and dropping the now-unused `Skeleton` import:

```tsx
import { VehicleCard, VehicleCardSkeleton } from './VehicleCard';
import type { Vehicle } from '../../../types/models/vehicle';

interface VehicleListProps {
  vehicles: Vehicle[];
  isLoading: boolean;
}

export function VehicleList({ vehicles, isLoading }: VehicleListProps) {
  if (isLoading) {
    return (
      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3">
        {Array.from({ length: 6 }).map((_, i) => (
          <VehicleCardSkeleton key={i} />
        ))}
      </div>
    );
  }
  // …the empty state and the grid below are unchanged…
```

The empty state (FR-7.2) and the populated grid, including the `grid-cols-1 sm:grid-cols-2 lg:grid-cols-3` breakpoints (FR-1.3, design D8), are untouched.

- [ ] **Step 5: Run the frontend suite**

```sh
npm run -w apps/web test
npm run -w apps/web build
```

Expected: PASS and a clean build.

- [ ] **Step 6: Write the manual-verification checklist**

Create `docs/tasks/task-007-vehicle-card-status-banner/manual-verification.md`:

```markdown
# task-007 — Manual verification

jsdom performs no layout, so the checks below cannot be automated in the unit
suite. Run them against the app before opening the PR.

Start the app, sign in, and open `/vehicles` with at least one vehicle in each of
the four statuses, one with a photo, one without, and one with a VIN.

## The Carfax regression (FR-6.6) — the highest-value check here

- [ ] Click the Carfax button. Carfax opens in a new tab and the app stays on
      `/vehicles` — it does **not** navigate to the vehicle detail page.
      A missing `relative z-10` on the Carfax wrapper breaks exactly this, looks
      completely fine, and no unit test can catch it.

## Navigation (FR-6.2, FR-6.4, FR-6.8)

- [ ] Clicking anywhere on the card body — photo, banner, stat strip — opens the
      detail page.
- [ ] Middle-click on the card body opens the detail page in a new tab.
- [ ] Cmd/ctrl-click does the same.
- [ ] Right-click on the card body offers the browser's link context menu.
- [ ] Tab reaches the card link first, then Carfax. Each shows a visible focus
      ring, and the card link's ring is **not clipped** by the card's edge.

## Layout (FR-1.2, FR-1.3, FR-2.3)

- [ ] Cards in the same grid row have equal height, regardless of status, photo
      presence, VIN presence, or missing mileage.
- [ ] No horizontal overflow at the single-column breakpoint (narrow the window).
- [ ] The card does not visibly jump when a photo finishes loading.
- [ ] A long nickname truncates and does not widen the card.

## Banner (FR-3.x, NFR-11)

- [ ] Overdue and Upcoming cards are tinted; Healthy and Inactive are not.
- [ ] Toggle dark mode: both tinted treatments stay legible in both themes.
- [ ] Each banner shows an icon as well as text.

## Network (FR-2.2, NFR-2, NFR-3)

- [ ] In the network panel, each photo request carries `?variant=thumbnail`.
- [ ] Loading `/vehicles` issues exactly one request for the list plus one per
      distinct media id — no per-vehicle metadata or schedule request.
- [ ] Nothing contacts carfax.com before an explicit click.

## Roles (FR-6.10, NFR-12)

- [ ] Repeat the navigation checks as a `viewer`-role user; behaviour is
      identical.

## Deferred, note only (design D5, D8)

- [ ] Judge whether the 320px `thumbnail` variant reads soft in the hero box at
      `lg:grid-cols-3` on a high-DPI display. If it does, that is a
      variant-sizing task in `media-service`, not a change here.
- [ ] Judge whether three columns still read well now that the card is taller.
```

- [ ] **Step 7: Run the full gate**

```sh
export NVM_DIR="$HOME/.nvm" && . "$NVM_DIR/nvm.sh" && nvm use 22
make ci
```

Expected: `lint-check`, `vet`, `test`, `build`, `fe-test`, `fe-build`, `manifests`, and `carfax-template` all pass. `deploy/k8s` is untouched by this task, so `manifests` should be unaffected — if it fails, the failure predates this branch.

- [ ] **Step 8: Commit**

```sh
git add apps/web/src/components/features/vehicles/VehicleCard.tsx apps/web/src/components/features/vehicles/VehicleCard.test.tsx apps/web/src/components/features/vehicles/VehicleList.tsx docs/tasks/task-007-vehicle-card-status-banner/manual-verification.md
git commit -m "feat(web): structural card skeleton and manual verification checklist"
```

- [ ] **Step 9: Run the code review before opening a PR**

Per CLAUDE.md, the review step is not optional. Both guideline reviewers apply — this branch changes Go and TypeScript:

```
superpowers:requesting-code-review
```

It dispatches `plan-adherence-reviewer`, `backend-guidelines-reviewer`, and `frontend-guidelines-reviewer`, which write to `docs/tasks/task-007-vehicle-card-status-banner/audit.md`.

- [ ] **Step 10: Confirm merge order**

Task-006 (`created_at` wipe) touches `apps/fleet-service/internal/vehicle/status.go`, which this branch rewrites. Land task-006 first. This branch degrades safely without it — a wiped `created_at` surfaces as an omitted `lastActivityAt` and an em-dash, never an absurd year-1 date — so this is a merge-order preference, not a blocker.

---

## Requirement coverage

| Requirement group | Task |
| --- | --- |
| FR-1.x card structure, height, responsiveness | 9, 10 |
| FR-2.x hero photo, variant, skeleton, placeholders | 8, 9 |
| FR-3.x banner treatment, fixed height, unknown status | 7, 9 |
| FR-4.x banner copy, axis-awareness, selection, fallbacks | 1, 3, 7 |
| FR-5.x stat strip, em-dashes, tabular figures | 9 |
| FR-6.x navigation, overlay link, Carfax preservation | 9, 10 |
| FR-7.x loading skeleton, empty state | 10 |
| FR-8.x resource attributes, read-only, omitempty, server-side selection | 3, 5 |
| FR-9.x widened gatherers, no extra query, boundary, adapter | 1, 2, 4 |
| NFR-1…5 performance and query count | 2 (measured), 4, 9 |
| NFR-6…8 security and Carfax protections | 5, 9 |
| NFR-9…13 accessibility | 7, 9, 10 |
| NFR-14…17 testing and the `make ci` gate | 1, 2, 3, 4, 5, 6, 7, 8, 9, 10 |
| Design D1–D9 | 3 (D1), 1 (D2), 1 (D3), 2 (D4), 9 (D5), 4 (D6), 4 (D7), 10 (D8, D9) |
