import { describe, it, expect, vi, beforeEach } from 'vitest';
import { screen, within } from '@testing-library/react';
import { renderWithProviders } from '../../test/renderWithProviders';
import { AdminAuditPage } from './AdminAuditPage';
import type { AuditEventAttributes } from '../../types/models/admin';

const useAuditEvents = vi.fn();
vi.mock('../../lib/hooks/api/admin', () => ({
  useAuditEvents: () => useAuditEvents(),
}));

const newer = '2026-08-07T14:00:00Z';
const older = '2026-08-02T14:00:00Z';

function mockAudit(rows: Array<{ id: string } & Partial<AuditEventAttributes>>) {
  useAuditEvents.mockReturnValue({
    data: {
      data: rows.map(({ id, ...over }) => ({
        id,
        type: 'admin-audit-events',
        attributes: {
          actor_user_id: 'u1',
          actor_email: 'admin@example.com',
          action: 'purge.created',
          scope: 'fleet',
          target_type: 'fleet',
          target_id: 'f1',
          target_label: 'Test Fleet',
          purge_operation_id: 'op-1',
          affected_counts: {},
          // Populated only for vehicle.transferred; the API sends "" otherwise.
          source_fleet_id: '',
          destination_fleet_id: '',
          correlation_id: 'corr-1',
          created_at: newer,
          ...over,
        } as AuditEventAttributes,
      })),
      meta: {},
    },
    isLoading: false,
    isError: false,
  });
}

describe('AdminAuditPage', () => {
  beforeEach(() => {
    useAuditEvents.mockReset();
  });

  // FR-ADMIN-UI-13: attributing a scheduled deletion to the person who
  // requested it days earlier would misread the trail.
  it('renders newest first and attributes reaper rows to "system"', async () => {
    mockAudit([
      {
        id: 'a1',
        action: 'purge.reaped',
        actor_user_id: 'system',
        actor_email: 'system',
        created_at: newer,
      },
      { id: 'a2', action: 'purge.created', actor_email: 'admin@example.com', created_at: older },
    ]);
    renderWithProviders(<AdminAuditPage />);
    const rows = await screen.findAllByRole('row');
    expect(within(rows[1]!).getByText('system')).toBeInTheDocument();
    expect(within(rows[2]!).getByText('admin@example.com')).toBeInTheDocument();
  });

  it('surfaces the correlation id so a row can be tied back to service logs', async () => {
    mockAudit([{ id: 'a1', correlation_id: 'corr-123' }]);
    renderWithProviders(<AdminAuditPage />);
    expect(await screen.findByText('corr-123')).toBeInTheDocument();
  });

  it('renders actions in user language rather than the API vocabulary', async () => {
    mockAudit([{ id: 'a1', action: 'purge.reaped' }]);
    renderWithProviders(<AdminAuditPage />);
    // Scoped to the row: the action FILTERS deliberately use the same user
    // language, so an unscoped query would match the buttons too.
    const row = await screen.findByTestId('audit-a1');
    expect(within(row).getByText('Deleted for good')).toBeInTheDocument();
    expect(screen.queryByText('purge.reaped')).not.toBeInTheDocument();
  });

  // FR-XFER-AUDIT-5, both halves. ACTIONS and ACTION_LABELS are separate lists
  // and it is entirely possible to update one and not the other; the badge's
  // `?? a.action` fallback would then hide the omission.
  it('offers a filter for vehicle transfers', async () => {
    mockAudit([]);
    renderWithProviders(<AdminAuditPage />);
    expect(await screen.findByRole('button', { name: 'Transferred' })).toBeInTheDocument();
  });

  it('renders the transfer badge label rather than the raw action string', async () => {
    mockAudit([
      {
        id: 'a1',
        action: 'vehicle.transferred',
        target_label: 'The Green Bean',
        source_fleet_id: 'fleet-a',
        destination_fleet_id: 'fleet-b',
      },
    ]);
    renderWithProviders(<AdminAuditPage />);

    const row = await screen.findByTestId('audit-a1');
    expect(within(row).getByText('Transferred')).toBeInTheDocument();
    expect(within(row).queryByText('vehicle.transferred')).toBeNull();
  });
});
