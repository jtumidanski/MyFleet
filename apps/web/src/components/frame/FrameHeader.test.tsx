import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { FrameHeader } from './FrameHeader';
import { SidebarProvider } from '../ui/sidebar';
import { ThemeProvider } from '../../context/ThemeContext';
import type { AuthContextValue } from '../../context/AuthContext';
import { resetMatchMedia } from '../../test/setup';

const mockAuth = vi.fn<() => AuthContextValue>();
vi.mock('../../context/AuthContext', () => ({
  useAuth: () => mockAuth(),
}));

// ThemeToggle drives a mutation; stub it the way the layout tests do so this
// file asserts on placement only.
const mutate = vi.fn();
vi.mock('../../lib/hooks/api/auth', () => ({
  useUpdateTheme: () => ({ mutate }),
}));

function renderHeader(path = '/vehicles') {
  mockAuth.mockReturnValue({
    user: null,
    activeFleetId: null,
    role: null,
    platformAdmin: false,
    isAuthenticated: true,
    isLoading: false,
    login: vi.fn(),
    logout: vi.fn(),
  });
  return render(
    <MemoryRouter initialEntries={[path]}>
      <ThemeProvider>
        <SidebarProvider>
          <FrameHeader />
        </SidebarProvider>
      </ThemeProvider>
    </MemoryRouter>,
  );
}

describe('FrameHeader', () => {
  beforeEach(() => {
    localStorage.clear();
    resetMatchMedia();
    document.documentElement.classList.remove('dark');
    mutate.mockReset();
    mockAuth.mockReset();
  });

  // FR-HEADER-1: trigger · breadcrumb · gap · theme toggle · profile.
  it('lays the controls out left to right', () => {
    renderHeader();

    const names = screen
      .getAllByRole('button')
      .map((button) => button.getAttribute('aria-label') ?? button.textContent ?? '');
    const trigger = names.findIndex((name) => name.includes('Toggle Sidebar'));
    const theme = names.findIndex((name) => name.startsWith('Theme:'));
    const profile = names.indexOf('Account menu');

    expect(trigger).toBeGreaterThanOrEqual(0);
    expect(theme).toBeGreaterThan(trigger);
    // FR-PROFILE-1: immediately right of the theme toggle.
    expect(profile).toBe(theme + 1);
  });

  it('renders the breadcrumb for the current route', () => {
    renderHeader('/vehicles');
    expect(screen.getByRole('navigation', { name: 'Breadcrumb' })).toBeInTheDocument();
    expect(screen.getByText('Vehicles')).toHaveAttribute('aria-current', 'page');
  });

  // FR-PROFILE-1: no loose name text, no standalone Sign out button.
  it('carries no standalone Sign out button', () => {
    renderHeader();
    expect(screen.queryByRole('button', { name: 'Sign out' })).not.toBeInTheDocument();
  });

  // FR-HEADER-2: the height does not change with the sidebar or the route.
  it('has a fixed height', () => {
    const { container } = renderHeader();
    expect(container.querySelector('header')).toHaveClass('h-14');
  });
});
