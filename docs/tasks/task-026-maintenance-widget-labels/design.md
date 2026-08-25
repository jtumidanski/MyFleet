# Maintenance Queue Labels — Design

Task: task-026-maintenance-widget-labels
PRD: `docs/tasks/task-026-maintenance-widget-labels/prd.md` (v1, approved)
Status: Draft
Created: 2026-08-25

---

## 1. Shape of the change

This is a presentation-layer fix confined to `apps/web`. No service, endpoint,
schema, or manifest is touched. The whole change is: two lookup tables, two
label functions, one extracted title helper, four render sites rewired, and the
loading gates widened to cover the queries the new labels depend on.

Nothing here is novel. `VehicleDetailPage` already builds a
`categoryId → name` `Map` with `useMemo` and hands it to
`UpcomingScheduleStrip`. This task promotes that one-off into a shared hook and
adopts it at the four sites that never got it.

### Findings from the code that shape the design

Three things surfaced during recon that the PRD did not know about. Each moves a
decision below.

**`MaintenanceQueueView` is not mounted anywhere.** A repo-wide grep for
`MaintenanceQueueView` returns only its own definition — it is absent from
`widgetRegistry.tsx`, from every page, and from every barrel. Two of the four
FR-LABEL-1 sites are therefore in dead code. Handling in §7.

**A source-pin test guards the vehicle-title expression.**
`components/frame/crumbs/VehicleNameCrumb.test.tsx:84-99` reads
`VehicleDetailPage.tsx` and `VehicleNameCrumb.tsx` off disk and asserts both
contain the literal string:

```
attributes.nickname?.trim() || `${attributes.year} ${attributes.make} ${attributes.model}`
```

Its own comment says the duplication is deliberate — extracting the rule would
mean editing `VehicleDetailPage`, which task-015 owns. Extracting a shared
helper and migrating those two files would break that test by design.
Handling in §4.

**Six call sites duplicate the title rule, not two.** `VehicleCard.tsx:53`,
`VehicleNameCrumb.tsx:29`, `VehicleDetailPage.tsx:129`,
`VehicleIdentityRail.tsx:37`, `ActivityPage.tsx:29`, and
`VehicleStatusWidget.tsx:39`. They are not all identical: `VehicleCard` appends
a second `.trim()` to the template literal; `VehicleStatusWidget` uses bare
`nickname ||` with no `.trim()` at all. FR-SHARED-2 scopes this task to
`VehicleCard`. Handling in §4.

---

## 2. Architecture

```
lib/utils/vehicleTitle.ts          pure: VehicleAttributes -> string
        ▲                                   ▲
        │                                   │
VehicleCard.tsx                    lib/hooks/api/labels.ts
                                     ├─ useCategoryNameMap()   -> { names: Map, isLoading }
                                     ├─ useVehicleTitleMap(id) -> { titles: Map, isLoading }
                                     ├─ categoryLabel(map, id) -> string
                                     └─ vehicleLabel(map, id)  -> string
                                            ▲          ▲
                    ┌───────────────────────┘          └──────────────┐
        OverdueMaintenanceWidget            UpcomingMaintenanceWidget  MaintenanceQueueView
```

Three new/changed modules, four rewired components. The hooks own *fetching and
memoizing*; the label functions own *the fallback string*. Splitting them is
what lets `UpcomingScheduleStrip` keep its raw-id fallback (FR-SHARED-4) while
consuming a compatible `Map<string, string>`, and it makes the fallback
independently unit-testable without a React render.

### 2.1 `lib/utils/vehicleTitle.ts` (new)

```ts
export function vehicleTitle(attributes: VehicleAttributes): string
```

Pure, no React. Sits beside `displayName.ts`, which solves the same class of
problem (a fallback chain over Go-marshalled attributes) and carries the same
`||`-not-`??` warning in its header comment. The body is `VehicleCard`'s current
expression verbatim, second `.trim()` included, so FR-SHARED-2's
byte-identical-output requirement holds by construction.

### 2.2 `lib/hooks/api/labels.ts` (new)

```ts
export const UNKNOWN_CATEGORY = 'Unknown category';
export const UNKNOWN_VEHICLE  = 'Unknown vehicle';

export function useCategoryNameMap(): { names: Map<string, string>; isLoading: boolean }
export function useVehicleTitleMap(fleetId: string | null | undefined):
  { titles: Map<string, string>; isLoading: boolean }

export function categoryLabel(names: Map<string, string>, id: string): string
export function vehicleLabel(titles: Map<string, string>, id: string): string
```

