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
import { mintAccessToken } from '../../api/refresh';
import { authKeys } from './auth';
import { userKeys } from './users';
import type { FleetRole } from '../../../types/models/user';

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
 * DELETE /api/fleet/fleets/{fleetId}/members/{userId}.
 *
 * Removing ANOTHER member is owner-only; removing YOURSELF is allowed for every
 * role (FR-3.1). `isSelf` is a mutation variable rather than something the hook
 * re-derives, because it decides two things at once — whether to re-mint the
 * session, and (server-side) whether the removal is logged as member.left or
 * member.removed. Re-deriving it here would create a second source of truth.
 *
 * Sole-owner guard: HTTP 409 is surfaced as toast.error.
 */
export function useRemoveMember(fleetId: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async ({ userId, isSelf }: { userId: string; isSelf: boolean }) => {
      await memberService.removeMember(fleetId, userId);
      return { isSelf };
    },
    onSuccess: async ({ isSelf }) => {
      if (!isSelf) return;
      // FR-4.1. active_fleet_id and role are JWT claims fixed at mint time,
      // so the token in hand still claims a fleet the user just left.
      //
      // mintAccessToken, NOT refreshAccessToken: the removal already
      // committed server-side, so a transient mint failure must not clear a
      // still-valid token — that would log the user out on the path that just
      // succeeded. Same reasoning as useAcceptInvite.
      const token = await mintAccessToken();
      if (!token) {
        toast.error(
          'You left the fleet, but your session could not be updated. Sign out and back in to see it.',
        );
        return;
      }
      // Refetching /auth/me is what routes the user onward: the refreshed
      // token resolves to an empty active_fleet_id and RequireAuth redirects
      // to onboarding on its own. A manual navigate() here would be a second,
      // racing source of truth for the same decision.
      await queryClient.invalidateQueries({ queryKey: authKeys.all });
    },
    onSettled: () => {
      void queryClient.invalidateQueries({ queryKey: memberKeys.lists() });
      void queryClient.invalidateQueries({ queryKey: fleetKeys.all });
      // Names are keyed off the member id set, so they go stale with it.
      void queryClient.invalidateQueries({ queryKey: userKeys.all });
    },
    onError: (err) => {
      const apiError = createErrorFromUnknown(err);
      // HTTP 409 = sole-owner guard. The new UI prevents reaching it in the
      // normal flow, but the endpoint still returns it and a stale member list
      // can still get here.
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

/**
 * PATCH /api/fleet/fleets/{fleetId}/members/{userId} — change a member's role.
 * Owner-only, enforced server-side at both the token and database layers.
 *
 * No session refresh (FR-4.4): the actor's own claims are untouched. The
 * PROMOTED user's token still says `member` until their next mint, which fails
 * CLOSED — they gain owner powers on their next refresh.
 */
export function useUpdateMemberRole(fleetId: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ userId, role }: { userId: string; role: FleetRole }) =>
      memberService.updateRole(fleetId, userId, role),
    onSettled: () => {
      void queryClient.invalidateQueries({ queryKey: memberKeys.lists() });
      void queryClient.invalidateQueries({ queryKey: fleetKeys.all });
    },
    onError: (err) => {
      const apiError = createErrorFromUnknown(err);
      if (apiError.status === 409) {
        toast.error('This fleet would be left with no owner. Promote someone else first.');
      } else {
        toast.error(apiError.message || 'Could not change this member’s role');
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
