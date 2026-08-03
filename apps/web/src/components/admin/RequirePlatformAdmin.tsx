import type { ReactNode } from 'react';
import { Navigate } from 'react-router-dom';
import { useAuth } from '../../context/AuthContext';
import { Skeleton } from '../ui/skeleton';

/**
 * Admin route guard.
 *
 * Requires authentication and `platformAdmin`; sends everyone else to `/`.
 *
 * It deliberately does NOT require an activeFleetId, and — the structural point
 * — the /admin branch is a SIBLING of the RequireAuth/AppLayout branch in
 * App.tsx rather than nested inside it, so RequireAuth's fleetless redirect to
 * /onboarding never applies here. An administrator standing in the wreckage of
 * the system purge they just ran must stay in the console to verify it and, if
 * they were wrong, cancel it (FR-ADMIN-UI-4, FR-ADMIN-UI-14).
 *
 * An exemption flag on the shared guard would have worked today and been
 * dropped silently by a later refactor; a route-tree position is harder to lose.
 *
 * Server-side authz remains authoritative; this is navigation convenience only.
 */
export function RequirePlatformAdmin({ children }: { children: ReactNode }) {
  const { isAuthenticated, isLoading, platformAdmin } = useAuth();

  if (isLoading) {
    return (
      <div className="flex min-h-screen items-center justify-center">
        <Skeleton className="h-8 w-48" />
      </div>
    );
  }
  if (!isAuthenticated) {
    return <Navigate to="/login" replace />;
  }
  // Home, not a 403 page: a non-admin has no business knowing what is here, and
  // the server returns 403 to anyone who asks anyway.
  if (!platformAdmin) {
    return <Navigate to="/" replace />;
  }
  return <>{children}</>;
}
