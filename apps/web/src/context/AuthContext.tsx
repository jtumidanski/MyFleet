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
import { useMe, logoutRequest, authKeys } from '../lib/hooks/api/auth';
import { captureTokenFromHash, clearAccessToken, getAccessToken } from '../lib/api/token';
import { buildLoginUrl } from '../lib/api/authRoutes';
import type { FleetRole, User } from '../types/models/user';

export interface AuthContextValue {
  user: User | null;
  activeFleetId: string | null;
  role: FleetRole | null;
  isAuthenticated: boolean;
  isLoading: boolean;
  /** `returnTo` is a site-relative path to land on after the OAuth round-trip. */
  login: (returnTo?: string) => void;
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
  useEffect(() => {
    if (me.isError) {
      clearAccessToken();
      setHasToken(false);
    }
  }, [me.isError]);

  const login = useCallback((returnTo?: string) => {
    // Full navigation to auth-service so the OAuth redirect chain works.
    window.location.href = buildLoginUrl(returnTo);
  }, []);

  const logout = useCallback(async () => {
    await logoutRequest();
    clearAccessToken();
    setHasToken(false);
    queryClient.removeQueries({ queryKey: authKeys.all });
  }, [queryClient]);

  const value = useMemo<AuthContextValue>(() => {
    const data = me.data;
    return {
      user: data?.user ?? null,
      activeFleetId: data?.activeFleetId ?? null,
      role: data?.role ?? null,
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
