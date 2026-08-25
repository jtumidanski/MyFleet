import { describe, it, expect, vi, beforeEach } from 'vitest';
import type { MockInstance } from 'vitest';
import { renderHook, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import React from 'react';
import {
  useRemoveMaintenanceRecordDocument,
  useAppendMaintenanceRecordDocument,
  maintenanceRecordKeys,
} from './maintenance';
import { maintenanceRecordService } from '../../../services/api/MaintenanceRecordService';
import { mediaService } from '../../../services/api/MediaService';
import { expectNoCall } from '../../../test/expectNoCall';

vi.mock('../../../services/api/MaintenanceRecordService', () => ({
  maintenanceRecordService: {
    removeDocumentMedia: vi.fn(),
    appendDocumentMedia: vi.fn(),
  },
}));
vi.mock('../../../services/api/MediaService', () => ({
  mediaService: { remove: vi.fn() },
}));

function makeWrapper() {
  const qc = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  const invalidate = vi.spyOn(qc, 'invalidateQueries');
  const wrapper = ({ children }: { children: React.ReactNode }) => (
    <QueryClientProvider client={qc}>{children}</QueryClientProvider>
  );
  return { wrapper, invalidate };
}

/** The keys invalidateQueries was called with, flattened for easy matching. */
function invalidatedKeys(spy: MockInstance<QueryClient['invalidateQueries']>): string[] {
  return spy.mock.calls.map((call) => JSON.stringify((call[0] as { queryKey: unknown }).queryKey));
}

describe('useRemoveMaintenanceRecordDocument', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(maintenanceRecordService.removeDocumentMedia).mockResolvedValue(undefined);
    vi.mocked(mediaService.remove).mockResolvedValue(undefined);
  });

  it('detaches the reference first, then deletes the media object', async () => {
    const order: string[] = [];
    vi.mocked(maintenanceRecordService.removeDocumentMedia).mockImplementation(async () => {
      order.push('detach');
    });
    vi.mocked(mediaService.remove).mockImplementation(async () => {
      order.push('delete');
    });
    const { wrapper } = makeWrapper();

    const { result } = renderHook(() => useRemoveMaintenanceRecordDocument('veh-1'), { wrapper });
    await result.current.mutateAsync({ id: 'rec-1', mediaId: 'media-1' });

    expect(maintenanceRecordService.removeDocumentMedia).toHaveBeenCalledWith('rec-1', 'media-1');
    expect(mediaService.remove).toHaveBeenCalledWith('media-1');
    expect(order).toEqual(['detach', 'delete']);
  });

  // The reference is what the user can see. If the object delete fails the UI
  // is already correct and the orphan is media-service's purge to reap, so
  // reporting a failure here would be a lie the user cannot act on.
  it('reports success when the media object delete fails', async () => {
    vi.mocked(mediaService.remove).mockRejectedValue(new Error('gone'));
    const { wrapper } = makeWrapper();

    const { result } = renderHook(() => useRemoveMaintenanceRecordDocument('veh-1'), { wrapper });

    await expect(
      result.current.mutateAsync({ id: 'rec-1', mediaId: 'media-1' }),
    ).resolves.toBeUndefined();
  });

  it('does not delete the media object when the detach itself fails', async () => {
    vi.mocked(maintenanceRecordService.removeDocumentMedia).mockRejectedValue(new Error('404'));
    const { wrapper } = makeWrapper();

    const { result } = renderHook(() => useRemoveMaintenanceRecordDocument('veh-1'), { wrapper });

    await expect(result.current.mutateAsync({ id: 'rec-1', mediaId: 'media-1' })).rejects.toThrow();
    await expectNoCall(vi.mocked(mediaService.remove));
  });

  it('invalidates the record detail and the record lists', async () => {
    const { wrapper, invalidate } = makeWrapper();

    const { result } = renderHook(() => useRemoveMaintenanceRecordDocument('veh-1'), { wrapper });
    await result.current.mutateAsync({ id: 'rec-1', mediaId: 'media-1' });

    await waitFor(() => {
      const keys = invalidatedKeys(invalidate);
      expect(keys).toContain(JSON.stringify(maintenanceRecordKeys.detail('rec-1')));
      expect(keys).toContain(JSON.stringify(maintenanceRecordKeys.lists()));
    });
  });
});

describe('useAppendMaintenanceRecordDocument', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(maintenanceRecordService.appendDocumentMedia).mockResolvedValue({
      id: 'rec-1',
      type: 'maintenanceRecords',
      attributes: {},
    } as never);
  });

  // The records table renders its attachment-count affordance from the LIST
  // payload's documentMediaIds. Without this invalidation, attaching a file
  // during an edit leaves that count stale until something else refetches.
  it('invalidates the record lists so the attachment count stays truthful', async () => {
    const { wrapper, invalidate } = makeWrapper();

    const { result } = renderHook(() => useAppendMaintenanceRecordDocument('veh-1'), { wrapper });
    await result.current.mutateAsync({ id: 'rec-1', mediaId: 'media-1' });

    await waitFor(() => {
      expect(invalidatedKeys(invalidate)).toContain(JSON.stringify(maintenanceRecordKeys.lists()));
    });
  });
});
