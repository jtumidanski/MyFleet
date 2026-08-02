# Vehicle Card Status Banner — Product Requirements Document

Version: v1
Status: Draft
Created: 2026-08-02
Supersedes card layout from: [task-005](../task-005-vehicle-card-photo-actions/prd.md)
Visual reference: [`ux-prototype.html`](./ux-prototype.html)
---

> **On the prototype.** `ux-prototype.html` is the static mockup this PRD was written
> against — open it in a browser; it has a light/dark toggle. Its "Conditional" grids are
> the chosen design. Two caveats for anyone implementing from it: it hardcodes the design
> tokens rather than importing them, so it will not track future changes to `index.css`;
> and it renders cards as plain `<div>`s with no navigation, so it demonstrates nothing
> about §4.6's overlay-link mechanics. It is a picture of the target, not a reference
> implementation.

## 1. Overview

The vehicles list page (`/vehicles`) renders each vehicle as a horizontal card: an
80×80 thumbnail on the left, a text block to its right, and a right-aligned row of
two icon-only ghost buttons (`apps/web/src/components/features/vehicles/VehicleCard.tsx:39-95`).
That layout came from task-005, which added the photo and the Carfax deep link. It
works, but it under-uses the space it occupies. The photo is too small to identify a
vehicle at a glance; the action row spends ~52px of vertical chrome on two icons; and
the only data point is mileage. Status is a bare chip — a card reading "Overdue"
tells the user something is wrong but not what, so every investigation starts with a
click into the detail page.

This task rebuilds the card around a 16:9 hero photo and a **conditional status
banner**: a tinted band directly beneath the photo for vehicles needing attention
(Upcoming Maintenance, Overdue), and a visually quiet row in the same position for
vehicles that do not (Healthy, Inactive). The band carries the *reason* — "Service
overdue by 1,120 mi" — not a restatement of the status. Because color appears only on
cards that need action, the one overdue truck in a garage of six healthy vehicles
becomes impossible to miss, which a per-card chip cannot achieve. Below the banner sit
the title, subtitle, and a two-up stat strip (Odometer, Last activity).

The banner's copy requires data the backend already computes and then discards.
`ScheduleStatesByVehicle` loads full `QueueRow`s and computes each schedule's
`DueState` (`apps/fleet-service/internal/maintenanceschedule/processor.go:167-178`),
then narrows its return to `[]string` — throwing away the `NextDueDate`/`NextDueMileage`
that would let the UI say how overdue a vehicle is. Likewise `LastActivityByVehicle` is
called on every vehicle read to derive status (`apps/fleet-service/internal/vehicle/status.go:45`)
and the timestamp is dropped once the status string is computed. Both gatherers already
run once per vehicle inside the list handler's loop (`apps/fleet-service/internal/vehicle/resource.go:52`).
**This task therefore adds no database queries.** It widens what the existing calls
return and surfaces two new read-only attributes on the vehicle resource.

## 2. Goals

Primary goals:

- Make a vehicle needing attention identifiable from the grid without opening it, by
  stating the reason on the card.
- Spend color only where action is required, so color in the grid always means
  "look here".
- Make the primary photo large enough to identify a vehicle at a glance.
- Surface the vehicle's maintenance urgency and recency using data the server already
  computes, without adding queries to the list endpoint.
- Restore whole-card navigation without reintroducing nested interactive elements.

Non-goals:

- No maintenance **category name** on the card ("Brake fluid", "Tire rotation").
  Resolving `Schedule.CategoryID` to a display name is a per-vehicle lookup into the
  separate `maintenancecategory` domain on a list endpoint — an N+1 that needs its own
  batching design. The distance already conveys urgency; the name only says which
  wrench. Deferred to a follow-up task.
- No change to photo upload, deletion, or primary-image selection.
- No change to the vehicle detail page, its gallery, or its sections.
- No change to `media-service`. Task-005's `?variant=thumbnail` endpoint already serves
  what this card needs.
