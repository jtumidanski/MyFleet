# Receipt Indicator on Vehicle Record Rows — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Show a `Paperclip` icon and attachment count on vehicle record rows that have documents attached, using data already present in the list response.

**Architecture:** Frontend-only, three files. `VehicleRecordRow` gains an optional `documentCount: number`; the maintenance adapter in `useVehicleRecords` populates it from `attributes.documentMediaIds?.length ?? 0` (fuel and mileage leave it unset); `VehicleRecordsTable` gains a sixth column rendering a module-local, non-interactive `AttachmentIndicator` when the count is `> 0`. No Go changes, no schema changes, no new network requests.

**Tech Stack:** React 19, TypeScript, Vite, Vitest + @testing-library/react, Tailwind, `lucide-react`.

**Spec:** `docs/tasks/task-025-record-receipt-indicator/design.md` (PRD: `docs/tasks/task-025-record-receipt-indicator/prd.md`)

## Global Constraints

- **Worktree.** All work happens in `/home/tumidanski/source/MyFleet/.worktrees/task-025-record-receipt-indicator` on branch `task-025-record-receipt-indicator`. Never edit the main checkout.
- **No backend changes.** `apps/fleet-service` and `apps/media-service` are untouched. PRD §5/§6/§7 state this as a requirement, not an observation.
- **No new network requests.** The indicator derives purely from `documentMediaIds.length`. Nothing in this change may import or render `RecordAttachmentList`, `useMediaObject`, or any other media hook. (`RecordAttachmentList.tsx:84-87` documents that it is mounted only for the expanded record specifically to avoid "25 × N metadata requests".)
- **No `kind` branching for the indicator.** The single render rule is `documentCount > 0`, applied uniformly to all four kinds (FR-UI-1). There must be no `row.kind === ...` check anywhere in this change.
- **Non-interactive indicator.** No `button`, no `onClick`, no `onKeyDown`, no `tabIndex` on the indicator (FR-UI-4). The `<tr role="button">` remains the only focusable target in the row.
- **Icon convention.** `lucide-react` icons are `className="h-4 w-4"` and `aria-hidden="true"` (`RecordAttachmentList.tsx:75-79`, `PhotoGalleryDialog.tsx:155`).
- **`lib/vehicleRecords.ts` stays pure data.** No `lucide-react`, no JSX, no component references in that module.
- **Negative call assertions must use `expectNoCall`.** `apps/web/eslint.config.js:56-77` makes a bare `expect(spy).not.toHaveBeenCalled()` (and `toHaveBeenCalledTimes(0)`) an eslint **error** in test files. Import `expectNoCall` from `src/test/expectNoCall` — it is `async`, so `await` it.
- **Node may not be on `PATH`.** If `npm` is missing: `export NVM_DIR="$HOME/.nvm" && . "$NVM_DIR/nvm.sh" && nvm use 22`.

---

### Task 1: Thread `documentCount` through the row type and adapter

**Files:**
- Modify: `apps/web/src/lib/vehicleRecords.ts:5-27` (the `VehicleRecordRow` interface)
- Modify: `apps/web/src/lib/hooks/api/vehicleRecords.ts:60-72` (the maintenance branch of the `sources` `useMemo`)
- Test: `apps/web/src/lib/hooks/api/vehicleRecords.test.ts` (append a new `describe` block)

**Interfaces:**
- Consumes: `MaintenanceRecordAttributes.documentMediaIds?: string[]` (`apps/web/src/types/models/maintenanceRecord.ts:23`) — already defined, already on the wire, currently dropped by the adapter.
- Produces: `VehicleRecordRow.documentCount?: number` — a plain number. Populated (possibly `0`) by the maintenance adapter for both `kind: 'maintenance'` and `kind: 'modification'` rows; left `undefined` by the fuel and mileage adapters. Task 2 reads this and nothing else.

- [ ] **Step 1: Write the failing adapter tests**

Append this block to the end of `apps/web/src/lib/hooks/api/vehicleRecords.test.ts`, immediately after the closing `});` of the existing `describe('useVehicleRecords', ...)`. It reuses the file's existing fixture builders (`maintenanceRecord`, `fuelLog`, `mileageRecord`, `stub`, `setupSources`, `NO_CATEGORIES`) and its existing `renderHook` import — add no new imports.

