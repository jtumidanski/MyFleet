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
