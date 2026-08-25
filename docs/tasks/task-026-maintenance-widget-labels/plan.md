# Maintenance Queue Labels Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the raw `categoryId` UUID rendered on four maintenance-queue render sites with the category's human-readable name, and give the two fleet-wide dashboard widgets the owning vehicle's title plus due-date/due-mileage context.

**Architecture:** Presentation-layer only, confined to `apps/web`. One new pure helper (`lib/utils/vehicleTitle.ts`) and one new module of two `Map`-building hooks plus their two fallback functions (`lib/hooks/api/labels.ts`). Four components are rewired to resolve labels through those helpers, and each component's loading gate is widened to OR in the `isLoading` of every query it now reads. No backend, endpoint, schema, or manifest change.

**Tech Stack:** React 19 + TypeScript, Vite, TanStack React Query v5, Tailwind CSS, shadcn/ui, Vitest + @testing-library/react.

**Spec:** `docs/tasks/task-026-maintenance-widget-labels/design.md` (PRD: `docs/tasks/task-026-maintenance-widget-labels/prd.md`)

## Global Constraints

- **No `any`.** Every lookup is typed against the existing model interfaces
  (`VehicleAttributes`, `MaintenanceCategoryAttributes`, `MaintenanceScheduleAttributes`).
- **`||`, never `??`, for presence checks.** Go marshals an unset string as
  `""`, not `null`. `??` lets an empty string through and renders a blank cell.
- **Unknown-value strings are exactly** `Unknown category` and `Unknown vehicle`.
- **Empty-state copy is unchanged, verbatim:** `No overdue maintenance.` /
  `No upcoming maintenance.` (widgets), `No overdue maintenance items.` /
  `No upcoming maintenance items.` (`MaintenanceQueueView`).
- **Due-line wording:** `Was due <date>` in overdue contexts, `Due <date>` in
  upcoming contexts, `At <n> miles` for mileage. Dates via
  `new Date(...).toLocaleDateString()`, mileage via `toLocaleString()`.
- **`nextDueMileage` uses a `? :` guard, not `&&`.** `&&` on a number renders a
  literal `0` into the row. Falsy means "render no mileage line".
- **`useMaintenanceCategories()` is called with no `kind` argument** at every
  new site, so `modification` categories resolve as well as `maintenance` ones.
- **Lookups are `Map`-based, built once per render in `useMemo`,** never rebuilt
  inside the row loop. No per-row fetch.
- **The 5-item cap** (`items.slice(0, 5)`) on both dashboard widgets is unchanged.
- **`UpcomingScheduleStrip` is not modified.** Its raw-id fallback stays.
- **`VehicleNameCrumb.test.tsx` is not modified,** and neither are the files its
  source-pin assertion reads (`VehicleDetailPage.tsx`, `VehicleNameCrumb.tsx`).
- **Only `VehicleCard` adopts the vehicle-title helper.** The other five
  duplicate call sites (`VehicleNameCrumb`, `VehicleDetailPage`,
  `VehicleIdentityRail`, `ActivityPage`, `VehicleStatusWidget`) are out of scope.
- All paths below are relative to the worktree root
  `/home/tumidanski/source/MyFleet/.worktrees/task-026-maintenance-widget-labels`.
- Node is not always on `PATH`. If `npm` is missing:
  `export NVM_DIR="$HOME/.nvm" && . "$NVM_DIR/nvm.sh" && nvm use 22`

---

## File Structure

| File | Responsibility |
| --- | --- |
| `apps/web/src/lib/utils/vehicleTitle.ts` (new) | Pure `VehicleAttributes -> string` title fallback chain. No React. |
| `apps/web/src/lib/utils/vehicleTitle.test.ts` (new) | Unit tests for the fallback chain, including the blank-nickname case. |
| `apps/web/src/lib/hooks/api/labels.ts` (new) | `useCategoryNameMap`, `useVehicleTitleMap`, `categoryLabel`, `vehicleLabel`, and the two unknown-value constants. |
| `apps/web/src/lib/hooks/api/labels.test.ts` (new) | Unit tests for the two pure label functions and the two hooks (source hooks mocked). |
| `apps/web/src/components/features/vehicles/VehicleCard.tsx` (modify) | Replace the inline title expression with `vehicleTitle(attributes)`. |
| `apps/web/src/components/features/vehicles/maintenance/MaintenanceQueueView.tsx` (modify) | Category names at both render sites; widened loading gate. |
| `apps/web/src/components/features/vehicles/maintenance/MaintenanceQueueView.test.tsx` (new) | Component tests, hooks mocked. |
| `apps/web/src/components/features/dashboard/widgets/OverdueMaintenanceWidget.tsx` (modify) | Category name, vehicle title, due context, stacked row, widened gate. |
| `apps/web/src/components/features/dashboard/widgets/OverdueMaintenanceWidget.test.tsx` (new) | Component tests, hooks mocked. |
| `apps/web/src/components/features/dashboard/widgets/UpcomingMaintenanceWidget.tsx` (modify) | Same as overdue, with upcoming wording. |
| `apps/web/src/components/features/dashboard/widgets/UpcomingMaintenanceWidget.test.tsx` (new) | Component tests, hooks mocked. |

---

## Task 1: The shared vehicle-title helper, adopted by `VehicleCard`

**Files:**
- Create: `apps/web/src/lib/utils/vehicleTitle.ts`
- Test: `apps/web/src/lib/utils/vehicleTitle.test.ts`
- Modify: `apps/web/src/components/features/vehicles/VehicleCard.tsx:52-54`

**Interfaces:**
- Consumes: `VehicleAttributes` from `apps/web/src/types/models/vehicle.ts`.
- Produces: `vehicleTitle(attributes: VehicleAttributes): string` — consumed by
  Task 2's `useVehicleTitleMap`.

**Why the exact expression matters:** `VehicleCard`'s current expression carries
a *second* `.trim()` on the template literal that the other five duplicate sites
do not have. Copying it verbatim is what makes FR-SHARED-2's
byte-identical-output requirement hold by construction, and it is why
`VehicleCard.test.tsx` needs no edit.

- [ ] **Step 1: Write the failing test**

Create `apps/web/src/lib/utils/vehicleTitle.test.ts`:

```ts
import { describe, it, expect } from 'vitest';
import { vehicleTitle } from './vehicleTitle';
import type { VehicleAttributes } from '../../types/models/vehicle';

function attrs(overrides: Partial<VehicleAttributes> = {}): VehicleAttributes {
  return { fleetId: 'f1', make: 'Ford', model: 'F-150', year: 2019, ...overrides };
}

describe('vehicleTitle', () => {
  it('prefers the nickname', () => {
    expect(vehicleTitle(attrs({ nickname: 'Weekend Truck' }))).toBe('Weekend Truck');
  });

  it('trims the nickname it returns', () => {
    expect(vehicleTitle(attrs({ nickname: '  Weekend Truck  ' }))).toBe('Weekend Truck');
  });

  // Load-bearing (FR-SHARED-2): fleet-service marshals an unset nickname as "",
  // and a whitespace-only nickname is user-enterable. `??` would let both
  // through and render a blank title.
  it('falls through a blank nickname to year make model', () => {
    expect(vehicleTitle(attrs({ nickname: '   ' }))).toBe('2019 Ford F-150');
  });

  it('falls through an empty-string nickname to year make model', () => {
    expect(vehicleTitle(attrs({ nickname: '' }))).toBe('2019 Ford F-150');
  });

  it('does not throw when the nickname is absent', () => {
    expect(vehicleTitle(attrs())).toBe('2019 Ford F-150');
  });
});
```

