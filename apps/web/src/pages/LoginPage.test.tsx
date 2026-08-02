import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter, Routes, Route } from 'react-router-dom';
import { LoginPage } from './LoginPage';
import type { AuthContextValue } from '../context/AuthContext';

const login = vi.fn();
const mockAuth = vi.fn<() => AuthContextValue>();
vi.mock('../context/AuthContext', () => ({
  useAuth: () => mockAuth(),
}));

function baseAuth(): AuthContextValue {
  return {
    user: null,
    activeFleetId: null,
    role: null,
    isAuthenticated: false,
    isLoading: false,
    login,
    logout: vi.fn(),
  };
}

function renderLogin(state?: { from: string }) {
  return render(
    <MemoryRouter initialEntries={[{ pathname: '/login', state }]}>
      <Routes>
        <Route path="/login" element={<LoginPage />} />
      </Routes>
    </MemoryRouter>,
  );
}

describe('LoginPage', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockAuth.mockReturnValue(baseAuth());
  });

  // RequireAuth stashes the attempted path in router state; LoginPage has to
  // forward it so auth-service can send the invitee back to the accept route.
  it('forwards the attempted path to login()', async () => {
    renderLogin({ from: '/invites/abc123/accept' });
    await userEvent.click(screen.getByRole('button', { name: /continue with google/i }));
    expect(login).toHaveBeenCalledWith('/invites/abc123/accept');
  });

  it('calls login() with no return path on a direct visit', async () => {
    renderLogin();
    await userEvent.click(screen.getByRole('button', { name: /continue with google/i }));
    expect(login).toHaveBeenCalledWith(undefined);
  });
});
