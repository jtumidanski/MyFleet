import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { renderHook, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import type { ReactNode } from 'react';
import { createElement } from 'react';
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
});
