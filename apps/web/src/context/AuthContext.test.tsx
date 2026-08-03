import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { MemoryRouter, Routes, Route, useLocation } from 'react-router-dom';
import { ApiError } from '@myfleet/shared-ts';
import { RequireAuth } from '../components/RequireAuth';
import { AuthProvider, type AuthContextValue } from './AuthContext';
import { ACCESS_TOKEN_KEY } from '../lib/api/token';

// Mock `useAuth` only, so RequireAuth can be exercised against arbitrary
// authentication states without standing up the full provider/query stack —
// while AuthProvider itself stays REAL, because its token-clearing effect is
// under test below.
const mockAuth = vi.fn<() => AuthContextValue>();
vi.mock('./AuthContext', async (importOriginal) => ({
  ...(await importOriginal<typeof import('./AuthContext')>()),
  useAuth: () => mockAuth(),
}));

// The identity query is the input to the effect under test; drive it directly
// rather than through fetch so each error shape is unambiguous.
const mockUseMe = vi.fn();
vi.mock('../lib/hooks/api/auth', () => ({
  useMe: () => mockUseMe(),
  logoutRequest: vi.fn(),
  authKeys: { all: ['auth'] as const, me: () => ['auth', 'me'] as const },
}));

function baseAuth(overrides: Partial<AuthContextValue>): AuthContextValue {
  return {
    user: null,
    activeFleetId: null,
    role: null,
    platformAdmin: false,
    isAuthenticated: false,
    isLoading: false,
    login: vi.fn(),
    logout: vi.fn(),
    ...overrides,
  };
}

// Stands in for LoginPage, exposing the path the guard asked login to return to.
function LoginProbe() {
  const location = useLocation();
  const from = (location.state as { from?: string } | null)?.from ?? '';
  return (
    <div>
      <div>login screen</div>
      <div data-testid="return-to">{from}</div>
    </div>
  );
}

function renderGuarded(initialEntry: string) {
  return render(
    <MemoryRouter initialEntries={[initialEntry]}>
      <Routes>
        <Route path="/login" element={<LoginProbe />} />
        <Route path="/onboarding" element={<div>onboarding screen</div>} />
        <Route
          path="/"
          element={
            <RequireAuth>
              <div>protected content</div>
            </RequireAuth>
          }
        />
        <Route
          path="/invites/:token/accept"
          element={
            <RequireAuth>
              <div>invite accept screen</div>
            </RequireAuth>
          }
        />
      </Routes>
    </MemoryRouter>,
  );
}

describe('RequireAuth', () => {
  beforeEach(() => mockAuth.mockReset());

  it('redirects unauthenticated users to /login', () => {
    mockAuth.mockReturnValue(baseAuth({ isAuthenticated: false }));
    renderGuarded('/');
    expect(screen.getByText('login screen')).toBeInTheDocument();
    expect(screen.queryByText('protected content')).not.toBeInTheDocument();
  });

  it('renders children when authenticated with an active fleet', () => {
    mockAuth.mockReturnValue(
      baseAuth({ isAuthenticated: true, activeFleetId: 'f1', role: 'owner' }),
    );
    renderGuarded('/');
    expect(screen.getByText('protected content')).toBeInTheDocument();
  });

  it('redirects authenticated users without a fleet to /onboarding', () => {
    mockAuth.mockReturnValue(baseAuth({ isAuthenticated: true, activeFleetId: null }));
    renderGuarded('/');
    expect(screen.getByText('onboarding screen')).toBeInTheDocument();
    expect(screen.queryByText('protected content')).not.toBeInTheDocument();
  });

  // Defence in depth for the fleetless-onboarding bug. Every other test in this
  // file mocks `activeFleetId: null` — a shape the backend did not actually
  // produce, which is why they stayed green while production stranded users.
  // The guard must agree with the pages, which all test `!activeFleetId`; if it
  // only recognises `null`, an empty string means "has a fleet" here and "has
  // none" there, and the user lands on "No fleet selected" with no way forward.
  it('treats an empty activeFleetId as no fleet, exactly as the pages do', () => {
    mockAuth.mockReturnValue(baseAuth({ isAuthenticated: true, activeFleetId: '' }));
    renderGuarded('/');
    expect(screen.getByText('onboarding screen')).toBeInTheDocument();
    expect(screen.queryByText('protected content')).not.toBeInTheDocument();
  });

  // An invitee has no fleet until the invite is accepted, so the onboarding
  // bounce must not swallow the accept route — it would make invites for new
  // users unacceptable.
  it('lets authenticated users without a fleet reach the invite accept route', () => {
    mockAuth.mockReturnValue(baseAuth({ isAuthenticated: true, activeFleetId: null }));
    renderGuarded('/invites/abc123/accept');
    expect(screen.getByText('invite accept screen')).toBeInTheDocument();
    expect(screen.queryByText('onboarding screen')).not.toBeInTheDocument();
  });

  it('still redirects unauthenticated visitors of the invite accept route to /login', () => {
    mockAuth.mockReturnValue(baseAuth({ isAuthenticated: false }));
    renderGuarded('/invites/abc123/accept');
    expect(screen.getByText('login screen')).toBeInTheDocument();
    expect(screen.queryByText('invite accept screen')).not.toBeInTheDocument();
  });

  // Without this the invite token is lost at the login bounce and the invitee
  // lands on onboarding with nothing to accept.
  it('hands the attempted path to /login so it can survive the OAuth round-trip', () => {
    mockAuth.mockReturnValue(baseAuth({ isAuthenticated: false }));
    renderGuarded('/invites/abc123/accept');
    expect(screen.getByTestId('return-to')).toHaveTextContent('/invites/abc123/accept');
  });
});

