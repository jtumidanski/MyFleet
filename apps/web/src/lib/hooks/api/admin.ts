import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { toast } from 'sonner';
import { createErrorFromUnknown } from '@myfleet/shared-ts';
import { adminService } from '../../../services/api/AdminService';
import type {
  CreatePurgeInput,
  DeletedFilter,
  TransferVehicleInput,
} from '../../../types/models/admin';

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
  transferPreview: (vehicleId: string, destinationFleetId: string) =>
    [...adminKeys.all, 'transfer-preview', vehicleId, destinationFleetId] as const,
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

/**
 * GET /api/fleet/admin/vehicles/{id}/transfer-preview.
 *
 * `enabled` is passed in rather than derived, because the dialog wants the
 * query to run only while it is open — a preview fetched behind a closed dialog
 * is wasted work whose result nobody reads. It is ANDed with a chosen
 * destination: the endpoint accepts an absent destination as a valid "not
 * chosen yet" state and answers with the source-side picture only, so firing
 * without one would spend a request on a half-answer the dialog cannot use.
 *
 * The destination is part of the key, so choosing a different one refetches
 * rather than reusing counts computed against the previous fleet.
 *
 * staleTime is 0, deliberately, unlike every other admin query here. These
 * counts sit directly above a confirmation input for an operation with no
 * one-click undo; a cached figure from thirty seconds ago is exactly the wrong
 * thing to show an operator about to type a phrase.
 */
export function useVehicleTransferPreview(
  vehicleId: string | undefined,
  destinationFleetId: string,
  enabled: boolean,
) {
  return useQuery({
    queryKey: adminKeys.transferPreview(vehicleId ?? '', destinationFleetId),
    queryFn: () => adminService.previewVehicleTransfer(vehicleId as string, destinationFleetId),
    enabled: enabled && !!vehicleId && !!destinationFleetId,
    staleTime: 0,
  });
}

/**
 * POST /api/fleet/admin/vehicles/{id}/transfer.
 *
 * Resolves to the whole `VehicleTransferResult` — `{ data, meta }` — rather
 * than just the resource. `meta.count_semantics` explains that the
 * `media_objects` and `notifications` figures are live rows now ON the
 * destination fleet, not rows this call moved; dropping it here would leave the
 * dialog presenting two numbers that mean something other than they appear to.
 *
 * Invalidates the WHOLE admin subtree on settle. A transfer changes the source
 * fleet's detail, the destination fleet's detail, both fleets' vehicle counts
 * in the list, the platform stats and the audit log — all at once. Naming them
 * individually would be a list to keep in sync forever, and a stale count in
 * this console is worse than a redundant fetch (FR-XFER-UI-6). On settle rather
 * than on success because a 503 means a downstream refused and the server rolled
 * back, which is still a round trip the cached picture should not be trusted
 * across.
 *
 * onError surfaces the server's `detail` VERBATIM, departing from
 * useCreatePurge's fixed strings. A purge has exactly one 409 meaning; a
 * transfer has four distinct 409/422 conditions whose whole value is the
 * specific sentence — "vehicle is pending purge" and "destination fleet is not
 * available" call for different actions from the operator (FR-XFER-UI-7).
 *
 * onSuccess shows a toast naming the destination. No other admin hook does,
 * and this one needs it: a purge lands the operator on a queue page that
 * confirms it happened, whereas a transfer closes a dialog over an unchanged
 * screen.
 */
export function useTransferVehicle() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (vars: {
      vehicleId: string;
      attributes: TransferVehicleInput;
      destinationName: string;
    }) => adminService.transferVehicle(vars.vehicleId, vars.attributes),
    onSuccess: (_data, vars) => {
      toast.success(`Vehicle transferred to ${vars.destinationName}.`);
    },
    onSettled: () => {
      void queryClient.invalidateQueries({ queryKey: adminKeys.all });
    },
    onError: (err) => {
      const apiError = createErrorFromUnknown(err);
      toast.error(apiError.detail || apiError.message || 'Could not transfer the vehicle');
    },
  });
}
