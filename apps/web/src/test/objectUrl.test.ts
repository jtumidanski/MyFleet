import { describe, it, expect } from 'vitest';
import { stubObjectUrl, unstubObjectUrl } from './objectUrl';

/**
 * The helper's own contract, because getting it wrong is silent: the previous
 * implementation did `vi.stubGlobal('URL', Object.assign(URL, {...}))`, which
 * mutates the real `URL` class BEFORE vi snapshots it. The snapshot therefore
 * already carried the mocks, `vi.unstubAllGlobals()` "restored" them, and the
 * stubs leaked into every test file that ran afterwards in the same process.
 */
describe('stubObjectUrl', () => {
  it('leaves the real URL class untouched, so the unstub genuinely restores it', () => {
    const realUrl = globalThis.URL;
    // Vitest 4's jsdom environment installs its own `URL` carrying static
    // createObjectURL/revokeObjectURL that delegate to Node's, so the ambient
    // class now HAS those methods and mere presence proves nothing. What the
    // helper must guarantee is that they are not *our mocks* — checked by
    // identity below.
    const realCreate = realUrl.createObjectURL;

    const { createObjectURL } = stubObjectUrl();
    expect(globalThis.URL.createObjectURL).toBe(createObjectURL);
    // The mutation-in-place bug shows up right here: the real class must still
    // carry the environment's implementation, not the mock.
    expect(realUrl.createObjectURL).toBe(realCreate);
    expect(realUrl.createObjectURL).not.toBe(createObjectURL);

    unstubObjectUrl();

    expect(globalThis.URL).toBe(realUrl);
    expect(globalThis.URL.createObjectURL).toBe(realCreate);
  });

  it('still parses URLs normally while stubbed', () => {
    stubObjectUrl();
    // Code under test does `new URL(...)`; a stub that broke construction would
    // be worse than no stub.
    expect(new URL('https://example.test/a?b=c').pathname).toBe('/a');
    unstubObjectUrl();
  });
});