```tsx
// ---------------------------------------------------------------------------
// documentCount (task-025)
// ---------------------------------------------------------------------------

describe('useVehicleRecords documentCount', () => {
  it('counts the attached documents on a maintenance row', () => {
    setupSources({
      maintenance: stub([
        maintenanceRecord('m1', 'oil-change', '2026-01-01T00:00:00Z', {
          documentMediaIds: ['a', 'b', 'c'],
        }),
      ]),
    });

    const { result } = renderHook(() => useVehicleRecords('v1', NO_CATEGORIES));

    expect(result.current.rows[0].documentCount).toBe(3);
  });

  // The server emits documentMediaIds with `omitempty`
  // (fleet-service/internal/maintenancerecord/rest.go:22), so a record with no
  // attachments omits the key entirely. "The backend said nothing" must mean
  // zero, not unknown — a bare `?.length` without `?? 0` would leave it
  // undefined here and make a maintenance row indistinguishable from a fuel
  // row, which genuinely has no document concept.
  it('treats an absent documentMediaIds key as zero', () => {
    setupSources({
      maintenance: stub([maintenanceRecord('m1', 'oil-change', '2026-01-01T00:00:00Z')]),
    });

    const { result } = renderHook(() => useVehicleRecords('v1', NO_CATEGORIES));

    expect(result.current.rows[0].documentCount).toBe(0);
  });

  it('treats an empty documentMediaIds array as zero', () => {
    setupSources({
      maintenance: stub([
        maintenanceRecord('m1', 'oil-change', '2026-01-01T00:00:00Z', {
          documentMediaIds: [],
        }),
      ]),
    });

    const { result } = renderHook(() => useVehicleRecords('v1', NO_CATEGORIES));

    expect(result.current.rows[0].documentCount).toBe(0);
  });

  it('leaves documentCount unset on fuel and mileage rows', () => {
    setupSources({
      fuel: stub([fuelLog('f1', '2026-03-01T00:00:00Z', 10)]),
      mileage: stub([mileageRecord('mi1', '2026-02-01T00:00:00Z', 12000)]),
    });

    const { result } = renderHook(() => useVehicleRecords('v1', NO_CATEGORIES));

    const byId = new Map(result.current.rows.map((r) => [r.id, r]));
    expect(byId.get('fuel:f1')?.documentCount).toBeUndefined();
    expect(byId.get('mileage:mi1')?.documentCount).toBeUndefined();
  });
});
```

- [ ] **Step 2: Run the tests to verify they fail**

```sh
npm run -w apps/web test -- src/lib/hooks/api/vehicleRecords.test.ts
```

