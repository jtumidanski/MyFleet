import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { downloadBlob } from './download';

describe('downloadBlob', () => {
  beforeEach(() => {
    vi.useFakeTimers();
    URL.createObjectURL = vi.fn(() => 'blob:test-url');
    URL.revokeObjectURL = vi.fn();
  });

  afterEach(() => {
    vi.useRealTimers();
    vi.restoreAllMocks();
  });

  it('clicks a detached anchor carrying the object URL and the filename', () => {
    const clicks: HTMLAnchorElement[] = [];
    const realCreate = document.createElement.bind(document);
    vi.spyOn(document, 'createElement').mockImplementation((tag: string) => {
      const el = realCreate(tag) as HTMLAnchorElement;
      if (tag === 'a') {
        el.click = () => clicks.push(el);
      }
      return el;
    });

    downloadBlob(new Blob(['x']), 'invoice.pdf');

    expect(clicks).toHaveLength(1);
    expect(clicks[0]!.href).toContain('blob:test-url');
    expect(clicks[0]!.download).toBe('invoice.pdf');
  });

  it('leaves no anchor behind in the document', () => {
    downloadBlob(new Blob(['x']), 'invoice.pdf');
    expect(document.querySelectorAll('a[download]')).toHaveLength(0);
  });

  // Revoking synchronously can cancel the download in some browsers, so it is
  // deferred to a macrotask — but it must still happen.
  it('revokes the object URL, but not before the click', () => {
    downloadBlob(new Blob(['x']), 'invoice.pdf');
    // task-019 probe: this is a deliberate ORDERING assertion, not a "never
    // happens" one — vi.runAllTimers() two lines below proves the revoke does
    // occur. Making download.ts revoke synchronously turns this line red with
    // no flush. expectNoCall is also unusable here: the file runs under
    // vi.useFakeTimers(), so the helper's setTimeout(0) never fires and the
    // test would hang until timeout.
    // eslint-disable-next-line no-restricted-syntax
    expect(URL.revokeObjectURL).not.toHaveBeenCalled();

    vi.runAllTimers();
    expect(URL.revokeObjectURL).toHaveBeenCalledWith('blob:test-url');
  });
});