- [ ] **Step 2: Run the test and verify it fails**

```sh
npm run -w apps/web test -- src/lib/utils/vehicleTitle.test.ts
```

Expected: FAIL — cannot resolve `./vehicleTitle`.

- [ ] **Step 3: Write the implementation**

Create `apps/web/src/lib/utils/vehicleTitle.ts`:

```ts
import type { VehicleAttributes } from '../../types/models/vehicle';

/**
 * A vehicle's display title: the nickname the user set, otherwise
 * "<year> <make> <model>".
 *
 * `||` not `??`: fleet-service marshals an unset nickname as "" (Attributes are
 * plain Go strings), so `??` would let the empty string through and render a
 * blank title — the same trap displayName.ts documents. `.trim()` extends the
 * check to a whitespace-only nickname, which is user-enterable.
 *
 * The body is VehicleCard's previous expression verbatim, trailing `.trim()`
 * included, so its rendered output is unchanged for every input.
 */
export function vehicleTitle(attributes: VehicleAttributes): string {
  return (
    attributes.nickname?.trim() ||
    `${attributes.year} ${attributes.make} ${attributes.model}`.trim()
  );
}
```

- [ ] **Step 4: Run the test and verify it passes**

```sh
npm run -w apps/web test -- src/lib/utils/vehicleTitle.test.ts
```

Expected: PASS, 5 tests.

- [ ] **Step 5: Adopt the helper in `VehicleCard`**

In `apps/web/src/components/features/vehicles/VehicleCard.tsx`, replace:

```tsx
  const title =
    attributes.nickname?.trim() ||
    `${attributes.year} ${attributes.make} ${attributes.model}`.trim();
```

with:

```tsx
  const title = vehicleTitle(attributes);
```

and add the import alongside the other `lib` imports:

```tsx
import { vehicleTitle } from '../../../lib/utils/vehicleTitle';
```

- [ ] **Step 6: Verify `VehicleCard`'s existing tests still pass, unmodified**

```sh
npm run -w apps/web test -- src/components/features/vehicles/VehicleCard.test.tsx
```

Expected: PASS. Do **not** edit `VehicleCard.test.tsx` — the point of the
verbatim body is that it needs no edit. If it fails, the helper body diverged
from the original expression; fix the helper, not the test.

- [ ] **Step 7: Confirm the source-pin test is untouched and green**

```sh
npm run -w apps/web test -- src/components/frame/crumbs/VehicleNameCrumb.test.tsx
git diff --name-only
```

Expected: the test PASSES, and `git diff --name-only` lists neither
`VehicleNameCrumb.tsx`, `VehicleNameCrumb.test.tsx`, nor `VehicleDetailPage.tsx`.

- [ ] **Step 8: Commit**

```sh
git add apps/web/src/lib/utils/vehicleTitle.ts \
        apps/web/src/lib/utils/vehicleTitle.test.ts \
        apps/web/src/components/features/vehicles/VehicleCard.tsx
git commit -m "refactor(web): extract the shared vehicleTitle helper"
```

---

## Task 2: The label maps and their fallbacks

**Files:**
- Create: `apps/web/src/lib/hooks/api/labels.ts`
- Test: `apps/web/src/lib/hooks/api/labels.test.ts`

**Interfaces:**
- Consumes: `vehicleTitle` (Task 1); `useMaintenanceCategories` from
  `./maintenance` (returns `MaintenanceCategory[] | undefined` — it has a
  `select: (result) => result.data`); `useVehicles` from `./vehicles` (returns
  `ListResult<VehicleAttributes> | undefined` — it has **no** `select`, so rows
  live at `data.data`).
- Produces, all consumed by Tasks 3–5:
  - `UNKNOWN_CATEGORY: 'Unknown category'`
  - `UNKNOWN_VEHICLE: 'Unknown vehicle'`
  - `categoryLabel(names: Map<string, string>, id: string): string`
  - `vehicleLabel(titles: Map<string, string>, id: string): string`
  - `useCategoryNameMap(): { names: Map<string, string>; isLoading: boolean }`
  - `useVehicleTitleMap(fleetId: string | null | undefined): { titles: Map<string, string>; isLoading: boolean }`

**Two traps this task exists to pin down:**

1. `useVehicles` has no `select`, unlike every maintenance hook. Reading
   `data` where the code means `data.data` yields an empty map and a widget
   full of `Unknown vehicle` — a failure the FR-LOAD-3 fallback makes silent in
   production. The hook test feeds a realistic `{ data: [...] }` envelope so
   getting this wrong fails loudly.
2. `useCategoryNameMap` takes **no parameter**. That is the enforcement of
   FR-LABEL-3: there is no way for a caller to pass a `kind` and silently break
   `modification` categories.

- [ ] **Step 1: Write the failing test**

Create `apps/web/src/lib/hooks/api/labels.test.ts`:

