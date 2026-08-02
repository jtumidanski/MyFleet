import { describe, it, expect, vi, beforeEach } from 'vitest';
import { renderHook, waitFor } from '@testing-library/react';
import React from 'react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { userKeys, useUsers } from './users';
import { userService } from '../../../services/api/UserService';

vi.mock('../../../services/api/UserService', () => ({
  userService: { listByIds: vi.fn() },
}));

function makeWrapper(queryClient: QueryClient) {
  return function Wrapper({ children }: { children: React.ReactNode }) {
    return React.createElement(QueryClientProvider, { client: queryClient }, children);
  };
}

function newClient() {
  return new QueryClient({ defaultOptions: { queries: { retry: false, gcTime: 0 } } });
}

beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(userService.listByIds).mockResolvedValue({
    data: [
      {
        type: 'users',
        id: 'u1',
        attributes: {
          email: 'one@example.com',
          displayName: 'One',
          avatarUrl: '',
          themePreference: 'system',
        },
      },
    ],
    meta: undefined,
  });
});

describe('userKeys', () => {
  it('is hierarchical', () => {
    expect(userKeys.all).toEqual(['users']);
    expect(userKeys.byIds(['a', 'b'])).toEqual(['users', 'byIds', 'a,b']);
  });

  // The key must not depend on the order useMembers happens to return rows in,
  // or a reordered list refetches every render.
  it('is stable under reordering and duplication', () => {
    expect(userKeys.byIds(['b', 'a'])).toEqual(userKeys.byIds(['a', 'b']));
  });
});

describe('useUsers', () => {
  it('indexes the response by user id', async () => {
    const { result } = renderHook(() => useUsers(['u1']), { wrapper: makeWrapper(newClient()) });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data?.u1.displayName).toBe('One');
  });

  // Sorting and de-duping happen inside the hook, so callers can pass the raw
  // membership order without thinking about it.
  it('sorts and de-duplicates the ids it requests', async () => {
    const { result } = renderHook(() => useUsers(['u2', 'u1', 'u1']), {
      wrapper: makeWrapper(newClient()),
    });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(userService.listByIds).toHaveBeenCalledWith(['u1', 'u2']);
  });

  // An empty id list means "the member list has not arrived yet". Firing a
  // request for it would be a guaranteed 422 from the server.
  it('does not fire a request when there are no ids', () => {
    const { result } = renderHook(() => useUsers([]), { wrapper: makeWrapper(newClient()) });

    expect(userService.listByIds).not.toHaveBeenCalled();
    expect(result.current.fetchStatus).toBe('idle');
  });

  // FR-1.7: a failed name lookup must be a normal, renderable state, not a
  // thrown error — the member list still renders with id fallbacks.
  it('surfaces a failure as an error state with undefined data', async () => {
    vi.mocked(userService.listByIds).mockRejectedValue(new Error('boom'));

    const { result } = renderHook(() => useUsers(['u1']), { wrapper: makeWrapper(newClient()) });

    await waitFor(() => expect(result.current.isError).toBe(true));
    expect(result.current.data).toBeUndefined();
  });
});
