/**
 * React Query hooks for fleet membership (Task 15.8).
 *
 * Covers:
 *   - List members (GET /fleets/{id}/members)
 *   - Remove member (DELETE /fleets/{id}/members/{userId}) — owner-only
 *     Sole-owner guard: 409 response surfaced as toast.error
 */
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { toast } from 'sonner';
import { createErrorFromUnknown } from '@myfleet/shared-ts';
import { memberService } from '../../../services/api/MemberService';
import { fleetKeys } from './fleets';

// ---------------------------------------------------------------------------
// Query key factory
// ---------------------------------------------------------------------------

/**
 * Hierarchical query-key factory for members.
 *
 * all                -> ['members']
 * lists()            -> ['members', 'list']
 * list({ fleetId })  -> ['members', 'list', { fleetId }]
 */
export const memberKeys = {
  all: ['members'] as const,
  lists: () => [...memberKeys.all, 'list'] as const,
  list: (params: { fleetId: string }) => [...memberKeys.lists(), params] as const,
};

// ---------------------------------------------------------------------------
// Queries
// ---------------------------------------------------------------------------

/**
 * GET /api/fleet/fleets/{fleetId}/members
 */
export function useMembers(fleetId: string | null | undefined) {
  return useQuery({
    queryKey: memberKeys.list({ fleetId: fleetId ?? '' }),
    queryFn: () => memberService.listByFleet(fleetId as string),
    enabled: !!fleetId,
    staleTime: 60 * 1000,
    gcTime: 5 * 60 * 1000,
    select: (result) => result.data,
  });
}

// ---------------------------------------------------------------------------
// Mutations
// ---------------------------------------------------------------------------

/**
 * DELETE /api/fleet/fleets/{fleetId}/members/{userId} — owner-only.
 *
 * Invalidates memberKeys.lists() and fleetKeys.all on settle.
 * Sole-owner guard: HTTP 409 from the backend is surfaced as toast.error
 * with a human-readable message via createErrorFromUnknown.
 */
export function useRemoveMember(fleetId: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (userId: string) => memberService.removeMember(fleetId, userId),
    onSettled: () => {
      void queryClient.invalidateQueries({ queryKey: memberKeys.lists() });
      void queryClient.invalidateQueries({ queryKey: fleetKeys.all });
    },
    onError: (err) => {
      const apiError = createErrorFromUnknown(err);
      // HTTP 409 = sole-owner guard — surface with a clear message
      if (apiError.status === 409) {
        toast.error(
          'Cannot remove the sole owner of the fleet. Transfer ownership first, or delete the fleet.',
        );
      } else {
        toast.error(apiError.message || 'Could not remove member');
      }
    },
  });
}

// ---------------------------------------------------------------------------
// Invalidation helper
// ---------------------------------------------------------------------------

export function useInvalidateMembers() {
  const queryClient = useQueryClient();
  return {
    invalidateAll: () => queryClient.invalidateQueries({ queryKey: memberKeys.all }),
    invalidateLists: () => queryClient.invalidateQueries({ queryKey: memberKeys.lists() }),
  };
}
