/**
 * Task 8 — useVehicleRecords composes the three infinite-query source hooks
 * (Task 6) with the pure merge function (Task 7). This is the untested seam
 * between two well-tested pieces: the source hooks are mocked here so the
 * tests focus on the adapter logic — field mapping (especially kind
 * resolution and gallons), the categories-query gating (Important 1 from
 * review), the loadMore/hasMore/isFetchingNextPage wiring, and the
 * memoized-identity contract the whole thing exists to provide.
 */
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { renderHook } from '@testing-library/react';
import { useVehicleRecords, type CategoriesQueryState } from './vehicleRecords';
import { useMaintenanceRecords } from './maintenance';
import { useFuelLogs } from './fuel';
import { useMileageRecords } from './mileage';
import type { MaintenanceRecord } from '../../../types/models/maintenanceRecord';
import type { FuelLog } from '../../../types/models/fuelLog';
import type { MileageRecord } from '../../../types/models/mileage';
import type { MaintenanceCategory } from '../../../types/models/maintenanceCategory';
import { expectNoCall } from '../../../test/expectNoCall';

vi.mock('./maintenance', () => ({ useMaintenanceRecords: vi.fn() }));
vi.mock('./fuel', () => ({ useFuelLogs: vi.fn() }));
vi.mock('./mileage', () => ({ useMileageRecords: vi.fn() }));

const mockedUseMaintenanceRecords = vi.mocked(useMaintenanceRecords);
const mockedUseFuelLogs = vi.mocked(useFuelLogs);
const mockedUseMileageRecords = vi.mocked(useMileageRecords);

// ---------------------------------------------------------------------------
// Fixture builders
// ---------------------------------------------------------------------------

// A sentinel deliberately far from any fixture's real date field
// (performedAt/date/recordedAt). If the adapter ever read `createdAt`
// instead of the real date field, every row would sort to "newest" and the
// newest-first / watermark assertions below would fail loudly instead of
// silently passing because createdAt happened to equal the real field.
const IRRELEVANT_CREATED_AT = '2099-12-31T00:00:00Z';

function maintenanceRecord(
  id: string,
  categoryId: string,
  performedAt: string,
  overrides: Partial<MaintenanceRecord['attributes']> = {},
): MaintenanceRecord {
  return {
    id,
    type: 'maintenanceRecords',
    attributes: {
      vehicleId: 'v1',
      categoryId,
      performedAt,
      mileage: 1000,
      cost: 50,
      createdAt: IRRELEVANT_CREATED_AT,
      ...overrides,
    },
  };
}

function fuelLog(id: string, date: string, gallons: number): FuelLog {
  return {
    id,
    type: 'fuelLogs',
    attributes: {
      vehicleId: 'v1',
      date,
      mileage: 2000,
      gallons,
      totalCost: gallons * 3.5,
      pricePerGallon: 3.5,
      createdAt: IRRELEVANT_CREATED_AT,
      updatedAt: IRRELEVANT_CREATED_AT,
    },
  };
}

function mileageRecord(id: string, recordedAt: string, mileage: number): MileageRecord {
  return {
    id,
    type: 'mileageRecords',
    attributes: {
      vehicleId: 'v1',
      mileage,
      recordedAt,
      source: 'manual',
      createdAt: IRRELEVANT_CREATED_AT,
      flagged: false,
    },
  };
}

function category(
  id: string,
  kind: 'maintenance' | 'modification',
  name = id,
): MaintenanceCategory {
  return {
    id,
    type: 'maintenanceCategories',
    attributes: { name, systemDefined: false, kind },
  };
}

/** Builds a settled (loaded, no error) CategoriesQueryState. */
function settledCategories(data: MaintenanceCategory[]): CategoriesQueryState {
  return { data, isLoading: false, isError: false, error: undefined };
}

const NO_CATEGORIES = settledCategories([]);

interface InfiniteQueryStub<T> {
  data?: { rows: T[]; total: number };
  hasNextPage: boolean;
  isLoading: boolean;
  isFetchingNextPage: boolean;
  fetchNextPage: ReturnType<typeof vi.fn>;
}

function stub<T>(
  rows: T[],
  opts: {
    total?: number;
    hasNextPage?: boolean;
    isLoading?: boolean;
    isFetchingNextPage?: boolean;
  } = {},
): InfiniteQueryStub<T> {
  return {
    data: { rows, total: opts.total ?? rows.length },
    hasNextPage: opts.hasNextPage ?? false,
    isLoading: opts.isLoading ?? false,
    isFetchingNextPage: opts.isFetchingNextPage ?? false,
    fetchNextPage: vi.fn(),
  };
}

