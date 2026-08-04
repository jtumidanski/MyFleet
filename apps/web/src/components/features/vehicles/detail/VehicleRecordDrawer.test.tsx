import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { VehicleRecordDrawer } from './VehicleRecordDrawer';
import { maintenanceRecordService } from '../../../../services/api/MaintenanceRecordService';
import { maintenanceCategoryService } from '../../../../services/api/MaintenanceCategoryService';
import { fuelService } from '../../../../services/api/FuelService';
import { mileageService } from '../../../../services/api/MileageService';
import type { VehicleRecordRow } from '../../../../lib/vehicleRecords';

vi.mock('../../../../services/api/MaintenanceRecordService', () => ({
  maintenanceRecordService: { get: vi.fn(), patch: vi.fn(), remove: vi.fn() },
}));
vi.mock('../../../../services/api/MaintenanceCategoryService', () => ({
  maintenanceCategoryService: { list: vi.fn() },
}));
vi.mock('../../../../services/api/FuelService', () => ({
  fuelService: { get: vi.fn(), patch: vi.fn(), remove: vi.fn() },
}));
vi.mock('../../../../services/api/MileageService', () => ({
  mileageService: { listByVehicle: vi.fn() },
}));

function renderDrawer(row: VehicleRecordRow | null, canWrite: boolean) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={client}>
      <VehicleRecordDrawer row={row} onClose={vi.fn()} vehicleId="v1" canWrite={canWrite} />
    </QueryClientProvider>,
  );
}

const maintenanceRow: VehicleRecordRow = {
  id: 'maintenance:m1',
  sourceId: 'm1',
  kind: 'maintenance',
  date: '2026-07-12T00:00:00Z',
  title: 'Front brake pads',
  mileage: 82940,
  cost: 612.4,
};

const fuelRow: VehicleRecordRow = {
  id: 'fuel:f1',
  sourceId: 'f1',
  kind: 'fuel',
  date: '2026-07-28T00:00:00Z',
  title: '16.204 gal',
  mileage: 84010,
  cost: 58.31,
};

const mileageRow: VehicleRecordRow = {
  id: 'mileage:mi1',
  sourceId: 'mi1',
  kind: 'mileage',
  date: '2026-07-01T00:00:00Z',
  title: 'Odometer reading',
  mileage: 84000,
};

describe('VehicleRecordDrawer', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(maintenanceCategoryService.list).mockResolvedValue({ data: [] });
    vi.mocked(maintenanceRecordService.get).mockResolvedValue({
      id: 'm1',
      type: 'maintenanceRecords',
      attributes: {
        vehicleId: 'v1',
        categoryId: 'c1',
        performedAt: '2026-07-12T00:00:00Z',
        mileage: 82940,
        cost: 612.4,
        createdAt: '2026-07-12T00:00:00Z',
        documentMediaIds: [],
      },
    } as never);
    vi.mocked(fuelService.get).mockResolvedValue({
      id: 'f1',
      type: 'fuelLogs',
      attributes: {
        vehicleId: 'v1',
        date: '2026-07-28T00:00:00Z',
        mileage: 84010,
        gallons: 16.204,
        totalCost: 58.31,
        pricePerGallon: 3.599,
        createdAt: '2026-07-28T00:00:00Z',
        updatedAt: '2026-07-28T00:00:00Z',
      },
    } as never);
    vi.mocked(mileageService.listByVehicle).mockResolvedValue({
      data: [
        {
          id: 'mi1',
          type: 'mileageRecords',
          attributes: {
            vehicleId: 'v1',
            mileage: 84000,
            recordedAt: '2026-07-01T00:00:00Z',
            source: 'manual',
            createdAt: '2026-07-01T00:00:00Z',
            flagged: false,
          },
        },
      ],
      meta: { total: 1, totalPages: 1 } as never,
    });
  });

  it('offers edit and delete for a maintenance row when canWrite is true', async () => {
    renderDrawer(maintenanceRow, true);
    expect(await screen.findByRole('button', { name: /^edit$/i })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /^delete$/i })).toBeInTheDocument();
  });

  it('hides edit and delete for a maintenance row when canWrite is false', async () => {
    renderDrawer(maintenanceRow, false);
    await screen.findByText('Front brake pads');
    expect(screen.queryByRole('button', { name: /^edit$/i })).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /^delete$/i })).not.toBeInTheDocument();
  });

  it('offers edit and delete for a fuel row when canWrite is true', async () => {
    renderDrawer(fuelRow, true);
    expect(await screen.findByRole('button', { name: /^edit$/i })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /^delete$/i })).toBeInTheDocument();
  });

  // Mileage records have no PATCH/DELETE endpoint (mileage-service only
  // exposes list + create) — the drawer must never offer either, regardless
  // of role, or the button would produce a 404/405 with no recovery.
  it('never offers edit or delete for a mileage row, even when canWrite is true', async () => {
    renderDrawer(mileageRow, true);
    expect(await screen.findByText('manual')).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /^edit$/i })).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /^delete$/i })).not.toBeInTheDocument();
  });

  // The drawer's edit form renders a category picker, but the PATCH body it
  // built omitted categoryId entirely, so choosing a different category
  // appeared to work and reverted on the next fetch. Without this test the
  // exact bug regresses green: every other assertion in this file passes with
  // categoryId dropped.
  it('sends categoryId in the PATCH body when editing a maintenance record', async () => {
    vi.mocked(maintenanceCategoryService.list).mockResolvedValue({
      data: [
        {
          id: 'c1',
          type: 'maintenanceCategories',
          attributes: { name: 'Oil Change', systemDefined: true, kind: 'maintenance' },
        },
      ],
    } as never);
    vi.mocked(maintenanceRecordService.patch).mockResolvedValue({} as never);

    const user = userEvent.setup();
    renderDrawer(maintenanceRow, true);

    await user.click(await screen.findByRole('button', { name: /^edit$/i }));
    await user.click(await screen.findByRole('button', { name: /log record|log modification/i }));

    await waitFor(() => expect(maintenanceRecordService.patch).toHaveBeenCalled());
    const [, attributes] = vi.mocked(maintenanceRecordService.patch).mock.calls[0] as [
      string,
      Record<string, unknown>,
    ];
    expect(attributes).toHaveProperty('categoryId', 'c1');
  });

  // Selecting a different row while mid-edit must land on the new row's view
  // pane. The reset runs during render off a remembered row id rather than from
  // an effect, so the new row's FIRST frame is already the view pane — an
  // effect would paint the previous row's edit form for one frame, with its
  // save button live over a record the user did not choose.
  it('returns to view mode when a different row is selected mid-edit', async () => {
    const user = userEvent.setup();
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    const drawer = (row: VehicleRecordRow) => (
      <QueryClientProvider client={client}>
        <VehicleRecordDrawer row={row} onClose={vi.fn()} vehicleId="v1" canWrite />
      </QueryClientProvider>
    );

    const { rerender } = render(drawer(maintenanceRow));
    await user.click(await screen.findByRole('button', { name: /^edit$/i }));
    expect(
      await screen.findByRole('button', { name: /log record|log modification/i }),
    ).toBeInTheDocument();

    rerender(drawer(fuelRow));

    expect(screen.queryByRole('button', { name: /log record|log modification/i })).toBeNull();
    expect(await screen.findByRole('button', { name: /^edit$/i })).toBeInTheDocument();
  });

  it('renders nothing open when row is null', () => {
    renderDrawer(null, true);
    expect(screen.queryByRole('button', { name: /^edit$/i })).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /^delete$/i })).not.toBeInTheDocument();
  });
});
