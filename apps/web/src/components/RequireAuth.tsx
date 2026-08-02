import type { ReactNode } from 'react';
import { Navigate, useLocation } from 'react-router-dom';
import { useAuth } from '../context/AuthContext';
import { Skeleton } from './ui/skeleton';

/**
 * Routes an authenticated user may visit without an active fleet. Invite accept
 * belongs here because an invitee has no fleet until the invite is accepted —
 * bouncing them to onboarding would make the accept route unreachable.
 */
const FLEETLESS_ROUTES = [/^\/onboarding$/, /^\/invites\/[^/]+\/accept$/];

function allowsFleetlessAccess(pathname: string): boolean {
  return FLEETLESS_ROUTES.some((route) => route.test(pathname));
}

/**
 * Route guard.
 * - Unauthenticated users are redirected to /login.
 * - Authenticated users without an active fleet are redirected to /onboarding,
 *   except on routes that exist precisely to get them a fleet
 *   (see FLEETLESS_ROUTES).
 * Server-side authz remains authoritative; this is navigation convenience only.
 */
export function RequireAuth({ children }: { children: ReactNode }) {
  const { isAuthenticated, isLoading, activeFleetId } = useAuth();
  const location = useLocation();

  if (isLoading) {
    return (
      <div className="flex min-h-screen items-center justify-center">
        <Skeleton className="h-8 w-48" />
      </div>
    );
  }

  if (!isAuthenticated) {
    // Carry the attempted path so LoginPage can ask auth-service to return
    // here; otherwise an invite link clicked while logged out is lost in the
    // OAuth round-trip and the invitee lands on onboarding with no token.
    return <Navigate to="/login" replace state={{ from: location.pathname + location.search }} />;
  }

  if (activeFleetId === null && !allowsFleetlessAccess(location.pathname)) {
    return <Navigate to="/onboarding" replace />;
  }

  return <>{children}</>;
}
