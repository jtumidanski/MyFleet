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
import { memberKeys } from './members';
import { fleetKeys } from './fleets';
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
 * Maps invite API failures to copy a person can act on (FR-UI-4).
 *
 * Kept here rather than in a shared error module: this is invite-specific copy,
 * and the two 429s need different sentences, which a generic status-to-string
 * map could not express.
 */
export function inviteErrorMessage(err: unknown, action: 'create' | 'resend'): string {
  const apiError = createErrorFromUnknown(err);
  if (apiError.status === 429) {
    return action === 'create'
      ? "You've sent too many invites today. Try again tomorrow."
      : 'You just resent this invite. Wait a few minutes before trying again.';
  }
  if (apiError.status === 409 && action === 'resend') {
    return 'That invite has already been accepted.';
  }
  return apiError.message || `Could not ${action} invite`;
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
 */
export function useAcceptInvite() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (token: string) => inviteService.acceptInvite(token),
    onSettled: () => {
      void queryClient.invalidateQueries({ queryKey: memberKeys.all });
      void queryClient.invalidateQueries({ queryKey: fleetKeys.all });
      void queryClient.invalidateQueries({ queryKey: inviteKeys.all });
    },
    onError: (err) => {
      const apiError = createErrorFromUnknown(err);
      toast.error(apiError.message || 'Could not accept invite');
    },
  });
}

/**
 * POST /api/fleet/fleets/{fleetId}/invites/{inviteId}/resend — owner-only.
 *
 * Invalidating the list is REQUIRED, not cosmetic: resend rotates the token, so
 * a stale cache would hand the copy-link button a dead token.
 */
export function useResendInvite(fleetId: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (inviteId: string) => inviteService.resendInvite(fleetId, inviteId),
    onSuccess: () => {
      toast.success('Invite resent');
    },
    onSettled: () => {
      void queryClient.invalidateQueries({ queryKey: inviteKeys.lists() });
    },
    onError: (err) => {
      toast.error(inviteErrorMessage(err, 'resend'));
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
