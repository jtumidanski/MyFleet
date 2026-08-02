/**
 * Task 15.8 — Fleet settings, member management, invite hooks tests.
 *
 * REQUIRED TEST: settings hooks invalidate fleet/member keys on success.
 * Tests verify that:
 *   1. memberKeys factory is hierarchical
 *   2. inviteKeys factory is hierarchical
 *   3. useRemoveMember invalidates memberKeys.lists() AND fleetKeys.all
 *   4. useCreateInvite invalidates inviteKeys.lists()
 *   5. useRenameFleet invalidates fleetKeys.all AND memberKeys.all
 *   6. useRevokeInvite invalidates inviteKeys.lists()
 *   7. useAcceptInvite invalidates memberKeys.all, fleetKeys.all, inviteKeys.all
 *
 * Invalidation tests render the real hooks with a real QueryClient so that
 * removing an invalidation call from the hook source WILL break the test.
 */
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { renderHook, act, waitFor } from '@testing-library/react';
import React from 'react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { memberKeys, useRemoveMember, useUpdateMemberRole } from './members';
import { authKeys } from './auth';
import { mintAccessToken } from '../../api/refresh';
import { toast } from 'sonner';
import { inviteKeys, useCreateInvite, useRevokeInvite, useAcceptInvite } from './invites';
import { fleetKeys } from './fleets';
import { useRenameFleet } from './fleetSettings';
import { userKeys } from './users';
import { memberService } from '../../../services/api/MemberService';

// ---------------------------------------------------------------------------
// Key factory hierarchy
// ---------------------------------------------------------------------------

describe('memberKeys', () => {
  it('is hierarchical', () => {
    expect(memberKeys.all).toEqual(['members']);
    expect(memberKeys.lists()).toEqual(['members', 'list']);
    expect(memberKeys.list({ fleetId: 'f1' })).toEqual(['members', 'list', { fleetId: 'f1' }]);
  });
});

describe('inviteKeys', () => {
  it('is hierarchical', () => {
    expect(inviteKeys.all).toEqual(['invites']);
    expect(inviteKeys.lists()).toEqual(['invites', 'list']);
    expect(inviteKeys.list({ fleetId: 'f1' })).toEqual(['invites', 'list', { fleetId: 'f1' }]);
  });
});

// ---------------------------------------------------------------------------
// Real invalidation tests — render actual hooks; mocking only the service.
//
// These tests will FAIL if the invalidation calls are removed from the hooks.
// ---------------------------------------------------------------------------

// Mock service modules so no network is needed; return minimal valid responses.
vi.mock('../../../services/api/MemberService', () => ({
  memberService: {
    removeMember: vi.fn().mockResolvedValue(undefined),
    updateRole: vi.fn().mockResolvedValue({
      id: 'm1',
      type: 'memberships',
      attributes: { fleetId: 'f1', userId: 'user-1', role: 'owner', status: 'active' },
    }),
  },
}));

vi.mock('../../../services/api/InviteService', () => ({
  inviteService: {
    createInvite: vi.fn().mockResolvedValue({ id: 'inv-1', type: 'invites', attributes: {} }),
    revokeInvite: vi.fn().mockResolvedValue(undefined),
    acceptInvite: vi.fn().mockResolvedValue({ id: 'inv-1', type: 'invites', attributes: {} }),
  },
}));

// Both exports: the API client imports refreshAccessToken from this module, so
// a partial mock would break its import.
vi.mock('../../api/refresh', () => ({
  mintAccessToken: vi.fn().mockResolvedValue('fresh-token'),
  refreshAccessToken: vi.fn().mockResolvedValue('fresh-token'),
}));

vi.mock('../../../services/api/FleetSettingsService', () => ({
  fleetSettingsService: {
    rename: vi.fn().mockResolvedValue({ id: 'f1', type: 'fleets', attributes: { name: 'New' } }),
  },
}));

// Silence sonner toast calls in tests.
vi.mock('sonner', () => ({
  toast: { success: vi.fn(), error: vi.fn() },
}));

// Silence createErrorFromUnknown for test purposes; preserve all other exports.
vi.mock('@myfleet/shared-ts', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@myfleet/shared-ts')>();
  return {
    ...actual,
    createErrorFromUnknown: vi.fn((e: unknown) => ({
      message: String(e),
      status: 500,
    })),
  };
});

function makeWrapper(queryClient: QueryClient) {
  return function Wrapper({ children }: { children: React.ReactNode }) {
    return React.createElement(QueryClientProvider, { client: queryClient }, children);
  };
}