```ts
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { renderHook } from '@testing-library/react';
import {
  UNKNOWN_CATEGORY,
  UNKNOWN_VEHICLE,
  categoryLabel,
  vehicleLabel,
  useCategoryNameMap,
  useVehicleTitleMap,
} from './labels';

// Mock the source query hooks rather than standing up a QueryClient: it matches
// the VehicleNameCrumb.test.tsx pattern and makes the loading state, otherwise
// a race, trivially reachable.
const mockUseMaintenanceCategories = vi.fn();
const mockUseVehicles = vi.fn();

vi.mock('./maintenance', () => ({
  useMaintenanceCategories: (...args: unknown[]) => mockUseMaintenanceCategories(...args),
}));
vi.mock('./vehicles', () => ({
  useVehicles: (fleetId: string | null | undefined) => mockUseVehicles(fleetId),
}));

describe('categoryLabel', () => {
  it('returns the mapped name on a hit', () => {
    expect(categoryLabel(new Map([['c1', 'Oil Change']]), 'c1')).toBe('Oil Change');
  });

  it('returns the unknown string on a miss', () => {
    expect(categoryLabel(new Map(), 'c1')).toBe(UNKNOWN_CATEGORY);
    expect(UNKNOWN_CATEGORY).toBe('Unknown category');
  });

  // `||` not `??`: fleet-service marshals an unset name as "", and a blank cell
  // is worse than an honest fallback.
  it('returns the unknown string for an empty-string name', () => {
    expect(categoryLabel(new Map([['c1', '']]), 'c1')).toBe(UNKNOWN_CATEGORY);
  });
});

describe('vehicleLabel', () => {
  it('returns the mapped title on a hit', () => {
    expect(vehicleLabel(new Map([['v1', 'Weekend Truck']]), 'v1')).toBe('Weekend Truck');
  });

  it('returns the unknown string on a miss', () => {
    expect(vehicleLabel(new Map(), 'v1')).toBe(UNKNOWN_VEHICLE);
    expect(UNKNOWN_VEHICLE).toBe('Unknown vehicle');
  });

  it('returns the unknown string for an empty-string title', () => {
    expect(vehicleLabel(new Map([['v1', '']]), 'v1')).toBe(UNKNOWN_VEHICLE);
  });
});

describe('useCategoryNameMap', () => {
  beforeEach(() => {
    mockUseMaintenanceCategories.mockReset();
  });

  it('maps id to name for every kind, and asks for no kind filter', () => {
    mockUseMaintenanceCategories.mockReturnValue({
      data: [
        { id: 'c1', type: 'maintenanceCategories', attributes: { name: 'Oil Change', kind: 'maintenance', systemDefined: true } },
        { id: 'c2', type: 'maintenanceCategories', attributes: { name: 'Cold Air Intake', kind: 'modification', systemDefined: false } },
      ],
      isLoading: false,
    });

    const { result } = renderHook(() => useCategoryNameMap());

    expect(result.current.names.get('c1')).toBe('Oil Change');
    expect(result.current.names.get('c2')).toBe('Cold Air Intake');
    expect(result.current.isLoading).toBe(false);
    // FR-LABEL-3: a kind filter here would reintroduce the bug for the other kind.
    expect(mockUseMaintenanceCategories).toHaveBeenCalledWith();
  });

  it('reports loading and an empty map while the query is in flight', () => {
    mockUseMaintenanceCategories.mockReturnValue({ data: undefined, isLoading: true });

    const { result } = renderHook(() => useCategoryNameMap());

    expect(result.current.isLoading).toBe(true);
    expect(result.current.names.size).toBe(0);
  });

  // FR-LOAD-3: a failed query settles with data undefined. The map is empty and
  // callers fall back; nothing throws.
  it('returns an empty map when the query failed', () => {
    mockUseMaintenanceCategories.mockReturnValue({ data: undefined, isLoading: false });

    const { result } = renderHook(() => useCategoryNameMap());

    expect(result.current.isLoading).toBe(false);
    expect(result.current.names.size).toBe(0);
  });
});

describe('useVehicleTitleMap', () => {
  beforeEach(() => {
    mockUseVehicles.mockReset();
  });

  // Guard for the projection mismatch: useVehicles has no `select`, so the rows
  // are at data.data. Feeding the realistic envelope makes a wrong read fail
  // here instead of degrading silently to "Unknown vehicle" in production.
  it('maps id to title from the list envelope', () => {
    mockUseVehicles.mockReturnValue({
      data: {
        data: [
          { id: 'v1', type: 'vehicles', attributes: { fleetId: 'f1', nickname: 'Weekend Truck', make: 'Ford', model: 'F-150', year: 2019 } },
          { id: 'v2', type: 'vehicles', attributes: { fleetId: 'f1', make: 'Honda', model: 'Civic', year: 2021 } },
        ],
      },
      isLoading: false,
    });

    const { result } = renderHook(() => useVehicleTitleMap('f1'));

    expect(result.current.titles.get('v1')).toBe('Weekend Truck');
    expect(result.current.titles.get('v2')).toBe('2021 Honda Civic');
    expect(mockUseVehicles).toHaveBeenCalledWith('f1');
  });

  it('reports loading and an empty map while the query is in flight', () => {
    mockUseVehicles.mockReturnValue({ data: undefined, isLoading: true });

    const { result } = renderHook(() => useVehicleTitleMap('f1'));

    expect(result.current.isLoading).toBe(true);
    expect(result.current.titles.size).toBe(0);
  });

  it('returns an empty map when the query failed', () => {
    mockUseVehicles.mockReturnValue({ data: undefined, isLoading: false });

    const { result } = renderHook(() => useVehicleTitleMap('f1'));

    expect(result.current.isLoading).toBe(false);
    expect(result.current.titles.size).toBe(0);
  });
});
```

- [ ] **Step 2: Run the test and verify it fails**

```sh
npm run -w apps/web test -- src/lib/hooks/api/labels.test.ts
```

Expected: FAIL — cannot resolve `./labels`.

- [ ] **Step 3: Write the implementation**

Create `apps/web/src/lib/hooks/api/labels.ts`:

```ts
import { useMemo } from 'react';
import { useMaintenanceCategories } from './maintenance';
import { useVehicles } from './vehicles';
import { vehicleTitle } from '../../utils/vehicleTitle';

/**
 * Client-side label resolution for maintenance queue rows.
 *
 * The queue endpoints return foreign keys, not names: shared-go/server has no
 * JSON:API `included` support, so there is no `?include=category` to ask for.
 * The category list is ~20 effectively-static rows cached for 10 minutes and
 * the fleet's vehicle list is already warm on the dashboard, so resolving in
 * the client costs no extra round trip in practice.
 *
 * The hooks own fetching and memoizing; the label functions own the fallback
 * string. Splitting them keeps the fallback unit-testable without a render, and
 * lets UpcomingScheduleStrip keep its own raw-id fallback over a compatible map.
 */

export const UNKNOWN_CATEGORY = 'Unknown category';
export const UNKNOWN_VEHICLE = 'Unknown vehicle';

/**
 * `||` not `??`: a category whose name marshalled as "" must fall through to
 * the placeholder rather than render a blank cell.
 */
export function categoryLabel(names: Map<string, string>, id: string): string {
  return names.get(id) || UNKNOWN_CATEGORY;
}

export function vehicleLabel(titles: Map<string, string>, id: string): string {
  return titles.get(id) || UNKNOWN_VEHICLE;
}

/**
 * categoryId -> name, for every kind.
 *
 * Deliberately takes no parameter: passing a `kind` to useMaintenanceCategories
 * would silently drop the other kind's categories and reintroduce the UUID for
 * them (FR-LABEL-3). The no-argument call shares its query key with
 * VehicleDetailPage's category query, so the two dedupe.
 */
export function useCategoryNameMap(): { names: Map<string, string>; isLoading: boolean } {
  const { data, isLoading } = useMaintenanceCategories();
  const names = useMemo(
    () => new Map((data ?? []).map((category) => [category.id, category.attributes.name])),
    [data],
  );
  return { names, isLoading };
}

/**
 * vehicleId -> display title.
 *
 * Note `data.data`: useVehicles has no `select`, unlike the maintenance hooks,
 * so the query data is the whole list envelope.
 */
export function useVehicleTitleMap(fleetId: string | null | undefined): {
  titles: Map<string, string>;
  isLoading: boolean;
} {
  const { data, isLoading } = useVehicles(fleetId);
  const titles = useMemo(
    () => new Map((data?.data ?? []).map((vehicle) => [vehicle.id, vehicleTitle(vehicle.attributes)])),
    [data],
  );
  return { titles, isLoading };
}
```

- [ ] **Step 4: Run the test and verify it passes**

```sh
npm run -w apps/web test -- src/lib/hooks/api/labels.test.ts
```

Expected: PASS, 12 tests.

- [ ] **Step 5: Commit**

```sh
git add apps/web/src/lib/hooks/api/labels.ts apps/web/src/lib/hooks/api/labels.test.ts
git commit -m "feat(web): add shared category and vehicle label maps"
```

