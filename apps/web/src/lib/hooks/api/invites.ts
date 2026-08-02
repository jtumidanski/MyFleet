/**
 * React Query hooks for fleet invites (Task 15.8).
 *
 * Covers:
 *   - List invites   (GET  /fleets/{id}/invites)
 *   - Create invite  (POST /fleets/{id}/invites) — owner-only
 *   - Revoke invite  (DELETE /invites/{id})      — owner-only
 *   - Accept invite  (POST /invites/{token}/accept)
 */
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { toast } from 'sonner';
import { createErrorFromUnknown } from '@myfleet/shared-ts';
import { inviteService } from '../../../services/api/InviteService';
import { mintAccessToken } from '../../api/refresh';
import { memberKeys } from './members';
import { fleetKeys } from './fleets';
import { authKeys } from './auth';
import type { CreateInviteAttributes } from '../../../types/models/invite';

// ---------------------------------------------------------------------------
// Query key factory
// ---------------------------------------------------------------------------

/**
 * Hierarchical query-key factory for invites.
 *
 * all                -> ['invites']
 * lists()            -> ['invites', 'list']
 * list({ fleetId })  -> ['invites', 'list', { fleetId }]
 */
export const inviteKeys = {
  all: ['invites'] as const,
  lists: () => [...inviteKeys.all, 'list'] as const,
  list: (params: { fleetId: string }) => [...inviteKeys.lists(), params] as const,
};

// ---------------------------------------------------------------------------
// Queries
// ---------------------------------------------------------------------------

/**
 * GET /api/fleet/fleets/{fleetId}/invites
 */
export function useInvites(fleetId: string | null | undefined) {
  return useQuery({
    queryKey: inviteKeys.list({ fleetId: fleetId ?? '' }),
    queryFn: () => inviteService.listByFleet(fleetId as string),
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
 * POST /api/fleet/fleets/{fleetId}/invites — create invite (owner-only).
 * Invalidates inviteKeys.lists() on settle.
 */
export function useCreateInvite(fleetId: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (attrs: CreateInviteAttributes) => inviteService.createInvite(fleetId, attrs),
    onSettled: () => {
      void queryClient.invalidateQueries({ queryKey: inviteKeys.lists() });
    },
  });
}

/**
 * DELETE /api/fleet/invites/{id} — revoke invite (owner-only).
 * Invalidates inviteKeys.lists() on settle.
 */
export function useRevokeInvite() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => inviteService.revokeInvite(id),
    onSettled: () => {
      void queryClient.invalidateQueries({ queryKey: inviteKeys.lists() });
    },
    onError: (err) => {
      const apiError = createErrorFromUnknown(err);
      toast.error(apiError.message || 'Could not revoke invite');
    },
  });
}

/**
 * POST /api/fleet/invites/{token}/accept — accept an invite.
 * Invalidates memberKeys and fleetKeys so the new membership is reflected.
 *
 * Acceptance creates the membership server-side, but `active_fleet_id` and
 * `role` are JWT claims resolved at mint time — the token in hand still says
 * the caller has no fleet. onSuccess therefore mints a fresh token BEFORE
 * refetching identity, so `/auth/me` reports the new fleet and RequireAuth
 * stops bouncing the brand-new member to onboarding. Both awaits matter: the
 * mutation must not settle until the session reflects the membership, because
 * the accept page navigates as soon as it does.
 */
export function useAcceptInvite() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (token: string) => inviteService.acceptInvite(token),
    onSuccess: async () => {
      // mintAccessToken, NOT refreshAccessToken: the invite is already accepted
      // server-side and the current token is still valid, so a transient mint
      // failure must not clear it — that would log the user out on the very
      // path that just succeeded, with the invite spent.
      const token = await mintAccessToken();
      if (!token) {
        toast.error(
          'You joined the fleet, but your session could not be updated. Sign out and back in to see it.',
        );
        return;
      }
      await queryClient.invalidateQueries({ queryKey: authKeys.all });
    },
    onSettled: () => {
      void queryClient.invalidateQueries({ queryKey: memberKeys.all });
      void queryClient.invalidateQueries({ queryKey: fleetKeys.all });
      void queryClient.invalidateQueries({ queryKey: inviteKeys.all });
    },
    onError: (err) => {
      const apiError = createErrorFromUnknown(err);
      // Same precedence as InviteAcceptPage: `message` comes from the
      // envelope's `title`, which for every invite conflict is the literal
      // "conflict". `detail` is the only field that separates already-accepted,
      // expired and wrong-account, and this toast fires alongside the page's
      // own handler on the very same request — so both must prefer it or the
      // user reads "conflict" next to the real reason.
      toast.error(apiError.detail || apiError.message || 'Could not accept invite');
    },
  });
}

// ---------------------------------------------------------------------------
// Invalidation helper
// ---------------------------------------------------------------------------

export function useInvalidateInvites() {
  const queryClient = useQueryClient();
  return {
    invalidateAll: () => queryClient.invalidateQueries({ queryKey: inviteKeys.all }),
    invalidateLists: () => queryClient.invalidateQueries({ queryKey: inviteKeys.lists() }),
  };
}
