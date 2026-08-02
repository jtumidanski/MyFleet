import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, act } from '@testing-library/react';
import { MemoryRouter, Routes, Route } from 'react-router-dom';
import type { AuthContextValue } from '../context/AuthContext';
import { THEME_STORAGE_KEY } from '../lib/theme';
import { resetMatchMedia } from '../test/setup';

// Mock the auth context so the page can be exercised without the provider/query
// stack — the pattern AppLayout.test.tsx already uses.
const mockAuth = vi.fn<() => AuthContextValue>();
vi.mock('../context/AuthContext', () => ({
  useAuth: () => mockAuth(),
}));

const login = vi.fn();

function baseAuth(overrides: Partial<AuthContextValue> = {}): AuthContextValue {
  return {
    user: null,
    activeFleetId: null,
    role: null,
    isAuthenticated: false,
    isLoading: false,
    login,
    logout: vi.fn(),
    ...overrides,
  };
}

/**
 * consumeLoginError memoises at module scope (design §4.1), so each case needs
 * a fresh module registry — hence resetModules plus a dynamic import of the
 * page rather than a top-level one.
 *
 * ThemeProvider must come from that same fresh import, not a top-level static
 * one: resetModules clears the registry, so LoginPage's own (transitive)
 * import of ThemeContext after the reset would otherwise create a *different*
 * React Context object than a ThemeProvider bound before the reset — same
 * category of module-identity bug as the loginError memoisation trap, one
 * layer up. useTheme would then throw "must be used within a ThemeProvider"
 * even though a ThemeProvider is right there in the tree.
 *
 * `state` is the router location state RequireAuth sets when it bounces an
 * unauthenticated visitor here.
 */
async function renderLogin(
  hash = '',
  auth: Partial<AuthContextValue> = {},
  state?: { from: string },
) {
  window.history.replaceState(null, '', `/login${hash}`);
  mockAuth.mockReturnValue(baseAuth(auth));
  vi.resetModules();
  const [{ LoginPage }, { ThemeProvider }] = await Promise.all([
    import('./LoginPage'),
    import('../context/ThemeContext'),
  ]);
  return render(
    <MemoryRouter initialEntries={[{ pathname: '/login', state }]}>
      <ThemeProvider>
        <Routes>
          <Route path="/login" element={<LoginPage />} />
          <Route path="/" element={<div>dashboard</div>} />
          <Route path="/invites/:token/accept" element={<div>invite accept</div>} />
        </Routes>
      </ThemeProvider>
    </MemoryRouter>,
  );
}

const GENERIC_FAILURE =
  "Sign-in didn't complete. Nothing was saved — try again, or use a different Google account.";

function signInButton() {
  return screen.getByRole('button', { name: /Google|Try again/ });
}

