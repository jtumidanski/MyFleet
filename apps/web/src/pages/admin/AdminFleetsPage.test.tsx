import { useState } from 'react';
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { Routes, Route } from 'react-router-dom';
import { ApiError } from '@myfleet/shared-ts';
import { renderWithProviders } from '../../test/renderWithProviders';
import { expectNoCall } from '../../test/expectNoCall';
import { AdminFleetsPage } from './AdminFleetsPage';
import type { VehicleTransferResult } from '../../services/api/AdminService';
import type {
  AdminFleetAttributes,
  AdminFleetDetailAttributes,
  AdminMemberRow,
  AdminVehicleRow,
  TransferVehicleInput,
} from '../../types/models/admin';

const useAdminFleets = vi.fn();
const useAdminFleet = vi.fn();
const createPurgeMutate = vi.fn();
const transferMutate = vi.fn();
const transferReset = vi.fn();
const useVehicleTransferPreview = vi.fn();

/** Set by a test that wants the mutation to resolve. Cleared in beforeEach. */
let transferResult: VehicleTransferResult | null = null;
/** Set by a test that wants the mutation to be rejected. */
let transferError: unknown = null;

/**
 * A real hook rather than a static object.
 *
 * The page hands the mutation's `data` straight to VehicleTransferDialog and
 * deliberately does NOT close the dialog on success, so a fake that could never
 * change would make the outcome screen — and `meta.count_semantics` with it —
 * unreachable from this file. This one re-renders the page the way the real
 * mutation does.
 */
function useFakeTransferVehicle() {
  const [data, setData] = useState<VehicleTransferResult | undefined>(undefined);
  const [error, setError] = useState<unknown>(null);
  return {
    mutate: (vars: {
      vehicleId: string;
      attributes: TransferVehicleInput;
      destinationName: string;
    }) => {
      transferMutate(vars);
      if (transferError) setError(transferError);
      if (transferResult) setData(transferResult);
    },
    reset: () => {
      transferReset();
      setData(undefined);
      setError(null);
    },
    isPending: false,
    error,
    data,
  };
}

