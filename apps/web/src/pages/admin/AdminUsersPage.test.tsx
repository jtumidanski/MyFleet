import { describe, it, expect, vi, beforeEach } from 'vitest';
import { screen, within } from '@testing-library/react';
import { renderWithProviders } from '../../test/renderWithProviders';
import { AdminUsersPage } from './AdminUsersPage';
import type { AdminUserAttributes } from '../../types/models/admin';

const useAdminUsers = vi.fn();
vi.mock('../../lib/hooks/api/admin', () => ({
  useAdminUsers: () => useAdminUsers(),
}));

function mockUsers(rows: Array<{ id: string } & Partial<AdminUserAttributes>>) {
  useAdminUsers.mockReturnValue({
    data: {
      data: rows.map(({ id, ...over }) => ({
        id,
        type: 'admin-users',
        attributes: {
          email: 'a@example.com',
          display_name: 'A',
          created_at: '2026-01-01T00:00:00Z',
          last_login_at: null,
          platform_admin: false,
          fleets: [],
          ...over,
        } as AdminUserAttributes,
      })),
      meta: {},
    },
    isLoading: false,
    isError: false,
  });
}

describe('AdminUsersPage', () => {
  beforeEach(() => {
    useAdminUsers.mockReset();
  });

  it('lists a user with the fleets they belong to', async () => {
    mockUsers([
      {
        id: 'u1',
        email: 'u1@example.com',
        fleets: [{ fleet_id: 'f1', name: 'Test Fleet', role: 'owner' }],
      },
    ]);
    renderWithProviders(<AdminUsersPage />);
    expect(await screen.findByText('u1@example.com')).toBeInTheDocument();
    expect(screen.getByText(/Test Fleet · owner/)).toBeInTheDocument();
  });

  it('says "None" rather than showing an empty cell for a fleetless user', async () => {
    mockUsers([{ id: 'u1', fleets: [] }]);
    renderWithProviders(<AdminUsersPage />);
    expect(await screen.findByText('None')).toBeInTheDocument();
  });

  // The PRD makes granting admin a non-goal: it is a deliberate out-of-band act
  // against auth.platform_admins, and the console must not imply otherwise.
  it('offers no way to grant or revoke platform admin', async () => {
    mockUsers([{ id: 'u1' }]);
    renderWithProviders(<AdminUsersPage />);
    await screen.findByTestId('user-u1');
    expect(
      screen.queryByRole('button', { name: /grant|revoke|make admin/i }),
    ).not.toBeInTheDocument();
  });

  // FR-ADMIN-FLEET-6: the directory must SHOW who holds platform admin. Showing
  // and granting are different things; only granting is a non-goal.
  it('marks a platform admin', async () => {
    mockUsers([
      { id: 'u1', platform_admin: true },
      { id: 'u2', platform_admin: false },
    ]);
    renderWithProviders(<AdminUsersPage />);
    const adminRow = await screen.findByTestId('user-u1');
    expect(within(adminRow).getByText(/platform admin/i)).toBeInTheDocument();
    expect(
      within(screen.getByTestId('user-u2')).queryByText(/platform admin/i),
    ).not.toBeInTheDocument();
  });
});
