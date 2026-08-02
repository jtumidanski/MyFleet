import { setAccessToken, clearAccessToken } from './token';

interface RefreshResponse {
  data?: { attributes?: { accessToken?: string } };
}

/**
 * In-flight mint, shared by every caller.
 *
 * auth-service rotates the refresh token on use and treats a replay as reuse,
 * revoking the whole token family — so two concurrent POSTs to /auth/refresh
 * with the same cookie log the user out everywhere. Callers are deduped onto
 * one request rather than being trusted to serialize themselves.
 */
let inflight: Promise<string | null> | null = null;

async function requestToken(): Promise<string | null> {
  try {
    const res = await fetch('/api/auth/refresh', {
      method: 'POST',
      credentials: 'include',
      headers: { 'Content-Type': 'application/vnd.api+json' },
    });
    if (!res.ok) return null;
    const body = (await res.json().catch(() => null)) as RefreshResponse | null;
    const token = body?.data?.attributes?.accessToken;
    if (!token) return null;
    setAccessToken(token);
    return token;
  } catch {
    return null;
  }
}

/**
 * Exchanges the HttpOnly refresh cookie for a new access token and stores it,
 * returning null if that fails. A failure leaves the existing token ALONE.
 *
 * Use this when the goal is fresher claims — e.g. after accepting an invite,
 * where `active_fleet_id` and `role` were fixed into the JWT at mint time and
 * only move when a new token is issued. The current token is still valid in
 * that case, so discarding it on a transient failure would log the user out of
 * a working session.
 */
export function mintAccessToken(): Promise<string | null> {
  if (!inflight) {
    inflight = requestToken().finally(() => {
      inflight = null;
    });
  }
  return inflight;
}

/**
 * mintAccessToken plus clear-on-failure, for the API client's one-shot 401
 * retry. There the request already came back unauthorized, so a failed mint
 * means the session is genuinely dead and the stale token should go.
 */
export async function refreshAccessToken(): Promise<string | null> {
  const token = await mintAccessToken();
  if (!token) clearAccessToken();
  return token;
}
