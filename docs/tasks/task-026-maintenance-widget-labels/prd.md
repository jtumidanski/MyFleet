# Maintenance Queue Labels — Product Requirements Document

Version: v1
Status: Draft
Created: 2026-08-25
---

## 1. Overview

The dashboard's **Overdue Maintenance** widget renders each queue row's primary
label as `item.attributes.categoryId` — a raw UUID. A user looking at their
dashboard sees `a3f2c1e0-…` where they expect "Oil Change". The same defect is
present verbatim in three sibling call sites: the **Upcoming Maintenance**
widget, and both the overdue and upcoming halves of `MaintenanceQueueView`.

The fix is not a new capability — the correct pattern already exists in this
codebase. `UpcomingScheduleStrip` accepts a `categoryId → name` map resolved by
its caller from `useMaintenanceCategories()`, and falls back to the raw id when
a lookup misses. The four broken call sites simply never adopted it, because
they fetch the queue directly and render it without a resolution step.

Beyond the UUID itself, the two dashboard widgets have a second, related gap.
Unlike `MaintenanceQueueView` (which lives on a vehicle-scoped page), the
dashboard widgets show **fleet-wide** queues. A row reading "Oil Change ·
Urgent" still does not answer *which vehicle is overdue* — the single most
important piece of information on a fleet dashboard. `attributes.vehicleId` is
present on every row and currently unused, and the widgets omit the due-date /
due-mileage line that the queue view already renders. This task closes both
gaps so that a dashboard row is self-describing.

## 2. Goals

Primary goals:

- Replace the raw `categoryId` UUID with the category's human-readable name at
  all four affected render sites.
- Identify the owning **vehicle** on each row of the two fleet-wide dashboard
  widgets.
- Show **due context** (due date and/or due mileage) on the dashboard widget
  rows, matching what `MaintenanceQueueView` already shows.
- Extract the vehicle-title fallback (`nickname || "year make model"`) into one
  shared helper, replacing the inline copy in `VehicleCard`.
- Resolve names entirely client-side, reusing the existing cached category and
  vehicle queries — no backend change.

Non-goals:

- Adding `categoryName` / `vehicleName` to the fleet-service JSON:API
  `Attributes` payload (`apps/fleet-service/internal/maintenanceschedule/rest.go`).
- Adding JSON:API `included` / compound-document support to
  `packages/shared-go/server`.
- New or modified HTTP endpoints of any kind.
- Redesigning widget layout, sizing, or the dashboard widget configuration flow.
- Revisiting the `status` vs `severity` vocabulary, or which of the two drives
  colour. Existing behaviour at each site is preserved as-is.
- Changing the 5-item cap on dashboard widget rows.

## 3. User Stories

- As a fleet owner, I want the Overdue Maintenance widget to name the service
  ("Oil Change") instead of a UUID, so that I can tell at a glance what is
  overdue.
- As a fleet owner with several vehicles, I want each dashboard maintenance row
  to name the vehicle it belongs to, so that I know which car to act on without
  clicking through.
- As a fleet owner, I want to see when an item was due (date or odometer
  reading), so that I can judge how urgent it actually is.
- As a fleet member viewing a vehicle's maintenance queue, I want the same
  readable category names there as everywhere else in the app.
- As a user whose fleet contains a schedule pointing at a category the client
  cannot resolve, I want a sane placeholder rather than a UUID or a blank cell.
- As a user on a slow connection, I do not want to briefly see UUIDs before they
  resolve into names.

## 4. Functional Requirements

### FR-LABEL — Category name resolution

- **FR-LABEL-1** — Every maintenance queue/schedule row that today renders
  `attributes.categoryId` MUST instead render the resolved category **name**.
  The four sites are:
  - `apps/web/src/components/features/dashboard/widgets/OverdueMaintenanceWidget.tsx:38`
  - `apps/web/src/components/features/dashboard/widgets/UpcomingMaintenanceWidget.tsx:38`
  - `apps/web/src/components/features/vehicles/maintenance/MaintenanceQueueView.tsx:51` (overdue)
  - `apps/web/src/components/features/vehicles/maintenance/MaintenanceQueueView.tsx:88` (upcoming)
- **FR-LABEL-2** — Resolution MUST be client-side, from the existing
  `useMaintenanceCategories()` query (`apps/web/src/lib/hooks/api/maintenance.ts`),
  which is already cached with a 10-minute `staleTime` and a 30-minute `gcTime`.