Expected: 4 failures in `useVehicleRecords documentCount`. The first three report `undefined` received instead of `3`/`0`/`0`. The fourth passes already (the field genuinely doesn't exist yet) — that is fine and expected; it is a guard against a later regression, not a red-first driver.

If TypeScript in the editor flags `documentCount` as not existing on `VehicleRecordRow`, that is the same red. `vitest run` transpiles without type-checking, so the failure surfaces as the runtime `undefined` assertions above.

- [ ] **Step 3: Add the field to `VehicleRecordRow`**

In `apps/web/src/lib/vehicleRecords.ts`, inside the `VehicleRecordRow` interface, add the field after the existing `gallons?: number;` member (the last one, ending the interface):

```ts
  /** Fuel volume in gallons. Populated by the fuel adapter; used to derive average economy. */
  gallons?: number;
  /**
   * Count of documents attached to this record. Populated (possibly 0) only by
   * the maintenance adapter; fuel and mileage have no document concept and
   * leave it unset. Deliberately a plain number, not an icon or component —
   * this module is pure data and its tests assert on plain values (the same
   * reason VehicleCard.tsx:37-40 keeps lucide-react out of modules like this).
   * A count rather than the raw id array: the row never uses the ids, and
   * carrying them would invite exactly the per-attachment fan-out that
   * RecordAttachmentList.tsx:84-87 exists to avoid.
   */
  documentCount?: number;
```

- [ ] **Step 4: Populate it in the maintenance adapter**

In `apps/web/src/lib/hooks/api/vehicleRecords.ts`, in the `maintenanceRows` map, add the field after `cost`:

```ts
        mileage: r.attributes.mileage,
        cost: r.attributes.cost,
        // `?? 0` is load-bearing: the server omits documentMediaIds entirely
        // when empty (omitempty, rest.go:22). An explicit 0 makes "no
        // attachments" a fact the adapter asserts, rather than leaving it
        // indistinguishable from fuel/mileage's "no such concept".
        documentCount: r.attributes.documentMediaIds?.length ?? 0,
```

Do **not** touch `fuelRows` or `mileageRows` — they must leave `documentCount` unset (FR-ROW-4). Do **not** change the `useMemo` dependency array: `maintenance.data` already covers this new read.

- [ ] **Step 5: Run the tests to verify they pass**

```sh
npm run -w apps/web test -- src/lib/hooks/api/vehicleRecords.test.ts
```

Expected: PASS — all 4 new tests plus every pre-existing test in the file. The merge, sort, watermark, categories-gating and memoized-identity tests must be untouched and green.

- [ ] **Step 6: Commit**

```sh
git add apps/web/src/lib/vehicleRecords.ts apps/web/src/lib/hooks/api/vehicleRecords.ts apps/web/src/lib/hooks/api/vehicleRecords.test.ts
git commit -m "feat(web): carry document count on vehicle record rows"
```

---

### Task 2: Render the indicator column in `VehicleRecordsTable`

**Files:**
- Modify: `apps/web/src/components/features/vehicles/detail/VehicleRecordsTable.tsx`
  - `:1-9` imports
  - `:56-68` (after `TypeBadge`) — new `AttachmentIndicator`
  - `:70-83` `SkeletonRows` `colSpan`
  - `:148-156` `<thead>` header row
  - `:161-165` empty-state `colSpan`
  - `:167-197` the `visible.map` row body
- Test: `apps/web/src/components/features/vehicles/detail/VehicleRecordsTable.test.tsx`

**Interfaces:**
- Consumes: `VehicleRecordRow.documentCount?: number` from Task 1.
- Produces: nothing consumed by a later task. `AttachmentIndicator` and `COLUMN_COUNT` are module-local and deliberately **not** exported — `react-refresh/only-export-components` is a warning in this config, and nothing else in the app renders this indicator (design D3).

- [ ] **Step 1: Write the failing table tests**

Two edits to `apps/web/src/components/features/vehicles/detail/VehicleRecordsTable.test.tsx`.

**(a)** Replace the import block at the top of the file with this (adds `within`, and mocks the media hook module):

```tsx
import { describe, it, expect, vi } from 'vitest';
import { render, screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { VehicleRecordsTable } from './VehicleRecordsTable';
import type { VehicleRecordRow } from '../../../../lib/vehicleRecords';
import { useMediaObject } from '../../../../lib/hooks/api/media';
import { expectNoCall } from '../../../../test/expectNoCall';

// Partial mock (importOriginal spread) rather than a bare factory: media.ts
// exports ~18 symbols and a factory replacing the module wholesale would break
// the moment anything here transitively imports one of the others. Only
// useMediaObject is spied, which is the one this table must never call — see
// RecordAttachmentList.tsx:84-87 on the 25 x N metadata fan-out this guards.
vi.mock('../../../../lib/hooks/api/media', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../../../lib/hooks/api/media')>()),
  useMediaObject: vi.fn(),
}));
```

Then extend the existing `rows` fixture so the maintenance row carries attachments, the mod row carries exactly one, and a fourth row carries an explicit zero. Replace the whole `const rows: VehicleRecordRow[] = [...]` array with:

```tsx
const rows: VehicleRecordRow[] = [
  {
    id: 'fuel:1',
    sourceId: '1',
    kind: 'fuel',
    date: '2026-07-28T00:00:00Z',
    title: '16.204 gal',
    mileage: 84010,
    cost: 58.31,
  },
  {
    id: 'maintenance:2',
    sourceId: '2',
    kind: 'maintenance',
    date: '2026-07-12T00:00:00Z',
    title: 'Front brake pads',
    mileage: 82940,
    cost: 612.4,
    documentCount: 3,
  },
  {
    id: 'maintenance:4',
    sourceId: '4',
    kind: 'maintenance',
    date: '2026-06-02T00:00:00Z',
    title: 'Cabin air filter',
    mileage: 81200,
    cost: 34.99,
    documentCount: 0,
  },
  {
    id: 'modification:3',
    sourceId: '3',
    kind: 'modification',
    date: '2026-05-18T00:00:00Z',
    title: 'Rock sliders',
    mileage: 79430,
    cost: 980,
    documentCount: 1,
  },
];
```

Adding a fourth row changes the row count the footer reports, so also update the existing footer test in the same file — change

```tsx
    expect(screen.getByText(/showing 3 of 41/i)).toBeInTheDocument();
```

to

```tsx
    expect(screen.getByText(/showing 4 of 41/i)).toBeInTheDocument();
```

**(b)** Append this `describe` block after the existing `describe('VehicleRecordsTable', ...)` block's closing `});`:

```tsx
describe('VehicleRecordsTable attachment indicator', () => {
  /** The <tr> containing a row's Item cell text. */
  function rowFor(title: string): HTMLElement {
    const tr = screen.getByText(title).closest('tr');
    if (!tr) throw new Error(`no <tr> found for row titled "${title}"`);
    return tr;
  }

  it('announces the attachment count on a record that has documents', () => {
    renderTable();
    expect(within(rowFor('Front brake pads')).getByText('3 attachments')).toBeInTheDocument();
  });

  it('uses the singular form for a single attachment', () => {
    renderTable();
    expect(within(rowFor('Rock sliders')).getByText('1 attachment')).toBeInTheDocument();
  });

  it('renders nothing for a record with zero attachments', () => {
    renderTable();
    const row = rowFor('Cabin air filter');
    expect(within(row).queryByText(/attachment/i)).not.toBeInTheDocument();
    // Not even a bare "0" — absence of the icon is the negative signal
    // (PRD section 2, Non-goals).
    expect(within(row).queryByText('0')).not.toBeInTheDocument();
  });

  it('renders nothing for fuel and mileage rows, whose count is unset', () => {
    renderTable({
      rows: [
        ...rows,
        {
          id: 'mileage:5',
          sourceId: '5',
          kind: 'mileage',
          date: '2026-04-01T00:00:00Z',
          title: 'Odometer reading',
          mileage: 78000,
        },
      ],
    });
    expect(within(rowFor('16.204 gal')).queryByText(/attachment/i)).not.toBeInTheDocument();
    expect(within(rowFor('Odometer reading')).queryByText(/attachment/i)).not.toBeInTheDocument();
  });

  // FR-UI-4: the indicator must not become a button or add a tab stop. The row
  // itself is the only click target; clicking the indicator must bubble to it.
  it('is not interactive and lets clicks fall through to the row', async () => {
    const onSelectRow = vi.fn();
    const user = userEvent.setup();
    renderTable({ onSelectRow });

    const row = rowFor('Front brake pads');
    const label = within(row).getByText('3 attachments');

    // queryAllByRole, not getAllByRole — the get* variant throws on zero
    // matches, which is exactly the passing case here.
    expect(within(row).queryAllByRole('button')).toHaveLength(0);
    expect(row).toHaveAttribute('role', 'button');
    expect(label.closest('button')).toBeNull();

    await user.click(label);
    expect(onSelectRow).toHaveBeenCalledWith(expect.objectContaining({ id: 'maintenance:2' }));
  });

  // The whole point of storing a count rather than the ids: rendering a page of
  // rows with attachments must issue zero media-metadata requests.
  it('fetches no media metadata when rows with attachments render', async () => {
    renderTable();
    await expectNoCall(vi.mocked(useMediaObject), 'useMediaObject');
  });

  it('spans every column in the skeleton state', () => {
    const { container } = renderTable({ isLoading: true });
    const headerCount = container.querySelectorAll('thead th').length;
    expect(headerCount).toBe(6);
    const spanned = container.querySelectorAll('tbody td[colspan]');
    expect(spanned.length).toBeGreaterThan(0);
    spanned.forEach((cell) => {
      expect(cell.getAttribute('colspan')).toBe(String(headerCount));
    });
  });

  it('spans every column in the empty state', () => {
    const { container } = renderTable({ rows: [] });
    const headerCount = container.querySelectorAll('thead th').length;
    const spanned = container.querySelectorAll('tbody td[colspan]');
    expect(spanned.length).toBeGreaterThan(0);
    spanned.forEach((cell) => {
      expect(cell.getAttribute('colspan')).toBe(String(headerCount));
    });
  });

  it('names the indicator column for assistive technology', () => {
    renderTable();
    expect(screen.getByText('Attachments')).toBeInTheDocument();
  });
});
```

- [ ] **Step 2: Run the tests to verify they fail**

```sh
npm run -w apps/web test -- src/components/features/vehicles/detail/VehicleRecordsTable.test.tsx
```

Expected: the count/singular/header-name tests fail with "Unable to find an element with the text"; the two `colSpan` tests fail with `expected 5 to be 6` / `expected "5" to be "6"`. The zero-attachment, fuel/mileage, non-interactive and no-fan-out tests pass already — they are regression guards.

- [ ] **Step 3: Add the `Paperclip` import and the `COLUMN_COUNT` constant**

In `apps/web/src/components/features/vehicles/detail/VehicleRecordsTable.tsx`, change the `lucide-react` import on line 2:

```tsx
import { Loader2, Paperclip } from 'lucide-react';
```

Then add the constant immediately after the `CHIPS` array (before the `KIND_BADGE` doc comment):

```ts
/**
 * Column count for the full-width skeleton and empty-state cells. Keep in sync
 * with <thead>. Two hardcoded `colSpan={5}` literals previously sat in
 * separate places and updating only one left a visibly misaligned table
 * (PRD FR-TBL-2); this reduces two silent drift sites to one named one, and
 * the colSpan tests assert it still matches the real header cell count.
 */
const COLUMN_COUNT = 6;
```

- [ ] **Step 4: Add the `AttachmentIndicator` component**

Add it directly after the `TypeBadge` function (which ends at line 68) and before `SkeletonRows`:

```tsx
/**
 * The paperclip + count shown on a record that has documents attached.
 *
 * Both the icon and the visible digit are aria-hidden, with exactly one
 * sr-only string carrying the whole message — otherwise a screen reader
 * announces "3, 3 attachments" (the bare number plus the label). Same pattern
 * as PhotoGalleryDialog.tsx:138-141.
 *
 * Spans only: no button, no handler, no tabIndex. Clicks bubble to the
 * enclosing <tr role="button">, so this adds no second affordance and no
 * extra tab stop (PRD FR-UI-4).
 */
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

- [ ] **Step 5: Update `SkeletonRows` to use the constant**

In `SkeletonRows`, replace `colSpan={5}` with `colSpan={COLUMN_COUNT}`:

```tsx
          <td className="p-2" colSpan={COLUMN_COUNT}>
```

- [ ] **Step 6: Add the header cell**

In the `<thead>` row, add a sixth `<th>` after the `Cost` header:

```tsx
                <th className="p-2 font-medium">Cost</th>
                {/*
                  Visually empty so it doesn't crowd the header row
                  (FR-TBL-3), but still named for table-navigation mode
                  rather than left a nameless column.
                */}
                <th className="p-2 font-medium">
                  <span className="sr-only">Attachments</span>
                </th>
```

- [ ] **Step 7: Update the empty-state `colSpan`**

```tsx
                  <td className="p-4 text-center text-sm text-muted-foreground" colSpan={COLUMN_COUNT}>
```

- [ ] **Step 8: Render the indicator cell**

Convert the `visible.map` callback from an expression body to a block body so the count can be narrowed once, and append the sixth `<td>`. Replace the whole `visible.map((row) => ( ... ))` expression with:

```tsx
                visible.map((row) => {
                  // Narrowed once here so the cell below needs no non-null
                  // assertion. Fuel and mileage rows land on 0 because their
                  // adapters leave the field unset, not because of a `kind`
                  // check — the `> 0` rule below is deliberately the only
                  // branch, which is what keeps a modification row behaving
                  // identically to a maintenance row by construction
                  // (PRD FR-UI-1).
                  const documentCount = row.documentCount ?? 0;
                  return (
                    <tr
                      key={row.id}
                      role="button"
                      tabIndex={0}
                      className="cursor-pointer border-b last:border-b-0 hover:bg-accent/50 focus-visible:outline-hidden focus-visible:ring-2 focus-visible:ring-ring"
                      onClick={() => onSelectRow(row)}
                      onKeyDown={(e) => {
                        if (e.key === 'Enter' || e.key === ' ') {
                          e.preventDefault();
                          onSelectRow(row);
                        }
                      }}
                    >
                      <td className="p-2 text-muted-foreground">
                        {new Date(row.date).toLocaleDateString()}
                      </td>
                      <td className="p-2">
                        <TypeBadge kind={row.kind} />
                      </td>
                      <td className="max-w-[240px] truncate p-2">{row.title}</td>
                      <td className="p-2 text-muted-foreground">
                        {typeof row.mileage === 'number' ? formatMileage(row.mileage) : '—'}
                      </td>
                      <td className="p-2 text-muted-foreground">
                        {typeof row.cost === 'number' ? formatMoney(row.cost) : '—'}
                      </td>
                      <td className="whitespace-nowrap p-2">
                        {documentCount > 0 && <AttachmentIndicator count={documentCount} />}
                      </td>
                    </tr>
                  );
                })
```

Note the `Item` cell keeps its `max-w-[240px] truncate` untouched (FR-TBL-4), and the new cell is `whitespace-nowrap` so the icon and count never wrap onto two lines. The table's existing `<div className="overflow-x-auto">` wrapper absorbs the extra column at narrow viewports.

- [ ] **Step 9: Run the tests to verify they pass**

```sh
npm run -w apps/web test -- src/components/features/vehicles/detail/VehicleRecordsTable.test.tsx
```

Expected: PASS — all 9 new tests plus all 7 pre-existing tests in the file (including the footer test updated in Step 1 to `showing 4 of 41`).

- [ ] **Step 10: Commit**

```sh
git add apps/web/src/components/features/vehicles/detail/VehicleRecordsTable.tsx apps/web/src/components/features/vehicles/detail/VehicleRecordsTable.test.tsx
git commit -m "feat(web): show attachment count on vehicle record rows"
```

---

### Task 3: Full frontend gate

**Files:** none modified unless a check fails.

**Interfaces:**
- Consumes: the finished work of Tasks 1 and 2.
- Produces: evidence the branch is green.

- [ ] **Step 1: Run the full frontend test suite**

```sh
make fe-test
```

This runs `apps/web`, `packages/shared-ts` and `packages/ui-components`. Expected: all three PASS. If `npm` is missing, load Node first (see Global Constraints).

- [ ] **Step 2: Run the type-check and build**

```sh
make fe-build
```

`apps/web`'s build script is `tsc -b && vite build`, so this is the type-check gate — it is what catches a `documentCount` typo that Vitest's transpile-only run would not. Expected: PASS.

- [ ] **Step 3: Run the lint check**

```sh
npm run -w apps/web lint
```

Expected: PASS with zero warnings (`eslint src --max-warnings 0`). Pay attention to two rules this change is near: `react-hooks/exhaustive-deps` (an **error** here — the `sources` `useMemo` dependency array must be unchanged, since `maintenance.data` already covers the new read) and the `no-restricted-syntax` ban on bare `not.toHaveBeenCalled()` in tests.

- [ ] **Step 4: Confirm no backend or dependency drift**

```sh
git diff --stat main...HEAD
```

Expected: exactly five files, all under `apps/web/src` plus this task's docs. No `apps/*-service` files, no `package.json`, no `package-lock.json`, no migrations. If anything else appears, stop and investigate.

- [ ] **Step 5: Commit any fixes**

Only if Steps 1-3 required changes:

```sh
git add -A
git commit -m "fix(web): address lint and type-check findings for the attachment indicator"
```

If everything passed clean, there is nothing to commit — do not create an empty commit.

---

## Out of Scope (recorded, do not implement here)

Both are carried over from design.md §9 and PRD §9.2. They are real defects; they belong to a separate task:

1. `POST /api/fleet/maintenance-records/{id}/document-media` is called by the frontend (`MaintenanceRecordService.ts:49-61`, `lib/hooks/api/maintenance.ts:206-216`) but is **not registered** in `fleet-service` (`resource.go` `InitializeRoutes` registers only list, create, get, patch, delete). Any call 404s.
2. `useAppendMaintenanceRecordDocument` (`maintenance.ts:204-216`) invalidates only `maintenanceRecordKeys.detail(id)` and `vehicleKeys.detail(vehicleId)` — not `lists()` — so an appended document would leave this new row count stale. Moot while (1) stands; fix it together with (1).

The **create** path, which is what user story 3 depends on, is already correct and needs no change: `useCreateMaintenanceRecord`'s `onSettled` invalidates `maintenanceRecordKeys.lists()` (`maintenance.ts:172`), so a record logged with receipts refetches the feed and arrives with its count already right.
