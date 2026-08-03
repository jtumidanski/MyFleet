import { describe, it, expect, vi, beforeEach } from 'vitest';
import { screen } from '@testing-library/react';
import { Route, Routes } from 'react-router-dom';
import { ApiError } from '@myfleet/shared-ts';
import { renderWithProviders } from '../test/renderWithProviders';
import { inviteService } from '../services/api/InviteService';
import { InviteAcceptPage } from './InviteAcceptPage';
import type { AuthContextValue } from '../context/AuthContext';
import type { User } from '../types/models/user';
import type { Invite } from '../types/models/invite';

const navigate = vi.fn();
vi.mock('react-router-dom', async () => {
  const actual = await vi.importActual<typeof import('react-router-dom')>('react-router-dom');
  return { ...actual, useNavigate: () => navigate };
});

vi.mock('../services/api/InviteService', () => ({
  inviteService: { listPending: vi.fn(), acceptInvite: vi.fn() },
}));

// useAcceptInvite mints a token on success, and the API client's 401 retry path
// imports refreshAccessToken from the same module.
vi.mock('../lib/api/refresh', () => ({
  mintAccessToken: vi.fn().mockResolvedValue('new-token'),
  refreshAccessToken: vi.fn().mockResolvedValue('new-token'),
}));

// useAcceptInvite raises its own toast on failure; there is no <Toaster> here.
vi.mock('sonner', () => ({ toast: { error: vi.fn(), success: vi.fn() } }));

const mockAuth = vi.fn<() => AuthContextValue>();
vi.mock('../context/AuthContext', () => ({
  useAuth: () => mockAuth(),
}));

function signedInUser(): User {
  return {
    id: 'u1',
    type: 'users',
    attributes: {
      displayName: 'Ada Lovelace',
      email: 'ada@example.com',
      avatarUrl: '',
      themePreference: 'system',
    },
  };
}

function acceptedInvite(): Invite {
  return {
    type: 'invites',
    id: 'i1',
    attributes: {
      fleetId: 'f1',
      fleetName: 'Tumidanski Household',
      email: 'ada@example.com',
      role: 'member',
      token: 'tok-abc',
      expiresAt: '2099-01-01T00:00:00Z',
      invitedByUserId: 'u2',
    },
  };
}

// useParams supplies the token, so the page must be mounted under its real
// route pattern — rendered bare, `token` is undefined, the effect returns
// early, and the page never leaves `pending`.
function renderAcceptPage() {
  return renderWithProviders(
    <Routes>
      <Route path="/invites/:token/accept" element={<InviteAcceptPage />} />
    </Routes>,
    { route: '/invites/tok-abc/accept' },
  );
}

beforeEach(() => {
  vi.clearAllMocks();
  mockAuth.mockReturnValue({
    user: signedInUser(),
    activeFleetId: null,
    role: null,
    platformAdmin: false,
    isAuthenticated: true,
    isLoading: false,
    login: vi.fn(),
    logout: vi.fn().mockResolvedValue(undefined),
  });
});

describe('InviteAcceptPage', () => {
  // FR-INVITE-1. This is the sharpest form of the trap: the screen tells the
  // user they used the wrong account, and its only control routes a fleetless
  // user straight back to /onboarding.
  it('offers a way out of the account on the error state', async () => {
    vi.mocked(inviteService.acceptInvite).mockRejectedValue(
      new ApiError(409, 'conflict', 'Wrong account for this invite', 'sent to another address'),
    );

    renderAcceptPage();

    expect(await screen.findByText('Could not accept invite')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Sign out' })).toBeInTheDocument();
    expect(screen.getByText('Signed in as ada@example.com')).toBeInTheDocument();
  });

  // FR-INVITE-3: adding a control must not disturb the screen it is added to.
  //
  // The asserted text is the ApiError's `message`, not its `detail`. That is
  // current behaviour, verified rather than assumed: InviteAcceptPage re-wraps
  // an already-wrapped ApiError through createErrorFromUnknown, which only
  // reads `detail` off a raw { status, body } envelope — an ApiError instance
  // falls to the `e instanceof Error` branch and arrives with
  // detail === undefined. So `detail || message` yields the envelope's title.
  // Pre-existing, out of scope here (this task must not touch this screen's
  // error handling), and recorded in context.md as a follow-up candidate.
  it('leaves the existing error copy and Go to Dashboard button intact', async () => {
    vi.mocked(inviteService.acceptInvite).mockRejectedValue(
      new ApiError(409, 'conflict', 'Wrong account for this invite', 'sent to another address'),
    );

    renderAcceptPage();

    expect(await screen.findByText('Wrong account for this invite')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Go to Dashboard' })).toBeInTheDocument();
  });

  // FR-INVITE-2: transient, traps nobody, so a sign-out control here is noise.
  it('offers no sign-out while the accept is still in flight', async () => {
    vi.mocked(inviteService.acceptInvite).mockImplementation(() => new Promise<Invite>(() => {}));

    renderAcceptPage();

    expect(await screen.findByText('Accepting invite…')).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Sign out' })).not.toBeInTheDocument();
  });

  it('offers no sign-out on the success state', async () => {
    vi.mocked(inviteService.acceptInvite).mockResolvedValue(acceptedInvite());

    renderAcceptPage();

    expect(await screen.findByText('You have joined the fleet!')).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Sign out' })).not.toBeInTheDocument();
  });
});