- **FR-LABEL-3** — `useMaintenanceCategories()` MUST be called with **no `kind`
  filter** at these sites, so that both `maintenance` and `modification`
  categories resolve. Filtering to one kind would reintroduce the bug for the
  other.
- **FR-LABEL-4** — When a `categoryId` has no match in the resolved category
  list, the row MUST display the literal string `Unknown category`. It MUST NOT
  display the raw UUID, an empty string, or `undefined`.
  - Note: this deliberately **differs** from the existing
    `UpcomingScheduleStrip` fallback, which shows the raw id. See FR-SHARED-3.
- **FR-LABEL-5** — A resolved category name MUST be rendered as plain text with
  no truncation-by-slicing; where horizontal space is constrained, CSS
  truncation (`truncate`) is used, as `UpcomingScheduleStrip` already does.

### FR-VEHICLE — Vehicle identity on dashboard widgets

Applies to `OverdueMaintenanceWidget` and `UpcomingMaintenanceWidget` only.
`MaintenanceQueueView` is vehicle-scoped in context and is explicitly excluded.

- **FR-VEHICLE-1** — Each row MUST display the owning vehicle's title, resolved
  from `attributes.vehicleId` against `useVehicles(fleetId)`
  (`apps/web/src/lib/hooks/api/vehicles.ts:25`).
- **FR-VEHICLE-2** — The vehicle title MUST use the shared helper defined in
  FR-SHARED-1: `nickname` when present and non-blank after trimming, otherwise
  `"<year> <make> <model>"`.
- **FR-VEHICLE-3** — When a `vehicleId` has no match in the vehicle list, the row
  MUST display `Unknown vehicle`, for symmetry with FR-LABEL-4.
- **FR-VEHICLE-4** — The category name remains the row's primary label; the
  vehicle title is secondary and MUST be visually subordinate (smaller and/or
  `text-muted-foreground`), so the row still reads "what is due" first.

### FR-DUE — Due context on dashboard widgets

Applies to `OverdueMaintenanceWidget` and `UpcomingMaintenanceWidget` only.

- **FR-DUE-1** — When `attributes.nextDueDate` is present, the row MUST render a
  date line, formatted with `new Date(...).toLocaleDateString()` to match the
  existing `MaintenanceQueueView` treatment.
- **FR-DUE-2** — The date line's wording MUST reflect state: `Was due <date>` in
  the overdue widget, `Due <date>` in the upcoming widget — matching
  `MaintenanceQueueView.tsx:56` and `:93` respectively.
- **FR-DUE-3** — When `attributes.nextDueMileage` is present and non-zero, the
  row MUST render `At <n> miles` with `toLocaleString()` thousands separators.
  A zero or absent value MUST render nothing (the backend emits `0` for
  pure-time schedules via `omitempty` on an `int`).
- **FR-DUE-4** — Both lines may appear together for `hybrid` schedules. When
  neither is present, the row renders category + vehicle + severity only, with
  no empty placeholder element.

### FR-SHARED — Shared helpers

- **FR-SHARED-1** — A vehicle-title helper MUST be extracted to a shared module
  under `apps/web/src/lib/` (sibling in spirit to
  `apps/web/src/lib/utils/displayName.ts`). It takes `VehicleAttributes` and
  returns the display title per FR-VEHICLE-2.
- **FR-SHARED-2** — `VehicleCard` (`apps/web/src/components/features/vehicles/VehicleCard.tsx:52-54`)
  MUST be refactored to call this helper. Its rendered output MUST be
  byte-identical to today's for every input, including the blank-nickname case
  — `nickname?.trim() || …` uses `||`, not `??`, and that behaviour is load-bearing.
- **FR-SHARED-3** — A category-name resolution helper (a `Map`-returning hook or
  a pure function over the category list) MUST be shared by all four FR-LABEL-1
  sites rather than duplicated four times.
- **FR-SHARED-4** — `UpcomingScheduleStrip` is **out of scope** for behaviour
  change. It already resolves names correctly via a caller-supplied map, and its
  raw-id fallback is left as-is. It MAY adopt the shared helper only if doing so
  requires no change to its rendered output; otherwise leave it untouched.

### FR-LOAD — Loading and empty states

- **FR-LOAD-1** — Each affected component MUST continue showing its existing
  skeleton until **all** queries it depends on have resolved — the queue query
  *and* the category query, plus the vehicle query in the two dashboard widgets.
  UUIDs MUST NOT be visible in an intermediate render.
