import { useQuery } from '@tanstack/react-query';
import { activityService } from '../../../services/api/ActivityService';
import type { ActivityEvent } from '../../../types/models/activity';
import type { PageMeta } from '@myfleet/shared-ts';

// ---------------------------------------------------------------------------
// Query key factory
// ---------------------------------------------------------------------------

/**
 * Hierarchical query-key factory for activity events.
 *
 * all                                          -> ['activityEvents']
 * lists()                                      -> ['activityEvents', 'list']
 * fleetFeed({ fleetId, page, pageSize })       -> ['activityEvents', 'list', { fleetId, ... }]
 * vehicleTimeline({ vehicleId, page,pageSize}) -> ['activityEvents', 'list', { vehicleId, ... }]
 */
export const activityKeys = {
  all: ['activityEvents'] as const,
  lists: () => [...activityKeys.all, 'list'] as const,
  fleetFeed: (params: { fleetId: string; page: number; pageSize: number }) =>
    [...activityKeys.lists(), params] as const,
  vehicleTimeline: (params: { vehicleId: string; page: number; pageSize: number }) =>
    [...activityKeys.lists(), params] as const,
};

// ---------------------------------------------------------------------------
// Pagination selector (pure — injectable/testable)
// ---------------------------------------------------------------------------

export interface ActivityPage {
  data: ActivityEvent[];
  total: number;
  totalPages: number;
  hasNextPage: boolean;
}

/**
 * Derives pagination metadata from the JSON:API response envelope.
 * Exposed as a named export so tests can assert the derivation logic directly.
 */
export function selectActivityPage(result: {
  data: ActivityEvent[];
  meta?: PageMeta;
}): ActivityPage {
  const meta = result.meta;
  const total = meta?.total ?? 0;
  const totalPages = meta?.totalPages ?? 1;
  const currentPage = meta?.number ?? 1;
  const hasNextPage = currentPage < totalPages;
  return { data: result.data, total, totalPages, hasNextPage };
}

// ---------------------------------------------------------------------------
// Queries (read-only — activity feed has no mutations)
// ---------------------------------------------------------------------------

const DEFAULT_PAGE = 1;
const DEFAULT_PAGE_SIZE = 25;

/**
 * GET /api/fleet/fleets/{fleetId}/activity — paginated fleet activity feed.
 * Exposes total/totalPages/hasNextPage from the response meta.
 */
export function useFleetActivity(
  fleetId: string | null | undefined,
  page = DEFAULT_PAGE,
  pageSize = DEFAULT_PAGE_SIZE,
) {
  return useQuery({
    queryKey: activityKeys.fleetFeed({ fleetId: fleetId ?? '', page, pageSize }),
    queryFn: () => activityService.listByFleet(fleetId as string, { page, pageSize }),
    enabled: !!fleetId,
    staleTime: 60 * 1000,
    gcTime: 5 * 60 * 1000,
    select: selectActivityPage,
  });
}

/**
 * GET /api/fleet/vehicles/{vehicleId}/activity — paginated per-vehicle timeline.
 * Exposes total/totalPages/hasNextPage from the response meta.
 */
export function useVehicleActivity(
  vehicleId: string | null | undefined,
  page = DEFAULT_PAGE,
  pageSize = DEFAULT_PAGE_SIZE,
) {
  return useQuery({
    queryKey: activityKeys.vehicleTimeline({ vehicleId: vehicleId ?? '', page, pageSize }),
    queryFn: () => activityService.listByVehicle(vehicleId as string, { page, pageSize }),
    enabled: !!vehicleId,
    staleTime: 60 * 1000,
    gcTime: 5 * 60 * 1000,
    select: selectActivityPage,
  });
}