- `useCategoryNameMap` wraps `useMaintenanceCategories()` **with no `kind`
  argument** (FR-LABEL-3). Taking no parameter at all is the enforcement: there
  is no way for a caller to reintroduce the kind filter and silently break
  modification categories. It shares `maintenanceCategoryKeys.list({ kind:
  undefined })` with `VehicleDetailPage`, so the two dedupe.
- `useVehicleTitleMap` wraps `useVehicles(fleetId)`. Note `useVehicles` has **no
  `select`**, so the rows are `data?.data` — unlike the maintenance hooks, which
  do project. Getting this wrong yields an empty map and a widget full of
  `Unknown vehicle`, which is exactly the kind of failure the FR-LOAD-3 fallback
  makes silent. It is called out here and pinned by a test in §6.
- Both build their `Map` inside `useMemo` keyed on the query data (NFR:
  `Map`-based, built once per render pass, never inside the row loop).
- `categoryLabel` / `vehicleLabel` use `||`, not `??`. A category whose `name`
  marshalled as `""` falls through to the unknown string rather than rendering a
  blank cell (§6 of the PRD, `displayName.ts`'s documented trap).

Colocating the two pure functions with the two hooks rather than splitting them
into `lib/utils/` is deliberate: the map and its fallback are one contract, and
a reader wiring up a widget should not have to find two files to do it. The
functions stay independently testable because they take the map as an argument.

### 2.3 Row layout for the two dashboard widgets

A row now carries four facts inside a narrow 2-column widget. The existing
single-line `flex items-center justify-between` cannot hold them, so the row
becomes a fixed right rail (severity chip) beside a stacked, truncating left
column — `MaintenanceQueueView`'s treatment, which the PRD names as the
starting point.

```
┌──────────────────────────────────────────────┐
│ Oil Change                        [ Urgent ] │  ← name: text-sm font-medium, truncate
│ Weekend Truck                                │  ← vehicle: text-xs muted, truncate
│ Was due 3/14/2026                            │  ← date line: text-xs muted
│ At 75,000 miles                              │  ← mileage line: text-xs muted
└──────────────────────────────────────────────┘
```

Structure:

```
<li className="flex items-start justify-between gap-2 border-b pb-2 last:border-0">
  <div className="min-w-0 flex-1 space-y-0.5"> …four lines… </div>
  <SeverityChip … />           ← sibling, shrink-0
</li>
```

`min-w-0` on the left column is load-bearing: a flex child defaults to
`min-width: auto`, so without it `truncate` does nothing and a long category
name pushes the chip out of the card. `items-start` (not `items-center`) keeps
the chip aligned to the title now that rows have variable height.

Colour is unchanged. The widgets keep `SeverityChip` on `severity`; the two new
lines are plain `text-muted-foreground` and add no new colour combination, so
task-003's contrast matrix does not need revisiting. The `Overdue` / `Upcoming`
headings still carry state (FR-A11Y-1), and every added line is ordinary text in
reading order with no `aria-hidden` (FR-A11Y-2).

### 2.4 Loading gate

Each component ORs the `isLoading` of every query it reads:

- widgets: `queueLoading || categoriesLoading || vehiclesLoading`
- `MaintenanceQueueView`: `upcomingLoading || overdueLoading || categoriesLoading`

React Query v5's `isLoading` is `isPending && isFetching`. That single flag
satisfies both requirements that look contradictory on paper:

- **FR-LOAD-1** — while a supporting query is genuinely in flight, `isLoading`
  is true and the skeleton holds. No frame can show a UUID, because the resolved
  name is the *only* thing the row ever renders.
- **FR-LOAD-3** — when a supporting query *fails*, `isLoading` flips to false
  with `data` undefined. The gate opens, the map is empty, and rows render with
  `Unknown category` / `Unknown vehicle`. A failure degrades the label; it never
  blanks the widget.

A disabled query (`enabled: !!fleetId`) reports `isPending: true, isFetching:
false` → `isLoading: false`, so a null `fleetId` does not wedge the skeleton on
forever. Using `isPending` here instead of `isLoading` would.

### 2.5 Due context

Lifted from `MaintenanceQueueView.tsx:54-63` and `:91-100` unchanged, including
its guards, which already encode the two backend serialization traps:

- `nextDueDate &&` — Go marshals an unset string as `""`, so truthiness is the
  correct presence check.
- `nextDueMileage ? … : null` — `int` + `omitempty` means a pure-time schedule
  arrives with the field absent and a zero is indistinguishable from unset;
  falsy means "no mileage line" (FR-DUE-3). The `? :` form returns `null`, not
  `0`; `&&` on a number would render a literal `0` into the row.

Wording differs by widget only: `Was due <date>` in overdue, `Due <date>` in
upcoming (FR-DUE-2). Both use `toLocaleDateString()` / `toLocaleString()` to
match the existing treatment.

---

## 3. Alternatives considered

### 3.1 Where the names come from

**A. Client-side resolution from existing cached queries — chosen.** No backend
change; the category list is ~20 effectively-static rows already cached for 10
minutes; the vehicle list is already warm on the dashboard because
`VehicleStatusWidget` and `MileageTrendsWidget` are siblings on the same page
sharing `vehicleKeys.list({ fleetId })`. `UpcomingScheduleStrip` establishes
this as the house pattern for exactly this problem.

**B. Denormalize `categoryName` / `vehicleName` into the schedule Attributes.**
One fetch, no lookup, no unknown-fallback. Rejected: the queue handler would
have to join two more tables per request, the names go stale in any cached
response, and every future field wanting a label repeats the argument. Explicit
PRD non-goal.

**C. JSON:API `?include=category,vehicle` compound documents.** The correct
answer in the abstract, and the one the spec was written for.
`packages/shared-go/server` has no `Included` field at all, so this is a
framework-level change to the shared transport — orders of magnitude beyond a
labelling bug, and it would need a client-side normalizer too. Explicit PRD
non-goal. Worth its own task if a second consumer ever needs it.

### 3.2 Hook-with-map vs. prop-drilled map

**A. Each component calls the hooks itself — chosen.** The widgets are mounted
by `widgetRegistry` with only `{ fleetId }`; there is no parent that could
supply a map without inventing one. React Query dedupes the shared query keys,
so N widgets on a dashboard still issue one category request and one vehicle
request (NFR: no per-row fetch, one extra query per component served from
cache).

**B. Keep components presentational, resolve in the parent** — the
`UpcomingScheduleStrip` shape. Better testability, but it would mean giving the
widget registry a data-fetching layer it does not have, for two widgets. YAGNI.
The chosen shape stays testable by mocking the two hooks, which is what
`VehicleNameCrumb.test.tsx` already does.

### 3.3 One hook returning both maps

Rejected. `useCategoryNameMap` takes no arguments and is fleet-independent;
`useVehicleTitleMap` takes a `fleetId`. `MaintenanceQueueView` needs only the
first. Fusing them would make it fetch a vehicle list it does not render.

---

## 4. The vehicle-title extraction, and how far it goes

FR-SHARED-1/2 ask for one shared helper and a `VehicleCard` refactor. Six sites
duplicate the rule and two of them are pinned by a test that exists *to keep
them duplicated*.

**Decision: extract the helper, migrate `VehicleCard` only. Leave the other
five untouched, and leave `VehicleNameCrumb.test.tsx` untouched and passing.**

The pin test asserts a literal string is present in `VehicleDetailPage.tsx` and
`VehicleNameCrumb.tsx`. Neither file is in this task's scope, so neither
changes, so the test stays green with no edit. `VehicleCard` is not named by the
pin — and could not be, since its expression carries a second `.trim()` the pin
string does not have.

The alternative — migrate all six and rewrite the pin to assert every site calls
`vehicleTitle()` — is the better end state and is genuinely tempting while the
helper is being written. It is rejected here because it edits a page whose title
contract task-015 owns, changes `VehicleStatusWidget`'s rendered output (it has
no `.trim()` today, so a `"   "` nickname currently renders as blank space and
would start rendering the year/make/model), and turns a labelling fix into a
cross-cutting refactor. That is a separate task with its own PRD.

