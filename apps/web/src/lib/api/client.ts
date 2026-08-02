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
 *   returns null on failure so the original request fails as 401.
 */
export const apiClient = new ApiClient({
  baseUrl: '',
  getAccessToken,
  onRefresh: refreshAccessToken,
});