// Clearing the stored access token IS the logout: `isAuthenticated` goes false
// and RequireAuth navigates to /login. These assert on the STORED TOKEN rather
// than on any context flag, because the token is the thing that survives the
// reload and decides whether the user is signed out.
describe('AuthProvider identity-error handling', () => {
  function renderProvider() {
    const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    return render(
      <QueryClientProvider client={queryClient}>
        <AuthProvider>
          <div>app shell</div>
        </AuthProvider>
      </QueryClientProvider>,
    );
  }

  function meFailedWith(error: unknown) {
    mockUseMe.mockReturnValue({ isError: true, error, data: undefined, isLoading: false });
  }

  beforeEach(() => {
    mockUseMe.mockReset();
    localStorage.clear();
  });

  // The residual logout path. `useMe` is the first request on every mount, so a
  // user with an expired token who reloads during a fleet-service outage goes
  // /auth/me 401 → refresh 503 → refreshAccessToken throws → me.isError. Before
  // the guard, this effect signed them out on someone else's outage — without
  // refresh.ts clearing anything, which is why the refresh-layer tests all
  // stayed green.
  it('keeps the stored token when the identity query fails with a 503', () => {
    localStorage.setItem(ACCESS_TOKEN_KEY, 'still-valid-token');
    meFailedWith(new ApiError(503, 'service_unavailable', 'Service temporarily unavailable'));

    renderProvider();

    expect(screen.getByText('app shell')).toBeInTheDocument();
    expect(localStorage.getItem(ACCESS_TOKEN_KEY)).toBe('still-valid-token');
  });

  // The other half of the guard: a genuinely dead session must still end. A
  // blanket "never clear" would be just as wrong as the blanket clear it
  // replaced, leaving a dead token in storage forever.
  it('still clears the stored token when the identity query fails with a 401', () => {
    localStorage.setItem(ACCESS_TOKEN_KEY, 'dead-token');
    meFailedWith(new ApiError(401, 'unauthorized', 'Unauthorized'));

    renderProvider();

    expect(localStorage.getItem(ACCESS_TOKEN_KEY)).toBeNull();
  });

  // An error that is not an ApiError at all — a thrown TypeError from a parse
  // failure, say — carries no status to trust, so it must take the clearing
  // path rather than being mistaken for an outage.
  it('clears the stored token when the failure carries no API status', () => {
    localStorage.setItem(ACCESS_TOKEN_KEY, 'dead-token');
    meFailedWith(new TypeError('Failed to fetch'));

    renderProvider();

    expect(localStorage.getItem(ACCESS_TOKEN_KEY)).toBeNull();
  });
});
