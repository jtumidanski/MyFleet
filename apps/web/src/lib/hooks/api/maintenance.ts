import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { maintenanceRecordService } from '../../../services/api/MaintenanceRecordService';
import { maintenanceScheduleService } from '../../../services/api/MaintenanceScheduleService';
import { maintenanceCategoryService } from '../../../services/api/MaintenanceCategoryService';
import { vehicleKeys } from './vehicles';
import type { CreateMaintenanceRecordAttributes } from '../../../types/models/maintenanceRecord';
import type {
  CreateMaintenanceScheduleAttributes,
  UpdateMaintenanceScheduleAttributes,
  CompleteMaintenanceScheduleAttributes,
} from '../../../types/models/maintenanceSchedule';
import type {
  CreateMaintenanceCategoryAttributes,
  MaintenanceCategoryKind,
} from '../../../types/models/maintenanceCategory';

// ---------------------------------------------------------------------------
// Query key factories
// ---------------------------------------------------------------------------

/**
 * Hierarchical query-key factory for maintenance records.
 * all                              -> ['maintenanceRecords']
 * lists()                          -> ['maintenanceRecords', 'list']
 * list({ vehicleId, kind })        -> ['maintenanceRecords', 'list', { vehicleId, kind }]
 * details()                        -> ['maintenanceRecords', 'detail']
 * detail(id)                       -> ['maintenanceRecords', 'detail', id]
 */
export const maintenanceRecordKeys = {
  all: ['maintenanceRecords'] as const,
  lists: () => [...maintenanceRecordKeys.all, 'list'] as const,
  list: (params: { vehicleId: string; kind?: MaintenanceCategoryKind }) =>
    [...maintenanceRecordKeys.lists(), params] as const,
  details: () => [...maintenanceRecordKeys.all, 'detail'] as const,
  detail: (id: string) => [...maintenanceRecordKeys.details(), id] as const,
};

/**
 * Hierarchical query-key factory for maintenance schedules.
 * all                              -> ['maintenanceSchedules']
 * lists()                          -> ['maintenanceSchedules', 'list']
 * list({ vehicleId })              -> ['maintenanceSchedules', 'list', { vehicleId }]
 * listQueue({ fleetId, state })    -> ['maintenanceSchedules', 'queue', { fleetId, state }]
 * details()                        -> ['maintenanceSchedules', 'detail']
 * detail(id)                       -> ['maintenanceSchedules', 'detail', id]
 */
export const maintenanceScheduleKeys = {
  all: ['maintenanceSchedules'] as const,
  lists: () => [...maintenanceScheduleKeys.all, 'list'] as const,
  list: (params: { vehicleId: string }) => [...maintenanceScheduleKeys.lists(), params] as const,
  queues: () => [...maintenanceScheduleKeys.all, 'queue'] as const,
  queue: (params: { fleetId: string; state: 'upcoming' | 'overdue' }) =>
    [...maintenanceScheduleKeys.queues(), params] as const,
  details: () => [...maintenanceScheduleKeys.all, 'detail'] as const,
  detail: (id: string) => [...maintenanceScheduleKeys.details(), id] as const,
};

/**
 * Hierarchical query-key factory for maintenance categories.
 * all              -> ['maintenanceCategories']
 * lists()          -> ['maintenanceCategories', 'list']
 * list({ kind })   -> ['maintenanceCategories', 'list', { kind }]
 */
export const maintenanceCategoryKeys = {
  all: ['maintenanceCategories'] as const,
  lists: () => [...maintenanceCategoryKeys.all, 'list'] as const,
  list: (params: { kind?: MaintenanceCategoryKind }) =>
    [...maintenanceCategoryKeys.lists(), params] as const,
};

// ---------------------------------------------------------------------------
// Category queries
// ---------------------------------------------------------------------------

