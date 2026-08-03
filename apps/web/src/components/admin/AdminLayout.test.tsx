import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
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

// The fleet-detail crumb reaches for this on /admin/fleets/:id; the console's
// data layer is not what is under test here.
vi.mock('../../lib/hooks/api/admin', () => ({
  useAdminFleet: () => ({ data: undefined, isLoading: true }),
}));

function renderAdminLayout(path = '/admin', overrides: Partial<AuthContextValue> = {}) {
  mockAuth.mockReturnValue({
    user: {
      id: 'u1',
      type: 'users',
      attributes: {
        displayName: 'Ada Lovelace',
        email: 'ada@example.com',
        avatarUrl: '',
        themePreference: 'system',
      },
    },
    activeFleetId: null,
    role: null,
    platformAdmin: true,
    isAuthenticated: true,
    isLoading: false,
    login: vi.fn(),
    logout: vi.fn(),
    ...overrides,
  });
  return render(
    <MemoryRouter initialEntries={[path]}>
      <ThemeProvider>
        <Routes>
          <Route path="/admin" element={<AdminLayout />}>
            <Route index element={<div>page content</div>} />
            <Route path="fleets" element={<div>fleets</div>} />
          </Route>
        </Routes>
      </ThemeProvider>
    </MemoryRouter>,
  );
}

describe('AdminLayout', () => {
  beforeEach(() => {
    localStorage.clear();
    resetMatchMedia();
    mutate.mockReset();
    document.cookie = 'sidebar_state=; path=/; max-age=0';
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

  // FR-BRAND-2: the console's lockup returns to the console overview rather
  // than ejecting the operator. "Back to my fleet" is how you leave.
  it('points the brand lockup at the console overview', () => {
    renderAdminLayout();
    expect(screen.getByRole('link', { name: 'MyFleet admin home' })).toHaveAttribute(
      'href',
      '/admin',
    );
  });

  it('still leaves the console via Back to my fleet', () => {
    renderAdminLayout();
    expect(screen.getByRole('link', { name: /back to my fleet/i })).toHaveAttribute('href', '/');
  });

  // FR-CRUMB-2: rooted at Admin, with no Home crumb anywhere under /admin. The
  // first crumb's target matches the brand lockup's, which is the whole point.
  it('roots the breadcrumb at Admin', () => {
    renderAdminLayout('/admin');
    expect(screen.getByText('Admin')).toHaveAttribute('aria-current', 'page');
    expect(screen.queryByText('Home')).not.toBeInTheDocument();
  });

  it('links the Admin crumb to the console root on a deeper route', () => {
    renderAdminLayout('/admin/fleets');
    expect(screen.getByRole('link', { name: 'Admin' })).toHaveAttribute('href', '/admin');
    // FrameNav's "Fleets" nav link and AppBreadcrumb's "Fleets" crumb both
    // render the text "Fleets" once this route is active, so the query is
    // scoped to the breadcrumb landmark to disambiguate.
    const breadcrumb = screen.getByRole('navigation', { name: 'Breadcrumb' });
    expect(within(breadcrumb).getByText('Fleets')).toHaveAttribute('aria-current', 'page');
  });

  it('gathers the identity and the sign-out action under one profile menu', async () => {
    const user = userEvent.setup();
    const logout = vi.fn();
    renderAdminLayout('/admin', { logout });

    expect(screen.queryByRole('button', { name: 'Sign out' })).not.toBeInTheDocument();

    await user.click(screen.getByRole('button', { name: 'Account menu' }));
    await user.click(screen.getByRole('menuitem', { name: 'Sign out' }));

    expect(logout).toHaveBeenCalledTimes(1);
  });

  it('collapses and expands the sidebar', async () => {
    const user = userEvent.setup();
    const { container } = renderAdminLayout();

    const shell = container.querySelector('[data-side="left"]');
    expect(shell).toHaveAttribute('data-state', 'expanded');

    await user.click(screen.getByRole('button', { name: 'Toggle Sidebar' }));

    expect(shell).toHaveAttribute('data-state', 'collapsed');
  });

  it('keeps main at p-6', () => {
    const { container } = renderAdminLayout();
    expect(container.querySelector('main')).toHaveClass('p-6');
  });

  // risks.md: "two main landmarks" is a named risk. querySelector alone finds
  // the FIRST <main>, so if SidebarInset ever reverted to upstream's own
  // <main> this would still pass on the p-6 check while silently leaving a
  // second, unstyled landmark in the DOM. State the invariant directly.
  it('renders exactly one main landmark', () => {
    const { container } = renderAdminLayout();
    expect(container.querySelectorAll('main')).toHaveLength(1);
  });
});
