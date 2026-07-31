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
  // on a pre-auth page. Set on ANY authenticated render, independent of
  // whether themePreference happens to validate this render: if it were only
  // set inside the adopt effect below, a user whose stored themePreference was
  // corrupted would sign out without ever setting this flag, leaving a stale
  // local override in place to suppress adoption on their NEXT sign-in too.
  const wasSignedIn = useRef(false);

  const serverPreference = user?.attributes.themePreference;

  useEffect(() => {
    if (user) wasSignedIn.current = true;
  }, [user]);

  useEffect(() => {
    // Validated even though it comes from our own service: an older service, or
    // a tampered response, must not put an out-of-range value into theme state.
    if (!isThemePreference(serverPreference)) return;
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
