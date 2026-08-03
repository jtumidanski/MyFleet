# Task 012 — Vehicle detail page redesign

Date: 2026-08-02
Status: design approved, not yet planned

## 1. Problem

`apps/web/src/pages/VehicleDetailPage.tsx` renders seven stacked cards inside a
`max-w-2xl` (672px) container. On a desktop monitor the page uses a fraction of
the available width and answers every question by scrolling.

Three specific faults:

1. **Wasted horizontal space.** The width cap is unconditional, so a 2560px
   display shows a 672px column.
2. **Inline forms shift the layout.** Six forms (`VehicleForm`, `MileageForm`,
   `FuelForm`, `MaintenanceRecordForm`, `MaintenanceScheduleForm`, media upload)
   expand in place, pushing everything below them down the page.
3. **No summary.** Answering "when is this due?" or "what has this cost me?"
   requires reading the whole page. Nothing is derived or surfaced.

A fourth fault surfaced during design and is folded in here: the maintenance and
modification category picker is limited to 20 seeded, global, read-only rows
(`apps/fleet-service/internal/maintenancecategory/entity.go:35`). Eight
maintenance categories and twelve modification ones do not cover real usage, and
there is no way to add more.

## 2. Goals

- Use horizontal space on desktop; degrade cleanly to a single column.
- Move every form into a dialog so page layout never shifts.
- Surface derived state (next service, overdue count, cost, economy).
- Let users enter free-form maintenance and modification categories that behave
  like real categories — they group, filter, and are reusable.

## 3. Non-goals

- The vehicles list page (`VehiclesPage.tsx`) is untouched.
- Renaming, deleting, or merging custom categories. A typo means an unused row.
- Cross-fleet category sharing. Custom categories never leave their fleet.
- Editing or deleting mileage records — no such endpoint exists.
- A server-side unified feed endpoint. The merge is client-side (§6).

## 4. Layout

`VehicleDetailPage` drops `max-w-2xl` for `mx-auto w-full max-w-[1600px]` and
becomes a two-column grid:

```
lg:grid-cols-[320px_minmax(0,1fr)]  gap-6
```

**Left rail (320px)** is `lg:sticky lg:top-16 self-start` and carries identity:
primary photo, title, status badge, key facts (odometer, VIN, notes), a
three-thumbnail photo strip, an Edit button, and a vertical quick-actions list.

**Right column** carries the operational content: stat strip, "Up next" schedule
tiles, the unified Records table, and a trends/activity block.

**Below `lg`** the grid collapses to one column, the rail un-sticks and stacks
first, and the vertical quick-actions list becomes a horizontal wrapping button
row. `minmax(0,1fr)` on the content column is required — without it the Records
table's intrinsic width prevents the grid from shrinking.

## 5. Component decomposition

`VehicleDetailPage.tsx` reduces to layout plus dialog state. New files under
`apps/web/src/components/features/vehicles/detail/`:

| Component | Responsibility |
|---|---|
| `VehicleIdentityRail` | Photo, title, status, facts, thumb strip, Edit |
| `VehicleQuickActions` | Action list; opens dialogs |
| `VehicleStatStrip` | Derived metrics (§7) |
| `UpcomingScheduleStrip` | Schedule tiles with Complete |
| `VehicleRecordsTable` | Unified feed, filter chips, Load more |
| `VehicleRecordDrawer` | Row detail sheet (§8) |
| `VehicleTrends` | Mileage sparkline + recent activity |

The four existing section components — `VehicleMileageSection`,
`VehicleMaintenanceSection`, `VehicleFuelSection`, `VehicleMediaGallery` — are
dissolved; their data hooks move into the components above.
`MileageSparkline`, `SeverityChip`, `RecordAttachmentList`, and
`AttachmentPicker` are reused as-is.

**The five form components keep their prop interfaces.** Each already accepts
`onSubmit` / `onCancel` / `submitting`, so each is wrapped in a dialog shell
rather than re-plumbed. This is what keeps the refactor tractable: the forms
carry the validation and are the riskiest code to touch.

The one exception is internal, not interfacial: `MaintenanceRecordForm` and
`MaintenanceScheduleForm` swap their category `<Select>` for `CategoryCombobox`
(§10.2). Their props, Zod schemas, and submit payloads are unchanged.

## 6. Unified records feed