/** GET /api/fleet/maintenance-categories — all categories, or one kind. */
export function useMaintenanceCategories(kind?: MaintenanceCategoryKind) {
  return useQuery({
    queryKey: maintenanceCategoryKeys.list({ kind }),
    queryFn: () => maintenanceCategoryService.list(kind),
    staleTime: 10 * 60 * 1000, // Categories are relatively static
    gcTime: 30 * 60 * 1000,
    select: (result) => result.data,
  });
}

// ---------------------------------------------------------------------------
// Category mutations
// ---------------------------------------------------------------------------

/**
 * POST /api/fleet/maintenance-categories — create a free-form category.
 *
 * The server is idempotent on case-insensitive name, so a "create" may return
 * a pre-existing system or fleet row. Callers must use the returned resource's
 * id rather than assuming a new one was minted.
 */
export function useCreateMaintenanceCategory() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (attrs: CreateMaintenanceCategoryAttributes) =>
      maintenanceCategoryService.create(attrs),
    onSettled: () => {
      void queryClient.invalidateQueries({ queryKey: maintenanceCategoryKeys.all });
    },
  });
}

// ---------------------------------------------------------------------------
// Maintenance record queries
// ---------------------------------------------------------------------------

/** GET /api/fleet/vehicles/{vehicleId}/maintenance-records[?kind=…] */
export function useMaintenanceRecords(
  vehicleId: string | null | undefined,
  kind?: MaintenanceCategoryKind,
) {
  return useQuery({
    queryKey: maintenanceRecordKeys.list({ vehicleId: vehicleId ?? '', kind }),
    queryFn: () => maintenanceRecordService.listByVehicle(vehicleId as string, kind),
    enabled: !!vehicleId,
    staleTime: 60 * 1000,
    gcTime: 5 * 60 * 1000,
    select: (result) => result.data,
  });
}

/** GET /api/fleet/maintenance-records/{id} */
export function useMaintenanceRecord(id: string | null | undefined) {
  return useQuery({
    queryKey: maintenanceRecordKeys.detail(id ?? ''),
    queryFn: () => maintenanceRecordService.get(id as string),
    enabled: !!id,
    staleTime: 60 * 1000,
    gcTime: 5 * 60 * 1000,
  });
}

// ---------------------------------------------------------------------------
// Maintenance record mutations
// ---------------------------------------------------------------------------

/** POST /api/fleet/vehicles/{vehicleId}/maintenance-records */
export function useCreateMaintenanceRecord(vehicleId: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (attrs: CreateMaintenanceRecordAttributes) =>
      maintenanceRecordService.createForVehicle(vehicleId, attrs),
    onSettled: () => {
      void queryClient.invalidateQueries({ queryKey: maintenanceRecordKeys.lists() });
    },
  });
}

/** DELETE /api/fleet/maintenance-records/{id} — soft delete */
export function useDeleteMaintenanceRecord(vehicleId: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => maintenanceRecordService.remove(id),
    onSettled: (_data, _error, id) => {
      void queryClient.invalidateQueries({ queryKey: maintenanceRecordKeys.lists() });
      void queryClient.invalidateQueries({ queryKey: maintenanceRecordKeys.detail(id) });
      void queryClient.invalidateQueries({ queryKey: vehicleKeys.detail(vehicleId) });
    },
  });
}

// ---------------------------------------------------------------------------
// Maintenance schedule queries
// ---------------------------------------------------------------------------

/** GET /api/fleet/vehicles/{vehicleId}/maintenance-schedules */
export function useMaintenanceSchedules(vehicleId: string | null | undefined) {
  return useQuery({
    queryKey: maintenanceScheduleKeys.list({ vehicleId: vehicleId ?? '' }),
    queryFn: () => maintenanceScheduleService.listByVehicle(vehicleId as string),
    enabled: !!vehicleId,
    staleTime: 60 * 1000,
    gcTime: 5 * 60 * 1000,
    select: (result) => result.data,
  });
}

