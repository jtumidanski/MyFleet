import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { updateThemePreference } from './auth';
import { setAccessToken, clearAccessToken } from '../../api/token';

describe('updateThemePreference', () => {
  beforeEach(() => {
    localStorage.clear();
    clearAccessToken();
  });
  afterEach(() => vi.unstubAllGlobals());

  // FR-PERSIST-8: no token, no request. It RESOLVES rather than rejecting —
  // rejecting would raise the save-failure toast for a user who never had a
  // session to save into.
  it('makes no request and resolves when there is no token', async () => {
    const fetchMock = vi.fn();
    vi.stubGlobal('fetch', fetchMock);

    await expect(updateThemePreference('dark')).resolves.toBeNull();
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it('PATCHes the JSON:API envelope to /api/auth/me', async () => {
    setAccessToken('token-123');
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      json: async () => ({
        data: {
          id: 'u1',
          type: 'users',
          attributes: {
            email: 'a@b.com',
            displayName: 'A',
            avatarUrl: '',
            themePreference: 'dark',
          },
        },
      }),
    });
    vi.stubGlobal('fetch', fetchMock);

    const user = await updateThemePreference('dark');

    expect(user?.attributes.themePreference).toBe('dark');
    const [path, init] = fetchMock.mock.calls[0] as [string, RequestInit];
    expect(path).toBe('/api/auth/me');
    expect(init.method).toBe('PATCH');
    expect(JSON.parse(init.body as string)).toEqual({
      data: { type: 'users', attributes: { themePreference: 'dark' } },
    });
  });

  // FR-SEC-1: the target user is the token's `sub`. Nothing in the request
  // names a user, so there is no identifier for a caller to tamper with.
  it('sends no user identifier in the path or body', async () => {
    setAccessToken('token-123');
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      json: async () => ({ data: { id: 'u1', type: 'users', attributes: {} } }),
    });
    vi.stubGlobal('fetch', fetchMock);

    await updateThemePreference('light');

    const [path, init] = fetchMock.mock.calls[0] as [string, RequestInit];
    expect(path).not.toMatch(/\/users\//);
    expect(init.body as string).not.toContain('"id"');
  });
});
