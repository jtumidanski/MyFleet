import { vi } from 'vitest';

/**
 * jsdom implements neither createObjectURL nor revokeObjectURL, so anything
 * going through useMediaContentUrl needs them stubbed. Returns the mocks so a
 * test can assert on call counts and arguments (the hook's create/revoke pairing
 * is what keeps it from leaking allocations under StrictMode).
 */
export function stubObjectUrl(): {
  createObjectURL: ReturnType<typeof vi.fn>;
  revokeObjectURL: ReturnType<typeof vi.fn>;
} {
  let counter = 0;
  const createObjectURL = vi.fn(() => `blob:mock-${counter++}`);
  const revokeObjectURL = vi.fn();
  vi.stubGlobal('URL', Object.assign(URL, { createObjectURL, revokeObjectURL }));
  return { createObjectURL, revokeObjectURL };
}
