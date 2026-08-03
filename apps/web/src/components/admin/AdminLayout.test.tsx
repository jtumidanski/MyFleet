import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen } from '@testing-library/react';
import { MemoryRouter, Routes, Route } from 'react-router-dom';
import { AdminLayout } from './AdminLayout';
import { ThemeProvider } from '../../context/ThemeContext';
import type { AuthContextValue } from '../../context/AuthContext';
import { resetMatchMedia } from '../../test/setup';

const mockAuth = vi.fn<() => AuthContextValue>();
vi.mock('../../context/AuthContext', () => ({
  useAuth: () => mockAuth(),
}));

// ThemeToggle drives a mutation; stub it the same way AppLayout.test.tsx does so
// this file asserts on the shell only.
const mutate = vi.fn();
vi.mock('../../lib/hooks/api/auth', () => ({
  useUpdateTheme: () => ({ mutate }),
}));

function renderAdminLayout() {
  mockAuth.mockReturnValue({
    user: null,
    activeFleetId: null,
    role: null,
    platformAdmin: true,
    isAuthenticated: true,
    isLoading: false,
    login: vi.fn(),
    logout: vi.fn(),
  });
  return render(
    <MemoryRouter initialEntries={['/admin']}>
      <ThemeProvider>
        <Routes>
          <Route path="/admin" element={<AdminLayout />}>
            <Route index element={<div>page content</div>} />
          </Route>
        </Routes>
      </ThemeProvider>
    </MemoryRouter>,
  );
}

describe('AdminLayout', () => {
  beforeEach(() => {
    resetMatchMedia();
  });

  it('shows the persistent mode band with an explicit exit', () => {
    // FR-ADMIN-UI-3: the band states the caller's scope in plain words and
    // offers a way out, on every screen.
    renderAdminLayout();
    expect(screen.getByText(/platform admin/i)).toBeInTheDocument();
    expect(screen.getByRole('link', { name: /back to my fleet/i })).toBeInTheDocument();
  });

  it('states the stale-claim caveat in plain words', () => {
    // FR-ADMIN-AUTH-7: the console must say that revoking admin does not take
    // effect until the token refreshes. Burying it in a tooltip defeats it.
    renderAdminLayout();
    expect(screen.getByText(/up to 15 minutes/i)).toBeInTheDocument();
  });

  it('links every admin section', () => {
    renderAdminLayout();
    for (const label of ['Overview', 'Fleets', 'Users', 'Purges', 'Audit log']) {
      expect(screen.getByRole('link', { name: label })).toBeInTheDocument();
    }
  });

  it('renders the routed screen', () => {
    renderAdminLayout();
    expect(screen.getByText('page content')).toBeInTheDocument();
  });
});
