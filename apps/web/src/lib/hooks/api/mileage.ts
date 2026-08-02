import { useInfiniteQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { mileageService } from '../../../services/api/MileageService';
import { RECORD_PAGE_SIZE } from './pageSize';
import type { CreateMileageAttributes, MileageRecord } from '../../../types/models/mileage';

// Hierarchical query-key factory mirroring vehicleKeys.
// all                          -> ['mileage']
// lists()                      -> ['mileage', 'list']
// list({ vehicleId })          -> ['mileage', 'list', { vehicleId }]
// list({ vehicleId, from, to}) -> ['mileage', 'list', { vehicleId, from, to }]
export const mileageKeys = {
  all: ['mileage'] as const,
  lists: () => [...mileageKeys.all, 'list'] as const,
  list: (params: { vehicleId: string; from?: string; to?: string }) =>
    [...mileageKeys.lists(), params] as const,
};

// ---------------------------------------------------------------------------
// Auto-fill helper (pure function — injectable for testing)
// ---------------------------------------------------------------------------

/**
 * Returns the mileage value of the record with the latest recordedAt timestamp,
 * or undefined if the array is empty. Used to pre-fill maintenance/fuel forms.
 */
export function getLatestMileage(records: MileageRecord[]): number | undefined {
  if (records.length === 0) return undefined;
  const latest = records.reduce((best, cur) =>
    cur.attributes.recordedAt > best.attributes.recordedAt ? cur : best,
  );
  return latest.attributes.mileage;
}

// ---------------------------------------------------------------------------
// Queries
// ---------------------------------------------------------------------------

export interface UseMileageParams {
  vehicleId: string;
  from?: string;
  to?: string;
}

/**
 * GET /api/fleet/vehicles/{vehicleId}/mileage — list mileage records (newest first).
 * Supports optional date filters.
 *
 * Infinite rather than single-page: the unified records feed merges this with
 * two other independently-paginated sources, and a merge over sources that
 * REPLACE their rows on page advance would drop the newest rows from view.
 * Pages accumulate; `rows` is every page fetched so far.
 */
export function useMileageRecords(params: UseMileageParams | null | undefined) {
  const { vehicleId = '', from, to } = params ?? {};
  return useInfiniteQuery({
    queryKey: mileageKeys.list({ vehicleId, from, to }),
    queryFn: ({ pageParam }) =>
      mileageService.listByVehicle(vehicleId, {
        from,
        to,
        page: pageParam,
        pageSize: RECORD_PAGE_SIZE,
      }),
    initialPageParam: 1,
    getNextPageParam: (lastPage, allPages) =>
      allPages.length < (lastPage.meta?.totalPages ?? 1) ? allPages.length + 1 : undefined,
    enabled: !!vehicleId,
    staleTime: 60 * 1000,
    gcTime: 5 * 60 * 1000,
    select: (data) => ({
      rows: data.pages.flatMap((p) => p.data),
      // Read from the LAST page, not the first: `total` can change between
      // fetches (a row created/removed while a later page was in flight),
      // and the last page is the freshest information we have.
      total: data.pages[data.pages.length - 1]?.meta?.total ?? 0,
    }),
  });
}

// ---------------------------------------------------------------------------
// Mutations
// ---------------------------------------------------------------------------

/**
 * POST /api/fleet/vehicles/{vehicleId}/mileage — append a manual mileage record.
 * Invalidates all mileage lists for this vehicle on settle.
 */
export function useCreateMileageRecord(vehicleId: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (attrs: CreateMileageAttributes) => mileageService.create(vehicleId, attrs),
    onSettled: () => {
      void queryClient.invalidateQueries({ queryKey: mileageKeys.lists() });
    },
  });
}
