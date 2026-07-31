import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import {
  THEME_STORAGE_KEY,
  applyThemeClass,
  isThemePreference,
  readCachedTheme,
  resolveTheme,
  writeCachedTheme,
} from './theme';

describe('isThemePreference', () => {
  it('accepts exactly the three valid values', () => {
    expect(isThemePreference('light')).toBe(true);
    expect(isThemePreference('dark')).toBe(true);
    expect(isThemePreference('system')).toBe(true);
  });

  it('rejects anything else', () => {
    expect(isThemePreference('purple')).toBe(false);
    expect(isThemePreference('')).toBe(false);
    expect(isThemePreference('Dark')).toBe(false);
    expect(isThemePreference(null)).toBe(false);
    expect(isThemePreference(undefined)).toBe(false);
    expect(isThemePreference(1)).toBe(false);
  });
});

describe('resolveTheme', () => {
  // FR-THEME-3. The media state is a parameter, so both branches are testable
  // without touching matchMedia.
  it('resolves system from the media state', () => {
    expect(resolveTheme('system', true)).toBe('dark');
    expect(resolveTheme('system', false)).toBe('light');
  });

  // FR-THEME-6: an explicit choice ignores the OS.
  it('ignores the media state for an explicit preference', () => {
    expect(resolveTheme('light', true)).toBe('light');
    expect(resolveTheme('dark', false)).toBe('dark');
  });
});

describe('readCachedTheme', () => {
  beforeEach(() => localStorage.clear());

  it('returns null when the key is absent', () => {
    expect(readCachedTheme()).toBeNull();
  });

  it('returns a cached valid value', () => {
    localStorage.setItem(THEME_STORAGE_KEY, 'dark');
    expect(readCachedTheme()).toBe('dark');
  });

  // FR-SEC-4: the stored value is attacker-controllable — any script or
  // extension on the origin can write it — so it is validated before use.
  it('returns null for a corrupted value', () => {
    localStorage.setItem(THEME_STORAGE_KEY, 'purple');
    expect(readCachedTheme()).toBeNull();
  });

  // FR-FLASH-3: localStorage blocked by privacy settings must not break boot.
  it('returns null when localStorage throws', () => {
    const spy = vi.spyOn(Storage.prototype, 'getItem').mockImplementation(() => {
      throw new Error('blocked');
    });
    expect(readCachedTheme()).toBeNull();
    spy.mockRestore();
  });
});

describe('writeCachedTheme', () => {
  beforeEach(() => localStorage.clear());

  it('writes under the shared key', () => {
    writeCachedTheme('light');
    expect(localStorage.getItem(THEME_STORAGE_KEY)).toBe('light');
  });

  it('swallows a throwing localStorage', () => {
    const spy = vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => {
      throw new Error('blocked');
    });
    expect(() => writeCachedTheme('dark')).not.toThrow();
    spy.mockRestore();
  });
});

describe('applyThemeClass', () => {
  afterEach(() => document.documentElement.classList.remove('dark'));

  // FR-THEME-4: the class on <html>, nothing else.
  it('adds and removes the dark class on the document element', () => {
    applyThemeClass('dark');
    expect(document.documentElement.classList.contains('dark')).toBe(true);
    applyThemeClass('light');
    expect(document.documentElement.classList.contains('dark')).toBe(false);
  });
});
