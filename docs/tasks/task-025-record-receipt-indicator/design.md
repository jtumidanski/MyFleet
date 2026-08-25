# Receipt Indicator on Vehicle Record Rows — Design

Task: task-025-record-receipt-indicator
PRD: `docs/tasks/task-025-record-receipt-indicator/prd.md` (approved)
Status: Draft
Created: 2026-08-25

---

## 1. Scope Recap

Frontend-only. Three files change:

1. `apps/web/src/lib/vehicleRecords.ts` — add the count to `VehicleRecordRow`.
2. `apps/web/src/lib/hooks/api/vehicleRecords.ts` — populate it in the maintenance adapter.
3. `apps/web/src/components/features/vehicles/detail/VehicleRecordsTable.tsx` — header cell, body
   cell, and the two `colSpan` sites.

No Go changes, no schema changes, no new requests. Verified against source: `documentMediaIds?:
string[]` is already on `MaintenanceRecordAttributes`
(`apps/web/src/types/models/maintenanceRecord.ts:23`), the list hook flat-maps whole resources, and
the adapter at `vehicleRecords.ts:60-72` currently drops it on the floor.

---

## 2. Data Shape

### D1 — `documentCount?: number` on `VehicleRecordRow`

```ts
/**
 * Count of documents attached to this record. Populated only by the
 * maintenance adapter; fuel and mileage have no document concept and leave
 * it unset. Deliberately a plain number, not an icon or component — this
 * module is pure data and its tests assert on plain values (same reason
 * VehicleCard.tsx:37-40 keeps lucide-react out of modules like this).
 */
documentCount?: number;
```

**Alternative rejected — `hasDocuments: boolean`.** Smaller and sufficient for "is there
paperwork", but FR-UI-2 requires rendering N, so a boolean would force a second field or a re-read
of the raw resource in the table. A single number carries both signals (`> 0` is the boolean).

**Alternative rejected — `documentMediaIds: string[]` passed straight through.** Tempting because
it is what the API sends, and it would let a future row-level hover preview name the files. But it
puts ids the row never uses into a normalized type whose whole job is to erase source-specific
shape, it makes the merge/sort tests carry array fixtures for no assertion, and it invites exactly
the per-attachment fan-out that NFR "Performance" forbids. YAGNI: store the count.

**Optionality.** The field is optional rather than `number` with a default, so the fuel and mileage
adapters stay untouched (FR-ROW-4). Under the FR-UI-1 render rule (`> 0`), `undefined` and `0` are
indistinguishable at the render layer, so optionality costs nothing in branching:
`(row.documentCount ?? 0) > 0` is the single guard.

### D2 — Adapter populates with `?? 0`

In the maintenance branch of `useVehicleRecords`:

```ts
documentCount: r.attributes.documentMediaIds?.length ?? 0,
```

The server emits `documentMediaIds` with `omitempty`
(`apps/fleet-service/internal/maintenancerecord/rest.go:22`), so a record with no attachments omits
the key entirely. Writing an explicit `0` rather than leaving it `undefined` makes "the backend
said nothing, which means none" an assertion the adapter test can pin (FR-ROW-3), instead of an
ambiguity that reads the same as "fuel rows don't have this concept".

---

## 3. Placement — resolving PRD Open Question 1

**Decision: a dedicated sixth column, appended after Cost.** This is what FR-TBL-1 specifies; the
design phase confirms it rather than reopening it.

| Option | For | Against |
|---|---|---|
| **Sixth column (chosen)** | Vertically aligned, scannable down the page — the actual user story is "scan the feed for paperwork". Never competes with `row.title`. | Two `colSpan` sites to update; one more column at narrow viewports. |
| Inline after `row.title` in the Item cell | No column, no `colSpan` churn. | The Item cell is `max-w-[240px] truncate`. An inline sibling either gets truncated away with the title or forces the cell to `flex` with a `min-w-0` child — added complexity and a real risk the indicator silently disappears on long descriptions, which is the one thing it must never do. Also unscannable: the icon lands at a different x on every row. |

