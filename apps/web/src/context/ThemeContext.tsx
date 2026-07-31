import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useRef,
  useState,
  type ReactNode,
} from 'react';
import type { ThemePreference } from '../types/models/user';
import {
  applyThemeClass,
  readCachedTheme,
  resolveTheme,
  writeCachedTheme,
  type ResolvedTheme,
} from '../lib/theme';

const MEDIA_QUERY = '(prefers-color-scheme: dark)';

export interface ThemeContextValue {
  preference: ThemePreference;
  resolvedTheme: ResolvedTheme;
  /** User intent: applies, caches, and pins the choice for the session. */
  setPreference: (preference: ThemePreference) => void;
  /** Server truth echoed back; ignored once the user has chosen this session. */
  adoptServerPreference: (preference: ThemePreference) => void;
  /** Called on sign-out so the next sign-in adopts the server value afresh. */
  clearLocalOverride: () => void;
}

const ThemeContext = createContext<ThemeContextValue | undefined>(undefined);

/**
 * Client theme state. Deliberately knows nothing about auth and issues no
 * network request — the server value arrives through ThemeSync, which is what
 * lets this provider be unit-tested with a bare render() rather than a
 * QueryClientProvider plus a token fixture (design §3.3).
 */
export function ThemeProvider({ children }: { children: ReactNode }) {
  const [preference, setPreferenceState] = useState<ThemePreference>(
    () => readCachedTheme() ?? 'system',
  );
  const [systemPrefersDark, setSystemPrefersDark] = useState<boolean>(
    () => window.matchMedia(MEDIA_QUERY).matches,
  );

  // Once the user has chosen this session, a background `me` refetch carrying
  // the pre-change server value must not flip the theme back under their cursor
  // (FR-PERSIST-5). Starts false, so the FIRST server value of a session always
  // wins — that is FR-PERSIST-3, and what lands the preference on a new device.
  const hasLocalOverride = useRef(false);

  const resolvedTheme = resolveTheme(preference, systemPrefersDark);

  useEffect(() => {
    applyThemeClass(resolvedTheme);
  }, [resolvedTheme]);

  // Subscribed unconditionally: resolveTheme ignores systemPrefersDark unless
  // the preference is `system`, so FR-THEME-6 falls out of the resolution rule
  // rather than needing conditional subscribe/unsubscribe logic.
  useEffect(() => {
    const query = window.matchMedia(MEDIA_QUERY);
    const onChange = (event: MediaQueryListEvent) => setSystemPrefersDark(event.matches);
    query.addEventListener('change', onChange);
    return () => query.removeEventListener('change', onChange);
  }, []);

  const setPreference = useCallback((next: ThemePreference) => {
    hasLocalOverride.current = true;
    setPreferenceState(next);
    writeCachedTheme(next);
  }, []);

  const adoptServerPreference = useCallback((next: ThemePreference) => {
    if (hasLocalOverride.current) return;
    setPreferenceState(next);
    writeCachedTheme(next);
  }, []);

  const clearLocalOverride = useCallback(() => {
    hasLocalOverride.current = false;
  }, []);

  const value = useMemo<ThemeContextValue>(
    () => ({
      preference,
      resolvedTheme,
      setPreference,
      adoptServerPreference,
      clearLocalOverride,
    }),
    [preference, resolvedTheme, setPreference, adoptServerPreference, clearLocalOverride],
  );

  return <ThemeContext.Provider value={value}>{children}</ThemeContext.Provider>;
}

export function useTheme(): ThemeContextValue {
  const ctx = useContext(ThemeContext);
  if (!ctx) throw new Error('useTheme must be used within a ThemeProvider');
  return ctx;
}