The design accepts the resulting oddity — a shared helper with one caller and
five holdouts — and records it as follow-up rather than pretending it is
resolved.

---

## 5. Open questions from the PRD, resolved

1. **Should the vehicle title link to the vehicle?** No. Plain text for v1, per
   the PRD's assumption. Widget rows are non-interactive today; making one line
   of one row a link — inside a `<li>` that is otherwise inert — is an
   inconsistent half-measure, and the row-as-link treatment `VehicleCard` uses
   (an `::after` overlay) is a layout change the PRD rules out. Revisit if
   dashboard rows become navigable as a whole.
2. **Row layout.** Settled in §2.3: stacked left column + fixed severity rail,
   mirroring `MaintenanceQueueView`.
3. **Helper locations.** Settled in §2.1/§2.2: `lib/utils/vehicleTitle.ts` for
   the pure title (beside `displayName.ts`), `lib/hooks/api/labels.ts` for the
   two map hooks and their label fallbacks. Not `lib/vehicleStats.ts` — that
   module is derived-metrics maths (`deriveOdometer`, `rankSchedule`), not
   string formatting.
4. **Styling for `Unknown category` / `Unknown vehicle`.** Same as a resolved
   value, per the PRD's assumption. Italics would read as emphasis, not as
   degradation, and there is no state in which a user can act on the
   distinction.