- No new persisted columns and no migrations.
- No changes to status derivation *rules* (`status.Derive`), thresholds
  (`DefaultThresholds`: 30 days / 500 mi), or the recurrence math. This task reports
  those results; it does not redefine them.
- No sorting or filtering of the vehicles grid by urgency. Worth doing, but it is a
  separate change to `VehiclesPage` and its API call.
- No dashboard widget changes, even though `dashboard/aggregate.go:214` consumes the
  same last-activity gatherer.

## 3. User Stories

- As a fleet owner, I want to see which vehicle needs service and by how much, directly
  on the vehicles list, so that I do not have to open each vehicle to find out.
- As a fleet owner whose vehicles are all fine, I want the list to look calm, so that
  when something *does* need attention it stands out rather than blending into a wall of
  status chips.
- As a fleet member, I want to recognise a vehicle by its photo at a glance, so that I
  pick the right one without reading the trim line.
- As a fleet member, I want to click anywhere on a card to open it, so that navigation
  does not depend on hitting a small icon.
- As a fleet member, I want the Carfax button to still work as its own link, so that
  clicking it does not navigate me into the vehicle detail page instead.
- As a fleet member with a vehicle I have not touched in a year, I want the card to tell
  me it is dormant rather than implying it is healthy.
- As a viewer-role user, I want the same information and navigation as members, since
  none of it reveals anything I am not permitted to see.

## 4. Functional Requirements

### 4.1 Card structure

- **FR-1.1** The card is a vertical stack in this fixed order: hero photo, status
  banner, text block (title + subtitle), stat strip, action footer.
- **FR-1.2** Every region is present on every card. No region is conditionally omitted
  in a way that changes the card's height, so cards in a grid row align regardless of
  status, photo presence, or missing data.
- **FR-1.3** The grid remains responsive at the existing breakpoints
  (`grid-cols-1 sm:grid-cols-2 lg:grid-cols-3`) with no horizontal overflow at the
  single-column breakpoint.
- **FR-1.4** Title and subtitle truncate rather than wrap, and must not widen the card.

### 4.2 Hero photo

- **FR-2.1** The photo region is a 16:9 box spanning the card's full width, rendering the
  vehicle's primary photo with `object-cover`.
- **FR-2.2** The image is requested as the `thumbnail` variant through the authenticated
  API, reusing the existing `useMediaContentUrl` object-URL mechanism. A bare `<img src>`
  cannot be used — the API requires an `Authorization` header the browser will not send
  for image subresources.
- **FR-2.3** While loading, the region shows a skeleton of identical dimensions; the card
  does not reflow when the image arrives.
