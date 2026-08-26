import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { renderHook, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import type { ReactNode } from 'react';
import { createElement } from 'react';
import { toast } from 'sonner';
import { ApiError } from '@myfleet/shared-ts';
import { adminKeys, useCreatePurge } from './admin';

vi.mock('sonner', () => ({ toast: { error: vi.fn(), success: vi.fn() } }));

const createPurge = vi.fn();
vi.mock('../../../services/api/AdminService', () => ({
  adminService: {
    createPurge: (...args: unknown[]) => createPurge(...args),
  },
}));

describe('adminKeys', () => {
  it('is hierarchical, so a mutation can invalidate a whole subtree by prefix', () => {
    expect(adminKeys.all).toEqual(['admin']);
    expect(adminKeys.stats()).toEqual(['admin', 'stats']);
    expect(adminKeys.fleets()).toEqual(['admin', 'fleets']);
    expect(adminKeys.fleet('f1')).toEqual(['admin', 'fleets', 'detail', 'f1']);
    expect(adminKeys.purges()).toEqual(['admin', 'purges']);
    expect(adminKeys.purge('op-1')).toEqual(['admin', 'purges', 'detail', 'op-1']);
  });

  it('keeps list keys stable for the same params across calls', () => {
    const params = { q: '', deleted: 'include' as const, page: 1 };
    expect(adminKeys.fleetList(params)).toEqual(adminKeys.fleetList({ ...params }));
  });
});

describe('useCreatePurge', () => {
  let queryClient: QueryClient;

  function wrapper({ children }: { children: ReactNode }) {
    return createElement(QueryClientProvider, { client: queryClient }, children);
  }

  beforeEach(() => {
    queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
    });
    createPurge.mockReset();
    vi.mocked(toast.error).mockReset();
  });

  afterEach(() => {
    queryClient.clear();
  });

  it('invalidates the whole admin subtree after a successful create', async () => {
    createPurge.mockResolvedValue({ id: 'op-1', type: 'purge-operations', attributes: {} });
    const invalidate = vi.spyOn(queryClient, 'invalidateQueries');

    const { result } = renderHook(() => useCreatePurge(), { wrapper });
    result.current.mutate({ scope: 'fleet', target_id: 'f1', confirmation: 'Fleet' });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(invalidate).toHaveBeenCalledWith({ queryKey: adminKeys.all });
  });

  // The important one. A create that FAILS may still have stamped locally and
  // left the operation partial, so the queue and the counts must refetch — which
  // is why the invalidation is onSettled rather than onSuccess.
  it('invalidates even when the create fails', async () => {
    createPurge.mockRejectedValue(new Error('boom'));
    const invalidate = vi.spyOn(queryClient, 'invalidateQueries');

    const { result } = renderHook(() => useCreatePurge(), { wrapper });
    result.current.mutate({ scope: 'system', confirmation: 'nope' });

    await waitFor(() => expect(result.current.isError).toBe(true));
    expect(invalidate).toHaveBeenCalledWith({ queryKey: adminKeys.all });
  });

  // These two branches were unreachable until createErrorFromUnknown stopped
  // rebuilding an ApiError it was handed — the rebuild reset status to 0, so
  // every failure fell through to the generic message. ApiClient.request throws
  // an ApiError, which is exactly what the mutation catches here.
  it('reports a confirmation mismatch specifically on a 409', async () => {
    createPurge.mockRejectedValue(
      new ApiError(409, 'conflict', 'conflict', 'confirmation did not match'),
    );

    const { result } = renderHook(() => useCreatePurge(), { wrapper });
    result.current.mutate({ scope: 'system', confirmation: 'nope' });

    await waitFor(() => expect(result.current.isError).toBe(true));
    expect(toast.error).toHaveBeenCalledWith(
      'That confirmation did not match. Nothing was deleted.',
    );
  });

  it('reports revoked access specifically on a 403', async () => {
    createPurge.mockRejectedValue(new ApiError(403, 'forbidden', 'forbidden', 'not an admin'));

    const { result } = renderHook(() => useCreatePurge(), { wrapper });
    result.current.mutate({ scope: 'system', confirmation: 'nope' });

    await waitFor(() => expect(result.current.isError).toBe(true));
    expect(toast.error).toHaveBeenCalledWith(
      'Your platform-admin access has been revoked. Nothing was deleted.',
    );
  });
});
