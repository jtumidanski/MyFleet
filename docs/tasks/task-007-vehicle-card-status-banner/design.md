# Vehicle Card Status Banner — Design

Task: [task-007](./prd.md)
Status: Approved for planning
Created: 2026-08-02
PRD: [`prd.md`](./prd.md) (v1, approved)
Visual reference: [`ux-prototype.html`](./ux-prototype.html) — "Conditional" grids

---

## 1. Summary

Three pieces of work, in dependency order:

1. **`maintenanceschedule` computes breach detail.** A pure function beside `DueState`
   turns a schedule + now + current mileage into zero or more *axis breaches* — one per
   axis that is actually breaching — each carrying its magnitude and a
   threshold-normalized urgency score. A widened processor method returns those per
   schedule for a vehicle. No new query.
2. **`vehicle` selects the governing breach and reports it.** `StatusDeps` gains a single
   `Derive` call returning status + last-activity time + the one governing `NextDue`. The
   selection is a pure function in `vehicle`, fed by a port type that carries the
   pre-computed urgency so `vehicle` never learns what a threshold is.
3. **`apps/web` rebuilds the card.** A pure `vehicleBanner()` module maps
   `(status, nextDue, lastActivityAt, now)` → `{tone, icon, text}`. `VehicleCard` becomes a
   vertical stack with a 16:9 hero and an overlay-link root. `VehiclePhotoThumbnail` gains
   a box override so all four of its states follow the hero's aspect ratio.

The seam that makes this work is the port type between §2 and §3: it is rich enough that
the frontend does no ranking and the vehicle domain does no recurrence math.

## 2. Decisions taken during design

The PRD left six open questions (§9) and one genuine gap in FR-4.6. Resolved here:

| # | Question | Decision |
| --- | --- | --- |
| D1 | PRD §9.6 — `nextDue` nested or flat? | **Nested object**, with `*int` for `miles`/`days` |
| D2 | FR-4.6 gap — how do you rank *upcoming* schedules? | A single monotone `Urgency` scale spanning both states |
| D3 | Where does breach magnitude get computed? | In `maintenanceschedule`, beside the thresholds |
| D4 | Fresh or stored `NextDueDate`/`NextDueMileage`? | **Freshly recomputed**, not the stored columns |
| D5 | PRD §9.1 — thumbnail sharpness at hero size | Ship `thumbnail`; no variant change |
| D6 | PRD §9.2 — task-006 sequencing | Zero-guard decouples us; still land task-006 first |
| D7 | PRD §9.3 — can Inactive carry due detail? | Invariant confirmed; asserted by test |
| D8 | PRD §9.4 — grid density at 3 columns | Keep `lg:grid-cols-3` unchanged |
| D9 | PRD §9.5 — text selection on the card | Accepted loss; overlay stays |

### D1 — `nextDue` is a nested object

The vehicle resource has only flat scalar attributes today, so PRD §9.6 asked whether a
nested object breaks convention. It does not: `dashboard/rest.go:23,32` already nests
(`Widgets []WidgetResource`), so nesting is precedented in this service.

More importantly, flattening to `nextDueState` / `nextDueAxis` / `nextDueMiles` /
`nextDueDays` makes four independently-optional fields out of one value with an internal
invariant — *axis determines which magnitude field is present*. Illegal combinations
(`axis: "time"` with a `miles` value; a `miles` with no `state`) become representable, and
the client's "is there due detail at all?" check (FR-4.8) turns into a multi-field
presence test. The nested form makes presence atomic and the invariant testable:

```go
type NextDue struct {
    State string `json:"state"`           // upcoming | overdue
    Axis  string `json:"axis"`            // time | mileage
    Miles *int   `json:"miles,omitempty"` // non-nil iff Axis == "mileage"
    Days  *int   `json:"days,omitempty"`  // non-nil iff Axis == "time"
}
```

`*int` rather than `int` matters: an upcoming time-axis schedule due *today* has
`days == 0`, and plain `omitempty` would silently drop the key, leaving the client an
`axis: "time"` object with no magnitude. Exactly one pointer is non-nil, always.

### D2 — Ranking upcoming schedules (a gap in FR-4.6)

FR-4.6 says to select "the largest breach normalized against its own due-soon threshold".
That is well-defined for **overdue** — bigger overrun, more urgent — and backwards for
**upcoming**, where there is no breach, only remaining margin. Ranking upcoming schedules
by "largest normalized value" would surface the *least* imminent one: a service due in 490
miles would outrank one due in 20.

