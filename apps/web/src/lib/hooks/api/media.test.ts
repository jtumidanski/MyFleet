import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { renderHook, waitFor } from '@testing-library/react';
import React from 'react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { mediaKeys, useMediaContentUrl } from './media';
import { performMediaUpload } from './media';
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
});

// ---------------------------------------------------------------------------
// useMediaContentUrl — object URL derivation must not flash a placeholder,
// and must never leak a createObjectURL allocation, including across
// React StrictMode's dev-only double-invocation.
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

  it('exposes the object URL on the same render the blob first resolves — no null-then-value flash', async () => {
    const blob = new Blob(['bytes'], { type: 'image/jpeg' });
    vi.mocked(mediaService.getContentBlob).mockResolvedValue(blob);

    // Record (isLoading, url) on every render, including any React internally
    // re-runs (e.g. StrictMode). If the fix regressed to state+effect, there
    // would be a render with isLoading: false and url: null before a later
    // render fills it in.
    const renders: Array<{ isLoading: boolean; url: string | null }> = [];
    function useTracked() {
      const value = useMediaContentUrl('m1');
      renders.push({ isLoading: value.isLoading, url: value.url });
      return value;
    }

    const { result, unmount } = renderHook(() => useTracked(), {
      wrapper: makeQueryWrapper(queryClient, false),
    });

    await waitFor(() => expect(result.current.isLoading).toBe(false));

    expect(renders.some((r) => !r.isLoading && r.url === null)).toBe(false);
    const settled = renders.find((r) => !r.isLoading);
    expect(settled?.url).not.toBeNull();

    // Drain the deferred revoke before the test ends so it can't fire during
    // (and pollute the mock-call-count assertions of) a later test — the
    // object URL cache this hook uses is a module-level singleton shared by
    // every test in this file.
    const settledUrl = settled?.url ?? null;
    unmount();
    if (settledUrl) {
      await waitFor(() => expect(revokeObjectURL).toHaveBeenCalledWith(settledUrl));
    }
  });

  it('revokes the previous URL exactly once when the id changes, and the new URL survives', async () => {
    const blob1 = new Blob(['one'], { type: 'image/jpeg' });
    const blob2 = new Blob(['two'], { type: 'image/jpeg' });
    vi.mocked(mediaService.getContentBlob).mockImplementation(async (id: string) =>
      id === 'm1' ? blob1 : blob2,
    );

    const { result, rerender, unmount } = renderHook<
      ReturnType<typeof useMediaContentUrl>,
      { id: string }
    >(({ id }) => useMediaContentUrl(id), {
      wrapper: makeQueryWrapper(queryClient, false),
      initialProps: { id: 'm1' },
    });

    await waitFor(() => expect(result.current.url).not.toBeNull());
    const firstUrl = result.current.url;

    rerender({ id: 'm2' });

    // Changing id points at a different query key entirely, so there is a
    // legitimate brief isLoading state while the new blob is fetched (unlike
    // the same-id flash the fix targets). Wait past that for the new URL.
    await waitFor(() => {
      expect(result.current.url).not.toBeNull();
      expect(result.current.url).not.toBe(firstUrl);
    });
    const secondUrl = result.current.url;

    // Revocation of the old URL is deferred a macrotask; waitFor polls past it.
    await waitFor(() => expect(revokeObjectURL).toHaveBeenCalledWith(firstUrl));
    expect(revokeObjectURL).toHaveBeenCalledTimes(1);
    expect(revokeObjectURL).not.toHaveBeenCalledWith(secondUrl);

    // Drain the second URL's revoke too so nothing is left pending for a
    // later test (see comment in the first test in this describe block).
    unmount();
    await waitFor(() => expect(revokeObjectURL).toHaveBeenCalledWith(secondUrl));
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

    await waitFor(() => expect(revokeObjectURL).toHaveBeenCalledWith(url));
    expect(revokeObjectURL).toHaveBeenCalledTimes(1);
  });

  it('survives React StrictMode double-invocation without leaking or prematurely revoking', async () => {
    const blob = new Blob(['bytes'], { type: 'image/jpeg' });
    vi.mocked(mediaService.getContentBlob).mockResolvedValue(blob);

    const { result, unmount } = renderHook(() => useMediaContentUrl('m1'), {
      wrapper: makeQueryWrapper(queryClient, true), // <React.StrictMode>
    });

    await waitFor(() => expect(result.current.url).not.toBeNull());
    const url = result.current.url;

    // StrictMode's dev-only mount → cleanup → mount replay must not revoke a
    // URL still bound to the rendered <img>, and the cache dedup must have
    // prevented a second native allocation for the same blob even though the
    // memo factory and/or render body may run more than once.
    expect(revokeObjectURL).not.toHaveBeenCalled();
    expect(createObjectURL).toHaveBeenCalledTimes(1);

    unmount();

    await waitFor(() => expect(revokeObjectURL).toHaveBeenCalledWith(url));
    expect(revokeObjectURL).toHaveBeenCalledTimes(1);
  });
});