describe('mutation invalidation contracts — real hooks', () => {
  let queryClient: QueryClient;
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  let invalidateSpy: ReturnType<typeof vi.spyOn<any, any>>;

  beforeEach(() => {
    vi.clearAllMocks();
    queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
    });
    invalidateSpy = vi.spyOn(queryClient, 'invalidateQueries');
  });

  // --------------------------------------------------------------------------
  // useRemoveMember
  // --------------------------------------------------------------------------
  it('useRemoveMember invalidates memberKeys.lists() and fleetKeys.all', async () => {
    const { result } = renderHook(() => useRemoveMember('f1'), {
      wrapper: makeWrapper(queryClient),
    });

    await act(async () => {
      result.current.mutate({ userId: 'user-1', isSelf: false });
    });

    await waitFor(() =>
      expect(result.current.isIdle || result.current.isSuccess || result.current.isError).toBe(
        true,
      ),
    );

    // Assert both invalidation calls were made with the correct query keys.
    const calls = invalidateSpy.mock.calls.map((c) => c[0]);
    expect(calls).toContainEqual(expect.objectContaining({ queryKey: memberKeys.lists() }));
    expect(calls).toContainEqual(expect.objectContaining({ queryKey: fleetKeys.all }));
  });

  // FR-4.1: role and active_fleet_id are JWT claims minted at login. After the
  // actor's OWN membership disappears, the token still claims the fleet, so the
  // SPA must re-mint before /auth/me is believed.
  //
  // mintAccessToken, NOT refreshAccessToken: the removal already committed
  // server-side, so a transient mint failure must not clear a still-valid token
  // and log the user out of a session they are mid-way through leaving. Same
  // reasoning as useAcceptInvite.
  it('useRemoveMember mints a fresh token and refetches identity on a self-leave', async () => {
    const { result } = renderHook(() => useRemoveMember('f1'), {
      wrapper: makeWrapper(queryClient),
    });

    await act(async () => {
      result.current.mutate({ userId: 'me', isSelf: true });
    });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(mintAccessToken).toHaveBeenCalledTimes(1);

    const calls = invalidateSpy.mock.calls.map((c) => c[0]);
    expect(calls).toContainEqual(expect.objectContaining({ queryKey: authKeys.all }));
  });

  // FR-4.4: removing SOMEONE ELSE leaves the actor's own claims untouched, so
  // re-minting would be a pointless round trip on every removal.
  it('useRemoveMember does not mint a token when removing another member', async () => {
    const { result } = renderHook(() => useRemoveMember('f1'), {
      wrapper: makeWrapper(queryClient),
    });

    await act(async () => {
      result.current.mutate({ userId: 'someone-else', isSelf: false });
    });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(mintAccessToken).not.toHaveBeenCalled();

    const calls = invalidateSpy.mock.calls.map((c) => c[0]);
    expect(calls).not.toContainEqual(expect.objectContaining({ queryKey: authKeys.all }));
  });

  // FR-4.3: names are keyed off the member list, so a membership change must
  // drop the cached name map too.
  it('useRemoveMember invalidates the user name cache', async () => {
    const { result } = renderHook(() => useRemoveMember('f1'), {
      wrapper: makeWrapper(queryClient),
    });

    await act(async () => {
      result.current.mutate({ userId: 'user-1', isSelf: false });
    });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    const calls = invalidateSpy.mock.calls.map((c) => c[0]);
    expect(calls).toContainEqual(expect.objectContaining({ queryKey: userKeys.all }));
  });

  it('useUpdateMemberRole PATCHes the role and invalidates members and fleets', async () => {
    const { result } = renderHook(() => useUpdateMemberRole('f1'), {
      wrapper: makeWrapper(queryClient),
    });

    await act(async () => {
      result.current.mutate({ userId: 'user-1', role: 'owner' });
    });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(memberService.updateRole).toHaveBeenCalledWith('f1', 'user-1', 'owner');

    const calls = invalidateSpy.mock.calls.map((c) => c[0]);
    expect(calls).toContainEqual(expect.objectContaining({ queryKey: memberKeys.lists() }));
    expect(calls).toContainEqual(expect.objectContaining({ queryKey: fleetKeys.all }));
  });

  // FR-4.4 again: promoting someone else does not touch the actor's claims.
  it('useUpdateMemberRole does not mint a token', async () => {
    const { result } = renderHook(() => useUpdateMemberRole('f1'), {
      wrapper: makeWrapper(queryClient),
    });

    await act(async () => {
      result.current.mutate({ userId: 'user-1', role: 'owner' });
    });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(mintAccessToken).not.toHaveBeenCalled();
  });

  // --------------------------------------------------------------------------
  // useRenameFleet (in fleetSettings.ts)
  // --------------------------------------------------------------------------
  it('useRenameFleet invalidates fleetKeys.all and memberKeys.all', async () => {
    const { result } = renderHook(() => useRenameFleet('f1'), {
      wrapper: makeWrapper(queryClient),
    });

    await act(async () => {
      result.current.mutate('New Name');
    });

    await waitFor(() =>
      expect(result.current.isIdle || result.current.isSuccess || result.current.isError).toBe(
        true,
      ),
    );

    const calls = invalidateSpy.mock.calls.map((c) => c[0]);
    expect(calls).toContainEqual(expect.objectContaining({ queryKey: fleetKeys.all }));
    expect(calls).toContainEqual(expect.objectContaining({ queryKey: memberKeys.all }));
  });

  // --------------------------------------------------------------------------
  // useCreateInvite
  // --------------------------------------------------------------------------
  it('useCreateInvite invalidates inviteKeys.lists()', async () => {
    const { result } = renderHook(() => useCreateInvite('f1'), {
      wrapper: makeWrapper(queryClient),
    });

    await act(async () => {
      result.current.mutate({ email: 'a@b.com', role: 'member' });
    });

    await waitFor(() =>
      expect(result.current.isIdle || result.current.isSuccess || result.current.isError).toBe(
        true,
      ),
    );

    const calls = invalidateSpy.mock.calls.map((c) => c[0]);
    expect(calls).toContainEqual(expect.objectContaining({ queryKey: inviteKeys.lists() }));
  });

  // --------------------------------------------------------------------------
  // useRevokeInvite
  // --------------------------------------------------------------------------
  it('useRevokeInvite invalidates inviteKeys.lists()', async () => {
    const { result } = renderHook(() => useRevokeInvite(), {
      wrapper: makeWrapper(queryClient),
    });

    await act(async () => {
      result.current.mutate('inv-1');
    });

    await waitFor(() =>
      expect(result.current.isIdle || result.current.isSuccess || result.current.isError).toBe(
        true,
      ),
    );

    const calls = invalidateSpy.mock.calls.map((c) => c[0]);
    expect(calls).toContainEqual(expect.objectContaining({ queryKey: inviteKeys.lists() }));
  });

  // --------------------------------------------------------------------------
  // useAcceptInvite
  // --------------------------------------------------------------------------
  it('useAcceptInvite invalidates memberKeys.all, fleetKeys.all, and inviteKeys.all', async () => {
    const { result } = renderHook(() => useAcceptInvite(), {
      wrapper: makeWrapper(queryClient),
    });

    await act(async () => {
      result.current.mutate('tok-abc');
    });

    await waitFor(() =>
      expect(result.current.isIdle || result.current.isSuccess || result.current.isError).toBe(
        true,
      ),
    );

    const calls = invalidateSpy.mock.calls.map((c) => c[0]);
    expect(calls).toContainEqual(expect.objectContaining({ queryKey: memberKeys.all }));
    expect(calls).toContainEqual(expect.objectContaining({ queryKey: fleetKeys.all }));
    expect(calls).toContainEqual(expect.objectContaining({ queryKey: inviteKeys.all }));
  });

  // The JWT still carries the pre-accept active_fleet_id claim (empty for a
  // first-time invitee), and /auth/me reports that claim. Without a fresh token
  // the new member is bounced back to onboarding by RequireAuth.
  //
  // This asserts the mint COMPLETES before the identity refetch starts, not
  // merely that it was called first: invocationCallOrder records call time, so
  // an un-awaited mint would satisfy an ordering-only assertion while still
  // racing /auth/me against the token write.
  it('useAcceptInvite waits for the fresh token before refetching identity', async () => {
    let releaseMint: ((token: string) => void) | undefined;
    vi.mocked(mintAccessToken).mockImplementationOnce(
      () =>
        new Promise<string | null>((resolve) => {
          releaseMint = resolve;
        }),
    );

    const { result } = renderHook(() => useAcceptInvite(), {
      wrapper: makeWrapper(queryClient),
    });

    await act(async () => {
      result.current.mutate('tok-abc');
    });

    await waitFor(() => expect(mintAccessToken).toHaveBeenCalledTimes(1));

    const authInvalidations = () =>
      (invalidateSpy.mock.calls as Array<[{ queryKey?: unknown }]>).filter(
        (c) => JSON.stringify(c[0]?.queryKey) === JSON.stringify(authKeys.all),
      ).length;

    // Mint is still pending: identity must not have been refetched yet.
    expect(authInvalidations()).toBe(0);

    await act(async () => {
      releaseMint?.('fresh-token');
    });

    await waitFor(() => expect(authInvalidations()).toBe(1));
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
  });

  // A failed mint means stale claims, not a dead session — the invite is
  // already spent server-side. Clearing the still-valid token here (which
  // refreshAccessToken would do) logs the user out on the success path.
  it('useAcceptInvite still succeeds, without refetching identity, when the mint fails', async () => {
    vi.mocked(mintAccessToken).mockResolvedValueOnce(null);

    const { result } = renderHook(() => useAcceptInvite(), {
      wrapper: makeWrapper(queryClient),
    });

    await act(async () => {
      result.current.mutate('tok-abc');
    });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    const calls = invalidateSpy.mock.calls.map((c) => c[0]);
    expect(calls).not.toContainEqual(expect.objectContaining({ queryKey: authKeys.all }));
    expect(toast.error).toHaveBeenCalled();
  });
});