---

## Task 3: `MaintenanceQueueView` renders category names

**Files:**
- Modify: `apps/web/src/components/features/vehicles/maintenance/MaintenanceQueueView.tsx:51` (overdue) and `:88` (upcoming), plus the loading gate at `:21`
- Test: `apps/web/src/components/features/vehicles/maintenance/MaintenanceQueueView.test.tsx` (new)

**Interfaces:**
- Consumes: `useCategoryNameMap`, `categoryLabel` (Task 2).
- Produces: nothing consumed downstream.

**Note:** nothing in the repo imports `MaintenanceQueueView` — a grep returns
only its own definition. The PRD names its two lines explicitly, so this task
fixes it in place rather than deleting it; it is also the reference layout the
two widgets are being aligned to. Whether to mount or delete it is recorded as a
follow-up in the design, not decided here.

- [ ] **Step 1: Write the failing test**

Create `apps/web/src/components/features/vehicles/maintenance/MaintenanceQueueView.test.tsx`:

```tsx
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen } from '@testing-library/react';
import { MaintenanceQueueView } from './MaintenanceQueueView';
import type { MaintenanceSchedule } from '../../../../types/models/maintenanceSchedule';

const mockUseUpcoming = vi.fn();
const mockUseOverdue = vi.fn();
const mockUseCategoryNameMap = vi.fn();

vi.mock('../../../../lib/hooks/api/maintenance', () => ({
  useUpcomingMaintenanceQueue: (fleetId: string) => mockUseUpcoming(fleetId),
  useOverdueMaintenanceQueue: (fleetId: string) => mockUseOverdue(fleetId),
}));

vi.mock('../../../../lib/hooks/api/labels', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../../../../lib/hooks/api/labels')>();
  return { ...actual, useCategoryNameMap: () => mockUseCategoryNameMap() };
});

const CATEGORY_ID = 'a3f2c1e0-0000-4000-8000-000000000001';

function schedule(overrides: Partial<MaintenanceSchedule['attributes']> = {}): MaintenanceSchedule {
  return {
    id: 's1',
    type: 'maintenanceSchedules',
    attributes: {
      vehicleId: 'v1',
      categoryId: CATEGORY_ID,
      recurrenceType: 'time',
      status: 'overdue',
      severity: 'urgent',
      active: true,
      ...overrides,
    },
  };
}

/** Every query settled, both queues empty, names resolvable. */
function settled(names: Map<string, string> = new Map([[CATEGORY_ID, 'Oil Change']])) {
  mockUseUpcoming.mockReturnValue({ data: [], isLoading: false });
  mockUseOverdue.mockReturnValue({ data: [], isLoading: false });
  mockUseCategoryNameMap.mockReturnValue({ names, isLoading: false });
}

describe('MaintenanceQueueView', () => {
  beforeEach(() => {
    mockUseUpcoming.mockReset();
    mockUseOverdue.mockReset();
    mockUseCategoryNameMap.mockReset();
  });

  it('renders the category name, not the id, in the overdue card', () => {
    settled();
    mockUseOverdue.mockReturnValue({ data: [schedule()], isLoading: false });

    render(<MaintenanceQueueView fleetId="f1" />);

    expect(screen.getByText('Oil Change')).toBeInTheDocument();
    expect(screen.queryByText(CATEGORY_ID)).not.toBeInTheDocument();
  });

  it('renders the category name, not the id, in the upcoming card', () => {
    settled();
    mockUseUpcoming.mockReturnValue({
      data: [schedule({ status: 'upcoming', severity: 'recommended' })],
      isLoading: false,
    });

    render(<MaintenanceQueueView fleetId="f1" />);

    expect(screen.getByText('Oil Change')).toBeInTheDocument();
    expect(screen.queryByText(CATEGORY_ID)).not.toBeInTheDocument();
  });

  // FR-LABEL-3: the hook asks for no kind filter, so a modification-kind
  // category resolves just like a maintenance one.
  it('resolves a modification-kind category', () => {
    settled(new Map([[CATEGORY_ID, 'Cold Air Intake']]));
    mockUseOverdue.mockReturnValue({ data: [schedule()], isLoading: false });

    render(<MaintenanceQueueView fleetId="f1" />);

    expect(screen.getByText('Cold Air Intake')).toBeInTheDocument();
  });

  // FR-LOAD-3: the category query failed (settled, no data) but the queue
  // succeeded. Rows still render, with the placeholder.
  it('falls back to Unknown category when the id does not resolve', () => {
    settled(new Map());
    mockUseOverdue.mockReturnValue({ data: [schedule()], isLoading: false });

    render(<MaintenanceQueueView fleetId="f1" />);

    expect(screen.getByText('Unknown category')).toBeInTheDocument();
    expect(screen.queryByText(CATEGORY_ID)).not.toBeInTheDocument();
  });

  // FR-LOAD-1: the queue has landed but the names have not. The skeleton holds;
  // no frame shows a UUID.
  it('holds the skeleton while the category query is in flight', () => {
    settled();
    mockUseOverdue.mockReturnValue({ data: [schedule()], isLoading: false });
    mockUseCategoryNameMap.mockReturnValue({ names: new Map(), isLoading: true });

    const { container } = render(<MaintenanceQueueView fleetId="f1" />);

    expect(container.querySelector('.animate-pulse')).toBeInTheDocument();
    expect(screen.queryByText(CATEGORY_ID)).not.toBeInTheDocument();
  });

  it('preserves the empty-state copy for both cards', () => {
    settled();

    render(<MaintenanceQueueView fleetId="f1" />);

    expect(screen.getByText('No overdue maintenance items.')).toBeInTheDocument();
    expect(screen.getByText('No upcoming maintenance items.')).toBeInTheDocument();
  });
});
```

- [ ] **Step 2: Run the test and verify it fails**

```sh
npm run -w apps/web test -- src/components/features/vehicles/maintenance/MaintenanceQueueView.test.tsx
```

Expected: FAIL — the rows render the UUID, so `getByText('Oil Change')` finds
nothing.

- [ ] **Step 3: Wire the hook and widen the gate**

In `MaintenanceQueueView.tsx`, add the import:

```tsx
import { categoryLabel, useCategoryNameMap } from '../../../../lib/hooks/api/labels';
```

Then replace the hook block and gate:

```tsx
  const { data: upcoming, isLoading: upcomingLoading } = useUpcomingMaintenanceQueue(fleetId);
  const { data: overdue, isLoading: overdueLoading } = useOverdueMaintenanceQueue(fleetId);
  const { names, isLoading: categoriesLoading } = useCategoryNameMap();

  // Every query this view reads, ORed: React Query v5's isLoading is
  // `isPending && isFetching`, so a genuinely in-flight category query holds the
  // skeleton (no frame can show a UUID), while a *failed* one settles to false
  // and lets rows through with the Unknown category fallback.
  const isLoading = upcomingLoading || overdueLoading || categoriesLoading;
```

- [ ] **Step 4: Replace both render sites**

In the overdue card, replace:

```tsx
                      <span className="text-sm font-medium">{item.attributes.categoryId}</span>
```