- **FR-LOAD-2** — The existing empty-state copy MUST be preserved exactly:
  `No overdue maintenance.` / `No upcoming maintenance.` (widgets) and
  `No overdue maintenance items.` / `No upcoming maintenance items.`
  (`MaintenanceQueueView`).
- **FR-LOAD-3** — If a supporting query (categories or vehicles) fails while the
  queue query succeeds, the component MUST still render its rows, using the
  FR-LABEL-4 / FR-VEHICLE-3 fallbacks. A failed lookup MUST NOT blank the widget.
- **FR-LOAD-4** — The 5-item cap (`items.slice(0, 5)`) on both dashboard widgets
  is unchanged.

### FR-A11Y — Accessibility

- **FR-A11Y-1** — Colour MUST remain reinforcement, never the sole carrier of
  meaning. The existing "Overdue Maintenance" / "Upcoming Maintenance" headings
  continue to carry the state, consistent with the FR-A11Y-2 note already in
  `MaintenanceQueueView`.
- **FR-A11Y-2** — Added vehicle and due-context lines are ordinary text content
  and MUST be reachable by a screen reader in reading order (no `aria-hidden`,
  no purely-decorative markup).

## 5. API Surface

**No API changes.** All data required already ships in existing responses.

Endpoints read (all pre-existing, all unchanged):

| Method | Path | Purpose |
| --- | --- | --- |
| `GET` | `/api/fleet/fleets/{fleetId}/maintenance/overdue` | Overdue queue |
| `GET` | `/api/fleet/fleets/{fleetId}/maintenance/upcoming` | Upcoming queue |
| `GET` | `/api/fleet/maintenance-categories?page[size]=100` | Category names |
| `GET` | `/api/fleet/fleets/{fleetId}/vehicles` | Vehicle titles |

Rationale for client-side resolution over backend denormalization:

1. `packages/shared-go/server` has **no** `Included` field — there is no
   compound-document support to hang an `?include=category` on. Adding it is a
   framework-level change far exceeding this bug's scope.
2. The category list is small (~20 seeded rows, `page[size]=100` ceiling),
   effectively static, and already cached for 10 minutes.
3. The vehicle list for the fleet is already fetched and cached by the
   dashboard's neighbours.
4. `UpcomingScheduleStrip` establishes caller-side resolution as the house
   pattern for exactly this problem.

## 6. Data Model

No schema changes, no migrations, no new entities.

Existing shapes relied upon (read-only):

- `MaintenanceScheduleAttributes` — `vehicleId`, `categoryId`, `nextDueDate`,
  `nextDueMileage`, `severity`, `status`
  (`apps/web/src/types/models/maintenanceSchedule.ts`).
- `MaintenanceCategoryAttributes` — `name`, `kind`
  (`apps/web/src/types/models/maintenanceCategory.ts`).
- `VehicleAttributes` — `nickname`, `year`, `make`, `model`
  (`apps/web/src/types/models/vehicle.ts`).

Two backend serialization details the implementation must respect:

- `NextDueMileage int` carries `omitempty`, so pure-time schedules arrive as an
  absent field, and a legitimately-zero mileage is indistinguishable from unset.
  Treat falsy as "no mileage" (FR-DUE-3) — this matches what `MaintenanceQueueView`
  already does with its `? :` guard.
- Go marshals an unset string as `""`, not `null`. Presence checks must use
  truthiness (`||`), not nullish coalescing (`??`) — the same trap already
  documented in `apps/web/src/lib/utils/displayName.ts`.

## 7. Service Impact

**`apps/web`** — the only service touched.

| File | Change |
| --- | --- |
| `components/features/dashboard/widgets/OverdueMaintenanceWidget.tsx` | Category name, vehicle title, due context, combined loading gate |
| `components/features/dashboard/widgets/UpcomingMaintenanceWidget.tsx` | Same |
| `components/features/vehicles/maintenance/MaintenanceQueueView.tsx` | Category name at both render sites, combined loading gate |
| `components/features/vehicles/VehicleCard.tsx` | Use the extracted title helper (FR-SHARED-2) |
| `lib/` (new module) | Shared vehicle-title helper |
| `lib/` (new or extended module) | Shared category-name resolution helper |
| Corresponding `*.test.ts(x)` | New and updated coverage per §10 |

**`apps/fleet-service`** — no change.
**`packages/shared-go`**, **`packages/shared-ts`** — no change.
**`deploy/k8s`** — no change; no manifest render required for this task.

