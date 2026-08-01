import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, act } from '@testing-library/react';
import { MemoryRouter, Routes, Route } from 'react-router-dom';
import { AppLayout } from './AppLayout';
import { ThemeProvider } from '../context/ThemeContext';
import type { AuthContextValue } from '../context/AuthContext';
import { resetMatchMedia } from '../test/setup';

// Mock the auth context so the layout can be exercised without standing up
// the full provider/query stack — mirrors the pattern in RequireAuth.test.tsx.
const mockAuth = vi.fn<() => AuthContextValue>();
vi.mock('../context/AuthContext', () => ({
  useAuth: () => mockAuth(),
}));

// ThemeToggle drives a mutation; stub it out the same way ThemeToggle.test.tsx
// does so this file only asserts on placement, not the toggle's own behaviour.
const mutate = vi.fn();
vi.mock('../lib/hooks/api/auth', () => ({
  useUpdateTheme: () => ({ mutate }),
}));

function baseAuth(overrides: Partial<AuthContextValue>): AuthContextValue {
  return {
    user: null,
    activeFleetId: null,
    role: null,
    isAuthenticated: true,
    isLoading: false,
    login: vi.fn(),
    logout: vi.fn(),
    ...overrides,
  };
}

function renderLayout(auth: AuthContextValue) {
  mockAuth.mockReturnValue(auth);
  return render(
    <MemoryRouter initialEntries={['/']}>
      <ThemeProvider>
        <Routes>
          <Route element={<AppLayout />}>
            <Route index element={<div>page content</div>} />
          </Route>
        </Routes>
      </ThemeProvider>
    </MemoryRouter>,
  );
}

describe('AppLayout', () => {
  beforeEach(() => {
    localStorage.clear();
    resetMatchMedia();
    document.documentElement.classList.remove('dark');
    mutate.mockReset();
  });

  // FR-ICON-10: the mark sits beside the wordmark in the sidebar header.
  it('renders the brand mark beside the MyFleet wordmark', () => {
    renderLayout(baseAuth({}));

    const brandRow = screen.getByText('MyFleet').parentElement;
    expect(brandRow?.querySelector('svg')).toBeTruthy();
  });

  // FR-TOGGLE-1: the theme control must be present and reachable in the header.
  it('renders a reachable theme toggle in the header', () => {
    renderLayout(baseAuth({}));

    expect(screen.getByRole('button', { name: /Theme:/ })).toBeInTheDocument();
  });

  it('still signs the user out when Sign out is clicked', () => {
    const logout = vi.fn();
    renderLayout(baseAuth({ logout }));

    act(() => screen.getByRole('button', { name: 'Sign out' }).click());

    expect(logout).toHaveBeenCalledTimes(1);
  });

  it('renders the routed page content via Outlet', () => {
    renderLayout(baseAuth({}));

    expect(screen.getByText('page content')).toBeInTheDocument();
  });
});