with:

```tsx
                      <span className="text-sm font-medium">
                        {categoryLabel(names, item.attributes.categoryId)}
                      </span>
```

Apply the identical replacement in the upcoming card. Both sites use the same
expression; nothing else in either card changes.

- [ ] **Step 5: Run the test and verify it passes**

```sh
npm run -w apps/web test -- src/components/features/vehicles/maintenance/MaintenanceQueueView.test.tsx
```

Expected: PASS, 6 tests.

- [ ] **Step 6: Commit**

```sh
git add apps/web/src/components/features/vehicles/maintenance/MaintenanceQueueView.tsx \
        apps/web/src/components/features/vehicles/maintenance/MaintenanceQueueView.test.tsx
git commit -m "fix(web): show category names in the maintenance queue view"
```

---

## Task 4: `OverdueMaintenanceWidget` — name, vehicle, due context

**Files:**
- Modify: `apps/web/src/components/features/dashboard/widgets/OverdueMaintenanceWidget.tsx`
- Test: `apps/web/src/components/features/dashboard/widgets/OverdueMaintenanceWidget.test.tsx` (new)

**Interfaces:**
- Consumes: `useCategoryNameMap`, `useVehicleTitleMap`, `categoryLabel`,
  `vehicleLabel` (Task 2).
- Produces: nothing consumed downstream. Task 5 mirrors this task with
  upcoming-flavoured wording.

**Row layout.** The row now carries four facts, so the single-line
`flex items-center justify-between` becomes a fixed right rail (the severity
chip) beside a stacked, truncating left column — `MaintenanceQueueView`'s
treatment. `min-w-0` on the left column is load-bearing: a flex child defaults
to `min-width: auto`, so without it `truncate` does nothing and a long category
name pushes the chip out of the card. `items-start` (not `items-center`) keeps
the chip aligned to the title now that rows have variable height. No new colour
is introduced — the two added lines are plain `text-muted-foreground`, the chip
still keys off `severity`, and the "Overdue Maintenance" heading still carries
the state.

- [ ] **Step 1: Write the failing test**

Create `apps/web/src/components/features/dashboard/widgets/OverdueMaintenanceWidget.test.tsx`:

```tsx
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen } from '@testing-library/react';
import { OverdueMaintenanceWidget } from './OverdueMaintenanceWidget';
import type { MaintenanceSchedule } from '../../../../types/models/maintenanceSchedule';

const mockUseOverdue = vi.fn();
const mockUseCategoryNameMap = vi.fn();
const mockUseVehicleTitleMap = vi.fn();

vi.mock('../../../../lib/hooks/api/maintenance', () => ({
  useOverdueMaintenanceQueue: (fleetId: string) => mockUseOverdue(fleetId),
}));

vi.mock('../../../../lib/hooks/api/labels', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../../../../lib/hooks/api/labels')>();
  return {
    ...actual,
    useCategoryNameMap: () => mockUseCategoryNameMap(),
    useVehicleTitleMap: (fleetId: string | null | undefined) => mockUseVehicleTitleMap(fleetId),
  };
});

const CATEGORY_ID = 'a3f2c1e0-0000-4000-8000-000000000001';
const VEHICLE_ID = 'b4e3d2f1-0000-4000-8000-000000000002';

function schedule(
  overrides: Partial<MaintenanceSchedule['attributes']> = {},
  id = 's1',
): MaintenanceSchedule {
  return {
    id,
    type: 'maintenanceSchedules',
    attributes: {
      vehicleId: VEHICLE_ID,
      categoryId: CATEGORY_ID,
      recurrenceType: 'time',
      status: 'overdue',
      severity: 'urgent',
      active: true,
      ...overrides,
    },
  };
}

function settled(items: MaintenanceSchedule[]) {
  mockUseOverdue.mockReturnValue({ data: items, isLoading: false });
  mockUseCategoryNameMap.mockReturnValue({
    names: new Map([[CATEGORY_ID, 'Oil Change']]),
    isLoading: false,
  });
  mockUseVehicleTitleMap.mockReturnValue({
    titles: new Map([[VEHICLE_ID, 'Weekend Truck']]),
    isLoading: false,
  });
}

describe('OverdueMaintenanceWidget', () => {
  beforeEach(() => {
    mockUseOverdue.mockReset();
    mockUseCategoryNameMap.mockReset();
    mockUseVehicleTitleMap.mockReset();
  });

  it('renders the category name and the owning vehicle, not ids', () => {
    settled([schedule()]);

    render(<OverdueMaintenanceWidget fleetId="f1" />);

    expect(screen.getByText('Oil Change')).toBeInTheDocument();
    expect(screen.getByText('Weekend Truck')).toBeInTheDocument();
    expect(screen.queryByText(CATEGORY_ID)).not.toBeInTheDocument();
    expect(screen.queryByText(VEHICLE_ID)).not.toBeInTheDocument();
  });

  it('falls back to the placeholders when neither id resolves', () => {
    settled([schedule()]);
    mockUseCategoryNameMap.mockReturnValue({ names: new Map(), isLoading: false });
    mockUseVehicleTitleMap.mockReturnValue({ titles: new Map(), isLoading: false });

    render(<OverdueMaintenanceWidget fleetId="f1" />);

    expect(screen.getByText('Unknown category')).toBeInTheDocument();
    expect(screen.getByText('Unknown vehicle')).toBeInTheDocument();
  });

  // FR-DUE-2: past tense here; the upcoming widget says "Due".
  it('renders the due date in the past tense', () => {
    const nextDueDate = '2026-03-14T00:00:00Z';
    settled([schedule({ nextDueDate })]);

    render(<OverdueMaintenanceWidget fleetId="f1" />);

    const expected = `Was due ${new Date(nextDueDate).toLocaleDateString()}`;
    expect(screen.getByText(expected)).toBeInTheDocument();
  });

  it('renders the due mileage with thousands separators', () => {
    settled([schedule({ recurrenceType: 'mileage', nextDueMileage: 75000 })]);

    render(<OverdueMaintenanceWidget fleetId="f1" />);

    expect(screen.getByText(`At ${(75000).toLocaleString()} miles`)).toBeInTheDocument();
  });

  // FR-DUE-3/4: a pure-time schedule has no mileage line, a pure-mileage
  // schedule no date line, and a zero mileage renders nothing — not a literal 0.
  it('omits the line each schedule kind has no value for', () => {
    settled([schedule({ nextDueDate: '2026-03-14T00:00:00Z', nextDueMileage: 0 })]);

    render(<OverdueMaintenanceWidget fleetId="f1" />);

    expect(screen.queryByText(/miles/)).not.toBeInTheDocument();
    expect(screen.queryByText('0')).not.toBeInTheDocument();
  });

  it('omits the date line for a pure-mileage schedule', () => {
    settled([schedule({ recurrenceType: 'mileage', nextDueMileage: 75000 })]);

    render(<OverdueMaintenanceWidget fleetId="f1" />);

    expect(screen.queryByText(/Was due/)).not.toBeInTheDocument();
  });

  it('renders both lines for a hybrid schedule', () => {
    const nextDueDate = '2026-03-14T00:00:00Z';
    settled([schedule({ recurrenceType: 'hybrid', nextDueDate, nextDueMileage: 75000 })]);

    render(<OverdueMaintenanceWidget fleetId="f1" />);

    expect(
      screen.getByText(`Was due ${new Date(nextDueDate).toLocaleDateString()}`),
    ).toBeInTheDocument();
    expect(screen.getByText(`At ${(75000).toLocaleString()} miles`)).toBeInTheDocument();
  });

  // FR-LOAD-1: the queue landed first; the skeleton holds until the supporting
  // queries do too, so no frame shows a UUID.
  it('holds the skeleton while the category query is in flight', () => {
    settled([schedule()]);
    mockUseCategoryNameMap.mockReturnValue({ names: new Map(), isLoading: true });

    const { container } = render(<OverdueMaintenanceWidget fleetId="f1" />);

    expect(container.querySelector('.animate-pulse')).toBeInTheDocument();
    expect(screen.queryByText(CATEGORY_ID)).not.toBeInTheDocument();
  });

  it('holds the skeleton while the vehicle query is in flight', () => {
    settled([schedule()]);
    mockUseVehicleTitleMap.mockReturnValue({ titles: new Map(), isLoading: true });

    const { container } = render(<OverdueMaintenanceWidget fleetId="f1" />);

    expect(container.querySelector('.animate-pulse')).toBeInTheDocument();
    expect(screen.queryByText(VEHICLE_ID)).not.toBeInTheDocument();
  });

  it('preserves the empty-state copy', () => {
    settled([]);

    render(<OverdueMaintenanceWidget fleetId="f1" />);

    expect(screen.getByText('No overdue maintenance.')).toBeInTheDocument();
  });

  // FR-LOAD-4: the cap is unchanged.
  it('caps the list at five rows', () => {
    settled(Array.from({ length: 7 }, (_, i) => schedule({}, `s${i}`)));

    render(<OverdueMaintenanceWidget fleetId="f1" />);

    expect(screen.getAllByText('Oil Change')).toHaveLength(5);
  });
});
```