Resolved with a single monotone urgency scale, higher = more urgent, continuous across the
state boundary:

```
overdue:   Urgency = 1 + breach    / threshold     ( > 1 )
upcoming:  Urgency = 1 - remaining / threshold     ( 0 … 1 )
```

`remaining` never exceeds the threshold — `DueState` only returns `upcoming` inside the
30-day / 500-mile window (`recurrence.go:44-49`) — so the upcoming branch stays in `[0,1]`
and the two branches never overlap. Within a single state, "max Urgency" is now correct in
both directions, and FR-4.6's intent ("a 600-mile overrun outranks a 5-day overrun") is
preserved exactly: `1 + 600/500 = 2.2` beats `1 + 5/30 = 1.17`.

Ties resolve to the mileage axis (FR-4.6), then to the lowest schedule ID. The second
tiebreak exists because the first is not total: two mileage schedules with identical
breaches would otherwise order by map/slice iteration.

### D3 — Breach magnitude is computed in `maintenanceschedule`

FR-8.5 requires server-side selection but does not say which package. Two candidates:

- **Selection entirely in `vehicle`.** Requires the thresholds (500 mi / 30 days) to reach
  `vehicle`, either as an import (forbidden by FR-9.5) or smuggled onto every port row.
  Threshold semantics would then live in two places.
- **Magnitude + normalization in `maintenanceschedule`; governing-schedule selection in
  `vehicle`.** Chosen.

The split follows what each domain actually owns. `maintenanceschedule` owns recurrence
math, thresholds, and what "overdue by 1,120 miles" means; it emits per-axis breaches with
urgency already normalized. `vehicle` owns "which state governs this vehicle" — the same
overdue-beats-upcoming priority `status.Derive` already encodes — and picks the max-urgency
breach within it. Neither reaches into the other, and `vehicle` never sees a threshold.

### D4 — Recompute next-due, don't read the stored columns

PRD §6 sources the numbers from `Model.NextDueDate()` / `Model.NextDueMileage()`. Those are
persisted columns, refreshed by the hourly recompute job (`processor.go:189`) and on
`Update` (`processor.go:77`). Meanwhile `DueState` — which decides the *state* we report —
always recomputes from `AsSchedule()` (`recurrence.go:34`).

Reading a stale column beside a fresh state can contradict: a schedule completed 20 minutes
ago reports state `ok` from fresh math while the stored `next_due_mileage` still describes
the previous cycle. Breach detail is therefore computed from `NextDue(s.AsSchedule())`, the
same call `DueState` makes, so state and magnitude are always derived from one snapshot.
This is a deliberate deviation from the PRD's phrasing, not an oversight.

### D5 — Ship the `thumbnail` variant

PRD FR-2.6 already decides this and §9.1 only asks whether the result looks soft. It cannot
be settled by argument, and shipping `display` (1280px) for N cards is precisely the cost
task-005 §4.7 existed to avoid. Ship `thumbnail`; if it reads soft on a high-DPI display at
`lg:grid-cols-3`, that is a variant-sizing task in `media-service`, not a change here.

### D6 — task-006 sequencing, and why we degrade safely regardless

`DeriveStatus` falls back to `m.CreatedAt()` when there is no activity record
(`status.go:51-53`), and task-006 is fixing `created_at` being wiped to zero. The exposed
`lastActivityAt` uses the *post-fallback* value — the same value status derivation used, so
the card can never show "—" beside a status that was computed from a real timestamp.

The wiped-`created_at` case is then handled by omission, not by an absurd date: if the
post-fallback value is still zero, the attribute is omitted and the card renders an em-dash
(FR-5.6). So this task does not *depend* on task-006. The status misreport task-006 names
remains task-006's to fix; both tasks touch `vehicle/status.go`, so land task-006 first to
avoid a conflict.

### D7 — Inactive never carries due detail (invariant confirmed)

PRD §9.3 asked the design to confirm rather than assume. Confirmed: `status.Derive`
(`status/derive.go:14-27`) returns `Inactive` only after both the overdue and upcoming scans
fall through, so a vehicle reported Inactive has no non-ok schedule and `selectNextDue`
returns nil by construction. This is not defended against in code — it is a property of the
existing derivation — but it *is* asserted by a test (§7), so a future change to `Derive`
that breaks it fails loudly instead of producing an Inactive card with a tinted banner.

### D8 / D9 — Grid density and text selection

