import { describe, it, expect, vi, beforeEach } from 'vitest';
import { apiClient } from './client';

describe('apiClient', () => {
  beforeEach(() => vi.restoreAllMocks());

  it('attaches the bearer token from the token store', async () => {
    localStorage.setItem('access_token', 'tok-123');
    const fetchMock = vi.spyOn(globalThis, 'fetch').mockResolvedValue(
      new Response(JSON.stringify({ data: { id: '1', type: 'vehicles', attributes: {} } }), { status: 200 }),
    );
    await apiClient.request('/api/fleet/vehicles/1');
    const headers = (fetchMock.mock.calls[0]![1]!.headers ?? {}) as Record<string, string>;
    expect(headers.Authorization).toBe('Bearer tok-123');
  });
});
