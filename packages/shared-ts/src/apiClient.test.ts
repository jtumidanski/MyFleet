import { describe, it, expect, vi, beforeEach } from 'vitest';
import { ApiClient } from './apiClient';

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
    const headers = fetchMock.mock.calls[0][1].headers as Record<string, string>;
    expect(headers.Authorization).toBe('Bearer tok-123');
    // A binary GET must not claim a JSON:API content type.
    expect(headers['Content-Type']).toBeUndefined();
  });

  it('refreshes once on 401 and retries', async () => {
    const blob = new Blob(['bytes']);
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce({ ok: false, status: 401, blob: async () => blob })
      .mockResolvedValueOnce({ ok: true, status: 200, blob: async () => blob });
    vi.stubGlobal('fetch', fetchMock);

    const onRefresh = vi.fn().mockResolvedValue('tok-new');
    const client = new ApiClient({
      baseUrl: '',
      getAccessToken: () => 'tok-old',
      onRefresh,
    });

    const out = await client.requestBlob('/api/media/m1/content');

    expect(out).toBe(blob);
    expect(onRefresh).toHaveBeenCalledTimes(1);
    expect(fetchMock).toHaveBeenCalledTimes(2);
  });

  it('throws on a non-OK response', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue({
        ok: false,
        status: 404,
        json: async () => ({ errors: [{ detail: 'not found' }] }),
        blob: async () => new Blob([]),
      }),
    );

    const client = new ApiClient({
      baseUrl: '',
      getAccessToken: () => 'tok-123',
      onRefresh: async () => null,
    });

    await expect(client.requestBlob('/api/media/missing/content')).rejects.toBeDefined();
  });
});
