import { describe, it, expect, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import { MemoryRouter, Routes, Route } from 'react-router-dom';
import { RequirePlatformAdmin } from './RequirePlatformAdmin';
import type { AuthContextValue } from '../../context/AuthContext';

// Mock the auth context rather than standing up the provider/query stack —
// the same pattern RequireAuth.test.tsx and AppLayout.test.tsx use.
const mockAuth = vi.fn<() => AuthContextValue>();
vi.mock('../../context/AuthContext', () => ({
  useAuth: () => mockAuth(),
}));

function baseAuth(overrides: Partial<AuthContextValue>): AuthContextValue {
  return {
    user: null,
    activeFleetId: null,
    role: null,
    platformAdmin: false,
    isAuthenticated: false,
    isLoading: false,
    login: vi.fn(),
    logout: vi.fn(),
    ...overrides,
  };
}

function renderAt(auth: AuthContextValue) {
  mockAuth.mockReturnValue(auth);
  return render(
    <MemoryRouter initialEntries={['/admin']}>
      <Routes>
        <Route path="/" element={<div>home</div>} />
        <Route path="/login" element={<div>login</div>} />
        <Route path="/onboarding" element={<div>onboarding</div>} />
        <Route
          path="/admin"
          element={
            <RequirePlatformAdmin>
              <div>console</div>
            </RequirePlatformAdmin>
          }
        />
      </Routes>
    </MemoryRouter>,
  );
}

describe('RequirePlatformAdmin', () => {
  it('sends an anonymous visitor to /login', () => {
    renderAt(baseAuth({}));
    expect(screen.getByText('login')).toBeInTheDocument();
  });

  it('sends a signed-in non-admin home, not to a 403 page', () => {
    renderAt(baseAuth({ isAuthenticated: true, activeFleetId: 'f1', role: 'owner' }));
    expect(screen.getByText('home')).toBeInTheDocument();
    expect(screen.queryByText('console')).not.toBeInTheDocument();
  });

  it('admits an admin', () => {
    renderAt(baseAuth({ isAuthenticated: true, platformAdmin: true, activeFleetId: 'f1' }));
    expect(screen.getByText('console')).toBeInTheDocument();
  });

  // FR-ADMIN-AUTH-9 / risks.md R5. This is the scenario that matters: the admin
  // has just run a system purge, their fleet is gone, and they need to stay in
  // the console to verify the result and cancel within the recovery window.
  it('admits a FLEETLESS admin and does not redirect to onboarding', () => {
    renderAt(baseAuth({ isAuthenticated: true, platformAdmin: true, activeFleetId: null }));
    expect(screen.getByText('console')).toBeInTheDocument();
    expect(screen.queryByText('onboarding')).not.toBeInTheDocument();
  });
});
