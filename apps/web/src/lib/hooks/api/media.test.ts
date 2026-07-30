import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { renderHook, waitFor } from '@testing-library/react';
import React from 'react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { ApiError } from '@myfleet/shared-ts';
import { mediaKeys, useMediaContentUrl } from './media';
import { performMediaUpload, MEDIA_MAX_UPLOAD_BYTES, MEDIA_TOO_LARGE_CODE } from './media';
import { mediaService } from '../../../services/api/MediaService';

// useMediaContentUrl goes through mediaService.getContentBlob; mock the
// module so no network call is needed and each test controls what blob
// resolves for a given id.
vi.mock('../../../services/api/MediaService', () => ({
  mediaService: {
    getContentBlob: vi.fn(),
  },
}));

describe('mediaKeys', () => {
  it('is hierarchical', () => {
    expect(mediaKeys.all).toEqual(['media']);
    expect(mediaKeys.detail('m1')).toEqual(['media', 'detail', 'm1']);
    expect(mediaKeys.content('m1')).toEqual(['media', 'content', 'm1']);
  });
});

describe('performMediaUpload', () => {
  beforeEach(() => vi.restoreAllMocks());

  it('calls init → putContent → confirm in order', async () => {
    const calls: string[] = [];

    const mockInit = vi.fn().mockImplementation(async () => {
      calls.push('init');
      return {
        id: 'm1',
        type: 'media-objects',
        attributes: {
          fleetId: 'f1',
          uploadedByUserId: 'u1',
          bucket: 'myfleet-media',
          objectKey: 'f1/m1/photo.jpg',
          status: 'uploaded',
        },
      };
    });

    const mockPut = vi.fn().mockImplementation(async () => {
      calls.push('put-content');
      return {
        id: 'm1',
        type: 'media-objects',
        attributes: { status: 'uploaded' },
      };
    });

    const mockConfirm = vi.fn().mockImplementation(async () => {
      calls.push('confirm');
      return {
        id: 'm1',
        type: 'media-objects',
        attributes: { status: 'processing' },
      };
    });

    const fakeFile = new File(['bytes'], 'photo.jpg', { type: 'image/jpeg' });

    const result = await performMediaUpload(fakeFile, {
      initUpload: mockInit,
      putContent: mockPut,
      confirm: mockConfirm,
    });

    expect(calls).toEqual(['init', 'put-content', 'confirm']);
    // The bytes go to the media row's own id — never to an external URL.
    expect(mockPut).toHaveBeenCalledWith('m1', fakeFile);
    expect(result.attributes.status).toBe('processing');
  });

  it('rejects an oversized file before any request, with a message naming the limit', async () => {
    const deps = {
      initUpload: vi.fn(),
      putContent: vi.fn(),
      confirm: vi.fn(),
    };

    const bigFile = new File(['x'], 'huge.jpg', { type: 'image/jpeg' });
    // File size is read-only in jsdom; define it rather than allocating 25 MiB.
    Object.defineProperty(bigFile, 'size', { value: MEDIA_MAX_UPLOAD_BYTES + 1 });

    await expect(performMediaUpload(bigFile, deps)).rejects.toMatchObject({
      status: 413,
      code: MEDIA_TOO_LARGE_CODE,
      // Naming the limit is the whole point: the server's 413 never does.
      message: expect.stringContaining('25 MB'),
    });

    // Nothing hit the network, so no orphaned media row was created either.
    expect(deps.initUpload).not.toHaveBeenCalled();
    expect(deps.putContent).not.toHaveBeenCalled();
    expect(deps.confirm).not.toHaveBeenCalled();
  });

  it('allows a file exactly at the limit', async () => {
    const media = { id: 'm1', type: 'media-objects', attributes: { status: 'uploaded' } };
    const deps = {
      initUpload: vi.fn().mockResolvedValue(media),
      putContent: vi.fn().mockResolvedValue(media),
      confirm: vi.fn().mockResolvedValue({ ...media, attributes: { status: 'processing' } }),
    };

    const file = new File(['x'], 'edge.jpg', { type: 'image/jpeg' });
    Object.defineProperty(file, 'size', { value: MEDIA_MAX_UPLOAD_BYTES });

    await expect(performMediaUpload(file, deps)).resolves.toBeDefined();
    expect(deps.initUpload).toHaveBeenCalledTimes(1);
  });
});

// ---------------------------------------------------------------------------
// useMediaContentUrl — must never resolve into a "no image" flash while the
// blob is available, and must never leak a createObjectURL allocation,
// including across React StrictMode's dev-only double-invocation.
// ---------------------------------------------------------------------------

// jsdom does not implement createObjectURL/revokeObjectURL; stub them so the
// hook has something to call, and so we can assert on call counts/args.
function stubObjectUrl() {
  let counter = 0;
  const createObjectURL = vi.fn(() => `blob:mock-${counter++}`);
  const revokeObjectURL = vi.fn();
  vi.stubGlobal('URL', Object.assign(URL, { createObjectURL, revokeObjectURL }));
  return { createObjectURL, revokeObjectURL };
}

