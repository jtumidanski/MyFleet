/** The closed set auth-service redirects with (FR-ERR-4). Nothing else is valid. */
export type LoginErrorCode = 'cancelled' | 'invalid_state' | 'auth_failed' | 'server_error';

export interface LoginErrorNotice {
  tone: 'neutral' | 'danger';
  message: string;
}

const CODES: readonly string[] = ['cancelled', 'invalid_state', 'auth_failed', 'server_error'];

// One sentence for all three failure codes. The invalid_state / auth_failed /
// server_error split exists for log correlation in auth-service, not for the
// reader — a person cannot act differently on it, and "invalid state" is jargon
// (design §4.2, open question 3). The table is keyed on all four codes anyway,
// so diverging one later is a one-line change.
const GENERIC_FAILURE =
  "Sign-in didn't complete. Nothing was saved — try again, or use a different Google account.";

const NOTICES: Record<LoginErrorCode, LoginErrorNotice> = {
  // Cancelling is a choice, not a fault: neutral tone, no alarm (FR-STATE-5).
  cancelled: { tone: 'neutral', message: 'Sign-in cancelled.' },
  invalid_state: { tone: 'danger', message: GENERIC_FAILURE },
  auth_failed: { tone: 'danger', message: GENERIC_FAILURE },
  server_error: { tone: 'danger', message: GENERIC_FAILURE },
};

/** Maps a code to how the login page should present it. */
export function noticeFor(code: LoginErrorCode): LoginErrorNotice {
  return NOTICES[code];
}

function isLoginErrorCode(value: string): value is LoginErrorCode {
  return CODES.includes(value);
}

function readAndStrip(): LoginErrorCode | null {
  const hash = window.location.hash;
  const params = new URLSearchParams(hash.startsWith('#') ? hash.slice(1) : hash);
  // The fragment belongs to captureTokenFromHash the moment it carries a token,
  // and this function strips the WHOLE hash rather than one key. Bail out first,
  // before the `error` test: auth-service emits success and failure on separate
  // branches so the two keys never arrive together today, but FR-STATE-8 must
  // hold structurally, not by provider convention plus provider ordering. A
  // combined `#access_token=…&error=…` fragment would otherwise destroy the
  // token — silently, and only for whichever consumer happened to run second.
  if (params.has('access_token')) return null;
  // No `error` key at all: nothing here for us.
  if (!params.has('error')) return null;

  // Strip before returning so a reload or a shared URL cannot resurrect a stale
  // message (FR-STATE-7). Mirrors captureTokenFromHash's replaceState.
  window.history.replaceState(null, '', window.location.pathname + window.location.search);

  const raw = params.get('error') ?? '';
  // FR-STATE-6: the raw value dies here. An unknown, empty, malformed or
  // injected string becomes the generic failure, so nothing downstream can
  // render attacker-supplied text.
  return isLoginErrorCode(raw) ? raw : 'server_error';
}

let captured: LoginErrorCode | null | undefined;

/**
 * Read the `#error=<code>` fragment auth-service redirects with, exactly once
 * per page load, and strip it.
 *
 * Memoised at module scope rather than read inside a hook: React StrictMode
 * mounts, unmounts and remounts in development, and a per-mount read would find
 * the fragment already stripped on the second mount — the callout would vanish
 * in dev but not in prod. Fresh page load = fresh module instance = fresh read,
 * which is exactly the lifetime FR-STATE-7 describes.
 */
export function consumeLoginError(): LoginErrorCode | null {
  if (captured !== undefined) return captured;
  captured = readAndStrip();
  return captured;
}
