import { ApiClient } from '@myfleet/shared-ts';
import { getAccessToken, setAccessToken, clearAccessToken } from './token';

interface RefreshResponse {
  data?: { attributes?: { accessToken?: string } };
}

/**
 * Singleton API client wired to the MyFleet auth contract.
 *
 * - baseUrl is '' so callers pass full gateway paths (`/api/<service>/...`).
 *   Traefik strips only `/api` before routing to auth-service and
 *   media-service (their routes already carry their own /auth or /media
 *   segment); fleet-service and notification-service still have their full
 *   `/api/<service>` prefix stripped.
 * - getAccessToken reads the bearer token from localStorage.
 * - onRefresh POSTs to /api/auth/refresh with `credentials: 'include'` so the
 *   browser attaches the HttpOnly refresh cookie; the new access token comes
 *   back at `data.attributes.accessToken`. On failure we clear the token
 *   (effective logout) and return null so the original request fails as 401.
 */
export const apiClient = new ApiClient({
  baseUrl: '',
  getAccessToken,
  onRefresh: async () => {
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
  },
});