- **FR-2.4** When `primaryImageMediaId` is absent or the fetch fails, the region renders
  the neutral car-icon placeholder at the same dimensions. The two cases remain
  distinguishable to assistive technology by accessible label ("No photo" vs "Photo
  unavailable"), as established in task-005 FR-3.3.
- **FR-2.5** A failed image load produces no toast and no error tile, per task-005
  FR-2.4. N broken thumbnails must produce N placeholders and zero notifications.
- **FR-2.6** The 320px-max-edge `thumbnail` variant is used even though the hero box is
  wider than 320px at some breakpoints. Requesting `display` (1280px) for a list of N
  cards is the exact cost task-005 §4.7 existed to avoid. If the thumbnail proves
  visibly soft at the widest breakpoint, that is a variant-sizing decision for a
  separate task, not a reason to ship `display` here. See §9.

### 4.3 Status banner — conditional treatment

- **FR-3.1** The banner occupies the same position and the same height on every card,
  regardless of status. Only its color treatment and copy change.
- **FR-3.2** For status **Overdue** and **Upcoming Maintenance**, the banner renders as a
  tinted band using the semantic subtle token families — `danger-subtle` and
  `warning-subtle` respectively — with a matching bottom border.
- **FR-3.3** For status **Healthy** and **Inactive**, the banner renders on the card
  background in muted foreground with the standard border. No status tint.
- **FR-3.4** Every banner carries a leading icon conveying its state, plus text. Color is
  never the only signal, consistent with the `StatusBadge` contract
  (`packages/ui-components/src/StatusBadge.tsx:5`).
- **FR-3.5** The banner **replaces** the status badge on the card. `StatusBadge` is not
  rendered by `VehicleCard`. The component itself is unchanged and remains in use by its
  other consumers.
- **FR-3.6** When the vehicle's `status` attribute is absent — `DeriveStatus` returns `""`
  on any gather error (`apps/fleet-service/internal/vehicle/status.go:42,47`) — the
  banner renders in the quiet treatment with the text "Status unavailable". The card must
  not omit the banner or change height.
- **FR-3.7** When `status` holds an unrecognised value, it is treated exactly as FR-3.6.
  The card must not crash or render a raw unknown string in a tinted band.

### 4.4 Banner copy

Copy is derived from the new attributes in §4.8. All rules are testable against a pure
function; §8 requires it be unit-tested independently of rendering.

- **FR-4.1** Status **Overdue** → "Service overdue by {amount}".
- **FR-4.2** Status **Upcoming Maintenance** → "Service due in {amount}".
- **FR-4.3** Status **Healthy** → "Up to date".
- **FR-4.4** Status **Inactive** → "No activity in {duration}".
- **FR-4.5** `{amount}` is **axis-aware**. `RecurrenceType` is `time | mileage | hybrid`
  (`apps/fleet-service/internal/maintenanceschedule/recurrence.go:35-36`), and `NextDue`
  leaves the unused axis at its zero value (`recurrence.go:23-30`). A mileage distance
  must never be rendered for a time-only schedule, nor a day count for a mileage-only
  schedule.
  - mileage axis → `"{n} mi"`, thousands-separated (e.g. `"1,120 mi"`)
  - time axis → `"{n} days"`, or `"{n} months"` when n ≥ 60 days
- **FR-4.6** When a vehicle has several non-ok schedules, the banner describes exactly
  one: the schedule that determined the vehicle's status. Among candidates of that
  state, select the largest breach **normalized against its own due-soon threshold**
  (miles ÷ 500, days ÷ 30, from `DefaultThresholds`, `recurrence.go:20`). This makes a
  600-mile overrun rank above a 5-day overrun rather than comparing raw numbers across
  incomparable units. Ties resolve to the mileage axis for determinism.
- **FR-4.7** A **hybrid** schedule can breach on either axis or both. The axis reported is
  the one selected by FR-4.6's normalized comparison, evaluated per-axis.
- **FR-4.8** When status is Overdue or Upcoming Maintenance but no due detail is available
  (attribute absent — see FR-8.4), the banner keeps its tinted treatment and falls back to
  "Maintenance overdue" / "Maintenance due soon". Urgency must survive missing detail.
- **FR-4.9** `{duration}` for Inactive is derived from `lastActivityAt` and expressed in
  whole months (the Inactive threshold is 365 days, `apps/fleet-service/internal/vehicle/status.go:11`,
  so a day count would be needlessly precise).
- **FR-4.10** Banner text truncates on overflow rather than wrapping to a second line,
  preserving FR-3.1's fixed height.

### 4.5 Stat strip

- **FR-5.1** A two-column strip below the text block, separated by a top border.
- **FR-5.2** Left slot: **Odometer**, from `currentMileage`, thousands-separated with a
  `mi` suffix, using the existing `formatMileage` helper from `@myfleet/ui-components`.
- **FR-5.3** Right slot: **Last activity**, from the new `lastActivityAt` attribute,
  rendered as relative time ("6 days ago", "3 weeks ago", "yesterday").
- **FR-5.4** Numeric values use tabular figures so columns align across cards.
- **FR-5.5** When `currentMileage` is absent, the Odometer slot renders an em-dash. The
  slot is never omitted (FR-1.2).
- **FR-5.6** When `lastActivityAt` is absent or zero, the Last activity slot renders an
  em-dash.
- **FR-5.7** Next-service distance is deliberately **not** shown in the strip. The banner
  already states it for the vehicles where it matters; repeating it would spend the slot
  on a value that reads "—" or is redundant on every healthy card.

### 4.6 Navigation and the Carfax action

Task-005 removed whole-card navigation (its FR-4.1, NFR-10) because the Carfax link
nested inside a card-wide link made click behaviour ambiguous. This task restores
whole-card navigation using a technique that does not nest the two.

- **FR-6.1** The card root is a positioned container. It is **not** an anchor element.
- **FR-6.2** The title is wrapped in a react-router `<Link>` to `/vehicles/{id}` whose
  `::after` pseudo-element is absolutely positioned to span the entire card. The clickable
  area is the whole card; the anchor is never a DOM ancestor of any other interactive
  element.
- **FR-6.3** Because the link's text content is the vehicle title, its accessible name is
  the vehicle's identity. This satisfies task-005 FR-4.4 (no grid of identically-named
  links) without a redundant `aria-label`.
