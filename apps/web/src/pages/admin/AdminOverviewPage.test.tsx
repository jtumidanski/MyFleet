import { describe, it, expect, vi, beforeEach } from 'vitest';
import { screen } from '@testing-library/react';
import { renderWithProviders } from '../../test/renderWithProviders';
import { AdminOverviewPage } from './AdminOverviewPage';
import type { AdminStatsAttributes } from '../../types/models/admin';

const useAdminStats = vi.fn();
vi.mock('../../lib/hooks/api/admin', () => ({
  useAdminStats: () => useAdminStats(),
  useCreatePurge: () => ({ mutateAsync: vi.fn(), isPending: false }),
}));

function mockStats(attrs: Partial<AdminStatsAttributes>) {
  useAdminStats.mockReturnValue({
    data: {
      id: 'platform',
      type: 'admin-stats',
      attributes: {
        fleets: 0,
        memberships: 0,
        maintenance_records: 0,
        maintenance_schedules: 0,
        fuel_logs: 0,
        mileage_records: 0,
        activity_events: 0,
        pending_invites: 0,
        users: 0,
        media_objects: 0,
        notifications: 0,
        vehicles: { active: 0, pending_purge: 0 },
        warnings: [],
        ...attrs,
      },
    },
    isLoading: false,
    isError: false,
  });
}

describe('AdminOverviewPage', () => {
  beforeEach(() => {
    useAdminStats.mockReset();
  });

  it('renders a stat tile per domain', async () => {
    mockStats({ fleets: 12, users: 21, vehicles: { active: 47, pending_purge: 3 } });
    renderWithProviders(<AdminOverviewPage />);
    expect(await screen.findByText('12')).toBeInTheDocument();
    expect(screen.getByText('47')).toBeInTheDocument();
    expect(screen.getByText('21')).toBeInTheDocument();
  });

  it('shows the recovery window in the vehicle tile', async () => {
    // FR-ADMIN-STATS-3: pending-purge is what the console can still undo, so it
    // belongs next to the number it will become.
    mockStats({ vehicles: { active: 47, pending_purge: 3 } });
    renderWithProviders(<AdminOverviewPage />);
    expect(await screen.findByText(/3 pending purge/i)).toBeInTheDocument();
  });

  // FR-ADMIN-UI-6. Rendering 0 here would tell an operator there are no
  // notifications, when the truth is that nobody could ask.
  it('renders an unavailable count as an em dash with the reason, never as 0', async () => {
    mockStats({
      notifications: null,
      warnings: ['notification-service unreachable; notifications count omitted'],
    });
    renderWithProviders(<AdminOverviewPage />);
    const tile = await screen.findByTestId('stat-notifications');
    expect(tile).toHaveTextContent('—');
    expect(tile).not.toHaveTextContent('0');
    // Deliberately getAll: the reason appears in the banner AND under the tile.
    // The banner says something is wrong; the tile says which number you cannot
    // trust. Asserting a single occurrence would forbid the more useful of the
    // two.
    expect(screen.getAllByText(/notification-service unreachable/i).length).toBeGreaterThan(0);
    expect(tile).toHaveTextContent(/unreachable/i);
  });

  it('renders warnings as a non-blocking banner, not an error state', async () => {
    mockStats({ notifications: null, warnings: ['notification-service unreachable'] });
    renderWithProviders(<AdminOverviewPage />);
    expect(await screen.findByRole('status')).toHaveTextContent(/unreachable/i);
    expect(screen.queryByRole('alert')).not.toBeInTheDocument();
  });

  it('shows no warning banner when every source answered', async () => {
    mockStats({ fleets: 12 });
    renderWithProviders(<AdminOverviewPage />);
    expect(await screen.findByText('12')).toBeInTheDocument();
    expect(screen.queryByRole('status')).not.toBeInTheDocument();
  });
});
