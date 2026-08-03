import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
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
    platformAdmin: false,
    isAuthenticated: true,
    isLoading: false,
    login: vi.fn(),
    logout: vi.fn(),
    ...overrides,
  };
}

function renderLayout(auth: AuthContextValue, path = '/') {
  mockAuth.mockReturnValue(auth);
  return render(
    <MemoryRouter initialEntries={[path]}>
      <ThemeProvider>
        <Routes>
          <Route element={<AppLayout />}>
            <Route index element={<div>page content</div>} />
            <Route path="/vehicles" element={<div>vehicles</div>} />
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
    // document.cookie persists across tests in a file: without this, one
    // test's collapse leaks into the next (design §8.3).
    document.cookie = 'sidebar_state=; path=/; max-age=0';
  });

  // FR-BRAND-1/2/3: the lockup is a link home with an accessible name, because
  // the wordmark is hidden in the collapsed rail.
  it('renders the brand lockup as a link to the dashboard', () => {
    renderLayout(baseAuth({}));

    const brand = screen.getByRole('link', { name: 'MyFleet home' });
    expect(brand).toHaveAttribute('href', '/');
    expect(brand.querySelector('svg')).toBeTruthy();
  });

  // FR-TOGGLE-1: the theme control must be present and reachable in the header.
  it('renders a reachable theme toggle in the header', () => {
    renderLayout(baseAuth({}));

    expect(screen.getByRole('button', { name: /Theme:/ })).toBeInTheDocument();
  });

  // FR-PROFILE-1: no loose name text and no standalone Sign out button.
  it('gathers the identity and the sign-out action under one profile menu', async () => {
    const user = userEvent.setup();
    renderLayout(baseAuth({}));

    expect(screen.queryByRole('button', { name: 'Sign out' })).not.toBeInTheDocument();

    await user.click(screen.getByRole('button', { name: 'Account menu' }));

    expect(screen.getByText('Ada Lovelace')).toBeInTheDocument();
    expect(screen.getByText('ada@example.com')).toBeInTheDocument();
  });

  it('still signs the user out from the profile menu', async () => {
    const user = userEvent.setup();
    const logout = vi.fn();
    renderLayout(baseAuth({ logout }));

    await user.click(screen.getByRole('button', { name: 'Account menu' }));
    await user.click(screen.getByRole('menuitem', { name: 'Sign out' }));

    expect(logout).toHaveBeenCalledTimes(1);
  });

  // FR-SIDEBAR-3/4/5. Asserted on data-state and the cookie, never on width:
  // jsdom has no CSS engine, so the rail's APPEARANCE is verified in real
  // Chromium instead (design §8.4).
  it('collapses and expands the sidebar, persisting the choice', async () => {
    const user = userEvent.setup();
    const { container } = renderLayout(baseAuth({}));

    const shell = container.querySelector('[data-side="left"]');
    expect(shell).toHaveAttribute('data-state', 'expanded');

    await user.click(screen.getByRole('button', { name: 'Toggle Sidebar' }));

    expect(shell).toHaveAttribute('data-state', 'collapsed');
    expect(document.cookie).toContain('sidebar_state=false');

    await user.click(screen.getByRole('button', { name: 'Toggle Sidebar' }));

    expect(shell).toHaveAttribute('data-state', 'expanded');
  });

  // FR-CRUMB-2/5: in this shell the trail is rooted at Home.
  it('renders the breadcrumb for the current route', () => {
    renderLayout(baseAuth({}), '/vehicles');

    expect(screen.getByRole('link', { name: 'Home' })).toHaveAttribute('href', '/');
    // Scoped to the breadcrumb landmark: the sidebar nav also renders a
    // "Vehicles" link at this route, so an unscoped query is ambiguous once
    // FrameNav and AppBreadcrumb are rendered together (they are each tested
    // in isolation elsewhere, e.g. FrameHeader.test.tsx, where this collision
    // does not arise because FrameNav isn't part of that render).
    const breadcrumbNav = screen.getByRole('navigation', { name: 'Breadcrumb' });
    expect(within(breadcrumbNav).getByText('Vehicles')).toHaveAttribute('aria-current', 'page');
  });

  it('renders the shell root crumb on the shell root', () => {
    renderLayout(baseAuth({}), '/');

    expect(screen.getByText('Home')).toHaveAttribute('aria-current', 'page');
  });

  it('renders the routed page content via Outlet', () => {
    renderLayout(baseAuth({}));

    expect(screen.getByText('page content')).toBeInTheDocument();
  });

  // FR-HEADER-4: no page's content position changes.
  it('keeps main at p-6', () => {
    const { container } = renderLayout(baseAuth({}));
    expect(container.querySelector('main')).toHaveClass('p-6');
  });

  // risks.md: "two main landmarks" is a named risk. querySelector alone finds
  // the FIRST <main>, so if SidebarInset ever reverted to upstream's own
  // <main> this would still pass on the p-6 check while silently leaving a
  // second, unstyled landmark in the DOM. State the invariant directly.
  it('renders exactly one main landmark', () => {
    const { container } = renderLayout(baseAuth({}));
    expect(container.querySelectorAll('main')).toHaveLength(1);
  });
});

// FR-ADMIN-UI-5: the nav entry is a convenience, not a control — its absence
// hides the door, the server refuses entry.
describe('AppLayout admin entry point', () => {
  beforeEach(() => {
    localStorage.clear();
    resetMatchMedia();
    document.cookie = 'sidebar_state=; path=/; max-age=0';
  });

  it('hides the Admin nav entry from non-admins', () => {
    renderLayout(baseAuth({ platformAdmin: false }));
    expect(screen.queryByRole('link', { name: 'Admin' })).not.toBeInTheDocument();
  });

  it('shows the Admin nav entry to admins', () => {
    renderLayout(baseAuth({ platformAdmin: true }));
    expect(screen.getByRole('link', { name: 'Admin' })).toBeInTheDocument();
  });

  // FR-NAV-5: exact matching, or the entry lights up on every route.
  it('does not light the Admin entry from the dashboard', () => {
    renderLayout(baseAuth({ platformAdmin: true }), '/');
    expect(screen.getByRole('link', { name: 'Admin' })).toHaveAttribute('data-active', 'false');
  });
});
