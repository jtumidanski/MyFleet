import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen } from '@testing-library/react';
import { MemoryRouter, Routes, Route } from 'react-router-dom';
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

function renderGuarded(initialEntry: string) {
  return render(
    <MemoryRouter initialEntries={[initialEntry]}>
      <Routes>
        <Route path="/login" element={<div>login screen</div>} />
        <Route path="/onboarding" element={<div>onboarding screen</div>} />
        <Route
          path="/"
          element={
            <RequireAuth>
              <div>protected content</div>
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
});