function setupSources(opts: {
  maintenance?: InfiniteQueryStub<MaintenanceRecord>;
  fuel?: InfiniteQueryStub<FuelLog>;
  mileage?: InfiniteQueryStub<MileageRecord>;
}) {
  const maintenance = opts.maintenance ?? stub<MaintenanceRecord>([]);
  const fuel = opts.fuel ?? stub<FuelLog>([]);
  const mileage = opts.mileage ?? stub<MileageRecord>([]);

  mockedUseMaintenanceRecords.mockReturnValue(maintenance as never);
  mockedUseFuelLogs.mockReturnValue(fuel as never);
  mockedUseMileageRecords.mockReturnValue(mileage as never);

  return { maintenance, fuel, mileage };
}

beforeEach(() => {
  vi.clearAllMocks();
});

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

describe('useVehicleRecords', () => {
  it('merges rows from all three sources, newest first, with each row carrying its source kind and sourceId', () => {
    setupSources({
      maintenance: stub([maintenanceRecord('m1', 'oil-change', '2026-01-01T00:00:00Z')]),
      fuel: stub([fuelLog('f1', '2026-03-01T00:00:00Z', 10)]),
      mileage: stub([mileageRecord('mi1', '2026-02-01T00:00:00Z', 12000)]),
    });

    const { result } = renderHook(() => useVehicleRecords('v1', NO_CATEGORIES));

    expect(result.current.rows.map((r) => r.id)).toEqual([
      'fuel:f1',
      'mileage:mi1',
      'maintenance:m1',
    ]);

    const byId = new Map(result.current.rows.map((r) => [r.id, r]));
    // Pins each source's own kind — a copy-paste bug (e.g. mileage's adapter
    // hardcoding 'fuel') would file odometer readings under the wrong chip
    // and pass every other assertion in this test.
    expect(byId.get('fuel:f1')?.kind).toBe('fuel');
    expect(byId.get('mileage:mi1')?.kind).toBe('mileage');
    expect(byId.get('maintenance:m1')?.kind).toBe('maintenance');

    // sourceId is the raw underlying resource id (not the prefixed feed id)
    // — Task 15's drawer keys edit/delete calls on this.
    expect(byId.get('fuel:f1')?.sourceId).toBe('f1');
    expect(byId.get('mileage:mi1')?.sourceId).toBe('mi1');
    expect(byId.get('maintenance:m1')?.sourceId).toBe('m1');
  });

  it('resolves kind from the category, not the record', () => {
    const categories = [category('cat-mod', 'modification', 'Lift kit')];
    setupSources({
      maintenance: stub([maintenanceRecord('m1', 'cat-mod', '2026-01-01T00:00:00Z')]),
    });

    const { result } = renderHook(() => useVehicleRecords('v1', settledCategories(categories)));

    const row = result.current.rows.find((r) => r.id === 'maintenance:m1');
    expect(row?.kind).toBe('modification');
  });

  it('falls back to "maintenance" kind when the category cannot be resolved, once categories have settled', () => {
    setupSources({
      maintenance: stub([maintenanceRecord('m1', 'unknown-category', '2026-01-01T00:00:00Z')]),
    });

    const { result } = renderHook(() => useVehicleRecords('v1', NO_CATEGORIES));

    const row = result.current.rows.find((r) => r.id === 'maintenance:m1');
    expect(row?.kind).toBe('maintenance');
    // The fallback is only trustworthy once the categories query has
    // actually settled — this fixture uses NO_CATEGORIES (settled, empty).
    expect(result.current.isLoading).toBe(false);
  });

  it('folds "categories still loading" into isLoading, independent of the record queries', () => {
    setupSources({
      maintenance: stub([maintenanceRecord('m1', 'cat-mod', '2026-01-01T00:00:00Z')]),
    });

    const loadingCategories: CategoriesQueryState = {
      data: undefined,
      isLoading: true,
      isError: false,
      error: undefined,
    };

    const { result } = renderHook(() => useVehicleRecords('v1', loadingCategories));

    // All three record queries are settled (isLoading: false via stub's
    // default), yet the hook must still report isLoading: true — otherwise a
    // consumer renders the maintenance row's fallback kind/title as if it
    // were final, when the category that would resolve it may still arrive.
    expect(result.current.isLoading).toBe(true);
  });

  it('exposes categoriesError when the categories query fails, without blocking isLoading forever', () => {
    setupSources({});
    const boom = new Error('categories fetch failed');
    const failedCategories: CategoriesQueryState = {
      data: undefined,
      isLoading: false,
      isError: true,
      error: boom,
    };

    const { result } = renderHook(() => useVehicleRecords('v1', failedCategories));

    expect(result.current.categoriesError).toBe(boom);
    // A failed categories fetch settles (isLoading transitions to false) —
    // the hook must not get stuck reporting isLoading forever just because
    // categories errored; the caller uses categoriesError to react instead.
    expect(result.current.isLoading).toBe(false);
  });

  it('falls through title: description, then category name, then raw categoryId', () => {
    const categories = [category('cat-1', 'maintenance', 'Oil Change')];
    setupSources({
      maintenance: stub([
        maintenanceRecord('m1', 'cat-1', '2026-01-01T00:00:00Z', { description: 'Full synthetic' }),
        maintenanceRecord('m2', 'cat-1', '2026-01-02T00:00:00Z'),
        maintenanceRecord('m3', 'unresolved-cat', '2026-01-03T00:00:00Z'),
      ]),
    });

    const { result } = renderHook(() => useVehicleRecords('v1', settledCategories(categories)));

    const byId = new Map(result.current.rows.map((r) => [r.id, r]));
    expect(byId.get('maintenance:m1')?.title).toBe('Full synthetic');
    expect(byId.get('maintenance:m2')?.title).toBe('Oil Change');
    expect(byId.get('maintenance:m3')?.title).toBe('unresolved-cat');
  });

  it('populates gallons on fuel rows', () => {
    setupSources({
      fuel: stub([fuelLog('f1', '2026-01-01T00:00:00Z', 12.5)]),
    });

    const { result } = renderHook(() => useVehicleRecords('v1', NO_CATEGORIES));

    const row = result.current.rows.find((r) => r.id === 'fuel:f1');
    expect(row?.gallons).toBe(12.5);
  });

  it('sums total across the three sources', () => {
    setupSources({
      maintenance: stub([maintenanceRecord('m1', 'c1', '2026-01-01T00:00:00Z')], { total: 5 }),
      fuel: stub([fuelLog('f1', '2026-01-01T00:00:00Z', 10)], { total: 7 }),
      mileage: stub([mileageRecord('mi1', '2026-01-01T00:00:00Z', 100)], { total: 2 }),
    });

    const { result } = renderHook(() => useVehicleRecords('v1', NO_CATEGORIES));

    expect(result.current.total).toBe(14);
  });

  it('reports isLoading true while any of the three record sources is loading', () => {
    setupSources({
      maintenance: stub([], { isLoading: true }),
      fuel: stub([], { isLoading: false }),
      mileage: stub([], { isLoading: false }),
    });

    const { result } = renderHook(() => useVehicleRecords('v1', NO_CATEGORIES));

    expect(result.current.isLoading).toBe(true);
  });

  it('loadMore calls fetchNextPage only on sources that still have a next page', async () => {
    const { maintenance, fuel, mileage } = setupSources({
      maintenance: stub([maintenanceRecord('m1', 'c1', '2026-01-01T00:00:00Z')], {
        hasNextPage: true,
      }),
      fuel: stub([fuelLog('f1', '2026-01-01T00:00:00Z', 10)], { hasNextPage: false }),
      mileage: stub([mileageRecord('mi1', '2026-01-01T00:00:00Z', 100)], { hasNextPage: true }),
    });

    const { result } = renderHook(() => useVehicleRecords('v1', NO_CATEGORIES));
    result.current.loadMore();

    expect(maintenance.fetchNextPage).toHaveBeenCalledTimes(1);
    await expectNoCall(fuel.fetchNextPage, 'fuel.fetchNextPage');
    expect(mileage.fetchNextPage).toHaveBeenCalledTimes(1);
  });

  it('hasMore is true when a source still has an unfetched page', () => {
    setupSources({
      maintenance: stub([maintenanceRecord('m1', 'c1', '2026-01-01T00:00:00Z')], {
        hasNextPage: true,
      }),
    });

    const { result } = renderHook(() => useVehicleRecords('v1', NO_CATEGORIES));

    expect(result.current.hasMore).toBe(true);
  });

  it('hasMore reflects withheld rows, not just unfetched pages', () => {
    // Fuel is fully loaded (hasNextPage: false) but maintenance still has an
    // unfetched page, so maintenance's oldest loaded row sets a watermark.
    // Fuel's older row falls below it and is withheld even though fuel itself
    // has nothing left to fetch. mergeVehicleRecords only ever reports
    // withheldCount > 0 while some source is still incomplete (see its
    // zero-watermark short-circuit), so anyHasMore is also true here — this
    // test exercises the withheldCount arm of the `||` rather than isolating
    // it, but the `not.toContain('fuel:f2')` + `withheldCount > 0`
    // assertions pin down that the withholding itself reaches `hasMore`.
    setupSources({
      maintenance: stub(
        [
          maintenanceRecord('m1', 'c1', '2026-03-01T00:00:00Z'),
          maintenanceRecord('m2', 'c1', '2026-02-01T00:00:00Z'),
        ],
        { hasNextPage: true },
      ),
      fuel: stub(
        [fuelLog('f1', '2026-03-01T00:00:00Z', 10), fuelLog('f2', '2026-01-01T00:00:00Z', 8)],
        { hasNextPage: false },
      ),
    });

    const { result } = renderHook(() => useVehicleRecords('v1', NO_CATEGORIES));

    // fuel:f2 (2026-01-01) is below maintenance's watermark (2026-02-01) and
    // must be withheld, even though fuel itself is fully loaded.
    expect(result.current.rows.map((r) => r.id)).not.toContain('fuel:f2');
    expect(result.current.withheldCount).toBeGreaterThan(0);
    expect(result.current.hasMore).toBe(true);
  });

  it('isFetchingNextPage is true when any source has a page fetch in flight', () => {
    setupSources({
      maintenance: stub([], { isFetchingNextPage: false }),
      fuel: stub([], { isFetchingNextPage: true }),
      mileage: stub([], { isFetchingNextPage: false }),
    });

    const { result } = renderHook(() => useVehicleRecords('v1', NO_CATEGORIES));

    expect(result.current.isFetchingNextPage).toBe(true);
  });

  it('isFetchingNextPage is false when no source is fetching', () => {
    setupSources({});

    const { result } = renderHook(() => useVehicleRecords('v1', NO_CATEGORIES));

    expect(result.current.isFetchingNextPage).toBe(false);
  });

  // ---------------------------------------------------------------------
  // Memoized-identity contract. This hook exists specifically so its
  // consumer (Task 17's records table) doesn't re-render on every parent
  // render; these tests pin that down and document the one way a caller can
  // still break it.
  // ---------------------------------------------------------------------

  it('keeps rows/loadMore identity stable across a rerender when nothing changed', () => {
    // A stable categories reference, reused across both renders — this is
    // what a real caller must do (e.g. hold the array from useMaintenanceCategories(),
    // which is itself kept stable by React Query's structural sharing).
    const categories = settledCategories([category('c1', 'maintenance')]);
    const { maintenance, fuel, mileage } = setupSources({
      maintenance: stub([maintenanceRecord('m1', 'c1', '2026-01-01T00:00:00Z')]),
    });

    const { result, rerender } = renderHook(
      ({ cat }: { cat: CategoriesQueryState }) => useVehicleRecords('v1', cat),
      { initialProps: { cat: categories } },
    );

    const first = result.current;

    // Re-render with the exact same stub objects and the exact same
    // categories reference — nothing has actually changed.
    mockedUseMaintenanceRecords.mockReturnValue(maintenance as never);
    mockedUseFuelLogs.mockReturnValue(fuel as never);
    mockedUseMileageRecords.mockReturnValue(mileage as never);
    rerender({ cat: categories });

    const second = result.current;

    expect(second.rows).toBe(first.rows);
    expect(second.loadMore).toBe(first.loadMore);
    expect(second).toBe(first);
  });

  it('documents the hazard: a fresh categories array reference each render churns rows identity', () => {
    // Same content, but a NEW array/object literal on the second render —
    // exactly what an inline `useVehicleRecords(id, { data: [], ... })` or
    // `useVehicleRecords(id, settledCategories([]))` at a caller's call site
    // would produce every render. Task 17's caller must hold a stable
    // reference (e.g. from useMaintenanceCategories()'s own memoized data),
    // or the entire memo chain in this hook is defeated.
    const { maintenance, fuel, mileage } = setupSources({
      maintenance: stub([maintenanceRecord('m1', 'c1', '2026-01-01T00:00:00Z')]),
    });

    const { result, rerender } = renderHook(
      ({ cat }: { cat: CategoriesQueryState }) => useVehicleRecords('v1', cat),
      { initialProps: { cat: settledCategories([category('c1', 'maintenance')]) } },
    );

    const first = result.current;

    mockedUseMaintenanceRecords.mockReturnValue(maintenance as never);
    mockedUseFuelLogs.mockReturnValue(fuel as never);
    mockedUseMileageRecords.mockReturnValue(mileage as never);
    // A content-equal but reference-distinct categories array/object.
    rerender({ cat: settledCategories([category('c1', 'maintenance')]) });

    const second = result.current;

    expect(second.rows).not.toBe(first.rows);
  });
});

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

    expect(result.current.rows[0]?.documentCount).toBe(3);
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

    expect(result.current.rows[0]?.documentCount).toBe(0);
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

    expect(result.current.rows[0]?.documentCount).toBe(0);
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
