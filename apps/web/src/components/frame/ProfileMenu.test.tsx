import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { ProfileMenu } from './ProfileMenu';
import type { AuthContextValue } from '../../context/AuthContext';
import type { User } from '../../types/models/user';

const mockAuth = vi.fn<() => AuthContextValue>();
vi.mock('../../context/AuthContext', () => ({
  useAuth: () => mockAuth(),
}));

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

function renderMenu(overrides: Partial<AuthContextValue> = {}) {
  mockAuth.mockReturnValue({
    user: user(),
    activeFleetId: null,
    role: null,
    platformAdmin: false,
    isAuthenticated: true,
    isLoading: false,
    login: vi.fn(),
    logout: vi.fn(),
    ...overrides,
  });
  return render(<ProfileMenu />);
}

describe('ProfileMenu', () => {
  beforeEach(() => {
    mockAuth.mockReset();
  });

  // FR-PROFILE-2: an icon-sized trigger with an accessible name.
  it('renders a labelled trigger', () => {
    renderMenu();
    expect(screen.getByRole('button', { name: 'Account menu' })).toBeInTheDocument();
  });

  // FR-PROFILE-3: exactly two regions — an identity header, then Sign out.
  it('shows the display name and the email when opened', async () => {
    const userEvents = userEvent.setup();
    renderMenu();

    await userEvents.click(screen.getByRole('button', { name: 'Account menu' }));

    expect(screen.getByText('Ada Lovelace')).toBeInTheDocument();
    expect(screen.getByText('ada@example.com')).toBeInTheDocument();
  });

  it('offers no Settings or Admin item — those stay sidebar destinations', async () => {
    const userEvents = userEvent.setup();
    renderMenu();

    await userEvents.click(screen.getByRole('button', { name: 'Account menu' }));

    expect(screen.getAllByRole('menuitem')).toHaveLength(1);
    expect(screen.getByRole('menuitem', { name: 'Sign out' })).toBeInTheDocument();
  });

  it('signs the user out', async () => {
    const userEvents = userEvent.setup();
    const logout = vi.fn();
    renderMenu({ logout });

    await userEvents.click(screen.getByRole('button', { name: 'Account menu' }));
    await userEvents.click(screen.getByRole('menuitem', { name: 'Sign out' }));

    expect(logout).toHaveBeenCalledTimes(1);
  });

  it('renders the avatar when one is set', () => {
    const { container } = renderMenu({ user: user({ avatarUrl: 'https://example.test/ada.png' }) });

    // Queried by tag, not by role: the image is decorative (alt="") so it is
    // deliberately absent from the accessibility tree.
    expect(container.querySelector('img')).toHaveAttribute('src', 'https://example.test/ada.png');
  });

  it('renders no image when there is no avatar url', () => {
    const { container } = renderMenu();
    expect(container.querySelector('img')).toBeNull();
  });

  // FR-PROFILE-4, through the component rather than the pure function.
  it('shows "Account" for a user with neither name nor email', async () => {
    const userEvents = userEvent.setup();
    renderMenu({ user: user({ displayName: '', email: '' }) });

    await userEvents.click(screen.getByRole('button', { name: 'Account menu' }));

    expect(screen.getByText('Account')).toBeInTheDocument();
  });
});
