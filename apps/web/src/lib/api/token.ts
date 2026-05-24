// Single source of truth for the access-token storage key + helpers.
//
// The access token lives in localStorage (readable by JS, sent as
// `Authorization: Bearer`). The refresh token is an HttpOnly cookie the browser
// attaches automatically on `credentials: 'include'` requests — it is never
// touched here.
export const ACCESS_TOKEN_KEY = 'access_token';

export function getAccessToken(): string | null {
  return localStorage.getItem(ACCESS_TOKEN_KEY);
}

export function setAccessToken(token: string): void {
  localStorage.setItem(ACCESS_TOKEN_KEY, token);
}

export function clearAccessToken(): void {
  localStorage.removeItem(ACCESS_TOKEN_KEY);
}

/**
 * On app load, capture an access token delivered by auth-service in the URL
 * fragment (`#access_token=<jwt>`), persist it, and strip the hash so it does
 * not linger in history. Returns the token if one was captured.
 */
export function captureTokenFromHash(): string | null {
  const hash = window.location.hash;
  if (!hash.includes('access_token=')) return null;
  const params = new URLSearchParams(hash.startsWith('#') ? hash.slice(1) : hash);
  const token = params.get('access_token');
  if (!token) return null;
  setAccessToken(token);
  // Strip the fragment without reloading or adding a history entry.
  const url = window.location.pathname + window.location.search;
  window.history.replaceState(null, '', url);
  return token;
}