Both are accepted as-is. `lg:grid-cols-3` stays; whether the taller card wants two wider
columns is a judgement to make against the running UI, and changing it pre-emptively would
be guessing. The overlay pattern's loss of text selection over the card body (FR-6.9) is
accepted — the alternative is returning to explicit buttons, which is the layout task-005
already shipped and this task exists to replace.

---

## 3. Backend — `maintenanceschedule`

### 3.1 `DueBreaches` — a pure function beside `DueState`

New in `recurrence.go`, mirroring `DueState`'s structure so the two cannot drift:

```go
// AxisBreach describes one axis of one schedule that is currently breaching, with
// its magnitude and a threshold-normalized urgency (higher = more urgent).
type AxisBreach struct {
    Axis    string  // "time" | "mileage"
    Days    int     // set when Axis == "time"
    Miles   int     // set when Axis == "mileage"
    Urgency float64 // overdue: 1 + breach/threshold; upcoming: 1 - remaining/threshold
}

// DueBreaches returns one entry per axis that is itself breaching, for a schedule
// whose DueState is non-ok. Returns nil for an "ok" schedule.
//
// A hybrid schedule is "overdue" if EITHER axis is overdue, so the per-axis
// conditions are re-tested here rather than inferred from the schedule's state:
// the time axis of a mileage-overdue hybrid may be perfectly fine.
func DueBreaches(s Schedule, today time.Time, currentMileage int, th Thresholds) []AxisBreach
```

Branch conditions are copied verbatim from `DueState` (`recurrence.go:38-49`) so an axis is
reported breaching under exactly the condition that made the schedule non-ok:

| State | Axis | Condition | Magnitude |
| --- | --- | --- | --- |
| overdue | time | `timed && today.After(nd)` | `days = today - nd`, floored at **1** |
| overdue | mileage | `miled && currentMileage > nm` | `miles = currentMileage - nm` |
| upcoming | time | `timed && !today.Before(nd.AddDate(0,0,-DueSoonDays)) && !today.After(nd)` | `days = nd - today`, floor, may be **0** |
| upcoming | mileage | `miled && currentMileage >= nm-DueSoonMiles && currentMileage <= nm` | `miles = nm - currentMileage` |

Day counts use whole days (`int(d.Hours() / 24)`). Overdue is floored at 1 because "overdue
by 0 days" is nonsense — anything past the due instant is at least a day late in the copy.
Upcoming is allowed to be 0 and means "due today"; the frontend has a copy rule for it
(§5.2).

The upcoming rows carry an upper bound (`<= nm`, `!today.After(nd)`) the `DueState` branches
omit, because `DueState` reaches those branches only after the overdue branches returned.
`DueBreaches` evaluates them independently per axis, so the bound has to be explicit or a
hybrid overdue on mileage would report its time axis as "upcoming in -40 days".

### 3.2 `ScheduleDueByVehicle` replaces `ScheduleStatesByVehicle`

```go
// ScheduleDue is one active schedule's live due-state plus, when non-ok, the
// per-axis breach detail behind it.
type ScheduleDue struct {
    ScheduleID string
    State      string // ok | upcoming | overdue
    Breaches   []AxisBreach
}

func (pr *Processor) ScheduleDueByVehicle(vehicleID string) ([]ScheduleDue, error)
```

The body is today's `ScheduleStatesByVehicle` (`processor.go:167-178`) with one added call
per row. Same `ListActiveByVehicle` query, same `DueState` call, same loop — the data was
already in hand and discarded at the return boundary (FR-9.2).

`ScheduleStatesByVehicle` is **deleted**, not kept alongside. It has exactly one consumer
(`vehicle/status.go:41`) and leaving both would create two paths to the same rows.
`ListActiveByFleet`, `Queue`, `DueAcrossAllFleets`, and `RecomputeAll` are untouched, so the
dashboard (FR-9.6) and the reminder feed are unaffected.

## 4. Backend — `vehicle`

### 4.1 The port, restated

`vehicle` must not import `maintenanceschedule` (FR-9.5), so it declares its own shape:

```go
// ScheduleDue mirrors maintenanceschedule.ScheduleDue across the domain boundary.
type ScheduleDue struct {
    ScheduleID string
    State      string
    Breaches   []Breach
}

type Breach struct {
    Axis    string  // "time" | "mileage"
    Days    int
    Miles   int
    Urgency float64
}

type ScheduleDueGatherer interface {
    ScheduleDueByVehicle(vehicleID string) ([]ScheduleDue, error)
}
```

