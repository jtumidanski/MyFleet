import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen } from '@testing-library/react';
import { MemoryRouter, Routes, Route, useLocation } from 'react-router-dom';
import { RequireAuth } from '../components/RequireAuth';
import type { AuthContextValue } from './AuthContext';

// Mock the auth context so RequireAuth can be exercised against arbitrary
// authentication states without standing up the full provider/query stack.
const mockAuth = vi.fn<() => AuthContextValue>();
vi.mock('./AuthContext', () => ({
  useAuth: () => mockAuth(),
}));

function baseAuth(overrides: Partial<AuthContextValue>): AuthContextValue {
  return {
    user: null,
    activeFleetId: null,
    role: null,
    isAuthenticated: false,
    isLoading: false,
    login: vi.fn(),
    logout: vi.fn(),
    ...overrides,
  };
}

// Stands in for LoginPage, exposing the path the guard asked login to return to.
function LoginProbe() {
  const location = useLocation();
  const from = (location.state as { from?: string } | null)?.from ?? '';
  return (
    <div>
      <div>login screen</div>
      <div data-testid="return-to">{from}</div>
    </div>
  );
}

function renderGuarded(initialEntry: string) {
  return render(
    <MemoryRouter initialEntries={[initialEntry]}>
      <Routes>
        <Route path="/login" element={<LoginProbe />} />
        <Route path="/onboarding" element={<div>onboarding screen</div>} />
        <Route
          path="/"
          element={
            <RequireAuth>
              <div>protected content</div>
            </RequireAuth>
          }
        />
        <Route
          path="/invites/:token/accept"
          element={
            <RequireAuth>
              <div>invite accept screen</div>
            </RequireAuth>
          }
        />
      </Routes>
    </MemoryRouter>,
  );
}

describe('RequireAuth', () => {
  beforeEach(() => mockAuth.mockReset());

  it('redirects unauthenticated users to /login', () => {
    mockAuth.mockReturnValue(baseAuth({ isAuthenticated: false }));
    renderGuarded('/');
    expect(screen.getByText('login screen')).toBeInTheDocument();
    expect(screen.queryByText('protected content')).not.toBeInTheDocument();
  });

  it('renders children when authenticated with an active fleet', () => {
    mockAuth.mockReturnValue(
      baseAuth({ isAuthenticated: true, activeFleetId: 'f1', role: 'owner' }),
    );
    renderGuarded('/');
    expect(screen.getByText('protected content')).toBeInTheDocument();
  });

  it('redirects authenticated users without a fleet to /onboarding', () => {
    mockAuth.mockReturnValue(baseAuth({ isAuthenticated: true, activeFleetId: null }));
    renderGuarded('/');
    expect(screen.getByText('onboarding screen')).toBeInTheDocument();
    expect(screen.queryByText('protected content')).not.toBeInTheDocument();
  });

  // An invitee has no fleet until the invite is accepted, so the onboarding
  // bounce must not swallow the accept route — it would make invites for new
  // users unacceptable.
  it('lets authenticated users without a fleet reach the invite accept route', () => {
    mockAuth.mockReturnValue(baseAuth({ isAuthenticated: true, activeFleetId: null }));
    renderGuarded('/invites/abc123/accept');
    expect(screen.getByText('invite accept screen')).toBeInTheDocument();
    expect(screen.queryByText('onboarding screen')).not.toBeInTheDocument();
  });

  it('still redirects unauthenticated visitors of the invite accept route to /login', () => {
    mockAuth.mockReturnValue(baseAuth({ isAuthenticated: false }));
    renderGuarded('/invites/abc123/accept');
    expect(screen.getByText('login screen')).toBeInTheDocument();
    expect(screen.queryByText('invite accept screen')).not.toBeInTheDocument();
  });

  // Without this the invite token is lost at the login bounce and the invitee
  // lands on onboarding with nothing to accept.
  it('hands the attempted path to /login so it can survive the OAuth round-trip', () => {
    mockAuth.mockReturnValue(baseAuth({ isAuthenticated: false }));
    renderGuarded('/invites/abc123/accept');
    expect(screen.getByTestId('return-to')).toHaveTextContent('/invites/abc123/accept');
  });
});
