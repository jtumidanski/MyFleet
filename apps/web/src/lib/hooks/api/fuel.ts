import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { fuelService } from '../../../services/api/FuelService';
import type { CreateFuelLogAttributes, UpdateFuelLogAttributes } from '../../../types/models/fuelLog';

// ---------------------------------------------------------------------------
// Query key factory
// ---------------------------------------------------------------------------

/**
 * Hierarchical query-key factory for fuel logs.
 * all                       -> ['fuelLogs']
 * lists()                   -> ['fuelLogs', 'list']
 * list({ vehicleId })       -> ['fuelLogs', 'list', { vehicleId }]
 * details()                 -> ['fuelLogs', 'detail']
 * detail(id)                -> ['fuelLogs', 'detail', id]
 */
export const fuelKeys = {
  all: ['fuelLogs'] as const,
  lists: () => [...fuelKeys.all, 'list'] as const,
  list: (params: { vehicleId: string }) => [...fuelKeys.lists(), params] as const,
  details: () => [...fuelKeys.all, 'detail'] as const,
  detail: (id: string) => [...fuelKeys.details(), id] as const,
};

// ---------------------------------------------------------------------------
// Queries
// ---------------------------------------------------------------------------

/** GET /api/fleet/vehicles/{vehicleId}/fuel-logs — list fuel logs (newest first). */
export function useFuelLogs(vehicleId: string | null | undefined) {
  return useQuery({
    queryKey: fuelKeys.list({ vehicleId: vehicleId ?? '' }),
    queryFn: () => fuelService.listByVehicle(vehicleId as string),
    enabled: !!vehicleId,
    staleTime: 60 * 1000,
    gcTime: 5 * 60 * 1000,
    select: (result) => result.data,
  });
}

/** GET /api/fleet/fuel-logs/{id} */
export function useFuelLog(id: string | null | undefined) {
  return useQuery({
    queryKey: fuelKeys.detail(id ?? ''),
    queryFn: () => fuelService.get(id as string),
    enabled: !!id,
    staleTime: 60 * 1000,
    gcTime: 5 * 60 * 1000,
  });
}

// ---------------------------------------------------------------------------
// Mutations
// ---------------------------------------------------------------------------

/**
 * POST /api/fleet/vehicles/{vehicleId}/fuel-logs — log a fuel entry.
 * Invalidates all fuel log lists for this vehicle on settle.
 */
export function useCreateFuelLog(vehicleId: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (attrs: CreateFuelLogAttributes) =>
      fuelService.createForVehicle(vehicleId, attrs),
    onSettled: () => {
      void queryClient.invalidateQueries({ queryKey: fuelKeys.lists() });
    },
  });
}

/** PATCH /api/fleet/fuel-logs/{id} — partial update. */
export function useUpdateFuelLog() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ id, attributes }: { id: string; attributes: UpdateFuelLogAttributes }) =>
      fuelService.patch(id, attributes),
    onSettled: (_data, _error, variables) => {
      void queryClient.invalidateQueries({ queryKey: fuelKeys.lists() });
      void queryClient.invalidateQueries({ queryKey: fuelKeys.detail(variables.id) });
    },
  });
}

/** DELETE /api/fleet/fuel-logs/{id} — soft delete. */
export function useDeleteFuelLog() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => fuelService.remove(id),
    onSettled: (_data, _error, id) => {
      void queryClient.invalidateQueries({ queryKey: fuelKeys.lists() });
      void queryClient.invalidateQueries({ queryKey: fuelKeys.detail(id) });
    },
  });
}