This duplicates a struct shape across the boundary. That is the intended cost: CLAUDE.md
prefers a straightforward move over a re-exported type alias, and an alias here would make
`vehicle` import `maintenanceschedule` transitively — the exact coupling FR-9.5 forbids.
The mapping lives in the composition root (§4.5), field-for-field. A field renamed or
removed on either side is a compile error there; a field added is not — the adapter's keyed
literal leaves it zero-valued silently — so the boundary relies on the mapping being touched
deliberately, not on the compiler catching every drift.

### 4.2 `selectNextDue` — the governing breach

Pure, in a new `internal/vehicle/nextdue.go`:

```go
// selectNextDue picks the single breach that explains the vehicle's status:
// the state that governs (overdue beats upcoming — the same priority
// status.Derive applies), then the max-urgency breach within it.
// Ties: mileage axis wins (FR-4.6), then lowest schedule ID.
// Returns nil when no schedule is non-ok.
func selectNextDue(dues []ScheduleDue) *NextDue
```

Algorithm:

1. `governing` = `"overdue"` if any `due.State == "overdue"`, else `"upcoming"` if any, else
   return nil.
2. Over every `Breach` of every `due` whose `State == governing`, keep the max by
   `(Urgency, Axis == "mileage", -ScheduleID)` in that precedence.
3. Return `&NextDue{State: governing, Axis: b.Axis, Miles/Days: &b.…}` — setting exactly the
   pointer matching the axis.

`Urgency` arrives pre-computed, so this function contains no domain math and no thresholds;
it is a max with a documented tiebreak, and it is where every FR-4.6 test case lands.

### 4.3 `StatusDeps.Derive` — one call, three values

`DeriveStatus` is replaced by:

```go
// Derived is everything the vehicle resource exposes that is computed on read.
type Derived struct {
    Status         string    // "" when a gather failed
    LastActivityAt time.Time // zero when unavailable
    NextDue        *NextDue  // nil when no schedule is non-ok
}

func (d StatusDeps) Derive(m Model, now time.Time) Derived
```

The body is today's `DeriveStatus` (`status.go:40-60`) with the return widened:

- Either gather error → `Derived{}`. All three values are then zero/empty and every
  attribute is omitted (FR-8.4). Reads still succeed; this is unchanged behaviour, just
  applied to three fields instead of one.
- `status.Derive` is called with `ScheduleStates: states(dues)`, where `states` projects each
  `ScheduleDue` down to its `State` string. Its `Input`, its rule, and its output are
  untouched (FR-9.3) — the projection is the only new code on that path, which is what makes
  "status values are byte-for-byte unchanged" (NFR-16) a testable claim rather than a hope.
- The `last.IsZero() → m.CreatedAt()` fallback stays exactly as-is, and the post-fallback
  value is what lands in `Derived.LastActivityAt` (D6).

### 4.4 Transport

`rest.go` gains the two attributes and one transform:

```go
type Attributes struct {
    // …existing fields unchanged…
    Status         string   `json:"status,omitempty"`
    LastActivityAt string   `json:"lastActivityAt,omitempty"` // RFC 3339 UTC
    NextDue        *NextDue `json:"nextDue,omitempty"`
}

// TransformDerived converts a Model to a JSON:API Resource, attaching all
// read-only derived attributes.
func TransformDerived(m Model, d Derived) server.Resource
```

`LastActivityAt` is a **string**, not a `time.Time`: `encoding/json`'s `omitempty` has no
effect on a struct, so a `time.Time` field would emit `"0001-01-01T00:00:00Z"` for the
absent case and defeat FR-8.4. `TransformDerived` formats with `time.RFC3339` in UTC and
leaves the field empty when the value is zero.

`TransformWithStatus` is **replaced** by `TransformDerived` — its only callers are
`resource.go:52` and `:121`, both of which now pass a `Derived`. `Transform` (used by
create/update/restore/primary-image) and `TransformSlice` keep their current shape and
continue to emit no derived attributes, which is correct: those responses echo a write, and
none of the three derived values is a property of the write.

`resource.go` changes at the two read sites only:

```go
// list, :52
resources = append(resources, TransformDerived(m, statusDeps.Derive(m, now)))

// single, :120
server.WriteJSON(w, http.StatusOK, server.Document{Data: TransformDerived(m, statusDeps.Derive(m, time.Now().UTC()))})
```

