import { describe, it, expect, vi, beforeEach } from 'vitest';
import { renderHook, act, waitFor } from '@testing-library/react';
import { usePendingAttachments, MAX_ATTACHMENTS } from './usePendingAttachments';
import { mediaService } from '../../services/api/MediaService';
import { expectNoCall } from '../../test/expectNoCall';

vi.mock('../../services/api/MediaService', () => ({
  mediaService: {
    initUpload: vi.fn(),
    putContent: vi.fn(),
    confirm: vi.fn(),
    remove: vi.fn(),
  },
}));

function file(name: string): File {
  return new File(['x'], name, { type: 'application/pdf' });
}

function mockUploadSucceeds(mediaId: string) {
  vi.mocked(mediaService.initUpload).mockResolvedValue({
    id: mediaId,
    type: 'media-objects',
    attributes: { status: 'uploaded' },
  } as never);
  vi.mocked(mediaService.putContent).mockResolvedValue({} as never);
  vi.mocked(mediaService.confirm).mockResolvedValue({
    id: mediaId,
    type: 'media-objects',
    attributes: { status: 'ready' },
  } as never);
}

describe('usePendingAttachments', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(mediaService.remove).mockResolvedValue(undefined);
  });

  it('uploads an added file and exposes its media id once confirmed', async () => {
    mockUploadSucceeds('m1');
    const { result } = renderHook(() => usePendingAttachments());

    act(() => result.current.add([file('invoice.pdf')]));

    expect(result.current.items).toHaveLength(1);
    expect(result.current.isUploading).toBe(true);

    await waitFor(() => expect(result.current.isUploading).toBe(false));
    expect(result.current.items[0]!.status).toBe('ready');
    expect(result.current.mediaIds).toEqual(['m1']);
  });

  // A failed upload keeps its row with the filename and the reason and is
  // simply absent from mediaIds — the save proceeds with what succeeded
  // (PRD FR-DOC-4).
  it('keeps a failed upload visible and excludes it from mediaIds', async () => {
    vi.mocked(mediaService.initUpload).mockRejectedValue(new Error('boom'));
    const { result } = renderHook(() => usePendingAttachments());

    act(() => result.current.add([file('bad.pdf')]));

    await waitFor(() => expect(result.current.items[0]!.status).toBe('failed'));
    expect(result.current.items[0]!.file.name).toBe('bad.pdf');
    expect(result.current.items[0]!.error).toBeTruthy();
    expect(result.current.mediaIds).toEqual([]);
    expect(result.current.isUploading).toBe(false);
  });

  // Removing a pending attachment soft-deletes the uploaded media object so it
  // does not linger (PRD FR-DOC-2).
  it('soft-deletes the media object when a ready item is removed', async () => {
    mockUploadSucceeds('m1');
    const { result } = renderHook(() => usePendingAttachments());

    act(() => result.current.add([file('invoice.pdf')]));
    await waitFor(() => expect(result.current.mediaIds).toEqual(['m1']));

    act(() => result.current.remove(result.current.items[0]!.localId));

    expect(result.current.items).toHaveLength(0);
    expect(mediaService.remove).toHaveBeenCalledWith('m1');
  });

  it('does not call remove for an item that never uploaded', async () => {
    vi.mocked(mediaService.initUpload).mockRejectedValue(new Error('boom'));
    const { result } = renderHook(() => usePendingAttachments());

    act(() => result.current.add([file('bad.pdf')]));
    await waitFor(() => expect(result.current.items[0]!.status).toBe('failed'));

    act(() => result.current.remove(result.current.items[0]!.localId));
    await expectNoCall(vi.mocked(mediaService.remove), 'mediaService.remove');
  });

  // Abandoning the form soft-deletes everything uploaded but never attached
  // (PRD FR-DOC-3).
  it('deletes uncommitted media on unmount', async () => {
    mockUploadSucceeds('m1');
    const { result, unmount } = renderHook(() => usePendingAttachments());

    act(() => result.current.add([file('invoice.pdf')]));
    await waitFor(() => expect(result.current.mediaIds).toEqual(['m1']));

    unmount();
    expect(mediaService.remove).toHaveBeenCalledWith('m1');
  });

  // commit() disarms the unmount cleanup, so a successful save never deletes
  // the media it just attached.
  it('commit disarms the unmount cleanup and returns the media ids', async () => {
    mockUploadSucceeds('m1');
    const { result, unmount } = renderHook(() => usePendingAttachments());

    act(() => result.current.add([file('invoice.pdf')]));
    await waitFor(() => expect(result.current.mediaIds).toEqual(['m1']));

    let committed: string[] = [];
    act(() => {
      committed = result.current.commit();
    });
    expect(committed).toEqual(['m1']);

    unmount();
    await expectNoCall(vi.mocked(mediaService.remove), 'mediaService.remove');
  });

  it('stops accepting files at the cap', async () => {
    mockUploadSucceeds('m1');
    const { result } = renderHook(() => usePendingAttachments());

    const many = Array.from({ length: MAX_ATTACHMENTS + 3 }, (_, i) => file(`f${i}.pdf`));
    act(() => result.current.add(many));

    expect(result.current.items).toHaveLength(MAX_ATTACHMENTS);
    expect(result.current.isFull).toBe(true);

    // The MAX_ATTACHMENTS uploads kicked off above resolve as microtasks
    // after this point; let them settle before the test body returns so
    // their state updates land inside act() instead of firing after
    // teardown and printing act() warnings.
    await waitFor(() => expect(result.current.isUploading).toBe(false));
    expect(result.current.items).toHaveLength(MAX_ATTACHMENTS);
    expect(result.current.isFull).toBe(true);
  });
});