- [ ] **Step 2: Run the test and verify it fails**

```sh
npm run -w apps/web test -- src/components/features/dashboard/widgets/OverdueMaintenanceWidget.test.tsx
```

Expected: FAIL — the widget renders the UUID and has no vehicle or due lines.

- [ ] **Step 3: Rewrite the widget**

Replace the whole of
`apps/web/src/components/features/dashboard/widgets/OverdueMaintenanceWidget.tsx`
with:

```tsx
import { Skeleton } from '../../../ui/skeleton';
import { Card, CardContent } from '../../../ui/card';
import { SeverityChip } from '../../vehicles/maintenance/SeverityChip';
import { useOverdueMaintenanceQueue } from '../../../../lib/hooks/api/maintenance';
import {
  categoryLabel,
  useCategoryNameMap,
  useVehicleTitleMap,
  vehicleLabel,
} from '../../../../lib/hooks/api/labels';

interface OverdueMaintenanceWidgetProps {
  fleetId: string;
}

export function OverdueMaintenanceWidget({ fleetId }: OverdueMaintenanceWidgetProps) {
  const { data: items, isLoading: queueLoading } = useOverdueMaintenanceQueue(fleetId);
  const { names, isLoading: categoriesLoading } = useCategoryNameMap();
  const { titles, isLoading: vehiclesLoading } = useVehicleTitleMap(fleetId);

  // Every query the rows read, ORed. React Query v5's isLoading is
  // `isPending && isFetching`: an in-flight supporting query holds the skeleton
  // so no frame can show a UUID (FR-LOAD-1), while a failed one settles to false
  // and lets rows through with the Unknown fallbacks (FR-LOAD-3). A disabled
  // query reports isLoading false, so a null fleetId never wedges the skeleton.
  if (queueLoading || categoriesLoading || vehiclesLoading) {
    return (
      <Card>
        <CardContent className="pt-4 space-y-2">
          <Skeleton className="h-5 w-40" />
          <Skeleton className="h-12 w-full" />
          <Skeleton className="h-12 w-full" />
        </CardContent>
      </Card>
    );
  }

  return (
    <Card>
      <CardContent className="pt-4">
        <h3 className="text-sm font-semibold mb-3 text-danger">Overdue Maintenance</h3>
        {!items || items.length === 0 ? (
          <p className="text-sm text-muted-foreground">No overdue maintenance.</p>
        ) : (
          <ul className="space-y-2">
            {items.slice(0, 5).map((item) => (
              // min-w-0 is load-bearing: a flex child defaults to
              // min-width:auto, so without it `truncate` does nothing and a long
              // category name pushes the chip out of the card.
              <li
                key={item.id}
                className="flex items-start justify-between gap-2 text-sm border-b pb-2 last:border-0"
              >
                <div className="min-w-0 flex-1 space-y-0.5">
                  <p className="truncate font-medium">
                    {categoryLabel(names, item.attributes.categoryId)}
                  </p>
                  <p className="truncate text-xs text-muted-foreground">
                    {vehicleLabel(titles, item.attributes.vehicleId)}
                  </p>
                  {item.attributes.nextDueDate && (
                    <p className="text-xs text-muted-foreground">
                      Was due {new Date(item.attributes.nextDueDate).toLocaleDateString()}
                    </p>
                  )}
                  {/* `? :` not `&&`: nextDueMileage is a Go int with omitempty,
                      and `&&` on a number renders a literal 0 into the row. */}
                  {item.attributes.nextDueMileage ? (
                    <p className="text-xs text-muted-foreground">
                      At {item.attributes.nextDueMileage.toLocaleString()} miles
                    </p>
                  ) : null}
                </div>
                <SeverityChip severity={item.attributes.severity} className="shrink-0" />
              </li>
            ))}
          </ul>
        )}
      </CardContent>
    </Card>
  );
}
```

- [ ] **Step 4: Run the test and verify it passes**

```sh
npm run -w apps/web test -- src/components/features/dashboard/widgets/OverdueMaintenanceWidget.test.tsx
```

Expected: PASS, 11 tests.

- [ ] **Step 5: Commit**

```sh
git add apps/web/src/components/features/dashboard/widgets/OverdueMaintenanceWidget.tsx \
        apps/web/src/components/features/dashboard/widgets/OverdueMaintenanceWidget.test.tsx
git commit -m "fix(web): label overdue maintenance rows with category, vehicle and due context"
```

---

## Task 5: `UpcomingMaintenanceWidget` — name, vehicle, due context

**Files:**
- Modify: `apps/web/src/components/features/dashboard/widgets/UpcomingMaintenanceWidget.tsx`
- Test: `apps/web/src/components/features/dashboard/widgets/UpcomingMaintenanceWidget.test.tsx` (new)

**Interfaces:**
- Consumes: `useCategoryNameMap`, `useVehicleTitleMap`, `categoryLabel`,
  `vehicleLabel` (Task 2).
- Produces: nothing consumed downstream.

**Differences from Task 4, and only these:** the queue hook is
`useUpcomingMaintenanceQueue`; the heading is `Upcoming Maintenance` with
`text-warning`; the empty state is `No upcoming maintenance.`; and the date line
reads `Due <date>`, not `Was due <date>` (FR-DUE-2). Everything else — row
structure, class names, guards, gate — is identical. The code is repeated in
full below rather than referenced, because the two widgets are read
independently.

