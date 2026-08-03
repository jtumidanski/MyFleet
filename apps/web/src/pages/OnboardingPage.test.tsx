import { describe, it, expect, vi, beforeEach } from 'vitest';
import { screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { renderWithProviders } from '../test/renderWithProviders';
import { inviteService } from '../services/api/InviteService';
import { OnboardingPage } from './OnboardingPage';
import type { Invite } from '../types/models/invite';
import type { AuthContextValue } from '../context/AuthContext';
import type { User } from '../types/models/user';

const navigate = vi.fn();
vi.mock('react-router-dom', async () => {
  const actual = await vi.importActual<typeof import('react-router-dom')>('react-router-dom');
  return { ...actual, useNavigate: () => navigate };
});

vi.mock('../services/api/InviteService', () => ({
  inviteService: { listPending: vi.fn(), acceptInvite: vi.fn() },
}));

// Both are mocked: useAcceptInvite mints a token, and the API client's 401
// retry path imports refreshAccessToken from the same module.
vi.mock('../lib/api/refresh', () => ({
  mintAccessToken: vi.fn().mockResolvedValue('new-token'),
  refreshAccessToken: vi.fn().mockResolvedValue('new-token'),
}));

// OnboardingPage now renders SignedInFooter, which consumes useAuth().
// renderWithProviders deliberately does NOT wrap AuthProvider, so the context
// module is mocked here — the pattern LoginPage.test.tsx, AppLayout.test.tsx
// and ProfileMenu.test.tsx already use.
const mockAuth = vi.fn<() => AuthContextValue>();
vi.mock('../context/AuthContext', () => ({
  useAuth: () => mockAuth(),
}));

function signedInUser(overrides: Partial<User['attributes']> = {}): User {
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

function fleetlessAuth(): AuthContextValue {
  return {
    user: signedInUser(),
    activeFleetId: null,
    role: null,
    platformAdmin: false,
    isAuthenticated: true,
    isLoading: false,
    login: vi.fn(),
    logout: vi.fn().mockResolvedValue(undefined),
  };
}

function pendingInvite(overrides: Partial<Invite['attributes']> = {}): Invite {
  return {
    type: 'invites',
    id: 'i1',
    attributes: {
      fleetId: 'f1',
      fleetName: 'Tumidanski Household',
      email: 'jane@example.com',
      role: 'member',
      token: 'tok-abc',
      expiresAt: '2099-01-01T00:00:00Z',
      invitedByUserId: 'u1',
      ...overrides,
    },
  };
}

beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(inviteService.listPending).mockResolvedValue({ data: [], meta: undefined });
  mockAuth.mockReturnValue(fleetlessAuth());
});

describe('OnboardingPage', () => {
  // The reported symptom: a user with a pending invite logs in, has no fleet,
  // and onboarding offered only "create a fleet" — so the invite waiting for
  // them was invisible and they were pushed into starting a second, empty fleet.
  it('offers a waiting invite by fleet name', async () => {
    vi.mocked(inviteService.listPending).mockResolvedValue({
      data: [pendingInvite()],
      meta: undefined,
    });

    renderWithProviders(<OnboardingPage />);

    expect(await screen.findByText(/Tumidanski Household/)).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /accept invite/i })).toBeInTheDocument();
  });

  it('accepts the invite by its token and lands on the dashboard', async () => {
    vi.mocked(inviteService.listPending).mockResolvedValue({
      data: [pendingInvite()],
      meta: undefined,
    });
    vi.mocked(inviteService.acceptInvite).mockResolvedValue(pendingInvite());

    renderWithProviders(<OnboardingPage />);
    await userEvent.click(await screen.findByRole('button', { name: /accept invite/i }));

    await waitFor(() => expect(inviteService.acceptInvite).toHaveBeenCalledWith('tok-abc'));
    await waitFor(() => expect(navigate).toHaveBeenCalledWith('/', { replace: true }));
  });

  // Creating a fleet stays available and unchanged — someone with a stale
  // invite they don't want must not be trapped on this page.
  it('still offers fleet creation when an invite is waiting', async () => {
    vi.mocked(inviteService.listPending).mockResolvedValue({
      data: [pendingInvite()],
      meta: undefined,
    });

    renderWithProviders(<OnboardingPage />);

    expect(await screen.findByLabelText(/fleet name/i)).toBeInTheDocument();
  });

  // With nothing waiting the page must not grow an empty invite section.
  it('shows no invite section when nothing is pending', async () => {
    renderWithProviders(<OnboardingPage />);

    expect(await screen.findByLabelText(/fleet name/i)).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /accept invite/i })).not.toBeInTheDocument();
  });

  // FR-ONBOARD-1/2. The exit from the account has to be here on the first-run
  // path too — the user who signed in with the wrong Google account has no
  // pending invite, and before this footer existed their only options were to
  // create a fleet they did not want or hand-clear localStorage.
  it('offers a way out of the account when nothing is pending', async () => {
    renderWithProviders(<OnboardingPage />);

    expect(await screen.findByLabelText(/fleet name/i)).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Sign out' })).toBeInTheDocument();
    expect(screen.getByText('Signed in as ada@example.com')).toBeInTheDocument();
  });

  it('offers a way out of the account when an invite is waiting', async () => {
    vi.mocked(inviteService.listPending).mockResolvedValue({
      data: [pendingInvite()],
      meta: undefined,
    });

    renderWithProviders(<OnboardingPage />);

    expect(await screen.findByText(/Tumidanski Household/)).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Sign out' })).toBeInTheDocument();
  });
});