- **FR-6.4** Being a real anchor, it preserves middle-click, cmd/ctrl-click, and the
  browser link context menu.
- **FR-6.5** The separate chevron/detail icon button is removed. It is redundant once the
  card is the link.
- **FR-6.6** The Carfax anchor is positioned with a stacking context above the overlay so
  it receives its own clicks. Clicking Carfax must **not** navigate to the detail page.
- **FR-6.7** All Carfax behaviour from task-005 is preserved unchanged: rendered only when
  a VIN is present and the configured template contains `{vin}` (FR-5.2, FR-5.6);
  `target="_blank"` with `rel="noopener noreferrer"` (FR-5.4); vehicle-identifying
  `aria-label` (FR-5.5); VIN URL-encoded; nothing contacts Carfax until clicked (FR-5.7).
  The template continues to be read through `useRuntimeConfig`, not build-time env.
- **FR-6.8** Tab order is DOM order: card link, then Carfax. Both show the existing focus
  ring. The card link's focus ring must be visible on the card, not clipped by
  `overflow-hidden` on the card root.
- **FR-6.9** The overlay suppresses text selection across the card body. This is an
  accepted trade-off of the pattern and is called out in §9.
- **FR-6.10** All navigation behaves identically for `viewer`-role users.

### 4.7 Loading and empty states

- **FR-7.1** The loading skeleton mirrors the card's *structure* — an aspect-ratio photo
  box plus rows matching banner, text, and strip — rather than a single fixed-height
  block. The current implementation hardcodes `h-40` with a comment deriving 160px
  arithmetically and conceding it is "computed, not measured"
  (`apps/web/src/components/features/vehicles/VehicleList.tsx:14-20`). A structural
  skeleton cannot drift from the card the way a magic number does.
- **FR-7.2** The existing empty state ("No vehicles yet…") is unchanged.

### 4.8 Backend — vehicle resource attributes

- **FR-8.1** The vehicle JSON:API resource gains `lastActivityAt`, an RFC 3339 timestamp,
  read-only, omitted when zero.
- **FR-8.2** The resource gains a read-only `nextDue` object, omitted entirely when the
  vehicle has no non-ok schedule. Shape:
  - `state` — `"upcoming" | "overdue"`
  - `axis` — `"time" | "mileage"`
  - `miles` — integer, present only when `axis` is `mileage`; the absolute breach or
    remaining distance
  - `days` — integer, present only when `axis` is `time`
- **FR-8.3** Both attributes are derived on read and never stored, never accepted on
  create or update, and never written by the client — the same contract `status` already
  has (`apps/fleet-service/internal/vehicle/rest.go:6-8`).
