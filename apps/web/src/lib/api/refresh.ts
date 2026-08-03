import { ApiError } from '@myfleet/shared-ts';
import { setAccessToken, clearAccessToken } from './token';

interface RefreshResponse {
  data?: { attributes?: { accessToken?: string } };
}

/**
 * What one POST to /auth/refresh concluded.
 *
 * `unavailable` is the distinction the whole type exists for. auth-service
 * answers 503 when it could not reach what it needed to resolve the session —
 * a fleet-service outage, a database blip — which says nothing about whether
 * the session is still good. Collapsing it into `dead`, as a bare `null` did,
 * turns someone else's brief outage into a forced sign-in through Google.
 */
type RefreshOutcome =
  { status: 'ok'; token: string } | { status: 'dead' } | { status: 'unavailable' };

/**
 * In-flight refresh, shared by every caller.
 *
 * auth-service rotates the refresh token on use and treats a replay as reuse,
 * revoking the whole token family — so two concurrent POSTs to /auth/refresh
 * with the same cookie log the user out everywhere. Callers are deduped onto
 * one request rather than being trusted to serialize themselves.
 */
let inflight: Promise<RefreshOutcome> | null = null;

async function requestToken(): Promise<RefreshOutcome> {
  try {
    const res = await fetch('/api/auth/refresh', {
      method: 'POST',
      credentials: 'include',
      headers: { 'Content-Type': 'application/vnd.api+json' },
    });
    // Checked before `ok` so it can never fall into the dead bucket.
    if (res.status === 503) return { status: 'unavailable' };
    if (!res.ok) return { status: 'dead' };
    const body = (await res.json().catch(() => null)) as RefreshResponse | null;
    const token = body?.data?.attributes?.accessToken;
    if (!token) return { status: 'dead' };
    setAccessToken(token);
    return { status: 'ok', token };
  } catch {
    return { status: 'dead' };
  }
}

/**
 * The shared request, unchanged in kind: the promise held in `inflight` still
 * RESOLVES — never rejects — so concurrent callers collapse onto one POST and
 * `.finally` frees the slot exactly as before. That is why the outcome travels
 * as a value rather than as a rejection at this layer: a shared promise that
 * rejects is where unhandled-rejection and double-settle bugs live. Callers
 * translate the outcome into their own contract.
 */
function refreshOnce(): Promise<RefreshOutcome> {
  if (!inflight) {
    inflight = requestToken().finally(() => {
      inflight = null;
    });
  }
  return inflight;
}

/**
 * Exchanges the HttpOnly refresh cookie for a new access token and stores it,
 * returning null if that fails. A failure leaves the existing token ALONE, and
 * never throws.
 *
 * Use this when the goal is fresher claims — e.g. after accepting an invite,
 * where `active_fleet_id` and `role` were fixed into the JWT at mint time and
 * only move when a new token is issued. The current token is still valid in
 * that case, so discarding it on a transient failure would log the user out of
 * a working session — and for the same reason a 503 is just another null here,
 * not something to surface to a caller running on a healthy session.
 */
export async function mintAccessToken(): Promise<string | null> {
  const outcome = await refreshOnce();
  return outcome.status === 'ok' ? outcome.token : null;
}

/**
 * mintAccessToken plus clear-on-failure, for the API client's one-shot 401
 * retry. There the request already came back unauthorized, so a failed mint
 * usually means the session is genuinely dead and the stale token should go.
 *
 * The exception is a 503: auth-service is telling us it could not answer, not
 * that the answer is no. Rejecting rather than resolving null is what carries
 * that distinction outward with no change to packages/shared-ts — the rejection
 * propagates through `onRefresh` and out of `fetchAuthenticated`, so the
 * original request fails with this 503 instead of the 401 that was never the
 * real problem.
 */
export async function refreshAccessToken(): Promise<string | null> {
  const outcome = await refreshOnce();
  if (outcome.status === 'ok') return outcome.token;
  if (outcome.status === 'unavailable') {
    throw new ApiError(503, 'service_unavailable', 'Service temporarily unavailable');
  }
  clearAccessToken();
  return null;
}
