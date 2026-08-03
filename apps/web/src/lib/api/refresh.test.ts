import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { ApiError } from '@myfleet/shared-ts';
import { mintAccessToken, refreshAccessToken } from './refresh';
import { ACCESS_TOKEN_KEY, getAccessToken, setAccessToken } from './token';

function jsonResponse(body: unknown, ok = true): Response {
  return {
    ok,
    json: () => Promise.resolve(body),
  } as unknown as Response;
}

/** A response with a real `status`, which `jsonResponse` deliberately omits. */
function statusResponse(status: number, body: unknown = null): Response {
  return {
    ok: status >= 200 && status < 300,
    status,
    json: () => Promise.resolve(body),
  } as unknown as Response;
}

describe('mintAccessToken', () => {
  beforeEach(() => {
    localStorage.removeItem(ACCESS_TOKEN_KEY);
    vi.restoreAllMocks();
  });

  afterEach(() => {
    localStorage.removeItem(ACCESS_TOKEN_KEY);
  });

  it('stores and returns the minted token', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue(jsonResponse({ data: { attributes: { accessToken: 'new-jwt' } } })),
    );

    await expect(mintAccessToken()).resolves.toBe('new-jwt');
    expect(getAccessToken()).toBe('new-jwt');
  });

  // The whole point of splitting mint from refresh: the invite-accept flow runs
  // this on a healthy session, so a failure must not take the session with it.
  it.each([
    ['a non-ok response', () => Promise.resolve(jsonResponse({}, false))],
    ['a response with no token', () => Promise.resolve(jsonResponse({ data: { attributes: {} } }))],
    ['a network error', () => Promise.reject(new Error('offline'))],
  ])('returns null and keeps the existing token on %s', async (_label, impl) => {
    setAccessToken('still-valid');
    vi.stubGlobal('fetch', vi.fn().mockImplementation(impl));

    await expect(mintAccessToken()).resolves.toBeNull();
    expect(getAccessToken()).toBe('still-valid');
  });

  // Concurrent refreshes would replay the rotating refresh cookie, which
  // auth-service treats as reuse and answers by revoking the whole family.
  it('dedupes concurrent callers onto one request', async () => {
    let resolveFetch: ((r: Response) => void) | undefined;
    const fetchMock = vi.fn().mockImplementation(
      () =>
        new Promise<Response>((resolve) => {
          resolveFetch = resolve;
        }),
    );
    vi.stubGlobal('fetch', fetchMock);

    const both = Promise.all([mintAccessToken(), mintAccessToken()]);
    resolveFetch?.(jsonResponse({ data: { attributes: { accessToken: 'one-jwt' } } }));

    expect(await both).toEqual(['one-jwt', 'one-jwt']);
    expect(fetchMock).toHaveBeenCalledTimes(1);
  });

  it('starts a new request once the previous one settled', async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValue(jsonResponse({ data: { attributes: { accessToken: 'jwt' } } }));
    vi.stubGlobal('fetch', fetchMock);

    await mintAccessToken();
    await mintAccessToken();
    expect(fetchMock).toHaveBeenCalledTimes(2);
  });

  // FR-SPA-7: mintAccessToken's contract is unchanged. Its callers
  // (invites.ts, members.ts) run it on a HEALTHY session to pick up fresher
  // claims, so it must neither throw at them nor discard a working token.
  it('resolves null without throwing or clearing when the service is unavailable', async () => {
    setAccessToken('still-valid');
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(statusResponse(503)));

    await expect(mintAccessToken()).resolves.toBeNull();
    expect(getAccessToken()).toBe('still-valid');
  });
});

describe('refreshAccessToken', () => {
  beforeEach(() => {
    localStorage.removeItem(ACCESS_TOKEN_KEY);
    vi.restoreAllMocks();
  });

  // The 401-retry caller owns clearing: the request already came back
  // unauthorized, so a failed mint means the session is genuinely dead.
  it('clears the stored token when the mint fails', async () => {
    setAccessToken('dead');
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(jsonResponse({}, false)));

    await expect(refreshAccessToken()).resolves.toBeNull();
    expect(getAccessToken()).toBeNull();
  });

  it('leaves the new token in place on success', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue(jsonResponse({ data: { attributes: { accessToken: 'fresh' } } })),
    );

    await expect(refreshAccessToken()).resolves.toBe('fresh');
    expect(getAccessToken()).toBe('fresh');
  });

  // The point of the whole task. A 503 says auth-service could not reach what
  // it needed to answer — it says nothing about whether this session is still
  // good. Clearing the token here is what navigates the user to /login.
  it('keeps the stored token and rejects with a 503 when the service is unavailable', async () => {
    setAccessToken('still-valid');
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(statusResponse(503)));

    await expect(refreshAccessToken()).rejects.toBeInstanceOf(ApiError);
    // The stored token is the state that decides isAuthenticated, and therefore
    // whether RequireAuth navigates away. Assert it, not just the rejection.
    expect(getAccessToken()).toBe('still-valid');
  });

  it('rejects with status 503 so the original request does not surface as a 401', async () => {
    setAccessToken('still-valid');
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(statusResponse(503)));

    await expect(refreshAccessToken()).rejects.toMatchObject({
      status: 503,
      code: 'service_unavailable',
    });
  });

  it('still clears the stored token on a 401', async () => {
    setAccessToken('dead');
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(statusResponse(401)));

    await expect(refreshAccessToken()).resolves.toBeNull();
    expect(getAccessToken()).toBeNull();
  });

  // The dedupe is load-bearing regardless of outcome: two POSTs carrying the
  // same rotating cookie are reuse, and auth-service answers reuse by revoking
  // the whole family — the logout this task is preventing.
  it('still dedupes concurrent callers onto one request during an outage', async () => {
    const fetchMock = vi.fn().mockResolvedValue(statusResponse(503));
    vi.stubGlobal('fetch', fetchMock);

    const results = await Promise.allSettled([refreshAccessToken(), refreshAccessToken()]);

    expect(fetchMock).toHaveBeenCalledTimes(1);
    expect(results.map((r) => r.status)).toEqual(['rejected', 'rejected']);
  });
});
