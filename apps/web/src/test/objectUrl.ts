import { vi } from 'vitest';
import { cleanup } from '@testing-library/react';

/**
 * Anything going through useMediaContentUrl needs createObjectURL and
 * revokeObjectURL stubbed. Vitest 4's jsdom environment does supply both (as
 * statics on its own `URL`, delegating to Node's), but they mint real blob URLs
 * and record nothing — so tests still need these mocks for the deterministic
 * `blob:mock-N` values and to assert on call counts and arguments (the hook's
 * create/revoke pairing is what keeps it from leaking allocations under
 * StrictMode).
 *
 * The stub is a SUBCLASS of URL rather than `Object.assign(URL, ...)`. Assigning
 * onto the real class mutates it before `vi.stubGlobal` snapshots the original,
 * so `vi.unstubAllGlobals()` restores a value that already carries the mocks and
 * the stubbed methods leak into every later test file in the process. Extending
 * leaves the real `URL` untouched, so the restore is real — and the subclass
 * still constructs and parses URLs exactly as before, which `new URL(...)` in
 * the code under test depends on.
 */
export function stubObjectUrl(): {
  createObjectURL: ReturnType<typeof vi.fn>;
  revokeObjectURL: ReturnType<typeof vi.fn>;
} {
  let counter = 0;
  const createObjectURL = vi.fn(() => `blob:mock-${counter++}`);
  const revokeObjectURL = vi.fn();

  class StubbedURL extends URL {
    static createObjectURL = createObjectURL;
    static revokeObjectURL = revokeObjectURL;
  }

  vi.stubGlobal('URL', StubbedURL);
  return { createObjectURL, revokeObjectURL };
}

/**
 * Unmounts every rendered tree, THEN removes the stub.
 *
 * Order is the whole point. Vitest runs `afterEach` hooks last-registered-first,
 * and Testing Library registers its auto-cleanup when it is imported — i.e.
 * before a test file's own `afterEach` — so the file's hook runs first and
 * `vi.unstubAllGlobals()` would restore the real `URL` while components are
 * still mounted. Their effect cleanups then call `URL.revokeObjectURL` on the
 * restored class, so the revokes land on the environment's implementation
 * instead of the mock and the create/revoke pairing a test just asserted on
 * silently comes up short.
 */
export function unstubObjectUrl(): void {
  cleanup();
  vi.unstubAllGlobals();
}
