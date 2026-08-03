// Entry points into auth-service's browser-facing OAuth flow.

const LOGIN_PATH = '/api/auth/login/google';

/**
 * True for values that are safe to hand back to the app as a post-login
 * destination: site-relative paths only. Protocol-relative forms ("//host",
 * "/\host") are rejected because browsers resolve them off-site.
 */
function isSiteRelativePath(path: string): boolean {
  return path.startsWith('/') && !path.startsWith('//') && !path.startsWith('/\\');
}

/**
 * Builds the login URL, optionally asking auth-service to send the browser back
 * to `returnTo` after the OAuth dance instead of the default home/onboarding
 * landing. Used so an invitee who clicked an invite link while logged out ends
 * up back on the accept route rather than losing the token.
 *
 * auth-service sanitizes `return_to` authoritatively; the check here just keeps
 * an off-site value from leaving the browser.
 */
export function buildLoginUrl(returnTo?: string): string {
  if (!returnTo || !isSiteRelativePath(returnTo)) return LOGIN_PATH;
  return `${LOGIN_PATH}?return_to=${encodeURIComponent(returnTo)}`;
}