---

## 6. Testing

No test file exists today for `OverdueMaintenanceWidget`,
`UpcomingMaintenanceWidget`, or `MaintenanceQueueView`. All three get one.

**Unit, no render:**

- `lib/utils/vehicleTitle.test.ts` — nickname preferred; `"   "` (blank after
  trim) falls through to year/make/model; missing nickname falls through;
  `undefined` nickname does not throw. The blank case is the one FR-SHARED-2
  calls load-bearing.
- `lib/hooks/api/labels.test.ts` — `categoryLabel` / `vehicleLabel` on a hit, a
  miss (→ the unknown strings), and an empty-string value in the map (→ the
  unknown strings, not a blank).

**Component, hooks mocked** (the `VehicleNameCrumb.test.tsx` pattern —
`vi.mock` the hook module, not a live `QueryClient` — which is what makes the
loading and failure states reachable without racing):

- Category name renders in place of the id at all four sites; a
  `modification`-kind category resolves, proving the no-`kind` call
  (FR-LABEL-3).
- An unresolvable `categoryId` renders `Unknown category` and the UUID appears
  nowhere in the DOM.
- Widgets: nickname vehicle, year/make/model vehicle, and unresolvable →
  `Unknown vehicle`.
- Widgets: `Was due <date>` in overdue and `Due <date>` in upcoming;
  `At 75,000 miles` with separators; a pure-time schedule renders no mileage
  line and a pure-mileage schedule renders no date line, asserted by absence of
  the text rather than by counting elements.
- Loading gate: with the queue resolved but categories still `isLoading`, the
  skeleton renders and no UUID is in the document; with categories *failed*
  (`isLoading: false, data: undefined`) rows render with the fallback. These two
  are the same assertion pair for the vehicle query in the widgets.
- Empty-state copy asserted verbatim at all four sites (FR-LOAD-2), and the
  5-item cap asserted with a 7-item fixture (FR-LOAD-4).

**Guard:** one assertion that `useVehicleTitleMap` reads `result.data`, covered
naturally by feeding the mocked `useVehicles` a realistic
`{ data: [...] }` envelope rather than a bare array — the projection mismatch
noted in §2.2 fails this test loudly instead of degrading to `Unknown vehicle`.

**Verification:** `make fe-test`, `make fe-build`, `make ci`. No
`kustomize build` — no manifest changes. Then
`superpowers:requesting-code-review` (frontend-guidelines-reviewer at minimum)
with findings in `audit.md` before the PR.

---

## 7. Risks and follow-ups

**`MaintenanceQueueView` is dead code.** Nothing imports it. Two of the four
FR-LABEL-1 sites, and the tests written for them, exercise a component no user
can reach. The PRD names those exact lines, so this task **fixes it in place**
rather than deleting it — deletion is a scope call the PRD did not make, and
leaving a known-broken component behind while claiming the "zero rendered
`categoryId`" acceptance criterion would be worse. Flagged for a follow-up
decision: either mount it or delete it. Fixing it costs little; it is also the
reference implementation the two widgets are being aligned to, which is its own
argument for keeping it correct.

**Five holdout copies of the vehicle-title rule** (§4), one of which
(`VehicleStatusWidget`) is subtly wrong today — no `.trim()`, so a
whitespace-only nickname renders as blank. Not fixed here; a behaviour change to
an untouched widget does not belong in a labelling task. Follow-up task:
migrate all six sites to `vehicleTitle()` and convert
`VehicleNameCrumb.test.tsx`'s source pin into a "every site calls the helper"
assertion.

**Cold-cache flash on a non-dashboard mount.** On the dashboard the vehicle and
category queries are already warm from sibling widgets, so the widened gate
costs nothing visible. A widget mounted somewhere with a cold cache will hold
its skeleton for one extra round trip. That is the correct trade — FR-LOAD-1
explicitly prefers a longer skeleton to a visible UUID.

**No new authz surface.** All three queries are already fleet-scoped and
authorized server-side; the widgets render only what the caller could already
fetch. No new telemetry: supporting-query failures are absorbed by the
fallbacks by design (NFR-Observability), which is also why the §6 failure-path
tests matter — a broken lookup is silent in production.
