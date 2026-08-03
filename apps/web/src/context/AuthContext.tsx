import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useState,
  type ReactNode,
} from 'react';
import { useQueryClient } from '@tanstack/react-query';
import { ApiError } from '@myfleet/shared-ts';
import { useMe, logoutRequest, authKeys } from '../lib/hooks/api/auth';
import { captureTokenFromHash, clearAccessToken, getAccessToken } from '../lib/api/token';
import { buildLoginUrl } from '../lib/api/authRoutes';
import type { FleetRole, User } from '../types/models/user';

export interface AuthContextValue {
  user: User | null;
  activeFleetId: string | null;
  role: FleetRole | null;
  platformAdmin: boolean;
  isAuthenticated: boolean;
  isLoading: boolean;
  /** `returnTo` is a site-relative path to land on after the OAuth round-trip. */
  login: (returnTo?: string) => void;
  /**
   * Ends the local session unconditionally; rejects when the server-side
   * revoke failed, so the caller can warn that the session may still be live
   * on the server. See AuthProvider for the full contract.
   */
  logout: () => Promise<void>;
}

const AuthContext = createContext<AuthContextValue | undefined>(undefined);

export function AuthProvider({ children }: { children: ReactNode }) {
  const queryClient = useQueryClient();
  // Capture an access token from the URL fragment exactly once on mount,
  // before any identity query runs. `hasToken` drives the `useMe` enablement.
  const [hasToken, setHasToken] = useState<boolean>(() => {
    captureTokenFromHash();
    return !!getAccessToken();
  });

  const me = useMe();

  // Keep `hasToken` in sync if the token query resolves/clears (e.g. refresh
  // failure clears the token inside the API client).
  //
  // Clearing the token IS the logout: `isAuthenticated` goes false and
  // RequireAuth navigates to /login. So a 503 must not reach it. `useMe` is the
  // first request on every mount, and during a fleet-service outage a user with
  // an expired token who reloads goes /auth/me 401 → refresh 503 →
  // refreshAccessToken throws ApiError(503) → out of fetchAuthenticated →
  // me.isError. Clearing here would sign them out on someone else's outage —
  // the exact logout the 503 refresh path exists to prevent, reached without
  // refresh.ts clearing anything.
  //
  // `instanceof ApiError` rather than createErrorFromUnknown: the rejection
  // arrives already typed, and createErrorFromUnknown would flatten it to
  // status 0 (it only reads `status` off a raw fetch envelope), silently
  // restoring the bug.
  useEffect(() => {
    if (!me.isError) return;
    if (me.error instanceof ApiError && me.error.status === 503) return;
    clearAccessToken();
    setHasToken(false);
  }, [me.isError, me.error]);

  const login = useCallback((returnTo?: string) => {
    // Full navigation to auth-service so the OAuth redirect chain works.
    window.location.href = buildLoginUrl(returnTo);
  }, []);

  /**
   * Ends the session.
   *
   * Signs out LOCALLY in every case — access token cleared, identity cache
   * purged — because the user asked to leave and a server failure does not
   * revoke that request. It then REJECTS to report that the *server* side may
   * not have completed: the refresh-token family may still be live, and only
   * the caller is positioned to tell the user so. Both happen, not either.
   *
   * `finally` rethrows implicitly, which is what makes "teardown always runs"
   * and "the rejection still propagates" one construct rather than two.
   */
  const logout = useCallback(async () => {
    try {
      await logoutRequest();
    } finally {
      clearAccessToken();
      setHasToken(false);
      queryClient.removeQueries({ queryKey: authKeys.all });
    }
  }, [queryClient]);

  const value = useMemo<AuthContextValue>(() => {
    const data = me.data;
    return {
      user: data?.user ?? null,
      activeFleetId: data?.activeFleetId ?? null,
      role: data?.role ?? null,
      platformAdmin: data?.platformAdmin ?? false,
      isAuthenticated: hasToken && !!data?.user,
      isLoading: hasToken && me.isLoading,
      login,
      logout,
    };
  }, [me.data, me.isLoading, hasToken, login, logout]);

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
}

export function useAuth(): AuthContextValue {
  const ctx = useContext(AuthContext);
  if (!ctx) throw new Error('useAuth must be used within an AuthProvider');
  return ctx;
}