Read-only enforcement (FR-8.3, NFR-7) needs **no new code**. Both POST and PATCH bind to
narrow anonymous structs (`resource.go:61-70`, `:125-129`) that list their accepted fields
explicitly; an unknown `lastActivityAt` or `nextDue` in a request body is simply not bound.
This is the same mechanism that already makes `status` unwritable. A test asserts it rather
than trusting it.

### 4.5 Composition root

`vehicleStatusDeps.Schedules = scheduleProc` (`cmd/main.go:122-125`) currently works by
structural typing because the return type is `[]string`. With a named struct it cannot, so
`main.go` gains the adapter FR-9.5 anticipates:

```go
// scheduleDueAdapter maps maintenanceschedule's due detail onto the vehicle
// domain's port type. The mapping lives here, in the composition root, so
// neither domain imports the other.
type scheduleDueAdapter struct{ p *maintenanceschedule.Processor }

func (a scheduleDueAdapter) ScheduleDueByVehicle(vehicleID string) ([]vehicle.ScheduleDue, error)
```

Field-for-field, no logic. `Activity` stays bound directly to `activityProc` — its interface
still returns `(time.Time, error)`.

---

## 5. Frontend

### 5.1 Types

`types/models/vehicle.ts`:

```ts
export interface VehicleNextDue {
  state: 'upcoming' | 'overdue';
  axis: 'time' | 'mileage';
  miles?: number;  // present iff axis === 'mileage'
  days?: number;   // present iff axis === 'time'
}

export interface VehicleAttributes {
  // …existing…
  lastActivityAt?: string;   // RFC 3339
  nextDue?: VehicleNextDue;
}
```

`CreateVehicleAttributes` and `UpdateVehicleAttributes` are not extended (FR-8.3).

### 5.2 `vehicleBanner` — the pure copy module

New file `components/features/vehicles/vehicleBanner.ts`. No React import, so it is
unit-testable without rendering (NFR-14):

```ts
export type BannerTone = 'danger' | 'warning' | 'quiet';
export type BannerIcon = 'overdue' | 'upcoming' | 'healthy' | 'inactive' | 'unknown';

export interface BannerContent {
  tone: BannerTone;
  icon: BannerIcon;
  text: string;
}

export function vehicleBanner(
  attributes: Pick<VehicleAttributes, 'status' | 'nextDue' | 'lastActivityAt'>,
  now: Date,
): BannerContent;
```

The icon is a **string token**, not a component. Keeping lucide out of this module is what
lets the tests assert on plain data (`{tone: 'danger', icon: 'overdue', text: '…'}`) instead
of on rendered output; `VehicleCard` owns the one `Record<BannerIcon, LucideIcon>` map.
`now` is injected for the same reason — the Inactive duration and nothing else depends on
it, and a test that cannot pin "now" cannot assert "11 months".

Copy rules:

| Status | Tone | Icon | Text |
| --- | --- | --- | --- |
| `Overdue` + `nextDue` | danger | overdue | `Service overdue by {amount}` |
| `Overdue`, no `nextDue` | danger | overdue | `Maintenance overdue` |
| `Upcoming Maintenance` + `nextDue`, days > 0 or mileage | warning | upcoming | `Service due in {amount}` |
| `Upcoming Maintenance` + `nextDue`, `axis: time`, `days === 0` | warning | upcoming | `Service due today` |
| `Upcoming Maintenance`, no `nextDue` | warning | upcoming | `Maintenance due soon` |
| `Healthy` | quiet | healthy | `Up to date` |
| `Inactive` + `lastActivityAt` | quiet | inactive | `No activity in {n} months` |
| `Inactive`, no `lastActivityAt` | quiet | inactive | `No activity recorded` |
| absent / unrecognised | quiet | unknown | `Status unavailable` |

`{amount}` is axis-driven, never inferred from which field happens to be set:

- `axis === 'mileage'` → `` `${miles.toLocaleString('en-US')} mi` `` → `"1,120 mi"`
- `axis === 'time'`, `days < 60` → `"1 day"` / `"12 days"`
- `axis === 'time'`, `days >= 60` → `` `${Math.round(days / 30)} months` ``

A `nextDue` whose axis field is missing (a server bug, or a hand-rolled fixture) falls back
to the no-detail row for its status — tinted, generic copy. Urgency survives bad data
(FR-4.8).

