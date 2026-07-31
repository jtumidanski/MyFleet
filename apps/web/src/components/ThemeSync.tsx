import { useEffect, useRef } from 'react';
import { useAuth } from '../context/AuthContext';
import { useTheme } from '../context/ThemeContext';
import { isThemePreference } from '../lib/theme';

/**
 * One-way bridge: server preference -> theme state. Renders nothing.
 *
 * Housed here rather than inside AuthContext so AuthContext stays unaware that
 * theming exists and ThemeContext stays unaware that auth exists (design §3.3).
 * That separation is the difference between a theme provider testable with a
 * bare render() and one needing a QueryClientProvider plus a token fixture.
 */
export function ThemeSync() {
  const { user } = useAuth();
  const { adoptServerPreference, clearLocalOverride } = useTheme();

  // Tracks whether a session has been observed, so sign-out is distinguishable
  // from "never signed in" — the latter must not clear an override the user set
  // on a pre-auth page.
  const wasSignedIn = useRef(false);

  const serverPreference = user?.attributes.themePreference;

  useEffect(() => {
    // Validated even though it comes from our own service: an older service, or
    // a tampered response, must not put an out-of-range value into theme state.
    if (!isThemePreference(serverPreference)) return;
    wasSignedIn.current = true;
    adoptServerPreference(serverPreference);
  }, [serverPreference, adoptServerPreference]);

  useEffect(() => {
    // AuthContext.logout already clears authKeys.all from the query cache; the
    // session-local override has to go too, or the next sign-in on this device
    // would ignore the server value. Per FR-PERSIST-7 the localStorage cache is
    // NOT cleared, so the login page keeps the theme the device was last using.
    if (user || !wasSignedIn.current) return;
    wasSignedIn.current = false;
    clearLocalOverride();
  }, [user, clearLocalOverride]);

  return null;
}
