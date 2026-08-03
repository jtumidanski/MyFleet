import { ApiClient } from '@myfleet/shared-ts';
import { getAccessToken } from './token';
import { refreshAccessToken } from './refresh';

/**
 * Singleton API client wired to the MyFleet auth contract.
 *
 * - baseUrl is '' so callers pass full gateway paths (`/api/<service>/...`).
 *   Traefik strips only `/api` before routing to auth-service and
 *   media-service (their routes already carry their own /auth or /media
 *   segment); fleet-service and notification-service still have their full
 *   `/api/<service>` prefix stripped.
 * - getAccessToken reads the bearer token from localStorage.
 * - onRefresh delegates to refreshAccessToken (see lib/api/refresh.ts), which
 *   has two failure modes, and the difference is the whole point: it returns
 *   null when the session is genuinely dead (clearing the stored token, so the
 *   original request fails as 401), and it THROWS
 *   `ApiError(503, 'service_unavailable')` when auth-service answered that it
 *   could not reach what it needed. That rejection propagates out of
 *   fetchAuthenticated and fails the original request as a 503, leaving the
 *   stored token alone — someone else's outage must not read as a dead session.
 */
export const apiClient = new ApiClient({
  baseUrl: '',
  getAccessToken,
  onRefresh: refreshAccessToken,
});
