import { describe, it, expect, beforeEach, vi } from 'vitest';
import type { LoginErrorCode } from './loginError';

/**
 * The module memoises its read at module scope (design §4.1), so every case
 * needs a fresh module instance. `vi.resetModules()` clears the source-module
 * registry; node_modules stay externalised, so this is cheap.
 */
async function freshModule(url: string) {
  window.history.replaceState(null, '', url);
  vi.resetModules();
  return import('./loginError');
}

describe('consumeLoginError', () => {
  beforeEach(() => {
    window.history.replaceState(null, '', '/login');
  });

  it.each<LoginErrorCode>([
    'cancelled',
    'invalid_state',
    'auth_failed',
    'server_error',
    'service_unavailable',
  ])(
    'parses the %s code',
    async (code) => {
      const { consumeLoginError } = await freshModule(`/login#error=${code}`);
      expect(consumeLoginError()).toBe(code);
    },
  );

  // FR-STATE-6: anything outside the closed set is a generic failure, and the
  // supplied string is discarded at the parser so nothing downstream can render
  // it.
  it.each([
    ['an unknown code', '/login#error=totally_made_up'],
    ['an empty value', '/login#error='],
    ['a malformed percent-encoding', '/login#error=%zz'],
    ['an injected string', '/login#error=%3Cscript%3Ealert(1)%3C%2Fscript%3E'],
  ])('normalises %s to server_error', async (_name, url) => {
    const { consumeLoginError } = await freshModule(url);
    expect(consumeLoginError()).toBe('server_error');
  });

  it('returns null when the hash carries no error key', async () => {
    const { consumeLoginError } = await freshModule('/login');
    expect(consumeLoginError()).toBeNull();
  });

  // FR-STATE-8: the two fragment consumers are mutually exclusive. Reading the
  // error must not eat the token AuthProvider is about to capture.
  it('ignores and preserves an access_token fragment', async () => {
    const { consumeLoginError } = await freshModule('/login#access_token=jwt-value');
    expect(consumeLoginError()).toBeNull();
    expect(window.location.hash).toBe('#access_token=jwt-value');
  });

  // FR-STATE-7: a reload or a shared URL must not resurrect a stale message.
  it('strips the fragment while preserving path and query', async () => {
    const { consumeLoginError } = await freshModule('/login?next=%2Fvehicles#error=auth_failed');
    consumeLoginError();
    expect(window.location.hash).toBe('');
    expect(window.location.pathname).toBe('/login');
    expect(window.location.search).toBe('?next=%2Fvehicles');
  });

  it('memoises so a second call survives the strip', async () => {
    const { consumeLoginError } = await freshModule('/login#error=cancelled');
    expect(consumeLoginError()).toBe('cancelled');
    expect(consumeLoginError()).toBe('cancelled');
  });

  // FR-STATE-8, made structural. readAndStrip removes the WHOLE hash, so a
  // fragment carrying both keys would destroy the access token. auth-service
  // never emits both today — success and failure are separate branches — and
  // AuthProvider's lazy initializer happens to run first, so the guarantee held
  // only by convention plus ordering. Now the token's presence alone is enough.
  it.each([
    ['token first', '/login#access_token=jwt-123&error=auth_failed'],
    ['error first', '/login#error=auth_failed&access_token=jwt-123'],
  ])('yields nothing and preserves the fragment when a token rides along (%s)', async (_n, url) => {
    const { consumeLoginError } = await freshModule(url);

    expect(consumeLoginError()).toBeNull();
    // The whole fragment survives, token included, for captureTokenFromHash.
    expect(window.location.hash).toContain('access_token=jwt-123');
  });
});

describe('noticeFor', () => {
  it('presents a cancellation neutrally', async () => {
    const { noticeFor } = await freshModule('/login');
    expect(noticeFor('cancelled')).toEqual({ tone: 'neutral', message: 'Sign-in cancelled.' });
  });

  it.each<LoginErrorCode>(['invalid_state', 'auth_failed', 'server_error'])(
    'presents %s as a danger with the shared copy',
    async (code) => {
      const { noticeFor } = await freshModule('/login');
      expect(noticeFor(code)).toEqual({
        tone: 'danger',
        message:
          "Sign-in didn't complete. Nothing was saved — try again, or use a different Google account.",
      });
    },
  );

  // The one failure whose ADVICE differs: wait and retry, rather than try a
  // different Google account. That is why it does not reuse GENERIC_FAILURE —
  // unlike the invalid_state / auth_failed / server_error split, which exists
  // for log correlation and which the reader cannot act on differently.
  it('tells the user an outage is temporary rather than reusing the generic copy', async () => {
    const { noticeFor } = await freshModule('/login');

    expect(noticeFor('service_unavailable')).toEqual({
      tone: 'danger',
      message: 'Sign-in is temporarily unavailable. Nothing was saved — try again in a moment.',
    });
    // Distinct from the generic failure, which advises a different Google
    // account — the wrong thing to do during an outage.
    expect(noticeFor('service_unavailable').message).not.toBe(noticeFor('server_error').message);
  });

  // The union member alone is not enough: CODES is a hand-maintained
  // readonly string[], and isLoginErrorCode silently degrades anything missing
  // from it to server_error (loginError.ts:58). That degradation is what makes
  // shipping the backend ahead of the frontend safe — and what would hide this
  // mistake in review.
  it('accepts service_unavailable through the CODES allowlist', async () => {
    const { consumeLoginError } = await freshModule('/login#error=service_unavailable');
    expect(consumeLoginError()).toBe('service_unavailable');
  });
});
