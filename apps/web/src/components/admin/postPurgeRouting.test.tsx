import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen } from '@testing-library/react';
import { MemoryRouter, Routes, Route } from 'react-router-dom';
import { RequireAuth } from '../RequireAuth';
import { RequirePlatformAdmin } from './RequirePlatformAdmin';
import { AdminLayout } from './AdminLayout';
import { ThemeProvider } from '../../context/ThemeContext';
import type { AuthContextValue } from '../../context/AuthContext';
import { resetMatchMedia } from '../../test/setup';

/**
 * risks.md R5's residual risk: a future refactor renesting /admin under
 * RequireAuth. That would compile, pass every other test, and only fail in
 * production at the exact moment recovery matters — an admin who has just run a
 * system purge would be bounced to /onboarding with a five-day window ticking.
 *
 * This test exercises the POST-SYSTEM-PURGE state specifically, not merely a
 * fleetless account, because those are different bugs with the same symptom and
 * only one of them is catastrophic.
 *
 * It mirrors App.tsx's structure rather than importing App, so it asserts the
 * PROPERTY (an admin branch that does not inherit the fleetless redirect)
 * without dragging every page's data layer into the test. The companion
 * assertion below proves the property is real by showing the same identity IS
 * redirected when it goes through RequireAuth.
 */

const mockAuth = vi.fn<() => AuthContextValue>();
vi.mock('../../context/AuthContext', () => ({
  useAuth: () => mockAuth(),
}));

const mutate = vi.fn();
vi.mock('../../lib/hooks/api/auth', () => ({
  useUpdateTheme: () => ({ mutate }),
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

/** The route tree as App.tsx declares it: /admin is a SIBLING, not a child. */
function renderTreeAt(route: string) {
  return render(
    <MemoryRouter initialEntries={[route]}>
      <ThemeProvider>
        <Routes>
          <Route path="/onboarding" element={<div>onboarding</div>} />
          <Route
            element={
              <RequireAuth>
                <div>app shell</div>
              </RequireAuth>
            }
          >
            <Route path="/" element={<div>dashboard</div>} />
          </Route>
          <Route
            path="/admin"
            element={
              <RequirePlatformAdmin>
                <AdminLayout />
              </RequirePlatformAdmin>
            }
          >
            <Route index element={<div>overview</div>} />
            <Route path="fleets" element={<div>fleets</div>} />
            <Route path="users" element={<div>users</div>} />
            <Route path="purges" element={<div>purges</div>} />
            <Route path="audit" element={<div>audit</div>} />
          </Route>
        </Routes>
      </ThemeProvider>
    </MemoryRouter>,
  );
}

describe('after a system purge', () => {
  beforeEach(() => {
    resetMatchMedia();
    mockAuth.mockReturnValue(postPurgeAdmin());
  });

  it('keeps a now-fleetless admin inside the console', async () => {
    renderTreeAt('/admin/purges');
    expect(await screen.findByText(/platform admin/i)).toBeInTheDocument();
    expect(screen.queryByText('onboarding')).not.toBeInTheDocument();
  });

  it('lets them reach every admin screen without a fleet', () => {
    for (const route of [
      '/admin',
      '/admin/fleets',
      '/admin/users',
      '/admin/purges',
      '/admin/audit',
    ]) {
      const { unmount } = renderTreeAt(route);
      expect(screen.getByText(/platform admin/i)).toBeInTheDocument();
      expect(screen.queryByText('onboarding')).not.toBeInTheDocument();
      unmount();
    }
  });

  // The control that gives the two assertions above their meaning. The SAME
  // identity, taken through RequireAuth, IS redirected to /onboarding — so the
  // admin branch is genuinely escaping that redirect rather than the redirect
  // being absent from the test tree altogether.
  it('is redirected to onboarding on the ordinary branch, proving the exemption is real', () => {
    renderTreeAt('/');
    expect(screen.getByText('onboarding')).toBeInTheDocument();
  });
});
