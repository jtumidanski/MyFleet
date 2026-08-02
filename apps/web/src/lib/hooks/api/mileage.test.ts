import { describe, it, expect, vi } from 'vitest';
import { renderHook, act, waitFor } from '@testing-library/react';
import React from 'react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { mileageKeys, getLatestMileage, useMileageRecords } from './mileage';
import { mileageService } from '../../../services/api/MileageService';
import type { MileageRecord, MileageRecordAttributes } from '../../../types/models/mileage';

// Mock the service module so no network call is needed; each test controls
// what page(s) resolve.
vi.mock('../../../services/api/MileageService', () => ({
  mileageService: {
    listByVehicle: vi.fn(),
  },
}));

/** Builds a single JsonApiResource<MileageRecordAttributes> for fixtures. */
function mileageResource(id: string, recordedAt: string, mileage: number): MileageRecord {
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
    } satisfies MileageRecordAttributes,
  };
}

function wrapper({ children }: { children: React.ReactNode }) {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return React.createElement(QueryClientProvider, { client: queryClient }, children);
}

describe('mileageKeys', () => {
  it('is hierarchical', () => {
    expect(mileageKeys.all).toEqual(['mileage']);
    expect(mileageKeys.lists()).toEqual(['mileage', 'list']);
    expect(mileageKeys.list({ vehicleId: 'v1' })).toEqual(['mileage', 'list', { vehicleId: 'v1' }]);
    expect(mileageKeys.list({ vehicleId: 'v1', from: '2024-01-01T00:00:00Z' })).toEqual([
      'mileage',
      'list',
      { vehicleId: 'v1', from: '2024-01-01T00:00:00Z' },
    ]);
  });
});

describe('getLatestMileage', () => {
  it('returns the mileage of the record with the latest recordedAt', () => {
    const records: MileageRecord[] = [
      {
        id: '1',
        type: 'mileageRecords',
        attributes: {
          vehicleId: 'v1',
          mileage: 10000,
          recordedAt: '2024-01-01T00:00:00Z',
          source: 'manual',
          createdAt: '2024-01-01T00:00:00Z',
          flagged: false,
        },
      },
      {
        id: '2',
        type: 'mileageRecords',
        attributes: {
          vehicleId: 'v1',
          mileage: 15000,
          recordedAt: '2024-06-01T00:00:00Z',
          source: 'manual',
          createdAt: '2024-06-01T00:00:00Z',
          flagged: false,
        },
      },
      {
        id: '3',
        type: 'mileageRecords',
        attributes: {
          vehicleId: 'v1',
          mileage: 12000,
          recordedAt: '2024-03-01T00:00:00Z',
          source: 'manual',
          createdAt: '2024-03-01T00:00:00Z',
          flagged: false,
        },
      },
    ];

    expect(getLatestMileage(records)).toBe(15000);
  });

  it('returns undefined for an empty array', () => {
    expect(getLatestMileage([])).toBeUndefined();
  });

  it('returns the single record mileage when array has one element', () => {
    const records: MileageRecord[] = [
      {
        id: '1',
        type: 'mileageRecords',
        attributes: {
          vehicleId: 'v1',
          mileage: 5000,
          recordedAt: '2024-01-01T00:00:00Z',
          source: 'manual',
          createdAt: '2024-01-01T00:00:00Z',
          flagged: false,
        },
      },
    ];
    expect(getLatestMileage(records)).toBe(5000);
  });
});

describe('useMileageRecords', () => {
  it('accumulates pages rather than replacing them', async () => {
    // Two pages of one row each, meta reporting two total pages.
    const listByVehicle = vi
      .spyOn(mileageService, 'listByVehicle')
      .mockResolvedValueOnce({
        data: [mileageResource('r1', '2026-06-01T00:00:00Z', 84000)],
        meta: { total: 2, totalPages: 2, number: 1, size: 100 },
      })
      .mockResolvedValueOnce({
        data: [mileageResource('r2', '2026-01-01T00:00:00Z', 80000)],
        meta: { total: 2, totalPages: 2, number: 2, size: 100 },
      });

    const { result } = renderHook(() => useMileageRecords({ vehicleId: 'v1' }), { wrapper });

    await waitFor(() => expect(result.current.data?.rows).toHaveLength(1));
    expect(result.current.hasNextPage).toBe(true);

    await act(() => result.current.fetchNextPage().then(() => undefined));

    // Page 2 ADDS to page 1 — the newest row must not disappear.
    await waitFor(() => expect(result.current.data?.rows).toHaveLength(2));
    expect(result.current.data?.rows.map((r) => r.id)).toEqual(['r1', 'r2']);
    expect(result.current.hasNextPage).toBe(false);
    expect(listByVehicle).toHaveBeenCalledTimes(2);

    // A call count alone would stay green even if the server-side truncation
    // bug this task exists to fix (page[size] silently defaulting to 25) came
    // back — assert the actual page-size argument was sent on both requests.
    expect(listByVehicle).toHaveBeenNthCalledWith(
      1,
      'v1',
      expect.objectContaining({ page: 1, pageSize: 100 }),
    );
    expect(listByVehicle).toHaveBeenNthCalledWith(
      2,
      'v1',
      expect.objectContaining({ page: 2, pageSize: 100 }),
    );
  });
});
