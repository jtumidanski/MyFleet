import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { toast } from 'sonner';
import { SignedInFooter } from './SignedInFooter';
import type { AuthContextValue } from '../../context/AuthContext';
import type { User } from '../../types/models/user';

// The pattern eleven other test files use (ProfileMenu.test.tsx:8-11): mock the
// context module rather than wrapping in a real AuthProvider, which would mount
// a live useMe query whose `enabled` depends on ambient localStorage.
const mockAuth = vi.fn<() => AuthContextValue>();
vi.mock('../../context/AuthContext', () => ({
  useAuth: () => mockAuth(),
}));

vi.mock('sonner', () => ({ toast: { error: vi.fn(), success: vi.fn() } }));

function user(overrides: Partial<User['attributes']> = {}): User {
  return {
    id: 'u1',
    type: 'users',
    attributes: {
      displayName: 'Ada Lovelace',
      email: 'ada@example.com',
      avatarUrl: '',
      themePreference: 'system',
      ...overrides,
    },
  };
}

function renderFooter(overrides: Partial<AuthContextValue> = {}) {
  mockAuth.mockReturnValue({
    user: user(),
    activeFleetId: null,
    role: null,
    platformAdmin: false,
    isAuthenticated: true,
    isLoading: false,
    login: vi.fn(),
    logout: vi.fn().mockResolvedValue(undefined),
    ...overrides,
  });
  return render(<SignedInFooter />);
}

describe('SignedInFooter', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockAuth.mockReset();
  });

  it('names the account by email', () => {
    renderFooter();
    expect(screen.getByText('Signed in as ada@example.com')).toBeInTheDocument();
  });

  it('falls back to the display name when there is no email', () => {
    renderFooter({ user: user({ email: '' }) });
    expect(screen.getByText('Signed in as Ada Lovelace')).toBeInTheDocument();
  });

  // FR-IDENT-3. The control must survive the missing-identity case — this is
  // the user who is MOST stuck, not least.
  it('omits the identity line entirely when there is neither, keeping the control', async () => {
    const userEvents = userEvent.setup();
    const logout = vi.fn().mockResolvedValue(undefined);
    renderFooter({ user: user({ email: '', displayName: '' }), logout });

    expect(screen.queryByText(/Signed in as/)).not.toBeInTheDocument();

    await userEvents.click(screen.getByRole('button', { name: 'Sign out' }));
    expect(logout).toHaveBeenCalledTimes(1);
  });

  // FR-A11Y-2: `Not you?` is adjacent text, so the accessible name is exactly
  // 'Sign out'. getByRole with an exact string name is the assertion — it fails
  // if the words leak into the button.
  it('exposes the control as a button named exactly "Sign out"', () => {
    renderFooter();

    const button = screen.getByRole('button', { name: 'Sign out' });
    expect(button).toHaveAttribute('type', 'button');
    expect(screen.getByText(/Not you\?/)).toBeInTheDocument();
  });

  // FR-A11Y-3: no heading element, so the host page's heading outline is
  // unchanged by mounting this.
  it('renders no heading element', () => {
    const { container } = renderFooter();
    expect(container.querySelector('h1, h2, h3, h4, h5, h6')).toBeNull();
  });

  // FR-A11Y-1: keyboard reachable and activatable with no custom key handling.
  it('is reachable by Tab and activated by Enter', async () => {
    const userEvents = userEvent.setup();
    const logout = vi.fn().mockResolvedValue(undefined);
    renderFooter({ logout });

    await userEvents.tab();
    expect(screen.getByRole('button', { name: 'Sign out' })).toHaveFocus();

    await userEvents.keyboard('{Enter}');
    expect(logout).toHaveBeenCalledTimes(1);
  });

  // FR-SIGNOUT-1: the ONE thing a click may do.
  it('calls logout exactly once on click', async () => {
    const userEvents = userEvent.setup();
    const logout = vi.fn().mockResolvedValue(undefined);
    renderFooter({ logout });

    await userEvents.click(screen.getByRole('button', { name: 'Sign out' }));

    expect(logout).toHaveBeenCalledTimes(1);
  });

  // FR-SIGNOUT-5 / FR-A11Y-4: the real `disabled` attribute, so a double-click
  // cannot issue two revocations and assistive tech is told the control is off.
  it('disables the control while sign-out is in flight, so a double click issues one logout', async () => {
    const userEvents = userEvent.setup();
    // Never settles: the component stays in its in-flight state for the whole
    // test, which is exactly the window being asserted on.
    const logout = vi.fn(() => new Promise<void>(() => {}));
    renderFooter({ logout });

    const button = screen.getByRole('button', { name: 'Sign out' });
    await userEvents.click(button);

    expect(button).toBeDisabled();

    await userEvents.click(button);
    expect(logout).toHaveBeenCalledTimes(1);
  });

  // FR-SIGNOUT-4. Unreachable through today's transport — logoutRequest()
  // terminates its fetch with `.catch(() => undefined)` (lib/hooks/api/auth.ts:70),
  // so logout() has exactly one outcome. Injecting a rejecting logout through
  // the mocked context is the only way to reach the branch, and the test stays
  // valid once task-022 makes the branch live.
  it('reports a failed sign-out, leaves the user signed in, and lets nothing escape', async () => {
    const escaped: unknown[] = [];
    const onUnhandled = (reason: unknown) => escaped.push(reason);
    process.on('unhandledRejection', onUnhandled);

    try {
      const userEvents = userEvent.setup();
      const logout = vi.fn().mockRejectedValue(new Error('network down'));
      renderFooter({ logout });

      await userEvents.click(screen.getByRole('button', { name: 'Sign out' }));

      expect(toast.error).toHaveBeenCalledWith('network down');
      // Re-enabled: the user is still signed in and must be able to retry.
      expect(screen.getByRole('button', { name: 'Sign out' })).toBeEnabled();

      // One macrotask boundary is enough for Node to have reported any
      // rejection that reached the top level.
      await new Promise((resolve) => setTimeout(resolve, 0));
      expect(escaped).toEqual([]);
    } finally {
      process.off('unhandledRejection', onUnhandled);
    }
  });

  it('falls back to generic copy when the failure carries no message', async () => {
    const userEvents = userEvent.setup();
    const logout = vi.fn().mockRejectedValue(new Error(''));
    renderFooter({ logout });

    await userEvents.click(screen.getByRole('button', { name: 'Sign out' }));

    expect(toast.error).toHaveBeenCalledWith('Could not sign out');
  });

  // FR-SHARED-3: defence-in-depth, so callers can mount it unconditionally.
  it('renders nothing when there is no authenticated user', () => {
    const { container } = renderFooter({ user: null });
    expect(container).toBeEmptyDOMElement();
  });
});