- [ ] **Step 1: Write the failing test**

Create `apps/web/src/components/features/dashboard/widgets/UpcomingMaintenanceWidget.test.tsx`:

```tsx
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen } from '@testing-library/react';
import { UpcomingMaintenanceWidget } from './UpcomingMaintenanceWidget';
import type { MaintenanceSchedule } from '../../../../types/models/maintenanceSchedule';

const mockUseUpcoming = vi.fn();
const mockUseCategoryNameMap = vi.fn();
const mockUseVehicleTitleMap = vi.fn();

vi.mock('../../../../lib/hooks/api/maintenance', () => ({
  useUpcomingMaintenanceQueue: (fleetId: string) => mockUseUpcoming(fleetId),
}));

vi.mock('../../../../lib/hooks/api/labels', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../../../../lib/hooks/api/labels')>();
  return {
    ...actual,
    useCategoryNameMap: () => mockUseCategoryNameMap(),
    useVehicleTitleMap: (fleetId: string | null | undefined) => mockUseVehicleTitleMap(fleetId),
  };
});

const CATEGORY_ID = 'a3f2c1e0-0000-4000-8000-000000000001';
const VEHICLE_ID = 'b4e3d2f1-0000-4000-8000-000000000002';

function schedule(
  overrides: Partial<MaintenanceSchedule['attributes']> = {},
  id = 's1',
): MaintenanceSchedule {
  return {
    id,
    type: 'maintenanceSchedules',
    attributes: {
      vehicleId: VEHICLE_ID,
      categoryId: CATEGORY_ID,
      recurrenceType: 'time',
      status: 'upcoming',
      severity: 'recommended',
      active: true,
      ...overrides,
    },
  };
}

function settled(items: MaintenanceSchedule[]) {
  mockUseUpcoming.mockReturnValue({ data: items, isLoading: false });
  mockUseCategoryNameMap.mockReturnValue({
    names: new Map([[CATEGORY_ID, 'Cold Air Intake']]),
    isLoading: false,
  });
  mockUseVehicleTitleMap.mockReturnValue({
    titles: new Map([[VEHICLE_ID, '2021 Honda Civic']]),
    isLoading: false,
  });
}

describe('UpcomingMaintenanceWidget', () => {
  beforeEach(() => {
    mockUseUpcoming.mockReset();
    mockUseCategoryNameMap.mockReset();
    mockUseVehicleTitleMap.mockReset();
  });

  // The fixture name is a modification-kind category on purpose (FR-LABEL-3):
  // the map comes from a no-kind-filter query, so both kinds resolve. The
  // vehicle has no nickname, exercising the year/make/model branch.
  it('renders the category name and the owning vehicle, not ids', () => {
    settled([schedule()]);

    render(<UpcomingMaintenanceWidget fleetId="f1" />);

    expect(screen.getByText('Cold Air Intake')).toBeInTheDocument();
    expect(screen.getByText('2021 Honda Civic')).toBeInTheDocument();
    expect(screen.queryByText(CATEGORY_ID)).not.toBeInTheDocument();
    expect(screen.queryByText(VEHICLE_ID)).not.toBeInTheDocument();
  });

  it('falls back to the placeholders when neither id resolves', () => {
    settled([schedule()]);
    mockUseCategoryNameMap.mockReturnValue({ names: new Map(), isLoading: false });
    mockUseVehicleTitleMap.mockReturnValue({ titles: new Map(), isLoading: false });

    render(<UpcomingMaintenanceWidget fleetId="f1" />);

    expect(screen.getByText('Unknown category')).toBeInTheDocument();
    expect(screen.getByText('Unknown vehicle')).toBeInTheDocument();
  });

  // FR-DUE-2: present tense here; the overdue widget says "Was due".
  it('renders the due date in the present tense', () => {
    const nextDueDate = '2026-09-14T00:00:00Z';
    settled([schedule({ nextDueDate })]);

    render(<UpcomingMaintenanceWidget fleetId="f1" />);

    expect(screen.getByText(`Due ${new Date(nextDueDate).toLocaleDateString()}`)).toBeInTheDocument();
    expect(screen.queryByText(/Was due/)).not.toBeInTheDocument();
  });

  it('renders the due mileage with thousands separators', () => {
    settled([schedule({ recurrenceType: 'mileage', nextDueMileage: 75000 })]);

    render(<UpcomingMaintenanceWidget fleetId="f1" />);

    expect(screen.getByText(`At ${(75000).toLocaleString()} miles`)).toBeInTheDocument();
  });

  it('omits the mileage line for a pure-time schedule and never renders a bare 0', () => {
    settled([schedule({ nextDueDate: '2026-09-14T00:00:00Z', nextDueMileage: 0 })]);

    render(<UpcomingMaintenanceWidget fleetId="f1" />);

    expect(screen.queryByText(/miles/)).not.toBeInTheDocument();
    expect(screen.queryByText('0')).not.toBeInTheDocument();
  });

  it('omits the date line for a pure-mileage schedule', () => {
    settled([schedule({ recurrenceType: 'mileage', nextDueMileage: 75000 })]);

    render(<UpcomingMaintenanceWidget fleetId="f1" />);

    expect(screen.queryByText(/^Due /)).not.toBeInTheDocument();
  });

  it('holds the skeleton while the category query is in flight', () => {
    settled([schedule()]);
    mockUseCategoryNameMap.mockReturnValue({ names: new Map(), isLoading: true });

    const { container } = render(<UpcomingMaintenanceWidget fleetId="f1" />);

    expect(container.querySelector('.animate-pulse')).toBeInTheDocument();
    expect(screen.queryByText(CATEGORY_ID)).not.toBeInTheDocument();
  });

  it('holds the skeleton while the vehicle query is in flight', () => {
    settled([schedule()]);
    mockUseVehicleTitleMap.mockReturnValue({ titles: new Map(), isLoading: true });

    const { container } = render(<UpcomingMaintenanceWidget fleetId="f1" />);

    expect(container.querySelector('.animate-pulse')).toBeInTheDocument();
    expect(screen.queryByText(VEHICLE_ID)).not.toBeInTheDocument();
  });

  it('preserves the empty-state copy', () => {
    settled([]);

    render(<UpcomingMaintenanceWidget fleetId="f1" />);

    expect(screen.getByText('No upcoming maintenance.')).toBeInTheDocument();
  });

  it('caps the list at five rows', () => {
    settled(Array.from({ length: 7 }, (_, i) => schedule({}, `s${i}`)));

    render(<UpcomingMaintenanceWidget fleetId="f1" />);

    expect(screen.getAllByText('Cold Air Intake')).toHaveLength(5);
  });
});
```

- [ ] **Step 2: Run the test and verify it fails**

```sh
npm run -w apps/web test -- src/components/features/dashboard/widgets/UpcomingMaintenanceWidget.test.tsx
```

Expected: FAIL — the widget renders the UUID and has no vehicle or due lines.

- [ ] **Step 3: Rewrite the widget**