`{n} months` for Inactive is `Math.floor(daysSince / 30)`, floored at 12: the Inactive
threshold is 365 days (`status.go:11`), so anything reaching this branch is at least a year
stale, and a rounding artefact reading "No activity in 11 months" on a 366-day-old vehicle
would be wrong in a way the user can see.

The status→tone map is keyed off the same `KNOWN_STATUSES` guard `VehicleCard` uses today
(`VehicleCard.tsx:11-22`); that helper moves into this module so there is one definition of
"is this a status I recognise" (FR-3.7).

### 5.3 `formatRelativeTime` in `packages/ui-components`

There is no relative-time helper and no date library in the repo (`date-fns`, `dayjs`, and
`Intl.RelativeTimeFormat` all return zero hits). Adding one beside `formatMileage`
(`formatters.ts`) rather than a third dependency:

```ts
export function formatRelativeTime(iso: string, now: Date = new Date()): string;
```

Backed by `Intl.RelativeTimeFormat('en-US', { numeric: 'auto' })`, which gives "yesterday"
and "last month" for free at `numeric: 'auto'`. Ladder: < 1 day → `"today"`; < 7 days →
days; < 5 weeks → weeks; < 12 months → months; else years. `now` is injected so tests are
deterministic. Returns `''` for an unparseable input, which the card renders as an em-dash.

### 5.4 `VehiclePhotoThumbnail` — a box override

`BOX` is a hardcoded `h-20 w-20 shrink-0 rounded-md` (`:11`) applied to all four states. The
only consumers are `VehicleCard` and its own test, so this is a safe widening:

```ts
interface VehiclePhotoThumbnailProps {
  mediaId?: string;
  vehicleLabel: string;
  /** Overrides the default 80×80 square. Applied to ALL states so they stay identical. */
  boxClassName?: string;
  className?: string;
}
```

`boxClassName` defaults to today's `BOX` and is threaded through the image, skeleton, and
both placeholder branches via `cn(boxClassName, …, className)`, so FR-2.3/FR-2.4's
"identical dimensions in every state" is structurally guaranteed rather than restated in
four places. The card passes `'aspect-[16/9] w-full rounded-none'`. The component's
`useMediaContentUrl(mediaId, 'thumbnail')` call, its no-toast behaviour, and its
"No photo" / "Photo unavailable" labels are all unchanged (FR-2.2, FR-2.5).

### 5.5 `VehicleCard` — structure

```
Card (relative isolate, p-0, group)
├── div.overflow-hidden.rounded-t-lg      ← photo wrapper; clips the hero's corners
│   └── VehiclePhotoThumbnail             ← aspect-[16/9] w-full
├── div  banner                           ← fixed height, tone classes
├── div  body (px-4 pt-3)
│   ├── Link title  (overlay)
│   └── div subtitle
├── div  stat strip (grid-cols-2, border-t)
└── div  footer (fixed height, Carfax only)
```

`overflow-hidden` sits on the **photo wrapper**, not on the card root. On the root it would
clip the card link's focus ring (FR-6.8); on the wrapper it does the one job it is needed
for — rounding the hero's top corners to match the card.

**Banner.** Fixed height (`h-9`) on every card regardless of tone, with `truncate` on the
text (FR-3.1, FR-4.10). Tone classes, taken as-is from the tokens task-003 measured
(NFR-11) — no new colour combinations:

| Tone | Classes |
| --- | --- |
| danger | `bg-danger-subtle text-danger-subtle-foreground border-b border-danger-border` |
| warning | `bg-warning-subtle text-warning-subtle-foreground border-b border-warning-border` |
| quiet | `bg-card text-muted-foreground border-b border-border` |

Note this diverges from the prototype's `.banner.healthy`, which tints Healthy with
`success-subtle`. That is the prototype's *always-on* grid; the chosen "Conditional" grid
uses its `.quietrow`, and FR-3.3 is explicit that Healthy and Inactive carry no tint. Colour
appears only where action is required.

Every banner renders `<Icon aria-hidden />` plus text, so state is never colour-only
(FR-3.4, NFR-10). `StatusBadge` is not imported (FR-3.5).

**Overlay link.** The mechanism that restores whole-card navigation without nesting
(FR-6.1–6.4):

```tsx
<Link
  to={`/vehicles/${vehicle.id}`}
  data-card-link
  className="after:absolute after:inset-0 after:content-[''] focus-visible:outline-none"
>
  {title}
</Link>
```

