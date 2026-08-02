import { setAccessToken, clearAccessToken } from './token';

interface RefreshResponse {
  data?: { attributes?: { accessToken?: string } };
}

/**
 * Exchanges the HttpOnly refresh cookie for a new access token and stores it.
 *
 * `credentials: 'include'` so the browser attaches the cookie; the new token
 * comes back at `data.attributes.accessToken`. On failure the stored token is
 * cleared (effective logout) and null is returned.
 *
 * Two callers: the API client's one-shot 401 retry, and any flow that changes
 * the caller's server-side membership — the fleet claims are baked into the JWT
 * at mint time, so they only move when a new token is issued.
 */
export async function refreshAccessToken(): Promise<string | null> {
  try {
    const res = await fetch('/api/auth/refresh', {
      method: 'POST',
      credentials: 'include',
      headers: { 'Content-Type': 'application/vnd.api+json' },
    });
    if (!res.ok) {
      clearAccessToken();
      return null;
    }
    const body = (await res.json().catch(() => null)) as RefreshResponse | null;
    const token = body?.data?.attributes?.accessToken;
    if (!token) {
      clearAccessToken();
      return null;
    }
    setAccessToken(token);
    return token;
  } catch {
    clearAccessToken();
    return null;
  }
}