function makeQueryWrapper(queryClient: QueryClient, strict: boolean) {
  return function Wrapper({ children }: { children: React.ReactNode }) {
    const tree = React.createElement(QueryClientProvider, { client: queryClient }, children);
    return strict ? React.createElement(React.StrictMode, null, tree) : tree;
  };
}

describe('useMediaContentUrl', () => {
  let queryClient: QueryClient;
  let createObjectURL: ReturnType<typeof stubObjectUrl>['createObjectURL'];
  let revokeObjectURL: ReturnType<typeof stubObjectUrl>['revokeObjectURL'];

  beforeEach(() => {
    vi.clearAllMocks();
    ({ createObjectURL, revokeObjectURL } = stubObjectUrl());
    queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('never reports settled (isLoading: false) with a null url once the blob has arrived — no "No image" flash', async () => {
    const blob = new Blob(['bytes'], { type: 'image/jpeg' });
    vi.mocked(mediaService.getContentBlob).mockResolvedValue(blob);

    // MediaThumbnail renders a skeleton while isLoading, "No image" once
    // settled with a null url, or the <img> once settled with a url. The
    // defect this hook exists to avoid is a render that is settled (not
    // loading) but still has a null url — that renders "No image" for a
    // frame before the real image. Record every render to catch it even if
    // it only happens transiently (e.g. under StrictMode).
    const renders: Array<{ isLoading: boolean; url: string | null }> = [];
    function useTracked() {
      const value = useMediaContentUrl('m1');
      renders.push({ isLoading: value.isLoading, url: value.url });
      return value;
    }

    const { result } = renderHook(() => useTracked(), {
      wrapper: makeQueryWrapper(queryClient, false),
    });

    await waitFor(() => expect(result.current.url).not.toBeNull());

    expect(renders.some((r) => !r.isLoading && r.url === null)).toBe(false);
  });

  it('surfaces isError/error so a failed fetch is distinguishable from "no image"', async () => {
    const failure = new ApiError(403, 'forbidden', 'Forbidden');
    vi.mocked(mediaService.getContentBlob).mockRejectedValue(failure);

    const { result } = renderHook(() => useMediaContentUrl('m1'), {
      wrapper: makeQueryWrapper(queryClient, false),
    });

    await waitFor(() => expect(result.current.isError).toBe(true));

    expect(result.current.url).toBeNull();
    expect(result.current.isLoading).toBe(false);
    expect(result.current.error).toBe(failure);
  });

  it('revokes the previous URL exactly once when the id changes, and the new URL survives', async () => {
    const blob1 = new Blob(['one'], { type: 'image/jpeg' });
    const blob2 = new Blob(['two'], { type: 'image/jpeg' });
    vi.mocked(mediaService.getContentBlob).mockImplementation(async (id: string) =>
      id === 'm1' ? blob1 : blob2,
    );

    const { result, rerender } = renderHook<ReturnType<typeof useMediaContentUrl>, { id: string }>(
      ({ id }) => useMediaContentUrl(id),
      {
        wrapper: makeQueryWrapper(queryClient, false),
        initialProps: { id: 'm1' },
      },
    );

    await waitFor(() => expect(result.current.url).not.toBeNull());
    const firstUrl = result.current.url;

    rerender({ id: 'm2' });

    await waitFor(() => {
      expect(result.current.url).not.toBeNull();
      expect(result.current.url).not.toBe(firstUrl);
    });
    const secondUrl = result.current.url;

    expect(revokeObjectURL).toHaveBeenCalledWith(firstUrl);
    expect(revokeObjectURL).toHaveBeenCalledTimes(1);
    expect(revokeObjectURL).not.toHaveBeenCalledWith(secondUrl);
  });

  it('revokes the URL exactly once on unmount', async () => {
    const blob = new Blob(['bytes'], { type: 'image/jpeg' });
    vi.mocked(mediaService.getContentBlob).mockResolvedValue(blob);

    const { result, unmount } = renderHook(() => useMediaContentUrl('m1'), {
      wrapper: makeQueryWrapper(queryClient, false),
    });

    await waitFor(() => expect(result.current.url).not.toBeNull());
    const url = result.current.url;

    unmount();

    expect(revokeObjectURL).toHaveBeenCalledWith(url);
    expect(revokeObjectURL).toHaveBeenCalledTimes(1);
  });

  it('does not leak an object URL under React StrictMode double-invocation — every create is matched by a revoke', async () => {
    const blob = new Blob(['bytes'], { type: 'image/jpeg' });
    vi.mocked(mediaService.getContentBlob).mockResolvedValue(blob);

    const { result, unmount } = renderHook(() => useMediaContentUrl('m1'), {
      wrapper: makeQueryWrapper(queryClient, true), // <React.StrictMode>
    });

    await waitFor(() => expect(result.current.url).not.toBeNull());

    unmount();

    // StrictMode's dev-only mount → cleanup → mount replay may create and
    // revoke more than one intermediate URL (create → revoke → create), but
    // every create must have a matching revoke by the time everything has
    // settled and unmounted — no orphaned allocation.
    expect(createObjectURL.mock.calls.length).toBeGreaterThan(0);
    expect(revokeObjectURL.mock.calls.length).toBe(createObjectURL.mock.calls.length);
  });
});
