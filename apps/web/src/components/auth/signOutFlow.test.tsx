import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { AuthProvider } from '../../context/AuthContext';
import { RequireAuth } from '../RequireAuth';
import { SignedInFooter } from './SignedInFooter';
import { clearAccessToken, setAccessToken } from '../../lib/api/token';

/**
 * The one test in this task that mounts real machinery: the real AuthProvider,
 * the real RequireAuth, a real two-route tree, with fetch stubbed at the
 * boundary and nothing else mocked.
 *
 * Asserting `logout` was called on a mock would pass just as happily against an
 * implementation that cleared localStorage itself — the exact failure NFR-1
 * exists to prevent. And because this tree contains no useNavigate, no
 * window.location write, and no route element other than RequireAuth's own
 * subject, the ONLY thing that can put "login screen" on the page is
 * RequireAuth's <Navigate> (FR-SIGNOUT-3).
 */

// The API client's 401-retry path imports refreshAccessToken from this module;
// stubbing it keeps a retry out of a test about one request.
vi.mock('../../lib/api/refresh', () => ({
  mintAccessToken: vi.fn().mockResolvedValue('new-token'),
  refreshAccessToken: vi.fn().mockResolvedValue(null),
}));

function meResponse() {
  return {
    ok: true,
    status: 200,
    json: async () => ({
      data: {
        id: 'u1',
        type: 'users',
        attributes: {
          email: 'ada@example.com',
          displayName: 'Ada Lovelace',
          avatarUrl: '',
          themePreference: 'system',
        },
      },
      // The fleetless identity this whole task exists for.
      meta: { activeFleetId: null, role: null, platformAdmin: false },
    }),
  };
}

function stubFetch() {
  const fetchMock = vi.fn(async (input: unknown) => {
    const url = String(input);
    if (url === '/api/auth/me') return meResponse();
    if (url === '/api/auth/logout') return { ok: true, status: 204, json: async () => null };
    throw new Error(`unexpected fetch: ${url}`);
  });
  vi.stubGlobal('fetch', fetchMock);
  return fetchMock;
}

function renderGuardedFooter() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false, gcTime: 0 }, mutations: { retry: false } },
  });
  return render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter initialEntries={['/onboarding']}>
        <AuthProvider>
          <Routes>
            <Route path="/login" element={<div>login screen</div>} />
            <Route
              path="/onboarding"
              element={
                <RequireAuth>
                  <SignedInFooter />
                </RequireAuth>
              }
            />
          </Routes>
        </AuthProvider>
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

describe('signing out from a fleetless page', () => {
  beforeEach(() => {
    localStorage.clear();
    // Before render: useMe's `enabled` is evaluated on the first render.
    setAccessToken('token-123');
  });

  afterEach(() => {
    vi.unstubAllGlobals();
    clearAccessToken();
    localStorage.clear();
  });

  it('issues POST /api/auth/logout with the refresh cookie attached', async () => {
    const userEvents = userEvent.setup();
    const fetchMock = stubFetch();
    renderGuardedFooter();

    await userEvents.click(await screen.findByRole('button', { name: 'Sign out' }));

    const logoutCall = await waitFor(() => {
      const call = fetchMock.mock.calls.find(([url]) => String(url) === '/api/auth/logout');
      expect(call).toBeDefined();
      return call as unknown as [string, RequestInit];
    });

    expect(logoutCall[1].method).toBe('POST');
    // The HttpOnly refresh cookie is the entire reason this round-trip exists:
    // only the server can invalidate the refresh-token family, and the browser
    // only attaches the cookie on credentialled requests.
    expect(logoutCall[1].credentials).toBe('include');
  });

  it('lands on /login by way of RequireAuth, with no navigation code of its own', async () => {
    const userEvents = userEvent.setup();
    stubFetch();
    renderGuardedFooter();

    await userEvents.click(await screen.findByRole('button', { name: 'Sign out' }));

    expect(await screen.findByText('login screen')).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Sign out' })).not.toBeInTheDocument();
  });

  // The control that gives the assertion above its meaning: before the click,
  // the guard is letting the fleetless user sit on /onboarding rather than
  // redirecting. Without this, "login screen appeared" could mean the guard
  // was redirecting all along.
  it('does not redirect the fleetless user before they ask to sign out', async () => {
    stubFetch();
    renderGuardedFooter();

    expect(await screen.findByRole('button', { name: 'Sign out' })).toBeInTheDocument();
    expect(screen.queryByText('login screen')).not.toBeInTheDocument();
  });
});