The column goes **last**, after Cost, not between Item and Odometer. Odometer and Cost are the two
numeric columns and reading them as a pair is the common case; splitting them with an icon column
would be worse than appending. The table already lives inside `<div className="overflow-x-auto">`,
so a sixth column scrolls rather than crushing the others — FR-TBL-4 is satisfied structurally, and
`Item`'s `max-w-[240px] truncate` is untouched. The cell gets `whitespace-nowrap` so the icon and
count never wrap onto two lines.

---

## 4. Component Design

### D3 — A local `AttachmentIndicator`, mirroring `TypeBadge`

`VehicleRecordsTable.tsx` already defines a small presentational `TypeBadge` above the main
component. The indicator follows that precedent: a module-local function component in the same
file, not a shared export. Nothing else in the app renders this, and premature promotion to
`components/ui` or `@myfleet/ui-components` would be speculative.

```tsx
function AttachmentIndicator({ count }: { count: number }) {
  return (
    <span className="inline-flex items-center gap-1 text-muted-foreground">
      <Paperclip className="h-4 w-4" aria-hidden="true" />
      <span aria-hidden="true">{count}</span>
      <span className="sr-only">{count === 1 ? '1 attachment' : `${count} attachments`}</span>
    </span>
  );
}
```

Three things worth stating explicitly:

- **The visible count is also `aria-hidden`.** If it weren't, a screen reader would announce
  "3, 3 attachments" — the bare number plus the label. Hiding both the icon and the digit and
  exposing exactly one `sr-only` string gives a single, correctly pluralized announcement
  (FR-UI-5). This is the same pattern as `PhotoGalleryDialog.tsx:141`.
- **No `button`, no handler, no `tabIndex`** (FR-UI-4). It is spans only, so clicks bubble to the
  `<tr role="button">`'s existing `onClick` and no tab stop is added.
- **Pluralization is a ternary, not a library.** The app has no i18n layer; introducing
  `Intl.PluralRules` for one string would be the heavier tool for no gain.

### D4 — Render guard in the row

```tsx
<td className="whitespace-nowrap p-2">
  {documentCount > 0 && <AttachmentIndicator count={documentCount} />}
</td>
```

with `const documentCount = row.documentCount ?? 0;` narrowed at the top of the `visible.map`
callback, so the cell needs no non-null assertion.

One rule, applied uniformly (FR-UI-1). Fuel and mileage rows render an empty `<td>` because their
count is unset, not because of a `kind` check — there is deliberately no `row.kind === ...` branch
anywhere in this change. That keeps the acceptance criterion "a modification record behaves
identically to a maintenance record" true by construction rather than by a second code path.

### D5 — Header cell

```tsx
<th className="p-2 font-medium">
  <span className="sr-only">Attachments</span>
</th>
```

Visually empty so it does not crowd the header row (FR-TBL-3), but announced to assistive
technology so the column is not a nameless void in table-navigation mode. `sr-only` is the
project's established utility (`sidebar.tsx:287`, `dialog.tsx:142`, `PhotoGalleryDialog.tsx:156`).

### D6 — `colSpan` derived from a constant

FR-TBL-2 flags the real hazard: `colSpan={5}` is hardcoded in two places (`SkeletonRows` at
`:71-83`, empty state at `:161-165`) and missing either leaves a visibly broken table. Rather than
bumping two literals to `6` and leaving the same trap armed for the seventh column, introduce:

```ts
/** Column count for the full-width skeleton and empty-state cells. Keep in sync with <thead>. */
const COLUMN_COUNT = 6;
```

and use `colSpan={COLUMN_COUNT}` at both sites. This is the "targeted improvement to code you are
working in" the guidelines call for — it is three lines, it is confined to the file being changed,
and it removes the exact failure mode the PRD warns about. It does not eliminate the need to update
the constant when a column is added, but it reduces two silent drift sites to one named one.

---

## 5. Performance

The indicator is a pure function of a number already present in the merged row array. It mounts no
hooks, issues no requests, and touches no media metadata. `RecordAttachmentList`'s deliberate
constraint — one `useMediaObject` per attachment, mounted only for the expanded record to avoid
"25 × N metadata requests" (`RecordAttachmentList.tsx:85-87`) — is preserved because nothing in
this change imports or renders that component.

The adapter adds one `?.length` per maintenance row inside an existing `.map` in an existing
`useMemo`; its dependency array is unchanged (`maintenance.data` already covers the new read).