Replace the whole of
`apps/web/src/components/features/dashboard/widgets/UpcomingMaintenanceWidget.tsx`
with:

```tsx
import { Skeleton } from '../../../ui/skeleton';
import { Card, CardContent } from '../../../ui/card';
import { SeverityChip } from '../../vehicles/maintenance/SeverityChip';
import { useUpcomingMaintenanceQueue } from '../../../../lib/hooks/api/maintenance';
import {
  categoryLabel,
  useCategoryNameMap,
  useVehicleTitleMap,
  vehicleLabel,
} from '../../../../lib/hooks/api/labels';

interface UpcomingMaintenanceWidgetProps {
  fleetId: string;
}

export function UpcomingMaintenanceWidget({ fleetId }: UpcomingMaintenanceWidgetProps) {
  const { data: items, isLoading: queueLoading } = useUpcomingMaintenanceQueue(fleetId);
  const { names, isLoading: categoriesLoading } = useCategoryNameMap();
  const { titles, isLoading: vehiclesLoading } = useVehicleTitleMap(fleetId);

  // Every query the rows read, ORed. React Query v5's isLoading is
  // `isPending && isFetching`: an in-flight supporting query holds the skeleton
  // so no frame can show a UUID (FR-LOAD-1), while a failed one settles to false
  // and lets rows through with the Unknown fallbacks (FR-LOAD-3). A disabled
  // query reports isLoading false, so a null fleetId never wedges the skeleton.
  if (queueLoading || categoriesLoading || vehiclesLoading) {
    return (
      <Card>
        <CardContent className="pt-4 space-y-2">
          <Skeleton className="h-5 w-40" />
          <Skeleton className="h-12 w-full" />
          <Skeleton className="h-12 w-full" />
        </CardContent>
      </Card>
    );
  }

  return (
    <Card>
      <CardContent className="pt-4">
        <h3 className="text-sm font-semibold mb-3 text-warning">Upcoming Maintenance</h3>
        {!items || items.length === 0 ? (
          <p className="text-sm text-muted-foreground">No upcoming maintenance.</p>
        ) : (
          <ul className="space-y-2">
            {items.slice(0, 5).map((item) => (
              // min-w-0 is load-bearing: a flex child defaults to
              // min-width:auto, so without it `truncate` does nothing and a long
              // category name pushes the chip out of the card.
              <li
                key={item.id}
                className="flex items-start justify-between gap-2 text-sm border-b pb-2 last:border-0"
              >
                <div className="min-w-0 flex-1 space-y-0.5">
                  <p className="truncate font-medium">
                    {categoryLabel(names, item.attributes.categoryId)}
                  </p>
                  <p className="truncate text-xs text-muted-foreground">
                    {vehicleLabel(titles, item.attributes.vehicleId)}
                  </p>
                  {item.attributes.nextDueDate && (
                    <p className="text-xs text-muted-foreground">
                      Due {new Date(item.attributes.nextDueDate).toLocaleDateString()}
                    </p>
                  )}
                  {/* `? :` not `&&`: nextDueMileage is a Go int with omitempty,
                      and `&&` on a number renders a literal 0 into the row. */}
                  {item.attributes.nextDueMileage ? (
                    <p className="text-xs text-muted-foreground">
                      At {item.attributes.nextDueMileage.toLocaleString()} miles
                    </p>
                  ) : null}
                </div>
                <SeverityChip severity={item.attributes.severity} className="shrink-0" />
              </li>
            ))}
          </ul>
        )}
      </CardContent>
    </Card>
  );
}
```

- [ ] **Step 4: Run the test and verify it passes**

```sh
npm run -w apps/web test -- src/components/features/dashboard/widgets/UpcomingMaintenanceWidget.test.tsx
```

Expected: PASS, 10 tests.

- [ ] **Step 5: Commit**

```sh
git add apps/web/src/components/features/dashboard/widgets/UpcomingMaintenanceWidget.tsx \
        apps/web/src/components/features/dashboard/widgets/UpcomingMaintenanceWidget.test.tsx
git commit -m "fix(web): label upcoming maintenance rows with category, vehicle and due context"
```

---

## Task 6: Whole-repo verification

**Files:** none modified unless a check fails.

**Interfaces:** none.

- [ ] **Step 1: Prove no raw `categoryId` reaches the DOM**

```sh
grep -rn "attributes\.categoryId" apps/web/src --include="*.tsx"
```

Expected: every remaining hit is an *argument* (`categoryLabel(names,
item.attributes.categoryId)`, `categoryNames.get(schedule.attributes.categoryId)`
in `UpcomingScheduleStrip`, form/dialog values) — none is interpolated directly
as display text. If a hit renders as text, it is a missed site: fix it the way
Task 3 fixed its two.

- [ ] **Step 2: Confirm the out-of-scope files are untouched**

```sh
git diff --name-only main...HEAD
```

Expected: the list contains only the eleven files named in the File Structure
table. It must **not** contain `UpcomingScheduleStrip.tsx`,
`VehicleNameCrumb.tsx`, `VehicleNameCrumb.test.tsx`, `VehicleDetailPage.tsx`,
`VehicleIdentityRail.tsx`, `ActivityPage.tsx`, `VehicleStatusWidget.tsx`, or
`VehicleCard.test.tsx`.

- [ ] **Step 3: Run the frontend test suite**

```sh
npm run -w apps/web test
```

Expected: PASS, no unhandled rejections.

- [ ] **Step 4: Run the full gate**

```sh
make ci
```

Expected: `lint-check`, `vet`, `test`, `build`, `fe-test`, `fe-build`,
`manifests`, and `carfax-template` all pass. No `kustomize build` beyond what
`manifests` runs — this task changes no manifest.

- [ ] **Step 5: Commit any lint fixes**

If `make lint` was needed to satisfy `lint-check`:

```sh
git add -A
git commit -m "chore(web): formatting"
```

Otherwise skip this step.

- [ ] **Step 6: Code review before the PR**

Invoke `superpowers:requesting-code-review`. Only frontend files changed, so
`frontend-guidelines-reviewer` is the required reviewer; `plan-adherence-reviewer`
verifies this plan. Findings go to
`docs/tasks/task-026-maintenance-widget-labels/audit.md`. Address findings before
opening the PR.

---

## Deferred (not in this task)

Recorded so a reviewer does not read them as omissions:

- **`MaintenanceQueueView` is dead code** — nothing imports it. Fixed in place
  because the PRD names its two lines, but whether to mount or delete it is an
  open follow-up.
- **Five holdout copies of the vehicle-title rule** — `VehicleNameCrumb`,
  `VehicleDetailPage`, `VehicleIdentityRail`, `ActivityPage`, and
  `VehicleStatusWidget`. The last is subtly wrong today (no `.trim()`, so a
  whitespace-only nickname renders as blank space). Migrating them changes
  rendered output and edits a page task-015 owns; that is a separate task, which
  should also convert `VehicleNameCrumb.test.tsx`'s source pin into an
  "every site calls `vehicleTitle()`" assertion.
- **No link from a dashboard row to its vehicle** — plain text for v1, per the
  design. Revisit if dashboard rows become navigable as a whole.