- The card root is `relative`, so `after:inset-0` resolves to the whole card.
- The anchor is a real `<a href>` (react-router `Link`), preserving middle-click,
  cmd/ctrl-click, and the context menu (FR-6.4) — the property task-005's test already
  asserts, retargeted at the card link.
- Its accessible name is the vehicle title, so a grid of cards is a list of distinct link
  names with no `aria-label` needed (FR-6.3, NFR-9).
- `isolate` on the root creates a local stacking context so the z-indices below cannot leak
  into or out of the card.

**Carfax.** Unchanged from task-005 in every respect (FR-6.7) except one class: the wrapping
`Button asChild` gets `relative z-10`, lifting it above the overlay pseudo-element so it
receives its own clicks (FR-6.6). This is the specific regression the layout risks and it
gets its own test.

**Focus ring.** The overlay anchor has no visible box of its own, so its ring is suppressed
and the *card* takes it:

```
has-[a[data-card-link]:focus-visible]:ring-2
has-[a[data-card-link]:focus-visible]:ring-ring
has-[a[data-card-link]:focus-visible]:ring-offset-2
```

The `data-card-link` attribute scopes the selector so focusing Carfax does not also ring the
whole card; Carfax keeps the Button component's own ring. Tailwind 3.4
(`apps/web/package.json:45`) supports the `has-` variant.

**Chevron removed** (FR-6.5). The footer holds only Carfax and keeps a fixed height so a
VIN-less card is the same height as its neighbours (FR-1.2).

**Stat strip.** `grid grid-cols-2 gap-4 border-t border-border px-4 py-3`, each slot a
`text-xs uppercase text-muted-foreground` label over a `tabular-nums` value (FR-5.4).
Odometer uses `formatMileage`; Last activity uses `formatRelativeTime`. Both render `—` when
their source is absent, and neither slot is ever omitted (FR-5.5, FR-5.6). Next-service
distance is deliberately absent (FR-5.7).

**Hover.** `transition-shadow hover:shadow-md` on the root — no new token; the repo has no
`shadow-lift` and the prototype's custom shadow is not worth introducing one for.

### 5.6 Skeleton

`VehicleCardSkeleton` is exported from `VehicleCard.tsx` and built from the same region
structure — an `aspect-[16/9]` box, then bars sized to the banner, title, subtitle, strip,
and footer. `VehicleList` renders six of them and its `h-40` magic number and the comment
justifying it (`VehicleList.tsx:14-23`) are deleted. Co-locating the skeleton with the card
is what stops the two drifting: a region added to one is visibly missing from the other in
the same file (FR-7.1). The empty state is untouched (FR-7.2).

---

## 6. Data flow

```
GET /fleets/{id}/vehicles
  └─ for each vehicle (unchanged loop, resource.go:51)
       StatusDeps.Derive(m, now)
         ├─ Schedules.ScheduleDueByVehicle(id)      ← query 1 (was already here)
         │    └─ per row: DueState + DueBreaches    ← pure, same snapshot
         ├─ Activity.LastActivityByVehicle(id)      ← query 2 (was already here)
         ├─ status.Derive(states(dues), last, now)  ← unchanged
         └─ selectNextDue(dues)                     ← pure
       TransformDerived(m, derived)
```

Query count per vehicle is 2, before and after (NFR-1). No new HTTP call per card: the list
response carries everything but the photo bytes (NFR-2), and those go through
`useMediaContentUrl`'s React Query cache keyed by `(id, variant)`, so a media id shared
across cards is fetched once (NFR-3). The banner, title, strip, and both links render
immediately and are usable while the photo is still resolving (NFR-4) — nothing in the card
awaits the image.

The pre-existing 2N shape is neither worsened nor fixed (NFR-5).

---

## 7. Testing

### Backend

`maintenanceschedule/recurrence_test.go` — `DueBreaches`:

- mileage-only overdue / upcoming; time-only overdue / upcoming; `ok` returns nil
- hybrid breaching on mileage only, on time only, and on both (two entries)
- hybrid overdue on mileage with a healthy time axis reports **one** breach, not a
  negative-day "upcoming" second entry — the bound added in §3.1
- overdue by a fraction of a day floors to 1; upcoming due today is 0
- `Urgency` values: `1 + 600/500 = 2.2` outranks `1 + 5/30 ≈ 1.167`; upcoming 20 mi
  remaining (`0.96`) outranks 490 mi remaining (`0.02`)

