import { describe, it, expect, vi, beforeEach, type Mock } from 'vitest';
import { ApiClient } from './apiClient';
import { ApiError } from './errors';

/**
 * Headers the nth fetch call was made with. Indexing `mock.calls` widens to
 * `undefined` under noUncheckedIndexedAccess, and a call that never happened is
 * worth failing on by name rather than through a header assertion on nothing.
 */
function headersOfCall(fetchMock: Mock, index: number): Record<string, string> {
  const call = fetchMock.mock.calls[index];
  if (!call) {
    throw new Error(
      `expected a fetch call at index ${index}; found ${fetchMock.mock.calls.length}`,
    );
  }
  return (call[1] as RequestInit).headers as Record<string, string>;
}

describe('ApiClient.request', () => {
  beforeEach(() => {
    vi.restoreAllMocks();
  });

  function stubJsonFetch() {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      json: async () => ({ data: { id: 'm1', type: 'media-objects', attributes: {} } }),
    });
    vi.stubGlobal('fetch', fetchMock);
    return fetchMock;
  }

  function makeClient() {
    return new ApiClient({
      baseUrl: '',
      getAccessToken: () => 'tok-123',
      onRefresh: async () => null,
    });
  }

  it('applies the JSON:API content type when the caller supplies none', async () => {
    const fetchMock = stubJsonFetch();

    await makeClient().request('/api/media/m1');

    const headers = headersOfCall(fetchMock, 0);
    expect(headers['Content-Type']).toBe('application/vnd.api+json');
  });

  it("lets a caller's Content-Type override the JSON:API default", async () => {
    // This is the invariant the binary media PUT rests on
    // (apps/web/src/services/api/MediaService.ts putContent): `init.headers` is
    // spread last in fetchAuthenticated, so a caller-supplied Content-Type
    // displaces `application/vnd.api+json`. Reordering that spread would send
    // image bytes labelled as JSON:API.
    const fetchMock = stubJsonFetch();

    await makeClient().request('/api/media/m1/content', {
      method: 'PUT',
      body: new Blob(['bytes'], { type: 'image/jpeg' }),
      headers: { 'Content-Type': 'image/jpeg' },
    });

    const headers = headersOfCall(fetchMock, 0);
    expect(headers['Content-Type']).toBe('image/jpeg');
    // The override must not cost the bearer token.
    expect(headers.Authorization).toBe('Bearer tok-123');
  });
});

describe('ApiClient.requestBlob', () => {
  beforeEach(() => {
    vi.restoreAllMocks();
  });

  it('sends the bearer token and returns the blob', async () => {
    const blob = new Blob(['bytes'], { type: 'image/jpeg' });
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      blob: async () => blob,
    });
    vi.stubGlobal('fetch', fetchMock);

    const client = new ApiClient({
      baseUrl: '',
      getAccessToken: () => 'tok-123',
      onRefresh: async () => null,
    });

    const out = await client.requestBlob('/api/media/m1/content');

    expect(out).toBe(blob);
    const headers = headersOfCall(fetchMock, 0);
    expect(headers.Authorization).toBe('Bearer tok-123');
    // A binary GET must not claim a JSON:API content type.
    expect(headers['Content-Type']).toBeUndefined();
  });

  it('refreshes once on 401 and retries with the refreshed token', async () => {
    const blob = new Blob(['bytes']);
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce({ ok: false, status: 401, blob: async () => blob })
      .mockResolvedValueOnce({ ok: true, status: 200, blob: async () => blob });
    vi.stubGlobal('fetch', fetchMock);

    // Mutable so the retried call, which re-invokes getAccessToken(), observes
    // the refreshed value rather than a token frozen at construction time.
    let token = 'tok-old';
    const onRefresh = vi.fn().mockImplementation(async () => {
      token = 'tok-new';
      return token;
    });
    const client = new ApiClient({
      baseUrl: '',
      getAccessToken: () => token,
      onRefresh,
    });

    const out = await client.requestBlob('/api/media/m1/content');

    expect(out).toBe(blob);
    expect(onRefresh).toHaveBeenCalledTimes(1);
    expect(fetchMock).toHaveBeenCalledTimes(2);

    const firstHeaders = headersOfCall(fetchMock, 0);
    const secondHeaders = headersOfCall(fetchMock, 1);
    expect(firstHeaders.Authorization).toBe('Bearer tok-old');
    // This is the assertion that catches a retry which reuses the stale token.
    expect(secondHeaders.Authorization).toBe('Bearer tok-new');
  });

  it('throws an ApiError with the JSON:API error shape on a non-OK response', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue({
        ok: false,
        status: 404,
        json: async () => ({
          errors: [{ status: '404', code: 'not_found', title: 'Not Found', detail: 'not found' }],
        }),
        blob: async () => new Blob([]),
      }),
    );

    const client = new ApiClient({
      baseUrl: '',
      getAccessToken: () => 'tok-123',
      onRefresh: async () => null,
    });

    await expect(client.requestBlob('/api/media/missing/content')).rejects.toBeInstanceOf(ApiError);

    try {
      await client.requestBlob('/api/media/missing/content');
      expect.unreachable('expected requestBlob to throw');
    } catch (err) {
      expect(err).toBeInstanceOf(ApiError);
      const apiErr = err as ApiError;
      expect(apiErr.status).toBe(404);
      expect(apiErr.code).toBe('not_found');
      expect(apiErr.detail).toBe('not found');
    }
  });
});

describe('ApiClient refresh failures', () => {
  beforeEach(() => {
    vi.restoreAllMocks();
  });

  // FR-SPA-6. When the original request 401s and the refresh answers 503, the
  // caller must see the 503. A 401 would be a lie — the caller's credentials
  // were never the problem — and downstream code keys on status. This works
  // with no code in this package: a rejection from onRefresh propagates
  // straight out of fetchAuthenticated, which is exactly why refresh.ts throws
  // instead of widening the onRefresh return type.
  it('propagates a rejecting onRefresh instead of surfacing the original 401', async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: false,
      status: 401,
      json: async () => ({ errors: [{ status: '401', code: 'unauthorized', title: 'unauthorized' }] }),
    });
    vi.stubGlobal('fetch', fetchMock);

    const client = new ApiClient({
      baseUrl: '',
      getAccessToken: () => 'tok-123',
      onRefresh: async () => {
        throw new ApiError(503, 'service_unavailable', 'Service temporarily unavailable');
      },
    });

    await expect(client.request('/api/fleet/vehicles')).rejects.toMatchObject({ status: 503 });
    // FR-SPA-5: one-shot, no retry loop. The original request was attempted
    // once and never retried.
    expect(fetchMock).toHaveBeenCalledTimes(1);
  });

  // The other half: an onRefresh that resolves null keeps today's behaviour
  // exactly — the original 401 surfaces, still one-shot.
  it('surfaces the original 401 when onRefresh resolves null', async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: false,
      status: 401,
      json: async () => ({ errors: [{ status: '401', code: 'unauthorized', title: 'unauthorized' }] }),
    });
    vi.stubGlobal('fetch', fetchMock);

    const client = new ApiClient({
      baseUrl: '',
      getAccessToken: () => 'tok-123',
      onRefresh: async () => null,
    });

    await expect(client.request('/api/fleet/vehicles')).rejects.toMatchObject({ status: 401 });
    expect(fetchMock).toHaveBeenCalledTimes(1);
  });
});
