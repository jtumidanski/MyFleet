/**
 * React Query hooks for the dashboard feature (Task 15.7).
 *
 * Covers:
 *   - Layout get/put (GET/PUT /fleets/{id}/dashboard)
 *   - Fleet overview (GET /fleets/{id}/dashboard/overview)
 *   - Spend-by-vehicle aggregation (GET /fleets/{id}/dashboard/spend-by-vehicle)
 *   - Mileage trends aggregation (GET /vehicles/{id}/dashboard/mileage-trends)
 */
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { toast } from 'sonner';
import { createErrorFromUnknown } from '@myfleet/shared-ts';
import { dashboardService, type DateRange } from '../../../services/api/DashboardService';
import type { WidgetInput } from '../../../types/models/dashboard';

// ---------------------------------------------------------------------------
// Query key factory
// ---------------------------------------------------------------------------

/**
 * Hierarchical query-key factory for dashboard queries.
 *
 * all                                      -> ['dashboards']
 * layout(fleetId)                          -> ['dashboards', 'layout', fleetId]
 * overview(fleetId)                        -> ['dashboards', 'overview', fleetId]
 * spendByVehicle(fleetId, range)           -> ['dashboards', 'spendByVehicle', fleetId, range]
 * mileageTrends(vehicleId, range)          -> ['dashboards', 'mileageTrends', vehicleId, range]
 */
export const dashboardKeys = {
  all: ['dashboards'] as const,
  layout: (fleetId: string) => [...dashboardKeys.all, 'layout', fleetId] as const,
  overview: (fleetId: string) => [...dashboardKeys.all, 'overview', fleetId] as const,
  spendByVehicle: (fleetId: string, range: DateRange) =>
    [...dashboardKeys.all, 'spendByVehicle', fleetId, range] as const,
  mileageTrends: (vehicleId: string, range: DateRange) =>
    [...dashboardKeys.all, 'mileageTrends', vehicleId, range] as const,
};

// ---------------------------------------------------------------------------
// Layout queries
// ---------------------------------------------------------------------------

/**
 * GET /api/fleet/fleets/{fleetId}/dashboard — fetch the per-user widget layout.
 */
export function useDashboardLayout(fleetId: string | null | undefined) {
  return useQuery({
    queryKey: dashboardKeys.layout(fleetId ?? ''),
    queryFn: () => dashboardService.getLayout(fleetId as string),
    enabled: !!fleetId,
    staleTime: 5 * 60 * 1000,
    gcTime: 10 * 60 * 1000,
  });
}

// ---------------------------------------------------------------------------
// Layout mutation
// ---------------------------------------------------------------------------

/**
 * PUT /api/fleet/fleets/{fleetId}/dashboard — replace the widget layout.
 * Invalidates the layout query on settle.
 */
export function useSaveDashboardLayout(fleetId: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (widgets: WidgetInput[]) => dashboardService.saveLayout(fleetId, widgets),
    onSettled: () => {
      void queryClient.invalidateQueries({ queryKey: dashboardKeys.layout(fleetId) });
    },
    onError: (err) => {
      const apiError = createErrorFromUnknown(err);
      toast.error(apiError.message || 'Could not save dashboard layout');
    },
  });
}

// ---------------------------------------------------------------------------
// Aggregation queries
// ---------------------------------------------------------------------------

/**
 * GET /api/fleet/fleets/{fleetId}/dashboard/overview
 */
export function useFleetOverview(fleetId: string | null | undefined) {
  return useQuery({
    queryKey: dashboardKeys.overview(fleetId ?? ''),
    queryFn: () => dashboardService.getOverview(fleetId as string),
    enabled: !!fleetId,
    staleTime: 60 * 1000,
    gcTime: 5 * 60 * 1000,
  });
}

/**
 * GET /api/fleet/fleets/{fleetId}/dashboard/spend-by-vehicle?from=&to=
 */
export function useSpendByVehicle(fleetId: string | null | undefined, range: DateRange = {}) {
  return useQuery({
    queryKey: dashboardKeys.spendByVehicle(fleetId ?? '', range),
    queryFn: () => dashboardService.getSpendByVehicle(fleetId as string, range),
    enabled: !!fleetId,
    staleTime: 2 * 60 * 1000,
    gcTime: 5 * 60 * 1000,
  });
}

/**
 * GET /api/fleet/vehicles/{vehicleId}/dashboard/mileage-trends?from=&to=
 */
export function useMileageTrends(vehicleId: string | null | undefined, range: DateRange = {}) {
  return useQuery({
    queryKey: dashboardKeys.mileageTrends(vehicleId ?? '', range),
    queryFn: () => dashboardService.getMileageTrends(vehicleId as string, range),
    enabled: !!vehicleId,
    staleTime: 2 * 60 * 1000,
    gcTime: 5 * 60 * 1000,
  });
}

// ---------------------------------------------------------------------------
// Invalidation helper
// ---------------------------------------------------------------------------

export function useInvalidateDashboard() {
  const queryClient = useQueryClient();
  return {
    invalidateAll: () => queryClient.invalidateQueries({ queryKey: dashboardKeys.all }),
    invalidateLayout: (fleetId: string) =>
      queryClient.invalidateQueries({ queryKey: dashboardKeys.layout(fleetId) }),
    invalidateOverview: (fleetId: string) =>
      queryClient.invalidateQueries({ queryKey: dashboardKeys.overview(fleetId) }),
  };
}
