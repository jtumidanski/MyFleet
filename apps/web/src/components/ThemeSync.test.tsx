import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, act } from '@testing-library/react';
import { ThemeSync } from './ThemeSync';
import { ThemeProvider, useTheme } from '../context/ThemeContext';
import { THEME_STORAGE_KEY } from '../lib/theme';
import { resetMatchMedia } from '../test/setup';
import type { AuthContextValue } from '../context/AuthContext';
import type { User } from '../types/models/user';

const mockAuth = vi.fn<() => AuthContextValue>();
vi.mock('../context/AuthContext', () => ({
  useAuth: () => mockAuth(),
}));

function userWithTheme(themePreference: string): User {
  return {
    id: 'u1',
    type: 'users',
    attributes: {
      email: 'a@b.com',
      displayName: 'A',
      avatarUrl: '',
      themePreference,
    },
  } as User;
}

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

function Probe() {
  const { preference } = useTheme();
  return <span data-testid="preference">{preference}</span>;
}

// Exposes ThemeContext's user-driven setter so a test can simulate the user
// making a local choice (the thing that sets hasLocalOverride in
// ThemeContext), independent of ThemeSync's server-driven path.
function Controls() {
  const { setPreference } = useTheme();
  return (
    <button type="button" onClick={() => setPreference('light')}>
      choose-light
    </button>
  );
}

function renderSync() {
  return render(
    <ThemeProvider>
      <ThemeSync />
      <Probe />
    </ThemeProvider>,
  );
}

describe('ThemeSync', () => {
  beforeEach(() => {
    localStorage.clear();
    resetMatchMedia();
    document.documentElement.classList.remove('dark');
    mockAuth.mockReset();
  });

  // FR-PERSIST-3: the server is authoritative, which is what makes the
  // preference propagate to a new device on first sign-in.
  it('adopts the server preference over the cached value', () => {
    localStorage.setItem(THEME_STORAGE_KEY, 'light');
    mockAuth.mockReturnValue(baseAuth({ user: userWithTheme('dark') }));

    renderSync();

    expect(screen.getByTestId('preference')).toHaveTextContent('dark');
    expect(document.documentElement.classList.contains('dark')).toBe(true);
  });

  it('leaves the cached preference alone when signed out', () => {
    localStorage.setItem(THEME_STORAGE_KEY, 'dark');
    mockAuth.mockReturnValue(baseAuth({ user: null }));

    renderSync();

    expect(screen.getByTestId('preference')).toHaveTextContent('dark');
  });

  // FR-PERSIST-7: sign-out must clear the SESSION-local override (not the
  // localStorage cache) so the next sign-in on this device adopts the server
  // value again rather than being suppressed by a choice made in a prior
  // session. Exercises the actual sign-out branch (a signed-in -> signed-out
  // transition), not just the "never signed in" short-circuit: it forces a
  // local override via the real ThemeContext setter, signs out, then signs
  // back in with a DIFFERENT server preference and asserts the new value wins
  // — which only happens if the override was actually cleared.
  it('clears the local override on sign-out so a later sign-in adopts the server value again', () => {
    mockAuth.mockReturnValue(baseAuth({ user: userWithTheme('dark') }));
    const { rerender } = render(
      <ThemeProvider>
        <ThemeSync />
        <Controls />
        <Probe />
      </ThemeProvider>,
    );
    expect(screen.getByTestId('preference')).toHaveTextContent('dark');

    // User makes a local choice this session — sets hasLocalOverride in
    // ThemeContext, which would normally make adoptServerPreference a no-op.
    act(() => screen.getByText('choose-light').click());
    expect(screen.getByTestId('preference')).toHaveTextContent('light');

    // Sign out.
    mockAuth.mockReturnValue(baseAuth({ user: null }));
    rerender(
      <ThemeProvider>
        <ThemeSync />
        <Controls />
        <Probe />
      </ThemeProvider>,
    );

    // Sign back in with a preference distinct from both the earlier server
    // value ('dark') and the local override ('light'), so adoption is
    // unambiguous.
    mockAuth.mockReturnValue(baseAuth({ user: userWithTheme('system') }));
    rerender(
      <ThemeProvider>
        <ThemeSync />
        <Controls />
        <Probe />
      </ThemeProvider>,
    );

    expect(screen.getByTestId('preference')).toHaveTextContent('system');
  });

  // FR-SEC-4 applies to the wire too: a value outside the allow-list — from an
  // older service, or a tampered response — must not reach theme state.
  it('ignores an out-of-range server value', () => {
    localStorage.setItem(THEME_STORAGE_KEY, 'light');
    mockAuth.mockReturnValue(baseAuth({ user: userWithTheme('purple') }));

    renderSync();

    expect(screen.getByTestId('preference')).toHaveTextContent('light');
  });

  it('renders nothing', () => {
    mockAuth.mockReturnValue(baseAuth({ user: null }));
    const { container } = render(
      <ThemeProvider>
        <ThemeSync />
      </ThemeProvider>,
    );
    expect(container.textContent).toBe('');
  });
});
