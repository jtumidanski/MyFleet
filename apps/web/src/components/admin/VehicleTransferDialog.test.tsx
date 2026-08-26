import { describe, it, expect, vi, beforeEach } from 'vitest';
import { screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { ApiError } from '@myfleet/shared-ts';
import { renderWithProviders } from '../../test/renderWithProviders';
import { VehicleTransferDialog } from './VehicleTransferDialog';
import type { VehicleTransferResult } from '../../services/api/AdminService';

const useAdminFleets = vi.fn();
const useVehicleTransferPreview = vi.fn();
vi.mock('../../lib/hooks/api/admin', () => ({
  useAdminFleets: (params: unknown) => useAdminFleets(params),
  useVehicleTransferPreview: (...args: unknown[]) => useVehicleTransferPreview(...args),
}));

const LABEL = 'The Green Bean';

function mockFleetOptions(rows: Array<{ id: string; name: string }>) {
  useAdminFleets.mockReturnValue({
    data: {
      data: rows.map((r) => ({ id: r.id, type: 'admin-fleets', attributes: { name: r.name } })),
      meta: {},
    },
    isLoading: false,
    isError: false,
  });
}

function mockPreview(over: Record<string, unknown> = {}) {
  useVehicleTransferPreview.mockReturnValue({
    data: {
      id: 'v1',
      type: 'vehicle-transfer-previews',
      attributes: {
        vehicle_label: LABEL,
        source_fleet_id: 'fleet-a',
        source_fleet_name: 'Tumidanski Household',
        destination_fleet_id: '',
        destination_fleet_name: '',
        counts: { fuel_logs: 118, maintenance_records: 42, widgets_removed: 2 },
        categories_to_create: [],
        warnings: [],
        ...over,
      },
    },
    isLoading: false,
    isError: false,
  });
}

function renderDialog(over: Partial<React.ComponentProps<typeof VehicleTransferDialog>> = {}) {
  const onConfirm = vi.fn();
  renderWithProviders(
    <VehicleTransferDialog
      open
      onOpenChange={vi.fn()}
      vehicleId="v1"
      sourceFleetId="fleet-a"
      onConfirm={onConfirm}
      isPending={false}
      {...over}
    />,
  );
  return { onConfirm };
}

beforeEach(() => {
  useAdminFleets.mockReset();
  useVehicleTransferPreview.mockReset();
  mockFleetOptions([
    { id: 'fleet-a', name: 'Tumidanski Household' },
    { id: 'fleet-b', name: 'Smith Household' },
  ]);
  mockPreview();
});

function confirmButton() {
  return screen.getByRole('button', { name: /transfer vehicle/i });
}

// FR-XFER-UI-3: the source fleet is never an option, and the query asks for
// live fleets only.
it('excludes the source fleet and requests live fleets only', () => {
  renderDialog();
  expect(screen.queryByRole('button', { name: 'Tumidanski Household' })).toBeNull();
  expect(screen.getByRole('button', { name: 'Smith Household' })).toBeInTheDocument();
  expect(useAdminFleets).toHaveBeenCalledWith(expect.objectContaining({ deleted: 'exclude' }));
});

// FR-XFER-UI-4: the counts come from the preview, not from anything computed
// here.
it('renders the preview counts and the categories it would create', () => {
  mockPreview({ categories_to_create: [{ name: 'Winter Tires', kind: 'maintenance' }] });
  renderDialog();
  const radius = screen.getByTestId('transfer-blast-radius');
  expect(within(radius).getByText('118')).toBeInTheDocument();
  expect(within(radius).getByText('42')).toBeInTheDocument();
  expect(screen.getByText(/Winter Tires/)).toBeInTheDocument();
});

// FR-XFER-UI-5, both halves.
it('keeps confirm disabled until a destination is chosen AND the label is typed', async () => {
  const user = userEvent.setup();
  renderDialog();
  expect(confirmButton()).toBeDisabled();

  await user.type(screen.getByLabelText(/type the vehicle name/i), LABEL);
  expect(confirmButton()).toBeDisabled(); // no destination yet

  await user.click(screen.getByRole('button', { name: 'Smith Household' }));
  expect(confirmButton()).toBeEnabled();
});

it('keeps confirm disabled for a near-miss in casing', async () => {
  const user = userEvent.setup();
  renderDialog();
  await user.click(screen.getByRole('button', { name: 'Smith Household' }));
  await user.type(screen.getByLabelText(/type the vehicle name/i), 'the green bean');
  expect(confirmButton()).toBeDisabled();
});

it('keeps confirm disabled for a trailing space', async () => {
  const user = userEvent.setup();
  renderDialog();
  await user.click(screen.getByRole('button', { name: 'Smith Household' }));
  await user.type(screen.getByLabelText(/type the vehicle name/i), `${LABEL} `);
  expect(confirmButton()).toBeDisabled();
});

// The server performs the real comparison, so it must receive what was TYPED.
it('passes the typed value and the chosen destination to onConfirm', async () => {
  const user = userEvent.setup();
  const { onConfirm } = renderDialog();
  await user.click(screen.getByRole('button', { name: 'Smith Household' }));
  await user.type(screen.getByLabelText(/type the vehicle name/i), LABEL);
  await user.click(confirmButton());

  expect(onConfirm).toHaveBeenCalledWith({
    destinationFleetId: 'fleet-b',
    destinationName: 'Smith Household',
    typed: LABEL,
  });
});

it('disables confirm while a transfer is in flight', async () => {
  const user = userEvent.setup();
  renderDialog({ isPending: true });
  await user.click(screen.getByRole('button', { name: 'Smith Household' }));
  await user.type(screen.getByLabelText(/type the vehicle name/i), LABEL);
  expect(confirmButton()).toBeDisabled();
});

// The preview is the single source of truth for the phrase; without it there is
// nothing safe to compare against, so the control is WITHHELD rather than shown
// live over numbers nobody could produce.
it('withholds the confirm control when the preview could not be loaded', () => {
  useVehicleTransferPreview.mockReturnValue({ data: undefined, isLoading: false, isError: true });
  renderDialog();
  expect(screen.queryByRole('button', { name: /transfer vehicle/i })).toBeNull();
  expect(screen.getByRole('alert')).toBeInTheDocument();
});

// The confirmation input must be marked required for the same reason every
// other required field in this app is.
it('marks the destination and confirmation as required', () => {
  renderDialog();
  expect(screen.getByLabelText(/search fleets/i)).toHaveAttribute('aria-required', 'true');
  expect(screen.getByLabelText(/type the vehicle name/i)).toHaveAttribute('aria-required', 'true');
});

// The hook is gated: it does not fire until a destination is chosen. The picker
// therefore CANNOT live inside the preview-loaded branch, or nobody could ever
// choose one. This is the case that catches a regression back to that shape.
describe('before a destination is chosen', () => {
  beforeEach(() => {
    useVehicleTransferPreview.mockReturnValue({
      data: undefined,
      isLoading: false,
      isError: false,
    });
  });

  it('still offers the destination picker, and says why the counts are missing', () => {
    renderDialog();
    expect(screen.getByRole('button', { name: 'Smith Household' })).toBeInTheDocument();
    expect(screen.getByText(/choose a destination fleet/i)).toBeInTheDocument();
    // Not an error: nothing has failed yet.
    expect(screen.queryByRole('alert')).toBeNull();
    expect(screen.queryByRole('button', { name: /transfer vehicle/i })).toBeNull();
  });
});

describe('rejections (FR-XFER-UI-7)', () => {
  it('surfaces a 409 detail verbatim', () => {
    const detail = 'The confirmation you typed does not match the vehicle name.';
    renderDialog({ error: new ApiError(409, 'conflict', 'Conflict', detail) });
    expect(within(screen.getByTestId('transfer-error')).getByText(detail)).toBeInTheDocument();
  });

  it('surfaces a 503 detail verbatim and says nothing moved', () => {
    const detail =
      'media-service could not reassign the vehicle’s media; the transfer was rolled back';
    renderDialog({ error: new ApiError(503, 'unavailable', 'Service Unavailable', detail) });
    const box = screen.getByTestId('transfer-error');
    expect(within(box).getByText(detail)).toBeInTheDocument();
    // The rollback is whole. The copy must not leave room for "some of it went".
    expect(within(box).getByText(/nothing was moved/i)).toBeInTheDocument();
  });

  it('falls back to the message when the rejection carries no detail', () => {
    renderDialog({ error: new ApiError(403, 'forbidden', 'Only a platform admin may do this') });
    expect(
      within(screen.getByTestId('transfer-error')).getByText('Only a platform admin may do this'),
    ).toBeInTheDocument();
  });

  it('keeps the confirm control available after a rejection so it can be retried', async () => {
    const user = userEvent.setup();
    renderDialog({ error: new ApiError(409, 'conflict', 'Conflict', 'Mismatch.') });
    await user.click(screen.getByRole('button', { name: 'Smith Household' }));
    await user.type(screen.getByLabelText(/type the vehicle name/i), LABEL);
    expect(confirmButton()).toBeEnabled();
  });
});

describe('the completed transfer', () => {
  function result(meta?: VehicleTransferResult['meta']): VehicleTransferResult {
    return {
      data: {
        id: 'v1',
        type: 'vehicle-transfers',
        attributes: {
          vehicle_id: 'v1',
          source_fleet_id: 'fleet-a',
          destination_fleet_id: 'fleet-b',
          transferred_at: '2026-08-25T10:00:00Z',
          affected_counts: { fuel_logs: 118, media_objects: 7, notifications: 3 },
        },
      },
      meta,
    };
  }

  const SEMANTICS =
    'live rows now in the destination fleet for the ids this transfer named, ' +
    'not the number moved by this transfer';

  // The load-bearing case. media_objects and notifications are read back from
  // the downstream service as "rows now ON the destination", so they can be
  // inflated by rows that were already there. This sentence is the only thing
  // in the whole response that says so, and this is its last hop.
  it('renders count_semantics against the two counts it describes', () => {
    renderDialog({
      result: result({ count_semantics: { media_objects: SEMANTICS, notifications: SEMANTICS } }),
    });
    const outcome = screen.getByTestId('transfer-outcome');
    expect(within(outcome).getAllByText(SEMANTICS)).toHaveLength(2);

    // Associated with the right rows, not floating loose at the bottom.
    expect(
      within(within(outcome).getByTestId('count-media_objects')).getByText(SEMANTICS),
    ).toBeInTheDocument();
    expect(
      within(within(outcome).getByTestId('count-notifications')).getByText(SEMANTICS),
    ).toBeInTheDocument();
    // A key that genuinely means "rows this transfer moved" is left unannotated.
    expect(
      within(within(outcome).getByTestId('count-fuel_logs')).queryByText(SEMANTICS),
    ).toBeNull();
  });

  it('renders the completed counts with no meta at all', () => {
    renderDialog({ result: result() });
    const outcome = screen.getByTestId('transfer-outcome');
    expect(within(outcome).getByText('118')).toBeInTheDocument();
    expect(within(outcome).getByText('7')).toBeInTheDocument();
    expect(within(outcome).queryByText(SEMANTICS)).toBeNull();
  });

  it('replaces the confirmation controls once the transfer has happened', () => {
    renderDialog({ result: result() });
    expect(screen.queryByRole('button', { name: /transfer vehicle/i })).toBeNull();
    expect(screen.queryByLabelText(/type the vehicle name/i)).toBeNull();
    expect(screen.getByRole('button', { name: /done/i })).toBeInTheDocument();
  });
});
