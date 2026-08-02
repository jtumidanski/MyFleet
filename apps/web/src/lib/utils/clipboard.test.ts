import { describe, it, expect, vi, afterEach } from 'vitest';
import { copyToClipboard } from './clipboard';

afterEach(() => {
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

describe('copyToClipboard', () => {
  it('uses the async clipboard API when available', async () => {
    const writeText = vi.fn().mockResolvedValue(undefined);
    vi.stubGlobal('navigator', { clipboard: { writeText } });

    await expect(copyToClipboard('hello')).resolves.toBe(true);
    expect(writeText).toHaveBeenCalledWith('hello');
  });

  // Local dev runs over plain HTTP on myfleet.home, where navigator.clipboard
  // is undefined. Without this fallback the button is dead in exactly the
  // environment where it gets tested first.
  it('falls back to execCommand in a non-secure context', async () => {
    vi.stubGlobal('navigator', {});
    const exec = vi.fn().mockReturnValue(true);
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    (document as any).execCommand = exec;

    await expect(copyToClipboard('hello')).resolves.toBe(true);
    expect(exec).toHaveBeenCalledWith('copy');
    // The scratch textarea must not survive, or it accumulates in the DOM.
    expect(document.querySelector('textarea')).toBeNull();
  });

  it('reports failure rather than throwing when both paths fail', async () => {
    vi.stubGlobal('navigator', {
      clipboard: { writeText: vi.fn().mockRejectedValue(new Error('denied')) },
    });
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    (document as any).execCommand = vi.fn().mockReturnValue(false);

    await expect(copyToClipboard('hello')).resolves.toBe(false);
  });
});