- **FR-8.4** Both are omitted rather than zero-valued when their source gather fails, so
  the client's fallback paths (FR-3.6, FR-4.8, FR-5.6) have something to key on.
- **FR-8.5** Attribute selection is computed server-side by the same normalized-threshold
  rule the UI copy depends on (FR-4.6). The server picks the single governing schedule;
  the client formats it. Ranking logic must not be duplicated in the frontend.

### 4.9 Backend — widening the gatherers

- **FR-9.1** `ScheduleStatesByVehicle` is widened to return per-schedule due detail
  (state, recurrence type, next-due date, next-due mileage, current mileage) instead of
  `[]string`. The existing `DueEntry` shape (`processor.go:147`) is the natural model.
- **FR-9.2** The widening must not add a query. `ListActiveByVehicle` already returns full
  `QueueRow`s and `DueState` is already computed per row (`processor.go:168-177`); the
  data is discarded at the return boundary today.
- **FR-9.3** `status.Derive`'s inputs and behaviour are unchanged. It continues to consume
  state strings; the widened type is projected down to those states at the call site so
  the derivation rule is untouched.
- **FR-9.4** `StatusDeps.DeriveStatus` is extended (or joined by a sibling) to return the
  last-activity timestamp and governing due detail alongside the status string, so the
  list handler's single per-vehicle call yields everything the resource needs.
- **FR-9.5** The `vehicle` package must not import `maintenanceschedule` internals. The
  widened gatherer interface stays defined in `vehicle` and satisfied by an adapter, as
  the current `ScheduleStateGatherer` is (`apps/fleet-service/internal/vehicle/status.go:17-19`).
  This preserves the boundary CLAUDE.md requires.
- **FR-9.6** The `dashboard` package's existing consumption of `LastActivityByVehicle`
  (`aggregate.go:214`) is not modified.

## 5. API Surface

No new endpoints. No changes to request bodies. Two read-only attributes are added to the
existing vehicle resource, returned by both the list and single-vehicle reads.

```
GET /api/fleets/{fleetId}/vehicles
GET /api/vehicles/{id}
```

Response `attributes`, additions only:

```jsonc
{
  "type": "vehicles",
  "id": "…",
  "attributes": {
    // …all existing attributes unchanged…
    "status": "Overdue",
    "lastActivityAt": "2026-04-02T14:31:07Z",   // new, omitempty
    "nextDue": {                                 // new, omitempty
      "state": "overdue",
      "axis": "mileage",
      "miles": 1120
    }
  }
}
```

Further examples:

```jsonc
"nextDue": { "state": "upcoming", "axis": "mileage", "miles": 310 }
"nextDue": { "state": "overdue",  "axis": "time",    "days": 12  }
```

Backwards compatibility: additive only. Existing clients ignoring unknown attributes are
unaffected. No endpoint changes anything about status derivation, so a client reading only
`status` sees identical values.

Frontend types in `apps/web/src/types/models/vehicle.ts` mirror the additions on
`VehicleAttributes` as optional fields. `CreateVehicleAttributes` and
`UpdateVehicleAttributes` are **not** extended — both attributes are read-only (FR-8.3).

## 6. Data Model

**No schema changes. No migrations.** Every value is derived on read from data that
already exists:

| Value | Source | Status |
| --- | --- | --- |
| `currentMileage` | `vehicles.current_mileage` | Exists, already exposed |
| `primaryImageMediaId` | `vehicles.primary_image_media_id` | Exists, already exposed |
| `status` | Derived via `status.Derive` | Exists, already exposed |
| `lastActivityAt` | `activity` provider, already called per vehicle read | Exists, **discarded today** |
| `nextDue` | `maintenance_schedules` via `ListActiveByVehicle` + `DueState` | Exists, **discarded today** |

The `nextDue` values come from `Model.NextDueDate()` / `Model.NextDueMileage()`
(`apps/fleet-service/internal/maintenanceschedule/model.go:32-33`) compared against
`QueueRow.CurrentMileage` and the request's `now`.