A new hook `useVehicleRecords(vehicleId)` composes `useMaintenanceRecords`,
`useFuelLogs`, and `useMileageRecords` into one list of a discriminated union:

```ts
type VehicleRecordRow = {
  id: string;                 // `${kind}:${sourceId}` — unique across sources
  sourceId: string;
  kind: 'maintenance' | 'modification' | 'fuel' | 'mileage';
  date: string;               // ISO; the sort key
  title: string;
  mileage?: number;
  cost?: number;
};
```

Rows sort date-descending. Filter chips (All / Maintenance / Mods / Fuel /
Mileage) filter the merged list client-side.

### 6.1 Pagination

`server.ParsePage` defaults page size to 25 and caps it at 100
(`packages/shared-go/server/pagination.go:25,29`). **No current hook passes
`page[size]`.** The vehicle page today therefore shows only the newest 25
mileage records, 25 maintenance records, and 25 fuel logs while presenting each
as a complete list. This is a pre-existing correctness bug, and the merged feed
cannot be built on top of it. The hooks gain explicit `page[size]=100` and expose
`meta.total`.

Because each source paginates independently, a naive concatenation of loaded
pages produces a merged list that is wrong past the point where any single
source runs out of loaded rows. The merge therefore applies a **watermark**:

> `safeUntil` = the newest value among `oldestLoadedDate(source)` across all
> sources that still have unloaded pages.
>
> Only rows with `date >= safeUntil` are rendered. Rows older than the watermark
> may interleave with rows not yet fetched, so they are withheld.

"Load more" fetches the next page for every source whose oldest loaded row is at
or above the watermark, which advances `safeUntil` and releases more rows. When
no source has unloaded pages, `safeUntil` is `-Infinity` and everything renders.

The footer states counts honestly: `Showing 25 of 138`. The merge, sort, filter,
and watermark logic live in one pure function so they are unit-testable without
React or the network.

## 7. Stat strip

Four tiles, all derived client-side from data the page already fetches. No new
endpoints.

| Tile | Derivation | Empty case |
|---|---|---|
| Odometer | Latest mileage record, else `vehicle.currentMileage` | `—` |
| Next service | Min remaining across schedules (miles or date) | `—` |
| Cost · 12 mo | Sum of maintenance `cost` + fuel `totalCost` in trailing 12 months | `$0` |
| Avg economy | Odometer deltas between consecutive fuel logs; needs ≥ 2 logs | `—` |

Every tile renders `—` rather than a misleading number when its inputs are
missing. Cost and economy are computed over *loaded* rows; when the watermark
indicates unloaded history, the tile carries a "based on recent records"
subtitle rather than implying completeness.

## 8. Record drawer

Row click opens a right-side sheet. Available actions are constrained by the
mutations that actually exist in `apps/web/src/lib/hooks/api/`:

| Row kind | Drawer offers |
|---|---|
| Maintenance / Mod | Attachments, Edit, Delete |
| Fuel | Edit, Delete (`useUpdateFuelLog`, `useDeleteFuelLog`) |
| Mileage | Read-only — no update or delete endpoint exists |

Maintenance record editing requires a new `useUpdateMaintenanceRecord` hook. The
route and types already exist (`PATCH /api/fleet/maintenance-records/{id}`,
`UpdateMaintenanceRecordAttributes`); only the hook and an edit-mode dialog
reusing `MaintenanceRecordForm` are missing.

## 9. Dialogs

Adds shadcn `dialog`, `sheet`, `command`, and `popover` to
`apps/web/src/components/ui/`. Radix provides focus trap, ESC handling, and
scroll lock.

| Dialog | Wraps |
|---|---|
| Edit vehicle | `VehicleForm` (mode="edit") |
| Log mileage | `MileageForm` |
| Log fill-up | `FuelForm` |
| Log maintenance / modification | `MaintenanceRecordForm` |
| Edit record | `MaintenanceRecordForm` / `FuelForm` |
| Add schedule | `MaintenanceScheduleForm` |
| Complete schedule | New small form (date, odometer) |
| Photo gallery | Grid, upload, set-primary, delete |
| Delete vehicle | Confirmation |

Dialogs close on mutation success and stay open on error, preserving today's
toast behavior.

## 10. Free-form categories