---

## 6. Cache Freshness — resolving PRD Open Question 3

**Verified, not assumed.**

- **Create path (user story 3) works.** `LogMaintenanceDialog` calls `useCreateMaintenanceRecord`
  with `documentMediaIds` in the create body (`LogMaintenanceDialog.tsx:40-49`), and that
  mutation's `onSettled` invalidates `maintenanceRecordKeys.lists()`
  (`lib/hooks/api/maintenance.ts:172`). The feed's list query refetches and the row arrives with
  its count already correct. No extra invalidation needed.

- **Append path has a pre-existing gap, out of scope.**
  `useAppendMaintenanceRecordDocument` (`maintenance.ts:204-216`) invalidates only
  `maintenanceRecordKeys.detail(id)` and `vehicleKeys.detail(vehicleId)` — **not** `lists()`. So
  attaching a document from the drawer's Edit flow would update the drawer but leave the row's
  count stale until the list refetches for another reason. This is moot today because, as PRD §9.2
  documents, `POST /maintenance-records/{id}/document-media` is **not registered in
  `fleet-service`** and any call 404s. The one-line fix (add a `lists()` invalidation) belongs with
  whatever task implements that endpoint, not here — fixing the cache key for a route that does not
  exist would be untestable and would imply the flow works.

  **Recorded as a follow-up**, alongside PRD §9.2's phantom-endpoint defect.

---

## 7. Testing

### `lib/hooks/api/vehicleRecords.test.ts` (adapter)

- Maintenance record with `documentMediaIds: ['a','b','c']` → `documentCount === 3`.
- Maintenance record with the key **absent** → `documentCount === 0` (the `omitempty` case,
  FR-ROW-3). This is the assertion that would catch a regression to `?.length` without `?? 0`.
- Maintenance record with `documentMediaIds: []` → `documentCount === 0`.
- Fuel row and mileage row → `documentCount === undefined`.

### `components/features/vehicles/detail/VehicleRecordsTable.test.tsx`

- Row with `documentCount: 3` → the accessible text `3 attachments` is present.
- Row with `documentCount: 1` → `1 attachment` (singular), proving pluralization.
- Row with `documentCount: 0` and a row with it `undefined` → no attachment text, and no `'0'`
  rendered in that cell.
- Fuel and mileage rows → no indicator.
- **Non-interactivity:** the indicator's container is not a `button` and adds no tab stop — assert
  the row remains the only element with `role="button"` in its `<tr>`, and that clicking the
  indicator calls `onSelectRow` (proving the click bubbles rather than being swallowed).
- **No network fan-out:** assert the media-object hook is never called when a page of rows with
  attachments renders. Mechanically: `vi.mock` the media hook module and assert the spy has zero
  calls. This is the acceptance criterion that guards the NFR.
- Skeleton and empty-state cells carry `colSpan={6}`.

Existing tests in both files must continue to pass unchanged; nothing in this design alters merge,
sort, filter, or footer behaviour.

---

## 8. Risks

| Risk | Mitigation |
|---|---|
| A future seventh column re-arms the `colSpan` drift | `COLUMN_COUNT` constant (D6) plus a test asserting the skeleton/empty `colSpan` matches the `<thead>` cell count. |
| Double announcement of the count by screen readers | Visible digit is `aria-hidden`; exactly one `sr-only` string (D3). Covered by the singular/plural tests. |
| Sixth column crowds narrow viewports | Table already wrapped in `overflow-x-auto`; new cell is `whitespace-nowrap`; `Item`'s truncation is untouched (§3). |
| Someone later "improves" the indicator into a button | FR-UI-4 is encoded as a test, not just prose (§7). |

---

## 9. Out of Scope / Follow-ups

- Register `POST /api/fleet/maintenance-records/{id}/document-media` in `fleet-service`, or delete
  the dead client (PRD §9.2).
- Add `maintenanceRecordKeys.lists()` invalidation to `useAppendMaintenanceRecordDocument` — do it
  with the above, per §6.
- Filtering or sorting the feed by "has attachments".
- Document type/thumbnail/filename in the row; `RecordAttachmentList` in the drawer stays the place
  for that.