/** GET /api/fleet/maintenance-schedules/{id} */
export function useMaintenanceSchedule(id: string | null | undefined) {
  return useQuery({
    queryKey: maintenanceScheduleKeys.detail(id ?? ''),
    queryFn: () => maintenanceScheduleService.get(id as string),
    enabled: !!id,
    staleTime: 60 * 1000,
    gcTime: 5 * 60 * 1000,
  });
}

/** GET /api/fleet/fleets/{fleetId}/maintenance/upcoming */
export function useUpcomingMaintenanceQueue(fleetId: string | null | undefined) {
  return useQuery({
    queryKey: maintenanceScheduleKeys.queue({ fleetId: fleetId ?? '', state: 'upcoming' }),
    queryFn: () => maintenanceScheduleService.listUpcoming(fleetId as string),
    enabled: !!fleetId,
    staleTime: 60 * 1000,
    gcTime: 5 * 60 * 1000,
    select: (result) => result.data,
  });
}

/** GET /api/fleet/fleets/{fleetId}/maintenance/overdue */
export function useOverdueMaintenanceQueue(fleetId: string | null | undefined) {
  return useQuery({
    queryKey: maintenanceScheduleKeys.queue({ fleetId: fleetId ?? '', state: 'overdue' }),
    queryFn: () => maintenanceScheduleService.listOverdue(fleetId as string),
    enabled: !!fleetId,
    staleTime: 60 * 1000,
    gcTime: 5 * 60 * 1000,
    select: (result) => result.data,
  });
}

// ---------------------------------------------------------------------------
// Maintenance schedule mutations
// ---------------------------------------------------------------------------

/** POST /api/fleet/vehicles/{vehicleId}/maintenance-schedules */
export function useCreateMaintenanceSchedule(vehicleId: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (attrs: CreateMaintenanceScheduleAttributes) =>
      maintenanceScheduleService.createForVehicle(vehicleId, attrs),
    onSettled: () => {
      void queryClient.invalidateQueries({ queryKey: maintenanceScheduleKeys.lists() });
    },
  });
}

/** PATCH /api/fleet/maintenance-schedules/{id} */
export function useUpdateMaintenanceSchedule() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({
      id,
      attributes,
    }: {
      id: string;
      attributes: UpdateMaintenanceScheduleAttributes;
    }) => maintenanceScheduleService.patch(id, attributes),
    onSettled: (_data, _error, variables) => {
      void queryClient.invalidateQueries({ queryKey: maintenanceScheduleKeys.lists() });
      void queryClient.invalidateQueries({
        queryKey: maintenanceScheduleKeys.detail(variables.id),
      });
    },
  });
}

/** DELETE /api/fleet/maintenance-schedules/{id} */
export function useDeleteMaintenanceSchedule() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => maintenanceScheduleService.remove(id),
    onSettled: (_data, _error, id) => {
      void queryClient.invalidateQueries({ queryKey: maintenanceScheduleKeys.lists() });
      void queryClient.invalidateQueries({ queryKey: maintenanceScheduleKeys.detail(id) });
    },
  });
}

/**
 * POST /api/fleet/maintenance-schedules/{id}/complete
 *
 * On success invalidates:
 *  - maintenanceRecordKeys.lists() — a new record was created
 *  - maintenanceScheduleKeys.lists() — schedule state changes
 *  - vehicleKeys.detail(vehicleId) — vehicle status may change
 */
export function useCompleteMaintenanceSchedule(vehicleId: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({
      id,
      attributes,
    }: {
      id: string;
      attributes: CompleteMaintenanceScheduleAttributes;
    }) => maintenanceScheduleService.complete(id, attributes),
    onSettled: () => {
      void queryClient.invalidateQueries({ queryKey: maintenanceRecordKeys.lists() });
      void queryClient.invalidateQueries({ queryKey: maintenanceScheduleKeys.lists() });
      void queryClient.invalidateQueries({ queryKey: vehicleKeys.detail(vehicleId) });
    },
  });
}