## 7. Service Impact

### `apps/fleet-service` — moderate

- `internal/maintenanceschedule/processor.go` — widen `ScheduleStatesByVehicle`'s return
  from `[]string` to a due-detail slice. No new query (FR-9.2).
- `internal/vehicle/status.go` — widen the `ScheduleStateGatherer` port; extend
  `DeriveStatus` (or add a sibling) to yield status + last activity + governing due detail
  in one pass (FR-9.4); implement the normalized-threshold selection (FR-8.5).
- `internal/vehicle/rest.go` — add `LastActivityAt` and `NextDue` to `Attributes`, both
  `omitempty`; extend `TransformWithStatus` (or add a richer transform) to carry them.
  `Transform` and `TransformSlice` — the status-free paths — keep their current shape.
- `internal/vehicle/resource.go` — pass the additional derived values through at the two
  call sites (`:52` list, `:120` single).
- `cmd/main.go` — adapter satisfying the widened gatherer interface.

### `apps/web` — the bulk of the work

- `components/features/vehicles/VehicleCard.tsx` — rebuilt: hero photo, banner, text,
  strip, footer; overlay-link navigation; chevron button removed.
- `components/features/vehicles/VehicleList.tsx` — structural skeleton (FR-7.1).
- `components/features/vehicles/VehiclePhotoThumbnail.tsx` — the `BOX` constant is a fixed
  `h-20 w-20` square (`:11`). It needs to accept the hero's aspect-ratio sizing while
  keeping all four states (image, skeleton, no-photo, failed) on identical dimensions.
  The detail-page consumers of this component, if any, must not regress.
- New pure module for banner copy: status + `nextDue` + `lastActivityAt` → `{ tone, icon,
  text }`. Unit-tested without rendering (FR-4.x, NFR-13).
- `types/models/vehicle.ts` — optional `lastActivityAt` and `nextDue` on
  `VehicleAttributes`.
- Relative-time formatting helper — check whether one already exists in
  `packages/ui-components/src/formatters.ts` before adding another.
- `VehicleCard.test.tsx` — substantially extended.

### `packages/ui-components` — minimal or none

`StatusBadge` is untouched (FR-3.5). If a relative-time formatter is added, it belongs
here beside `formatMileage`.

### `apps/media-service`, `apps/auth-service`, `apps/notification-service` — none

### `deploy/k8s` — none

## 8. Non-Functional Requirements

### Performance

- **NFR-1** The vehicles list issues **no additional database queries**. Both gatherers
  already run per vehicle inside the list loop (`resource.go:52`); this task changes what
  they return, not how often they are called. A design that adds a query per vehicle —
  for example resolving category names — fails this requirement and is out of scope
  (§2 non-goals).
- **NFR-2** No additional HTTP requests per card. The list response carries everything the
  card renders except the photo bytes; the card must not issue a per-vehicle metadata or
  schedule request.
- **NFR-3** Photo bytes are fetched via the `thumbnail` variant, once per media id per
  React Query cache window, regardless of how many components render it.
- **NFR-4** Banner, title, strip, and both interactive elements render and are usable
  while the photo is still loading.
- **NFR-5** The existing per-vehicle gather pattern in the list handler is a pre-existing
  2N-query shape. This task does not worsen it and does not fix it; noting it so a
  reviewer does not attribute it to this change. Batching it is a separate task.

### Security & privacy

- **NFR-6** `lastActivityAt` and `nextDue` are fleet-scoped by the same authorization as
  the vehicle resource that carries them. No new authorization surface.
- **NFR-7** Both attributes are read-only and must be rejected — or silently ignored,
  matching `status`'s existing handling — if supplied on create or update. A client must
  not be able to write a derived value.
- **NFR-8** All task-005 Carfax protections carry over unchanged (FR-6.7): VIN
  URL-encoded, `rel="noopener noreferrer"`, no contact with Carfax before an explicit
  click.

