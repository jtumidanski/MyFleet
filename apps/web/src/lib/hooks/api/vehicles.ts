import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { vehicleService } from '../../../services/api/VehicleService';
import type {
  CreateVehicleAttributes,
  UpdateVehicleAttributes,
} from '../../../types/models/vehicle';

// Hierarchical query-key factory (frontend-dev-guidelines). The intermediate
// `lists()` / `details()` tiers enable scoped invalidation, while `list()` /
// `detail()` build on them. The shapes match the canonical test in
// vehicles.test.ts exactly:
//   all                     -> ['vehicles']
//   list({ fleetId: 'f1' }) -> ['vehicles', 'list', { fleetId: 'f1' }]
//   detail('v1')            -> ['vehicles', 'detail', 'v1']
export const vehicleKeys = {
  all: ['vehicles'] as const,
  lists: () => [...vehicleKeys.all, 'list'] as const,
  list: (params: { fleetId: string }) => [...vehicleKeys.lists(), params] as const,
  details: () => [...vehicleKeys.all, 'detail'] as const,
  detail: (id: string) => [...vehicleKeys.details(), id] as const,
};

// --- Queries ---

export function useVehicles(fleetId: string | null | undefined) {
  return useQuery({
    queryKey: vehicleKeys.list({ fleetId: fleetId ?? '' }),
    queryFn: () => vehicleService.listByFleet(fleetId as string),
    enabled: !!fleetId,
    staleTime: 60 * 1000,
    gcTime: 5 * 60 * 1000,
  });
}

export function useVehicle(id: string | null | undefined) {
  return useQuery({
    queryKey: vehicleKeys.detail(id ?? ''),
    queryFn: () => vehicleService.get(id as string),
    enabled: !!id,
    staleTime: 60 * 1000,
    gcTime: 5 * 60 * 1000,
  });
}

// --- Mutations (invalidate vehicleKeys.all / detail) ---

export function useCreateVehicle(fleetId: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (attributes: CreateVehicleAttributes) =>
      vehicleService.createInFleet(fleetId, attributes),
    onSettled: () => {
      void queryClient.invalidateQueries({ queryKey: vehicleKeys.lists() });
    },
  });
}

export function useUpdateVehicle() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ id, attributes }: { id: string; attributes: UpdateVehicleAttributes }) =>
      vehicleService.patch(id, attributes),
    onSettled: (_data, _error, variables) => {
      void queryClient.invalidateQueries({ queryKey: vehicleKeys.lists() });
      void queryClient.invalidateQueries({ queryKey: vehicleKeys.detail(variables.id) });
    },
  });
}

export function useSoftDeleteVehicle() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => vehicleService.remove(id),
    onSettled: (_data, _error, id) => {
      void queryClient.invalidateQueries({ queryKey: vehicleKeys.lists() });
      void queryClient.invalidateQueries({ queryKey: vehicleKeys.detail(id) });
    },
  });
}

export function useRestoreVehicle() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => vehicleService.restore(id),
    onSettled: (_data, _error, id) => {
      void queryClient.invalidateQueries({ queryKey: vehicleKeys.lists() });
      void queryClient.invalidateQueries({ queryKey: vehicleKeys.detail(id) });
    },
  });
}

// --- Invalidation helper ---

export function useInvalidateVehicles() {
  const queryClient = useQueryClient();
  return {
    invalidateAll: () => queryClient.invalidateQueries({ queryKey: vehicleKeys.all }),
    invalidateLists: () => queryClient.invalidateQueries({ queryKey: vehicleKeys.lists() }),
    invalidateVehicle: (id: string) =>
      queryClient.invalidateQueries({ queryKey: vehicleKeys.detail(id) }),
  };
}
