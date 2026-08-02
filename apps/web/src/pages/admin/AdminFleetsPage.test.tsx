import { describe, it, expect, vi, beforeEach } from 'vitest';
import { screen, within } from '@testing-library/react';
import { Routes, Route } from 'react-router-dom';
import { renderWithProviders } from '../../test/renderWithProviders';
import { AdminFleetsPage } from './AdminFleetsPage';
import type {
  AdminFleetAttributes,
  AdminFleetDetailAttributes,
  AdminMemberRow,
} from '../../types/models/admin';

const useAdminFleets = vi.fn();
const useAdminFleet = vi.fn();
vi.mock('../../lib/hooks/api/admin', () => ({
  useAdminFleets: () => useAdminFleets(),
  useAdminFleet: () => useAdminFleet(),
}));

const futureIso = new Date(Date.now() + 3 * 24 * 60 * 60 * 1000).toISOString();

function fleetAttrs(over: Partial<AdminFleetAttributes> = {}): AdminFleetAttributes {
  return {
    name: 'Test Fleet',
    created_at: '2026-01-01T00:00:00Z',
    owner_user_id: 'u1',
    owner_email: 'u1@example.com',
    owner_display_name: 'Owner One',
    member_count: 1,
    vehicle_count: 2,
    pending_purge: false,
    purge_after: null,
    ...over,
  };
}

function mockFleets(rows: Array<{ id: string } & Partial<AdminFleetAttributes>>) {
  useAdminFleets.mockReturnValue({
    data: {
      data: rows.map(({ id, ...attrs }) => ({
        id,
        type: 'admin-fleets',
        attributes: fleetAttrs(attrs),
      })),
      meta: {},
    },
    isLoading: false,
    isError: false,
  });
}

function mockFleet(over: Partial<AdminFleetDetailAttributes> = {}) {
  useAdminFleet.mockReturnValue({
    data: {
      id: 'f1',
      type: 'admin-fleets',
      attributes: {
        ...fleetAttrs(),
        members: [] as AdminMemberRow[],
        vehicles: [],
        invites: [],
        counts: { vehicles: 2 },
        warnings: [],
        ...over,
      },
    },
    isLoading: false,
    isError: false,
  });
}

function renderAt(route: string) {
  return renderWithProviders(
    <Routes>
      <Route path="/admin/fleets" element={<AdminFleetsPage />} />
      <Route path="/admin/fleets/:id" element={<AdminFleetsPage />} />
    </Routes>,
    { route },
  );
}

describe('AdminFleetsPage', () => {
  beforeEach(() => {
    useAdminFleets.mockReset();
    useAdminFleet.mockReset();
    mockFleets([{ id: 'f1' }]);
    mockFleet();
  });

  it('shows list and detail side by side above md, and a single column below', () => {
    // The two panes are one grid whose columns collapse; assert the classes
    // rather than the viewport, which jsdom does not model.
    const { container } = renderAt('/admin/fleets');
    expect(container.querySelector('[data-testid="fleet-inspector"]')).toHaveClass(
      'md:grid-cols-[320px_1fr]',
    );
  });

  it('shows a pending-purge fleet struck through with a countdown', async () => {
    mockFleets([{ id: 'f1', name: 'Test Fleet', pending_purge: true, purge_after: futureIso }]);
    renderAt('/admin/fleets');
    const row = await screen.findByText('Test Fleet');
    expect(row).toHaveClass('line-through');
    expect(screen.getByText(/\d+ days? left/i)).toBeInTheDocument();
  });

  it('offers back-navigation from the detail view on small screens', async () => {
    renderAt('/admin/fleets/f1');
    expect(await screen.findByRole('link', { name: /all fleets/i })).toBeInTheDocument();
  });

  // FR-ADMIN-UI-8: a fleet must never lose its only owner, and the console must
  // not offer an action it will refuse.
  it('renders the owner row without an enabled remove action', async () => {
    mockFleet({
      members: [
        { user_id: 'u1', email: '', display_name: 'A', role: 'owner', status: 'active', joined_at: '' },
        { user_id: 'u2', email: '', display_name: 'B', role: 'member', status: 'active', joined_at: '' },
      ],
    });
    renderAt('/admin/fleets/f1');
    const ownerRow = await screen.findByTestId('member-u1');
    expect(within(ownerRow).getByRole('button', { name: /remove/i })).toBeDisabled();
    const memberRow = screen.getByTestId('member-u2');
    expect(within(memberRow).getByRole('button', { name: /remove/i })).toBeEnabled();
  });

  // FR-ADMIN-FLEET-5: a degraded user lookup shows ids, not invented names.
  it('surfaces a degradation warning on the detail view', async () => {
    mockFleet({ warnings: ['auth-service unreachable; member names omitted'] });
    renderAt('/admin/fleets/f1');
    expect(await screen.findByRole('status')).toHaveTextContent(/auth-service unreachable/i);
  });
});
