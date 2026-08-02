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
import { memberKeys, useRemoveMember } from './members';
import { authKeys } from './auth';
import { refreshAccessToken } from '../../api/refresh';
import { inviteKeys, useCreateInvite, useRevokeInvite, useAcceptInvite } from './invites';
import { fleetKeys } from './fleets';
import { useRenameFleet } from './fleetSettings';

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
  },
}));

vi.mock('../../../services/api/InviteService', () => ({
  inviteService: {
    createInvite: vi.fn().mockResolvedValue({ id: 'inv-1', type: 'invites', attributes: {} }),
    revokeInvite: vi.fn().mockResolvedValue(undefined),
    acceptInvite: vi.fn().mockResolvedValue({ id: 'inv-1', type: 'invites', attributes: {} }),
  },
}));

vi.mock('../../api/refresh', () => ({
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
      result.current.mutate('user-1');
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
  it('useAcceptInvite mints a fresh token and refetches identity, in that order', async () => {
    const { result } = renderHook(() => useAcceptInvite(), {
      wrapper: makeWrapper(queryClient),
    });

    await act(async () => {
      result.current.mutate('tok-abc');
    });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(refreshAccessToken).toHaveBeenCalledTimes(1);

    const invalidateCalls = invalidateSpy.mock.calls as Array<[{ queryKey?: unknown }]>;
    const authCall = invalidateCalls.findIndex(
      (c) => JSON.stringify(c[0]?.queryKey) === JSON.stringify(authKeys.all),
    );
    expect(authCall).toBeGreaterThanOrEqual(0);

    // Fallbacks are deliberately order-violating so a missing call fails here
    // rather than silently passing the comparison.
    const refreshOrder = vi.mocked(refreshAccessToken).mock.invocationCallOrder[0] ?? Infinity;
    const authOrder = invalidateSpy.mock.invocationCallOrder[authCall] ?? -1;
    expect(refreshOrder).toBeLessThan(authOrder);
  });
});
