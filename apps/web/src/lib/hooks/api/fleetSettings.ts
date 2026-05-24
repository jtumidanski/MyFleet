/**
 * React Query hooks for fleet settings (Task 15.8).
 *
 * Covers:
 *   - Get fleet details (GET /fleets/{id})
 *   - Rename fleet     (PATCH /fleets/{id}) — owner-only
 */
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { toast } from 'sonner';
import { createErrorFromUnknown } from '@myfleet/shared-ts';
import { fleetSettingsService } from '../../../services/api/FleetSettingsService';
import { fleetKeys } from './fleets';
import { memberKeys } from './members';

// ---------------------------------------------------------------------------
// Query
// ---------------------------------------------------------------------------

/**
 * GET /api/fleet/fleets/{fleetId}
 */
export function useFleet(fleetId: string | null | undefined) {
  return useQuery({
    queryKey: fleetKeys.detail(fleetId ?? ''),
    queryFn: () => fleetSettingsService.getFleet(fleetId as string),
    enabled: !!fleetId,
    staleTime: 5 * 60 * 1000,
    gcTime: 10 * 60 * 1000,
  });
}

// ---------------------------------------------------------------------------
// Mutations
// ---------------------------------------------------------------------------

/**
 * PATCH /api/fleet/fleets/{fleetId} — rename the fleet (owner-only).
 *
 * On settle invalidates fleetKeys.all and memberKeys.all (rename affects
 * member views that display the fleet name).
 */
export function useRenameFleet(fleetId: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (name: string) => fleetSettingsService.rename(fleetId, name),
    onSettled: () => {
      void queryClient.invalidateQueries({ queryKey: fleetKeys.all });
      void queryClient.invalidateQueries({ queryKey: memberKeys.all });
    },
    onSuccess: () => {
      toast.success('Fleet name updated');
    },
    onError: (err) => {
      const apiError = createErrorFromUnknown(err);
      toast.error(apiError.message || 'Could not rename fleet');
    },
  });
}