### Accessibility

- **NFR-9** The card link's accessible name identifies the vehicle (FR-6.3).
- **NFR-10** Banner state is conveyed by icon and text, never by color alone (FR-3.4).
- **NFR-11** Both tinted banner treatments must meet WCAG AA contrast in light and dark
  themes. The `-subtle` / `-subtle-foreground` pairs are documented as measured in
  `docs/tasks/task-003-dark-mode-branding/contrast.md`; the banner uses those pairs
  as-is rather than introducing new color combinations.
- **NFR-12** Restoring whole-card navigation increases the primary target size versus
  task-005's 40×40 icon button. The Carfax button retains the `size="icon"` 40×40 target.
- **NFR-13** Keyboard traversal reaches the card link and the Carfax link in DOM order,
  with a visible focus ring on each (FR-6.8).

### Testing

- **NFR-14** The banner-copy function is pure and directly unit-tested: each of the four
  statuses; mileage vs time axis; hybrid breaching on each axis and both; the normalized
  multi-schedule selection (FR-4.6) including the mileage tie-break; the days→months
  threshold; missing-detail fallback (FR-4.8); absent and unrecognised status (FR-3.6,
  FR-3.7).
- **NFR-15** Component tests cover: photo present/absent/failed; VIN present/absent; each
  status treatment tinted vs quiet; missing `currentMileage` and missing `lastActivityAt`;
  and that clicking Carfax does not trigger detail navigation (FR-6.6) — the specific
  regression this layout risks.
- **NFR-16** Backend tests cover the widened gatherer returning due detail without extra
  queries, the normalized selection rule, `omitempty` behaviour on both attributes when
  gathers fail, and that `status` values are byte-for-byte unchanged from today for a
  representative set of fixtures.
- **NFR-17** `make ci` passes.

## 9. Open Questions

1. **Thumbnail sharpness at the hero size.** FR-2.6 keeps the 320px `thumbnail` variant.
   At `lg:grid-cols-3` on a wide viewport a card is roughly 360px wide, so the hero box
   slightly exceeds the variant's max edge and may look soft on high-DPI displays. Options
   are to accept it, to add a mid-size variant, or to use `display` at the widest
   breakpoint only. Best judged against the running UI; deliberately not decided here.
2. **Task-006 interaction on `lastActivityAt`.** `DeriveStatus` falls back to the
   vehicle's `CreatedAt` when no activity is recorded (`vehicle/status.go:49-53`), and
   task-006 is in flight to fix `created_at` being wiped to zero — which its PRD names as
   causing exactly this vehicle status to misreport. Until task-006 lands, a vehicle with
   a wiped `created_at` will surface a zero or absurd `lastActivityAt`. The two tasks touch
   the same files and should be sequenced, not merged in parallel.
3. **Inactive vehicles show no due detail.** A vehicle can be Inactive *and* have overdue
   schedules; `status.Derive` returns Inactive only when nothing is overdue or upcoming
   (`status/derive.go:14-27`), so this cannot currently happen. The design should confirm
   that invariant rather than assume it.
4. **Grid density at three columns.** The hero card is materially taller than today's.
   Whether `lg:grid-cols-3` still reads well, or wants two wider columns, is a visual
   judgement — the same question task-005 left open (its §9.4) and equally unresolved by
   argument alone.
5. **Text selection on the card.** FR-6.9 accepts that the overlay blocks selecting the
   title or mileage text. If copying a value off the card matters, the overlay pattern is
   the wrong choice and explicit buttons should return.
6. **`nextDue` as a nested object.** The shape in §5 nests. If the project's JSON:API
   conventions favour flat scalar attributes, `nextDueState` / `nextDueAxis` /
   `nextDueMiles` / `nextDueDays` is an equivalent encoding. No existing nested attribute
   object appears on this resource, so the design phase should confirm against convention.

## 10. Acceptance Criteria