## 8. Non-Functional Requirements

- **Performance** — The change MUST NOT introduce a per-row fetch. Exactly one
  additional category query and (in the widgets) one additional vehicle query
  per component, both served from React Query cache on the dashboard where
  siblings have already warmed them. Lookup structures MUST be `Map`-based
  (O(1) per row), built once per render pass via `useMemo`, not rebuilt inside
  the row loop.
- **Correctness** — No `any`. Category and vehicle lookups are typed against the
  existing model interfaces.
- **Security** — No new data is exposed. All three queries are already
  fleet-scoped and authorized server-side; the widgets render only what the
  caller could already fetch. No change to authz.
- **Observability** — No new telemetry. Failures of the supporting queries are
  absorbed by the FR-LOAD-3 fallbacks rather than surfaced as errors.
- **Consistency** — After this task, every place in the app that displays a
  maintenance category displays a name. A grep for `attributes.categoryId`
  rendered directly into JSX MUST return zero hits.

## 9. Open Questions

1. Should the vehicle title on dashboard rows link to that vehicle's detail
   page? Naming the vehicle without a way to act on it is a half-step, but
   adding navigation widens the change beyond a labelling fix. **Assumption for
   v1: plain text, no link.** Revisit in design.
2. Exact row layout once a row carries four pieces of information (category,
   vehicle, severity chip, due context) inside a narrow 2-column widget. Left to
   the design phase; `MaintenanceQueueView`'s existing stacked treatment is the
   obvious starting point.
3. Where precisely the two shared helpers live — `lib/utils/` alongside
   `displayName.ts`, versus `lib/vehicleStats.ts` (vehicle title) and a
   maintenance-specific module (category map). Design phase decides; the
   requirement is only that they are shared, not duplicated.
4. Whether `Unknown category` / `Unknown vehicle` should be styled distinctly
   (e.g. italic, muted) to signal degraded data. **Assumption for v1: styled the
   same as a resolved value.**

## 10. Acceptance Criteria

Labelling:

- [ ] No rendered maintenance row anywhere in `apps/web` displays a raw
      `categoryId` UUID. Verified by grep: no JSX interpolation of
      `attributes.categoryId` as display text remains.
- [ ] `OverdueMaintenanceWidget` rows show the category name.
- [ ] `UpcomingMaintenanceWidget` rows show the category name.
- [ ] `MaintenanceQueueView` shows category names in both the overdue and the
      upcoming card.
- [ ] An unresolvable `categoryId` renders `Unknown category` at all four sites.
- [ ] Categories of kind `modification` resolve correctly, not just
      `maintenance` (FR-LABEL-3).

Vehicle identity and due context:

- [ ] Both dashboard widgets show the owning vehicle's title on each row.
- [ ] A vehicle with a non-blank `nickname` shows the nickname; one without
      shows `"<year> <make> <model>"`; an unresolvable id shows
      `Unknown vehicle`.
- [ ] The overdue widget renders `Was due <date>`; the upcoming widget renders
      `Due <date>`; both render `At <n> miles` with thousands separators when a
      non-zero `nextDueMileage` is present.
- [ ] A pure-time schedule renders no mileage line; a pure-mileage schedule
      renders no date line; neither leaves an empty element behind.

Loading and resilience:

- [ ] Each affected component shows its skeleton until every query it depends on
      has settled — no intermediate frame displays a UUID.
- [ ] With the category query failing and the queue query succeeding, rows still
      render, using `Unknown category`.
- [ ] With the vehicle query failing, widget rows still render, using
      `Unknown vehicle`.
- [ ] Empty-state copy is unchanged at all four sites.

Refactor hygiene:

- [ ] A single shared vehicle-title helper exists and is unit-tested, covering
      the blank-nickname (`"   "`) case.
- [ ] `VehicleCard`'s title output is unchanged for every input; its existing
      tests still pass without modification.
- [ ] Category-name resolution is implemented once and consumed by all four
      sites — no copy-pasted lookup logic.

Verification:

- [ ] New/updated unit tests cover: category resolution, the unknown-category
      fallback, vehicle-title fallback chain, unknown-vehicle fallback,
      due-context date and mileage rendering, and the combined loading gate.
- [ ] `make fe-test` passes.
- [ ] `make fe-build` passes.
- [ ] `make ci` passes.
- [ ] `superpowers:requesting-code-review` run (frontend-guidelines-reviewer at
      minimum) with findings recorded in `audit.md` before a PR is opened.
