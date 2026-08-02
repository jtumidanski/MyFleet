import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { MemoryRouter } from 'react-router-dom';
import type { ReactNode } from 'react';
import { AppRoutes } from '../../App';
import { ThemeProvider } from '../../context/ThemeContext';
import type { AuthContextValue } from '../../context/AuthContext';
import { resetMatchMedia } from '../../test/setup';

/**
 * risks.md R5's residual risk: a future refactor renesting /admin under
 * RequireAuth. That would compile, pass every other test, and only fail in
 * production at the exact moment recovery matters — an admin who has just run a
 * system purge would be bounced to /onboarding with a five-day window ticking.
 *
 * This imports the REAL route tree. An earlier version rebuilt a replica of it
 * here, which meant renesting /admin in App.tsx would not have failed anything:
 * the test exercised its own copy and proved only that the copy was right.
 *
 * It exercises the POST-SYSTEM-PURGE state specifically, not merely a fleetless
 * account, because those are different bugs with the same symptom and only one
 * of them is catastrophic.
 */

const mockAuth = vi.fn<() => AuthContextValue>();
vi.mock('../../context/AuthContext', () => ({
  useAuth: () => mockAuth(),
  AuthProvider: ({ children }: { children: ReactNode }) => children,
}));

const mutate = vi.fn();
vi.mock('../../lib/hooks/api/auth', () => ({
  useUpdateTheme: () => ({ mutate }),
  authKeys: { all: ['auth'], me: () => ['auth', 'me'] },
}));

// The admin screens' data layer is not what is under test; stub it so the route
// tree renders without a server.
vi.mock('../../lib/hooks/api/admin', () => ({
  useAdminStats: () => ({ data: undefined, isLoading: true, isError: false }),
  useAdminFleets: () => ({ data: undefined, isLoading: true, isError: false }),
  useAdminFleet: () => ({ data: undefined, isLoading: true, isError: false }),
  useAdminUsers: () => ({ data: undefined, isLoading: true, isError: false }),
  usePurgeOperations: () => ({ data: undefined, isLoading: true, isError: false }),
  useAuditEvents: () => ({ data: undefined, isLoading: true, isError: false }),
  useCreatePurge: () => ({ mutate: vi.fn(), mutateAsync: vi.fn(), isPending: false }),
  useCancelPurge: () => ({ mutate: vi.fn(), isPending: false }),
  useRetryPurge: () => ({ mutate: vi.fn(), isPending: false }),
}));

/** The identity /auth/me returns after the admin's own fleet was purged. */
function postPurgeAdmin(): AuthContextValue {
  return {
    user: null,
    activeFleetId: null,
    role: null,
    platformAdmin: true,
    isAuthenticated: true,
    isLoading: false,
    login: vi.fn(),
    logout: vi.fn(),
  };
}

function renderAppAt(route: string) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter initialEntries={[route]}>
        <ThemeProvider>
          <AppRoutes />
        </ThemeProvider>
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

describe('after a system purge', () => {
  beforeEach(() => {
    resetMatchMedia();
    mockAuth.mockReturnValue(postPurgeAdmin());
  });

  it('keeps a now-fleetless admin inside the console', () => {
    renderAppAt('/admin/purges');
    expect(screen.getByText(/platform admin/i)).toBeInTheDocument();
  });

  it('lets them reach every admin screen without a fleet', () => {
    for (const route of [
      '/admin',
      '/admin/fleets',
      '/admin/users',
      '/admin/purges',
      '/admin/audit',
    ]) {
      const { unmount } = renderAppAt(route);
      expect(screen.getByText(/platform admin/i)).toBeInTheDocument();
      unmount();
    }
  });

  // The control that gives the assertions above their meaning. The SAME identity,
  // taken through the ordinary branch, IS redirected — so /admin is genuinely
  // escaping RequireAuth's fleetless redirect rather than the redirect being
  // absent from the tree altogether.
  it('is redirected away from the ordinary shell, proving the exemption is real', () => {
    renderAppAt('/');
    expect(screen.queryByText(/platform admin/i)).not.toBeInTheDocument();
  });
});