Backend:

- [ ] `GET /api/fleets/{id}/vehicles` returns `lastActivityAt` for a vehicle with recorded
      activity, and omits it when the gather fails.
- [ ] The same response returns `nextDue` with `axis: "mileage"` and a `miles` value for a
      vehicle whose governing schedule is mileage-based.
- [ ] `nextDue` returns `axis: "time"` with a `days` value for a time-based schedule, and
      no `miles` key.
- [ ] `nextDue` is omitted entirely for a vehicle with no non-ok schedules.
- [ ] A vehicle with several overdue schedules reports the one with the largest
      threshold-normalized breach, with the mileage tie-break applied.
- [ ] A hybrid schedule breaching on both axes reports the axis selected by the normalized
      comparison.
- [ ] `status` values are unchanged from current behaviour across the existing status
      fixtures.
- [ ] Neither new attribute can be set via POST or PATCH.
- [ ] The list endpoint issues the same number of queries per vehicle as before the change
      (verified by test or query-count instrumentation, not by inspection).
- [ ] The `vehicle` package does not import `maintenanceschedule` internals.
- [ ] `make vet` and `make test` pass.

Frontend — layout:

- [ ] Each card renders photo, banner, text, strip, and footer in that order.
- [ ] Cards in a grid row have equal height regardless of status, photo, or missing data.
- [ ] A vehicle with a photo shows it in a 16:9 hero fetched as `?variant=thumbnail`
      (verifiable in the network panel).
- [ ] A vehicle without a photo shows the car-icon placeholder at identical dimensions.
- [ ] A vehicle whose photo fails shows the placeholder, no error tile, no toast.
- [ ] The skeleton mirrors the card's structure and does not visibly jump on load.
- [ ] No horizontal overflow at the single-column breakpoint.

Frontend — banner:

- [ ] An Overdue vehicle shows a `danger`-tinted banner reading "Service overdue by
      1,120 mi".
- [ ] An Upcoming vehicle shows a `warning`-tinted banner reading "Service due in 310 mi".
- [ ] A Healthy vehicle shows an untinted banner reading "Up to date".
- [ ] An Inactive vehicle shows an untinted banner reading "No activity in 11 months".
- [ ] A time-based schedule renders a day or month count, never a mileage figure.
- [ ] A vehicle with `status` absent shows the quiet banner reading "Status unavailable"
      at the same height.
- [ ] An Overdue vehicle with `nextDue` absent still shows a tinted banner, reading
      "Maintenance overdue".
- [ ] `StatusBadge` is not rendered anywhere on the card.
- [ ] Both tinted treatments pass AA contrast in light and dark themes.
- [ ] Long banner text truncates without increasing card height.

Frontend — strip and actions:

- [ ] The strip shows Odometer and Last activity, with tabular figures aligned across
      cards.
- [ ] A vehicle without `currentMileage` shows an em-dash and the slot keeps its position.
- [ ] A vehicle without `lastActivityAt` shows an em-dash.
- [ ] Clicking anywhere on the card body navigates to `/vehicles/{id}`.
- [ ] Middle-click and cmd/ctrl-click on the card open the detail page in a new tab.
- [ ] **Clicking the Carfax button opens Carfax and does not navigate to the detail page.**
- [ ] The Carfax link has `target="_blank"` and `rel="noopener noreferrer"`.
- [ ] A vehicle without a VIN shows no Carfax button and the footer keeps its height.
- [ ] No request reaches Carfax before an explicit click.
- [ ] Keyboard traversal reaches the card link then the Carfax link, each with a visible
      focus ring.
- [ ] All of the above behave identically for a `viewer`-role user.

Frontend — tests:

- [ ] The banner-copy function has direct unit tests covering every case in NFR-14.
- [ ] Component tests cover every case in NFR-15.
- [ ] `make fe-test` and `make fe-build` pass.

Whole branch:

- [ ] `make ci` passes.
