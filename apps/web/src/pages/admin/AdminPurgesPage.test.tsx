import { describe, it, expect, vi, beforeEach } from 'vitest';
import { screen, within } from '@testing-library/react';
import { renderWithProviders } from '../../test/renderWithProviders';
import { AdminPurgesPage } from './AdminPurgesPage';
import type { PurgeOperationAttributes } from '../../types/models/admin';

const usePurgeOperations = vi.fn();
const cancelMutate = vi.fn();
const retryMutate = vi.fn();
vi.mock('../../lib/hooks/api/admin', () => ({
  usePurgeOperations: () => usePurgeOperations(),
  useCancelPurge: () => ({ mutate: cancelMutate, isPending: false }),
  useRetryPurge: () => ({ mutate: retryMutate, isPending: false }),
}));

const futureIso = new Date(Date.now() + 3 * 24 * 60 * 60 * 1000).toISOString();

function mockPurges(rows: Array<{ id: string } & Partial<PurgeOperationAttributes>>) {
  usePurgeOperations.mockReturnValue({
    data: {
      data: rows.map(({ id, ...over }) => ({
        id,
        type: 'purge-operations',
        attributes: {
          scope: 'fleet',
          target_type: 'fleet',
          target_id: 'f1',
          target_label: 'Test Fleet',
          status: 'pending',
          requested_by_user_id: 'u1',
          requested_by_email: 'admin@example.com',
          requested_at: '2026-08-02T00:00:00Z',
          purge_after: futureIso,
          reaped_at: null,
          cancelled_at: null,
          affected_counts: {},
          failed_services: [],
          ...over,
        } as PurgeOperationAttributes,
      })),
      meta: {},
    },
    isLoading: false,
    isError: false,
  });
}

describe('AdminPurgesPage', () => {
  beforeEach(() => {
    usePurgeOperations.mockReset();
    cancelMutate.mockReset();
    retryMutate.mockReset();
  });

  it('renders status in user language, never the API vocabulary', async () => {
    mockPurges([
      { id: 'op-1', status: 'pending', failed_services: [] },
      { id: 'op-2', status: 'partial', failed_services: ['media'] },
    ]);
    renderWithProviders(<AdminPurgesPage />);
    // Scoped to the rows: the status FILTERS deliberately use the same user
    // language, so an unscoped query would match the buttons too.
    const row1 = await screen.findByTestId('purge-op-1');
    const row2 = screen.getByTestId('purge-op-2');
    expect(within(row1).getByText('Recoverable')).toBeInTheDocument();
    expect(within(row2).getByText('Media not deleted')).toBeInTheDocument();
    expect(screen.queryByText('pending')).not.toBeInTheDocument();
    expect(screen.queryByText('partial')).not.toBeInTheDocument();
  });

  it('shows a countdown to permanence for recoverable operations', async () => {
    mockPurges([{ id: 'op-1', status: 'pending', purge_after: futureIso }]);
    renderWithProviders(<AdminPurgesPage />);
    expect(await screen.findByText(/left to restore/i)).toBeInTheDocument();
  });

  it('offers restore only while the operation is recoverable', async () => {
    mockPurges([
      { id: 'op-1', status: 'pending' },
      { id: 'op-2', status: 'reaped' },
    ]);
    renderWithProviders(<AdminPurgesPage />);
    expect(
      within(await screen.findByTestId('purge-op-1')).getByRole('button', { name: /restore/i }),
    ).toBeEnabled();
    expect(
      within(screen.getByTestId('purge-op-2')).queryByRole('button', { name: /restore/i }),
    ).not.toBeInTheDocument();
  });

  // FR-ADMIN-UI-11: retry must READ as safe to repeat, or an operator will not
  // press it after the first failure.
  it('presents retry as safe to repeat', async () => {
    mockPurges([{ id: 'op-1', status: 'partial', failed_services: ['media'] }]);
    renderWithProviders(<AdminPurgesPage />);
    const retry = await screen.findByRole('button', { name: /retry/i });
    expect(retry).toHaveAccessibleDescription(/safe to run again/i);
  });

  // Retry is offered only where it means something: a pending operation has
  // nothing outstanding to re-attempt.
  it('offers retry only for an incomplete operation', async () => {
    mockPurges([{ id: 'op-1', status: 'pending' }]);
    renderWithProviders(<AdminPurgesPage />);
    await screen.findByTestId('purge-op-1');
    expect(screen.queryByRole('button', { name: /retry/i })).not.toBeInTheDocument();
  });
});