`maintenanceschedule/processor_test.go` — `ScheduleDueByVehicle` returns state + breaches per
schedule; a fake `Provider` counts `ListActiveByVehicle` calls and asserts exactly one per
invocation (the NFR-1 / acceptance-criteria "same number of queries", measured rather than
inspected).

`vehicle/nextdue_test.go` — `selectNextDue`, table-driven, one row per NFR-14 case:

- nil when all schedules are ok
- overdue outranks upcoming even when the upcoming one is more urgent on its own scale
- max-urgency selection across several overdue schedules
- mileage tiebreak on equal urgency; schedule-ID tiebreak on equal urgency and axis
- the returned pointer matches the axis: `Miles` non-nil and `Days` nil for mileage, and
  the reverse for time

`vehicle/status_test.go` (new) — `StatusDeps.Derive` with fake gatherers:

- schedule-gather error and activity-gather error each yield `Derived{}`
- the `CreatedAt` fallback still applies, and a zero created-at leaves `LastActivityAt` zero
- **Inactive implies `NextDue == nil`** (D7)
- status strings are byte-for-byte identical to today's `DeriveStatus` across the existing
  fixtures (NFR-16)

`vehicle/rest_test.go` (new) — `TransformDerived`: both attributes omitted from the JSON when
`Derived` is zero; `lastActivityAt` is RFC 3339; `nextDue` emits `miles` xor `days`;
`Transform` (write path) emits neither. Plus a handler-level test that a POST and a PATCH
body carrying `lastActivityAt` / `nextDue` changes nothing (NFR-7).

### Frontend

`vehicleBanner.test.ts` — every row of the §5.2 table, plus: mileage thousands separator;
`1 day` singular; the 60-day → months threshold at 59/60; `Math.round` on months;
`axis: 'time'` never renders a mileage figure and vice versa; a malformed `nextDue` falls
back to generic tinted copy; unrecognised status → quiet + "Status unavailable".

`formatRelativeTime.test.ts` — each ladder rung with an injected `now`, plus the unparseable
input.

`VehicleCard.test.tsx` — extended, keeping every task-005 assertion that still applies:

- photo present / absent / failed (existing three, retargeted at the hero)
- each of the four statuses renders the right text, and the tinted-vs-quiet distinction is
  asserted on the banner element's classes
- absent status → "Status unavailable" and no crash; unrecognised status → same
- Overdue with `nextDue` absent → still tinted, "Maintenance overdue"
- `StatusBadge` renders nowhere on the card
- missing `currentMileage` and missing `lastActivityAt` each render an em-dash in their slot
- the card link is a real `<a href="/vehicles/v1">` named for the vehicle
- **clicking Carfax does not navigate to the detail page** — the FR-6.6 regression, asserted
  via a `MemoryRouter` location probe, not by inspecting classes
- Carfax `target`/`rel`/VIN-substitution and the runtime-config subscription tests carry over
  verbatim
- tab order is card link then Carfax (NFR-13)

`VehiclePhotoThumbnail.test.tsx` — one added case: a `boxClassName` override reaches all four
states.

`make ci` gates the branch (NFR-17).

---

## 8. Risks

- **The overlay is easy to get subtly wrong.** If the Carfax button loses its `z-10`, or a
  future wrapper introduces a stacking context, Carfax silently starts navigating to the
  detail page. It looks fine and behaves wrong. The dedicated FR-6.6 test is the only thing
  standing between that and production.
- **Two struct shapes must stay in sync** across the `maintenanceschedule` ↔ `vehicle`
  boundary. The adapter only catches a renamed or removed field at compile time; an added
  field is silently zero-valued instead, so keeping the two in sync still means deliberately
  touching all three declarations.
- **`Urgency` is a designed quantity, not a PRD requirement.** D2 resolves a real gap, and if
  the resulting rankings read wrong against real fleet data, the fix is a formula change in
  one pure function with its own tests — cheap, but a change nonetheless.
- **Hero thumbnails may look soft** at `lg:grid-cols-3` on high-DPI displays (D5). Visible,
  low-severity, and deliberately deferred.
- **Card height grows substantially.** Six vehicles no longer fit one screen. D8 accepts
  this pending a look at the running UI.

## 9. Out of scope

Everything in PRD §2's non-goals, unchanged: no maintenance category name on the card, no
photo-upload or detail-page changes, no `media-service` change, no migrations, no change to
status derivation rules or thresholds, no grid sorting or filtering, no dashboard changes.
Batching the list handler's 2N gathers is a separate task (NFR-5).