// Exhaustive by design: the page throws the moment it calls a hook this factory
// does not provide, so every hook FleetDetail *or the dialog it renders* uses
// has to appear here.
vi.mock('../../lib/hooks/api/admin', () => ({
  useAdminFleets: () => useAdminFleets(),
  useAdminFleet: () => useAdminFleet(),
  useCreatePurge: () => ({ mutate: createPurgeMutate, isPending: false }),
  useTransferVehicle: () => useFakeTransferVehicle(),
  useVehicleTransferPreview: (...args: unknown[]) => useVehicleTransferPreview(...args),
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

const VEHICLE_LABEL = 'The Green Bean';

function vehicle(over: Partial<AdminVehicleRow> = {}): AdminVehicleRow {
  return {
    id: 'veh-1',
    nickname: VEHICLE_LABEL,
    make: 'Toyota',
    model: 'Corolla',
    year: 2020,
    mileage: 50000,
    status: 'Active',
    pending_purge: false,
    ...over,
  };
}

/**
 * The real hook is GATED on a destination, so the fake is too: answering before
 * one is chosen would let the confirm controls appear without the operator ever
 * picking a destination, which is not the shape the dialog is written against.
 */
function mockPreview() {
  useVehicleTransferPreview.mockImplementation(
    (_vehicleId: unknown, destinationFleetId: unknown) =>
      destinationFleetId
        ? {
            data: {
              id: 'veh-1',
              type: 'vehicle-transfer-previews',
              attributes: {
                vehicle_label: VEHICLE_LABEL,
                source_fleet_id: 'f1',
                source_fleet_name: 'Test Fleet',
                destination_fleet_id: String(destinationFleetId),
                destination_fleet_name: 'Destination Fleet',
                counts: { fuel_logs: 118 },
                categories_to_create: [],
                warnings: [],
              },
            },
            isLoading: false,
            isError: false,
          }
        : { data: undefined, isLoading: false, isError: false },
  );
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
    createPurgeMutate.mockReset();
    transferMutate.mockReset();
    transferReset.mockReset();
    useVehicleTransferPreview.mockReset();
    transferResult = null;
    transferError = null;
    mockFleets([{ id: 'f1' }]);
    mockFleet();
    mockPreview();
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
        {
          user_id: 'u1',
          email: '',
          display_name: 'A',
          role: 'owner',
          status: 'active',
          joined_at: '',
        },
        {
          user_id: 'u2',
          email: '',
          display_name: 'B',
          role: 'member',
          status: 'active',
          joined_at: '',
        },
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

  // The purge control opens the confirmation rather than firing the mutation:
  // the phrase gate is the whole point, and a one-click purge from the panel
  // would bypass it (FR-ADMIN-UI-10).
  it('opens the confirmation dialog instead of purging directly', async () => {
    const user = userEvent.setup();
    renderAt('/admin/fleets/f1');
    await user.click(await screen.findByRole('button', { name: /purge this fleet/i }));
    await expectNoCall(createPurgeMutate, 'createPurgeMutate');
    expect(await screen.findByLabelText(/type the fleet name/i)).toBeInTheDocument();
  });

  // FR-ADMIN-UI-9 again, at the page level: a fleet whose counts are missing
  // gets no purge control at all.
  it('withholds the purge control when the server sent no counts', async () => {
    mockFleet({ counts: undefined as unknown as Record<string, number> });
    renderAt('/admin/fleets/f1');
    expect(await screen.findByTestId('fleet-detail')).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /purge this fleet/i })).not.toBeInTheDocument();
  });

  describe('vehicle transfer', () => {
    // The server's own sentence, verbatim (fleet-service rest.go
    // downstreamCountSemantics). It is the only thing in the whole response
    // that says media_objects and notifications are NOT "rows this transfer
    // moved".
    const SEMANTICS =
      'live rows now in the destination fleet for the ids this transfer named, ' +
      'not the number moved by this transfer';

    function transferred(): VehicleTransferResult {
      return {
        data: {
          id: 'veh-1',
          type: 'vehicle-transfers',
          attributes: {
            vehicle_id: 'veh-1',
            source_fleet_id: 'f1',
            destination_fleet_id: 'f2',
            transferred_at: '2026-08-25T10:00:00Z',
            affected_counts: { fuel_logs: 118, media_objects: 7, notifications: 3 },
          },
        },
        meta: { count_semantics: { media_objects: SEMANTICS, notifications: SEMANTICS } },
      };
    }

    /** Open the dialog, pick the destination, type the phrase, confirm. */
    async function transferVehicle(user: ReturnType<typeof userEvent.setup>) {
      await user.click(
        within(await screen.findByTestId('vehicle-veh-1')).getByRole('button', {
          name: /transfer/i,
        }),
      );
      await user.click(await screen.findByRole('button', { name: 'Destination Fleet' }));
      await user.type(await screen.findByLabelText(/type the vehicle name/i), VEHICLE_LABEL);
      await user.click(screen.getByRole('button', { name: /transfer vehicle/i }));
    }

    // FR-XFER-UI-1: an action on every vehicle row.
    it('renders a Transfer action on each vehicle row', async () => {
      mockFleet({
        vehicles: [
          vehicle(),
          vehicle({ id: 'veh-2', nickname: '', make: 'Honda', model: 'Civic', year: 2018 }),
        ],
      });
      renderAt('/admin/fleets/f1');

      expect(
        within(await screen.findByTestId('vehicle-veh-1')).getByRole('button', {
          name: /transfer/i,
        }),
      ).toBeEnabled();
      expect(
        within(screen.getByTestId('vehicle-veh-2')).getByRole('button', { name: /transfer/i }),
      ).toBeEnabled();
    });

    // FR-XFER-UI-8: a pending-purge vehicle cannot be transferred, and the
    // button says why rather than being a dead control.
    it('disables Transfer for a pending-purge vehicle and explains why', async () => {
      mockFleet({ vehicles: [vehicle({ nickname: 'Doomed', status: '', pending_purge: true })] });
      renderAt('/admin/fleets/f1');

      const button = within(await screen.findByTestId('vehicle-veh-1')).getByRole('button', {
        name: /transfer/i,
      });
      expect(button).toBeDisabled();
      expect(button).toHaveAttribute('title', expect.stringMatching(/pending purge/i));
    });

    // The mutation must not fire from merely opening the dialog.
    it('does not transfer until the dialog is confirmed', async () => {
      const user = userEvent.setup();
      mockFleet({ vehicles: [vehicle()] });
      renderAt('/admin/fleets/f1');

      await user.click(
        within(await screen.findByTestId('vehicle-veh-1')).getByRole('button', {
          name: /transfer/i,
        }),
      );
      await expectNoCall(transferMutate, 'transferMutate');
      // The dialog opened instead — the phrase gate is the whole point.
      expect(
        within(await screen.findByRole('dialog')).getByText(/transfer this vehicle/i),
      ).toBeInTheDocument();
    });

    it('sends the typed phrase and the chosen destination for the row that opened it', async () => {
      const user = userEvent.setup();
      mockFleets([{ id: 'f1' }, { id: 'f2', name: 'Destination Fleet' }]);
      mockFleet({ vehicles: [vehicle()] });
      renderAt('/admin/fleets/f1');

      await transferVehicle(user);

      expect(transferMutate).toHaveBeenCalledWith({
        vehicleId: 'veh-1',
        attributes: { destination_fleet_id: 'f2', confirmation: VEHICLE_LABEL },
        destinationName: 'Destination Fleet',
      });
    });

    // Ruling R22, end to end from the page. `meta.count_semantics` exists ONLY
    // on the completed-transfer response, so closing the dialog on success —
    // which the page must NOT do — would destroy it at the final hop.
    it('keeps the dialog open on success and shows the server’s count_semantics', async () => {
      const user = userEvent.setup();
      mockFleets([{ id: 'f1' }, { id: 'f2', name: 'Destination Fleet' }]);
      mockFleet({ vehicles: [vehicle()] });
      transferResult = transferred();
      renderAt('/admin/fleets/f1');

      await transferVehicle(user);

      const outcome = await screen.findByTestId('transfer-outcome');
      expect(within(outcome).getAllByText(SEMANTICS)).toHaveLength(2);
      // Attached to the two counts it corrects, not floating at the bottom.
      expect(
        within(within(outcome).getByTestId('count-media_objects')).getByText(SEMANTICS),
      ).toBeInTheDocument();
      expect(
        within(within(outcome).getByTestId('count-notifications')).getByText(SEMANTICS),
      ).toBeInTheDocument();
      // Still open: the operator closes it once they have read the numbers.
      expect(screen.getByRole('button', { name: /done/i })).toBeInTheDocument();
    });

    // FR-XFER-UI-7, end to end: the rejection has to reach the dialog the
    // operator is standing in front of, not just a toast that vanishes.
    it('shows a rejected transfer’s detail on the dialog itself', async () => {
      const user = userEvent.setup();
      const detail = 'The vehicle is pending purge and cannot be transferred.';
      mockFleets([{ id: 'f1' }, { id: 'f2', name: 'Destination Fleet' }]);
      mockFleet({ vehicles: [vehicle()] });
      transferError = new ApiError(409, 'conflict', 'Conflict', detail);
      renderAt('/admin/fleets/f1');

      await transferVehicle(user);

      expect(
        within(await screen.findByTestId('transfer-error')).getByText(detail),
      ).toBeInTheDocument();
    });

    it('closes only when the operator asks, and reopens clean', async () => {
      const user = userEvent.setup();
      mockFleets([{ id: 'f1' }, { id: 'f2', name: 'Destination Fleet' }]);
      mockFleet({ vehicles: [vehicle()] });
      transferResult = transferred();
      renderAt('/admin/fleets/f1');

      await transferVehicle(user);
      await user.click(await screen.findByRole('button', { name: /done/i }));
      expect(screen.queryByTestId('transfer-outcome')).toBeNull();

      // Reopening must not show the previous outcome: the page resets the
      // mutation before handing the dialog a new target.
      await user.click(
        within(screen.getByTestId('vehicle-veh-1')).getByRole('button', { name: /transfer/i }),
      );
      expect(transferReset).toHaveBeenCalled();
      expect(
        within(await screen.findByRole('dialog')).getByText(/transfer this vehicle/i),
      ).toBeInTheDocument();
      expect(screen.queryByTestId('transfer-outcome')).toBeNull();
    });
  });
});
