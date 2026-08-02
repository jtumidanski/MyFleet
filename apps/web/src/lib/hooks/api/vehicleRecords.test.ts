/**
 * Task 8 — useVehicleRecords composes the three infinite-query source hooks
 * (Task 6) with the pure merge function (Task 7). This is the untested seam
 * between two well-tested pieces: the source hooks are mocked here so the
 * tests focus on the adapter logic — field mapping (especially kind
 * resolution and gallons), and the loadMore/hasMore wiring.
 */
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { renderHook } from '@testing-library/react';
import { useVehicleRecords } from './vehicleRecords';
import { useMaintenanceRecords } from './maintenance';
import { useFuelLogs } from './fuel';
import { useMileageRecords } from './mileage';
import type { MaintenanceRecord } from '../../../types/models/maintenanceRecord';
import type { FuelLog } from '../../../types/models/fuelLog';
import type { MileageRecord } from '../../../types/models/mileage';
import type { MaintenanceCategory } from '../../../types/models/maintenanceCategory';

vi.mock('./maintenance', () => ({ useMaintenanceRecords: vi.fn() }));
vi.mock('./fuel', () => ({ useFuelLogs: vi.fn() }));
vi.mock('./mileage', () => ({ useMileageRecords: vi.fn() }));

const mockedUseMaintenanceRecords = vi.mocked(useMaintenanceRecords);
const mockedUseFuelLogs = vi.mocked(useFuelLogs);
const mockedUseMileageRecords = vi.mocked(useMileageRecords);

// ---------------------------------------------------------------------------
// Fixture builders
// ---------------------------------------------------------------------------

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
      createdAt: performedAt,
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
      createdAt: date,
      updatedAt: date,
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
      createdAt: recordedAt,
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

interface InfiniteQueryStub<T> {
  data?: { rows: T[]; total: number };
  hasNextPage: boolean;
  isLoading: boolean;
  fetchNextPage: ReturnType<typeof vi.fn>;
}

function stub<T>(
  rows: T[],
  opts: { total?: number; hasNextPage?: boolean; isLoading?: boolean } = {},
): InfiniteQueryStub<T> {
  return {
    data: { rows, total: opts.total ?? rows.length },
    hasNextPage: opts.hasNextPage ?? false,
    isLoading: opts.isLoading ?? false,
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
  it('merges rows from all three sources, newest first', () => {
    setupSources({
      maintenance: stub([maintenanceRecord('m1', 'oil-change', '2026-01-01T00:00:00Z')]),
      fuel: stub([fuelLog('f1', '2026-03-01T00:00:00Z', 10)]),
      mileage: stub([mileageRecord('mi1', '2026-02-01T00:00:00Z', 12000)]),
    });

    const { result } = renderHook(() => useVehicleRecords('v1', []));

    expect(result.current.rows.map((r) => r.id)).toEqual([
      'fuel:f1',
      'mileage:mi1',
      'maintenance:m1',
    ]);
  });

  it('resolves kind from the category, not the record', () => {
    const categories = [category('cat-mod', 'modification', 'Lift kit')];
    setupSources({
      maintenance: stub([maintenanceRecord('m1', 'cat-mod', '2026-01-01T00:00:00Z')]),
    });

    const { result } = renderHook(() => useVehicleRecords('v1', categories));

    const row = result.current.rows.find((r) => r.id === 'maintenance:m1');
    expect(row?.kind).toBe('modification');
  });

  it('falls back to "maintenance" kind when the category cannot be resolved', () => {
    setupSources({
      maintenance: stub([maintenanceRecord('m1', 'unknown-category', '2026-01-01T00:00:00Z')]),
    });

    const { result } = renderHook(() => useVehicleRecords('v1', []));

    const row = result.current.rows.find((r) => r.id === 'maintenance:m1');
    expect(row?.kind).toBe('maintenance');
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

    const { result } = renderHook(() => useVehicleRecords('v1', categories));

    const byId = new Map(result.current.rows.map((r) => [r.id, r]));
    expect(byId.get('maintenance:m1')?.title).toBe('Full synthetic');
    expect(byId.get('maintenance:m2')?.title).toBe('Oil Change');
    expect(byId.get('maintenance:m3')?.title).toBe('unresolved-cat');
  });

  it('populates gallons on fuel rows', () => {
    setupSources({
      fuel: stub([fuelLog('f1', '2026-01-01T00:00:00Z', 12.5)]),
    });

    const { result } = renderHook(() => useVehicleRecords('v1', []));

    const row = result.current.rows.find((r) => r.id === 'fuel:f1');
    expect(row?.gallons).toBe(12.5);
  });

  it('sums total across the three sources', () => {
    setupSources({
      maintenance: stub([maintenanceRecord('m1', 'c1', '2026-01-01T00:00:00Z')], { total: 5 }),
      fuel: stub([fuelLog('f1', '2026-01-01T00:00:00Z', 10)], { total: 7 }),
      mileage: stub([mileageRecord('mi1', '2026-01-01T00:00:00Z', 100)], { total: 2 }),
    });

    const { result } = renderHook(() => useVehicleRecords('v1', []));

    expect(result.current.total).toBe(14);
  });

  it('loadMore calls fetchNextPage only on sources that still have a next page', () => {
    const { maintenance, fuel, mileage } = setupSources({
      maintenance: stub([maintenanceRecord('m1', 'c1', '2026-01-01T00:00:00Z')], {
        hasNextPage: true,
      }),
      fuel: stub([fuelLog('f1', '2026-01-01T00:00:00Z', 10)], { hasNextPage: false }),
      mileage: stub([mileageRecord('mi1', '2026-01-01T00:00:00Z', 100)], { hasNextPage: true }),
    });

    const { result } = renderHook(() => useVehicleRecords('v1', []));
    result.current.loadMore();

    expect(maintenance.fetchNextPage).toHaveBeenCalledTimes(1);
    expect(fuel.fetchNextPage).not.toHaveBeenCalled();
    expect(mileage.fetchNextPage).toHaveBeenCalledTimes(1);
  });

  it('hasMore is true when a source still has an unfetched page', () => {
    setupSources({
      maintenance: stub([maintenanceRecord('m1', 'c1', '2026-01-01T00:00:00Z')], {
        hasNextPage: true,
      }),
    });

    const { result } = renderHook(() => useVehicleRecords('v1', []));

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
        [
          fuelLog('f1', '2026-03-01T00:00:00Z', 10),
          fuelLog('f2', '2026-01-01T00:00:00Z', 8),
        ],
        { hasNextPage: false },
      ),
    });

    const { result } = renderHook(() => useVehicleRecords('v1', []));

    // fuel:f2 (2026-01-01) is below maintenance's watermark (2026-02-01) and
    // must be withheld, even though fuel itself is fully loaded.
    expect(result.current.rows.map((r) => r.id)).not.toContain('fuel:f2');
    expect(result.current.withheldCount).toBeGreaterThan(0);
    expect(result.current.hasMore).toBe(true);
  });
});
