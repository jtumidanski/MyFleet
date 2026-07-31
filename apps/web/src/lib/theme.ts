import type { ThemePreference } from '../types/models/user';

/**
 * Pure theme helpers. No React, no network; the only DOM this module touches is
 * the single classList call in applyThemeClass. Keeping the logic here rather
 * than inside the provider is what makes it testable without a DOM-heavy
 * harness.
 */

// Shared with the pre-paint script in apps/web/index.html. If this changes,
// that script changes too — src/test/conventions.test.ts pins the script's
// presence but cannot check the key for you.
export const THEME_STORAGE_KEY = 'myfleet.theme';

// The COMPUTED outcome, distinct from the user's ThemePreference: `system` is a
// valid preference but never a valid resolved value. This type stays out of
// types/models/user.ts because it never crosses the wire.
export type ResolvedTheme = 'light' | 'dark';

const VALID: readonly string[] = ['light', 'dark', 'system'];

export function isThemePreference(value: unknown): value is ThemePreference {
  return typeof value === 'string' && VALID.includes(value);
}

/**
 * The cached preference, or null if absent, corrupted, or unreadable.
 *
 * The stored value is attacker-controllable — any script or extension on the
 * origin can write it — so it is validated against the allow-list before use
 * (FR-SEC-4), and a blocked localStorage yields null rather than throwing
 * (FR-FLASH-3).
 */
export function readCachedTheme(): ThemePreference | null {
  try {
    const raw = localStorage.getItem(THEME_STORAGE_KEY);
    return isThemePreference(raw) ? raw : null;
  } catch {
    return null;
  }
}

/** Best-effort cache write. This is a cache, not a source of truth (FR-PERSIST-2). */
export function writeCachedTheme(preference: ThemePreference): void {
  try {
    localStorage.setItem(THEME_STORAGE_KEY, preference);
  } catch {
    // Storage blocked by privacy settings. The preference still applies for
    // this session; only the pre-paint hint is lost, so the next load flashes
    // once rather than failing to boot.
  }
}

/**
 * FR-THEME-3. `prefersDark` is a parameter rather than a matchMedia read so
 * both branches are testable without mocking jsdom.
 */
export function resolveTheme(preference: ThemePreference, prefersDark: boolean): ResolvedTheme {
  if (preference === 'system') return prefersDark ? 'dark' : 'light';
  return preference;
}

/**
 * FR-THEME-4: the `dark` class on <html>, and nothing else — tailwind.config.ts
 * is already set to `darkMode: ['class']`. The class name is a literal; the
 * stored preference is never concatenated into it (FR-SEC-4).
 */
export function applyThemeClass(resolved: ResolvedTheme): void {
  document.documentElement.classList.toggle('dark', resolved === 'dark');
}
