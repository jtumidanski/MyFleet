import { describe, it, expect, vi, beforeEach } from 'vitest';
import { act, render, screen, fireEvent } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { AuthProvider, useAuth } from './AuthContext';
import { ACCESS_TOKEN_KEY, setAccessToken } from '../lib/api/token';

/**
 * The REAL AuthProvider, exercised through logout().
 *
 * AuthContext.test.tsx cannot host this: it mocks the whole ./AuthContext
 * module to drive RequireAuth, so the actual provider never runs there.
 *
 * useMe is stubbed to a settled, non-error result on purpose. The provider
 * clears the access token from a `me.isError` effect, and a test that let that
 * fire would find an empty token no matter what logout() did — the assertion
 * would pass for the wrong reason.
 */
const logoutRequestMock = vi.fn<() => Promise<void>>();

vi.mock('../lib/hooks/api/auth', () => ({
  authKeys: { all: ['auth'], me: () => ['auth', 'me'] },
  useMe: () => ({ data: undefined, isError: false, isLoading: false }),
  logoutRequest: () => logoutRequestMock(),
}));

function SignOutProbe({ onSettled }: { onSettled: (outcome: unknown) => void }) {
  const { logout } = useAuth();
  return (
    <button
      type="button"
      onClick={() => {
        logout().then(() => onSettled(null), onSettled);
      }}
    >
      sign out
    </button>
  );
}

function renderProbe(onSettled: (outcome: unknown) => void): QueryClient {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  queryClient.setQueryData(['auth', 'me'], { user: { id: 'u1' } });
  render(
    <QueryClientProvider client={queryClient}>
      <AuthProvider>
        <SignOutProbe onSettled={onSettled} />
      </AuthProvider>
    </QueryClientProvider>,
  );
  return queryClient;
}

describe('AuthProvider logout', () => {
  beforeEach(() => {
    localStorage.clear();
    logoutRequestMock.mockReset();
  });

  // Decision D1, and the one test here that can fail. The user asked to leave,
  // so the local session ends even when the server never heard about it — AND
  // the caller still learns the request failed, so it can say so. Asserting on
  // stored state rather than on the promise alone is what gives it teeth:
  // against the old `await logoutRequest(); clearAccessToken(); …` the
  // rejection aborted the teardown and the token survived in localStorage.
  it('clears the local session and still rejects when the request fails', async () => {
    setAccessToken('tok-123');
    logoutRequestMock.mockRejectedValue(new Error('network down'));
    const outcomes: unknown[] = [];
    const queryClient = renderProbe((outcome) => outcomes.push(outcome));

    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: 'sign out' }));
    });

    expect(outcomes).toHaveLength(1);
    expect(outcomes[0]).toBeInstanceOf(Error);
    expect(localStorage.getItem(ACCESS_TOKEN_KEY)).toBeNull();
    expect(queryClient.getQueryData(['auth', 'me'])).toBeUndefined();
  });

  it('clears the local session and resolves when the request succeeds', async () => {
    setAccessToken('tok-123');
    logoutRequestMock.mockResolvedValue(undefined);
    const outcomes: unknown[] = [];
    const queryClient = renderProbe((outcome) => outcomes.push(outcome));

    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: 'sign out' }));
    });

    expect(outcomes).toHaveLength(1);
    expect(outcomes[0]).toBeNull();
    expect(localStorage.getItem(ACCESS_TOKEN_KEY)).toBeNull();
    expect(queryClient.getQueryData(['auth', 'me'])).toBeUndefined();
  });
});