describe('LoginPage', () => {
  beforeEach(() => {
    localStorage.clear();
    resetMatchMedia();
    document.documentElement.classList.remove('dark');
    login.mockReset();
  });

  // FR-PAGE-1 / FR-PAGE-2 / FR-PAGE-4 / FR-PAGE-6: a typographic composition,
  // not a card, that says what MyFleet is before asking for an account.
  it('renders the headline, the scopes line and no card', async () => {
    const { container } = await renderLogin();

    expect(screen.getByText('Your cars.')).toBeInTheDocument();
    expect(screen.getByText('One place.')).toBeInTheDocument();
    expect(screen.getByText(/name, email address, and profile photo/i)).toBeInTheDocument();
    expect(container.querySelector('.bg-card')).toBeNull();
  });

  // FR-STATE-1: the default state.
  it('offers an enabled Continue with Google button by default', async () => {
    await renderLogin();

    expect(signInButton()).toHaveTextContent('Continue with Google');
    expect(signInButton()).toBeEnabled();
    expect(screen.queryByRole('alert')).toBeNull();
  });

  // FR-STATE-2 / FR-STATE-3 / FR-A11Y-2: the press is acknowledged, a second
  // activation is impossible, and the status is announced non-visually.
  it('disables itself and announces the handoff when pressed', async () => {
    await renderLogin();

    act(() => signInButton().click());

    expect(signInButton()).toBeDisabled();
    expect(screen.getByRole('status')).toHaveTextContent('Redirecting to Google');
    expect(login).toHaveBeenCalledTimes(1);

    // A double-click cannot start two OAuth flows.
    act(() => signInButton().click());
    expect(login).toHaveBeenCalledTimes(1);
  });

  // RequireAuth stashes the attempted path in router state; LoginPage has to
  // forward it so auth-service can send the invitee back to the accept route.
  // This is the ONLY guard on the return-path handoff — a redesign of this page
  // that drops `from` still typechecks, because login's parameter is optional.
  it('forwards the attempted path to login()', async () => {
    await renderLogin('', {}, { from: '/invites/abc123/accept' });

    act(() => signInButton().click());

    expect(login).toHaveBeenCalledWith('/invites/abc123/accept');
  });

  it('calls login() with no return path on a direct visit', async () => {
    await renderLogin();

    act(() => signInButton().click());

    expect(login).toHaveBeenCalledWith(undefined);
  });

  // FR-STATE-4 / FR-A11Y-1.
  it('shows an announced danger callout and relabels the button on failure', async () => {
    await renderLogin('#error=auth_failed');

    expect(screen.getByRole('alert')).toHaveTextContent(GENERIC_FAILURE);
    expect(signInButton()).toHaveTextContent('Try again');
  });

  // FR-STATE-5: cancelling is a choice, not a fault — no red, no alert, no
  // relabelled button.
  it('shows a neutral line and keeps the label when the user cancelled', async () => {
    await renderLogin('#error=cancelled');

    expect(screen.queryByRole('alert')).toBeNull();
    expect(screen.getByText('Sign-in cancelled.')).toBeInTheDocument();
    expect(signInButton()).toHaveTextContent('Continue with Google');
  });

  // FR-STATE-6: an attacker-supplied fragment never reaches the DOM.
  it('renders generic copy for a garbage code without echoing it', async () => {
    await renderLogin('#error=%3Cscript%3Ealert(1)%3C%2Fscript%3E');

    expect(screen.getByRole('alert')).toHaveTextContent(GENERIC_FAILURE);
    expect(document.body.textContent).not.toContain('alert(1)');
  });

  // FR-STATE-7.
  it('strips the error fragment from the URL', async () => {
    await renderLogin('#error=auth_failed');

    expect(window.location.hash).toBe('');
  });

  // FR-PRETOGGLE-1 / FR-PRETOGGLE-2 / FR-PRETOGGLE-3: a working theme control
  // with no session behind it. Spying on `fetch` rather than on a mocked
  // useUpdateTheme is the stronger claim — it fails if ANY request appears.
  it('cycles the theme without issuing a request', async () => {
    const fetchSpy = vi
      .spyOn(globalThis, 'fetch')
      .mockImplementation(() => Promise.reject(new Error('no network in tests')));
    localStorage.setItem(THEME_STORAGE_KEY, 'light');
    await renderLogin();

    const toggle = () => screen.getByRole('button', { name: /^Theme:/ });
    expect(toggle()).toHaveAttribute('aria-label', 'Theme: light. Switch to dark.');

    act(() => toggle().click());
    expect(toggle()).toHaveAttribute('aria-label', 'Theme: dark. Switch to system.');

    act(() => toggle().click());
    expect(toggle()).toHaveAttribute('aria-label', 'Theme: system. Switch to light.');

    act(() => toggle().click());
    expect(toggle()).toHaveAttribute('aria-label', 'Theme: light. Switch to dark.');

    expect(fetchSpy).not.toHaveBeenCalled();
    fetchSpy.mockRestore();
  });

  // FR-STATE-9: the pre-existing bounce is preserved.
  it('sends an already-authenticated visitor into the app', async () => {
    await renderLogin('', { isAuthenticated: true });

    expect(screen.getByText('dashboard')).toBeInTheDocument();
  });

  // …and the bounce honours `from` too: a visitor who signed in in another tab
  // and came back must not lose the invite they were bounced off.
  it('bounces an already-authenticated visitor to the attempted path', async () => {
    await renderLogin('', { isAuthenticated: true }, { from: '/invites/abc123/accept' });

    expect(screen.getByText('invite accept')).toBeInTheDocument();
  });
});
