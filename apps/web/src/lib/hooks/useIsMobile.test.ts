import { describe, it, expect, afterEach } from 'vitest';
import { renderHook } from '@testing-library/react';
import { useIsMobile } from './useIsMobile';
import { mediaListenerCount, resetMatchMedia } from '../../test/setup';

function setWidth(width: number): void {
  Object.defineProperty(window, 'innerWidth', {
    value: width,
    writable: true,
    configurable: true,
  });
}

describe('useIsMobile', () => {
  afterEach(() => {
    setWidth(1024);
    resetMatchMedia();
  });

  it('is false at desktop widths', () => {
    setWidth(1024);
    const { result } = renderHook(() => useIsMobile());
    expect(result.current).toBe(false);
  });

  it('is true below the 768px breakpoint', () => {
    setWidth(500);
    const { result } = renderHook(() => useIsMobile());
    expect(result.current).toBe(true);
  });

  it('is false exactly at the breakpoint', () => {
    setWidth(768);
    const { result } = renderHook(() => useIsMobile());
    expect(result.current).toBe(false);
  });

  // A sidebar that keeps a listener per mount leaks one on every route change
  // that remounts the shell.
  it('unsubscribes on unmount', () => {
    setWidth(1024);
    const before = mediaListenerCount();
    const { unmount } = renderHook(() => useIsMobile());
    expect(mediaListenerCount()).toBe(before + 1);
    unmount();
    expect(mediaListenerCount()).toBe(before);
  });
});
