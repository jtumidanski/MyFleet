import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { toast } from 'sonner';
import { createErrorFromUnknown } from '@myfleet/shared-ts';
import { adminService } from '../../../services/api/AdminService';
import type { CreatePurgeInput, DeletedFilter } from '../../../types/models/admin';

/**
 * Query keys for the admin console, hierarchical so a mutation can invalidate a
 * whole subtree by prefix.
 */
export const adminKeys = {
  all: ['admin'] as const,
  stats: () => [...adminKeys.all, 'stats'] as const,
  fleets: () => [...adminKeys.all, 'fleets'] as const,
  fleetList: (params: { q: string; deleted: DeletedFilter; page: number }) =>
    [...adminKeys.fleets(), 'list', params] as const,
  fleet: (id: string) => [...adminKeys.fleets(), 'detail', id] as const,
  users: (params: { page: number }) => [...adminKeys.all, 'users', params] as const,
  purges: () => [...adminKeys.all, 'purges'] as const,
  purgeList: (params: { status: string; page: number }) =>
    [...adminKeys.purges(), 'list', params] as const,
  purge: (id: string) => [...adminKeys.purges(), 'detail', id] as const,
  audit: (params: { action: string; actor: string; page: number }) =>
    [...adminKeys.all, 'audit', params] as const,
};

/**
 * GET /api/fleet/admin/stats.
 *
 * staleTime is short: these counts sit next to destructive controls, and a
 * cached figure that predates a purge would understate what is about to be
 * deleted.
 */
export function useAdminStats() {
  return useQuery({
    queryKey: adminKeys.stats(),
    queryFn: () => adminService.stats(),
    staleTime: 30 * 1000,
  });
}

export function useAdminFleets(params: { q: string; deleted: DeletedFilter; page: number }) {
  return useQuery({
    queryKey: adminKeys.fleetList(params),
    queryFn: () =>
      adminService.listFleets({ q: params.q, deleted: params.deleted, page: params.page }),
    staleTime: 30 * 1000,
  });
}

export function useAdminFleet(id: string | undefined) {
  return useQuery({
    queryKey: adminKeys.fleet(id ?? ''),
    queryFn: () => adminService.getFleet(id as string),
    enabled: !!id,
    staleTime: 30 * 1000,
  });
}

export function useAdminUsers(params: { page: number }) {
  return useQuery({
    queryKey: adminKeys.users(params),
    queryFn: () => adminService.listUsers({ page: params.page }),
    staleTime: 30 * 1000,
  });
}

export function usePurgeOperations(params: { status: string; page: number }) {
  return useQuery({
    queryKey: adminKeys.purgeList(params),
    queryFn: () => adminService.listPurges({ status: params.status, page: params.page }),
    staleTime: 15 * 1000,
  });
}

export function usePurgeOperation(id: string | undefined) {
  return useQuery({
    queryKey: adminKeys.purge(id ?? ''),
    queryFn: () => adminService.getPurge(id as string),
    enabled: !!id,
  });
}

export function useAuditEvents(params: { action: string; actor: string; page: number }) {
  return useQuery({
    queryKey: adminKeys.audit(params),
    queryFn: () =>
      adminService.listAuditEvents({
        action: params.action,
        actor: params.actor,
        page: params.page,
      }),
    staleTime: 30 * 1000,
  });
}

/**
 * POST /api/fleet/admin/purge-operations.
 *
 * Invalidates broadly on SETTLE, not success. A purge changes fleets, stats,
 * the purge queue and the audit log at once — and a create that returns an
 * error may still have stamped locally and marked the operation partial, so the
 * queue must refetch either way. A stale count next to a destructive control is
 * worse than a redundant fetch.
 */
export function useCreatePurge() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (attrs: CreatePurgeInput) => adminService.createPurge(attrs),
    onSettled: () => {
      void queryClient.invalidateQueries({ queryKey: adminKeys.all });
    },
    onError: (err) => {
      const apiError = createErrorFromUnknown(err);
      // Both of these say "nothing was deleted" because on both paths the
      // server refused before writing anything. Saying so is the difference
      // between an operator retrying calmly and one assuming half a purge ran.
      if (apiError.status === 409) {
        toast.error('That confirmation did not match. Nothing was deleted.');
      } else if (apiError.status === 403) {
        toast.error('Your platform-admin access has been revoked. Nothing was deleted.');
      } else {
        toast.error(apiError.message || 'Could not start the purge');
      }
    },
  });
}

/**
 * DELETE /api/fleet/admin/purge-operations/{id} — cancel and restore.
 *
 * A 409 here means the operation was already reaped, which is the one state
 * this console cannot undo. The message says so plainly rather than offering
 * hope.
 */
export function useCancelPurge() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => adminService.cancelPurge(id),
    onSettled: () => {
      void queryClient.invalidateQueries({ queryKey: adminKeys.all });
    },
    onError: (err) => {
      const apiError = createErrorFromUnknown(err);
      if (apiError.status === 409) {
        toast.error('This purge has already been completed and cannot be undone.');
      } else {
        toast.error(apiError.message || 'Could not cancel the purge');
      }
    },
  });
}

/**
 * POST /api/fleet/admin/purge-operations/{id}/retry.
 *
 * Every downstream stamp is idempotent on purge_operation_id, so this is safe
 * to press repeatedly — which is exactly how the console presents it, and why
 * it needs no special error case.
 */
export function useRetryPurge() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => adminService.retryPurge(id),
    onSettled: () => {
      void queryClient.invalidateQueries({ queryKey: adminKeys.all });
    },
    onError: (err) => {
      const apiError = createErrorFromUnknown(err);
      toast.error(apiError.message || 'Could not retry the purge');
    },
  });
}