Categories are currently global system data with a GET-only route. Custom
entries must not leak across fleets, so they become fleet-scoped. The
`SystemDefined bool` column already defaults to `false`, so the schema
anticipated user-authored rows.

### 10.1 Backend

- **`entity.go`** — add `FleetID *string` (`gorm:"type:uuid;index"`). `NULL`
  means system/global. `Seed()` is unchanged: it is keyed by name and writes
  `NULL` fleet IDs.
- **`model.go`** — add an immutable `fleetID` field and accessor.
- **`provider.go`** — `List` and `IDsByKind` take a `fleetID` and scope with
  `WHERE fleet_id IS NULL OR fleet_id = ?`. Add `Create(Entity)` and a
  case-insensitive `FindByName(fleetID, name, kind)`.
- **`processor.go`** — add `Create(fleetID, name, kind)`: trims, rejects empty,
  caps length at 60, and does a case-insensitive lookup against system ∪ fleet
  rows first. A match returns the existing model rather than creating a
  duplicate, which makes the endpoint idempotent and prevents "oil change"
  landing beside "Oil Change".
- **`resource.go`** — add `POST /maintenance-categories`. The fleet comes from
  the caller's identity, not the body. Write authorization reuses the same role
  check as maintenance record creation; viewers cannot create categories.
- A unique index on `(fleet_id, name, kind)` is a backstop only. PostgreSQL
  treats `NULL`s as distinct, so it does not constrain system rows — those stay
  protected by `Seed()`'s existing `FirstOrCreate`-by-name.

**Ripple:** `IDsByKind` is consumed by `maintenancerecord` through the
`CategoryAccessor` interface. Its signature changes, so that interface and its
implementations move together in one commit.

### 10.2 Frontend

- `MaintenanceCategoryService` gains `create({ name, kind })`.
- New `useCreateMaintenanceCategory()` hook invalidating the category list.
- New `CategoryCombobox` component (shadcn `command` + `popover`): filters as
  you type, groups results under "Suggested" and "Custom", and offers
  `Create "<input>"` only when the trimmed input has no case-insensitive match
  in the visible list. On create it mutates, then selects the returned ID.
- `MaintenanceRecordForm` and `MaintenanceScheduleForm` swap their `<Select>`
  for `CategoryCombobox`. The schedule form keeps its maintenance-kind-only
  filter (`maintenance_schedules` is maintenance-only per PRD §2 non-goals).
- Kind is fixed by the calling dialog, so a category created from the
  "Log modification" dialog is created with `kind: 'modification'`.
- The Zod schemas are unchanged. `categoryId` remains a required string,
  because creation resolves to an ID inside the combobox before submit.

## 11. Testing

- **Pure functions first.** The merge/sort/filter/watermark reducer and the four
  stat derivations are pure and carry the most logic. They get direct unit
  tests, including empty-data and single-record cases.
- **Backend.** Processor tests for `Create`: case-insensitive dedupe against
  system rows, dedupe within a fleet, trim and length rejection. Provider tests
  for fleet scoping — a category created in fleet A must not appear in fleet B's
  list or in its `IDsByKind` result.
- **Components.** Chip filtering, drawer opening with the correct row, and
  `CategoryCombobox` showing the create affordance only for genuinely new names.
- **Must be updated: `MaintenanceRecordForm.test.tsx`.** It opens the category
  picker via `getByRole('combobox')` and inspects the Radix listbox directly,
  and it carries a jsdom polyfill for the pointer-event methods Radix `Select`
  calls on open. Replacing `Select` with `CategoryCombobox` breaks both. The
  existing assertion — "offers only the categories of the requested kind" — must
  be preserved against the new picker, and a case is added for the create
  affordance.
- **Unaffected.** `RecordAttachmentList.test.tsx` and `VehicleCard.test.tsx`
  cover code this design does not touch. They must pass unmodified; if they need
  edits, a boundary was crossed that was not intended.

## 12. Risks

- **Scope.** This is a frontend layout task with a backend feature attached.
  The category work (§10) is independently shippable and should be sequenced
  first in the plan so the dialogs are built once against their final picker.
- **The watermark merge** is the most subtle logic here. It is isolated in a
  pure function specifically so it can be tested rather than reasoned about.
- **Dissolving four section components** touches every data hook on the page at
  once. The forms staying untouched is the mitigation.
