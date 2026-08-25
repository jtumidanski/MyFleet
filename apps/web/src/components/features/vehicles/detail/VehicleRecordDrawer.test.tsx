import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { toast } from 'sonner';
import { VehicleRecordDrawer } from './VehicleRecordDrawer';
import { maintenanceRecordService } from '../../../../services/api/MaintenanceRecordService';
import { maintenanceCategoryService } from '../../../../services/api/MaintenanceCategoryService';
import { fuelService } from '../../../../services/api/FuelService';
import { mileageService } from '../../../../services/api/MileageService';
import { mediaService } from '../../../../services/api/MediaService';
import { expectNoCall } from '../../../../test/expectNoCall';
import type { VehicleRecordRow } from '../../../../lib/vehicleRecords';

vi.mock('../../../../services/api/MaintenanceRecordService', () => ({
  maintenanceRecordService: {
    get: vi.fn(),
    patch: vi.fn(),
    remove: vi.fn(),
    removeDocumentMedia: vi.fn(),
    appendDocumentMedia: vi.fn(),
  },
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
vi.mock('../../../../services/api/MediaService', () => ({
  mediaService: {
    get: vi.fn(),
    getContentBlob: vi.fn(),
    remove: vi.fn(),
    initUpload: vi.fn(),
    putContent: vi.fn(),
    confirm: vi.fn(),
  },
}));
vi.mock('sonner', () => ({ toast: { success: vi.fn(), error: vi.fn() } }));

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
        documentMediaIds: ['m1'],
      },
    } as never);
    vi.mocked(maintenanceRecordService.appendDocumentMedia).mockResolvedValue({} as never);
    vi.mocked(maintenanceRecordService.removeDocumentMedia).mockResolvedValue(undefined as never);
    vi.mocked(mediaService.remove).mockResolvedValue(undefined as never);
    vi.mocked(mediaService.get).mockResolvedValue({
      id: 'm1',
      type: 'media-objects',
      attributes: {
        fleetId: 'f1',
        uploadedByUserId: 'u1',
        bucket: 'b',
        objectKey: 'k',
        contentType: 'application/pdf',
        originalFilename: 'invoice.pdf',
        status: 'ready',
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

  // Uploads picked mid-edit are attached one at a time through the append
  // endpoint (UpdateMaintenanceRecordAttributes has no documentMediaIds
  // field) — but the user should see one outcome, not one toast per file.
  it('attaches every uploaded file exactly once and shows a single success toast', async () => {
    vi.mocked(maintenanceRecordService.patch).mockResolvedValue({} as never);
    vi.mocked(mediaService.putContent).mockResolvedValue(undefined as never);
    let uploadCount = 0;
    vi.mocked(mediaService.initUpload).mockImplementation(async () => {
      uploadCount += 1;
      return { id: `new-${uploadCount}`, type: 'media-objects', attributes: {} } as never;
    });
    vi.mocked(mediaService.confirm).mockImplementation(
      async (id: string) => ({ id, type: 'media-objects', attributes: {} }) as never,
    );

    const user = userEvent.setup();
    renderDrawer(maintenanceRow, true);

    await user.click(await screen.findByRole('button', { name: /^edit$/i }));

    const input = document.querySelector('input[type="file"]') as HTMLInputElement;
    await user.upload(input, [
      new File(['a'], 'a.pdf', { type: 'application/pdf' }),
      new File(['b'], 'b.pdf', { type: 'application/pdf' }),
    ]);

    const submit = await screen.findByRole('button', { name: /log record|log modification/i });
    await waitFor(() => expect(submit).toBeEnabled());
    await user.click(submit);

    await waitFor(() =>
      expect(maintenanceRecordService.appendDocumentMedia).toHaveBeenCalledTimes(2),
    );
    const attachedIds = vi
      .mocked(maintenanceRecordService.appendDocumentMedia)
      .mock.calls.map(([, mediaId]) => mediaId)
      .sort();
    expect(attachedIds).toEqual(['new-1', 'new-2']);
    await waitFor(() => expect(toast.success).toHaveBeenCalledTimes(1));
  });

  // One file failing to attach must not silently drop the other, and must not
  // be reported as a full success or a full failure.
  it('reports an aggregate error when one append fails and still attaches the other', async () => {
    vi.mocked(maintenanceRecordService.patch).mockResolvedValue({} as never);
    vi.mocked(mediaService.putContent).mockResolvedValue(undefined as never);
    let uploadCount = 0;
    vi.mocked(mediaService.initUpload).mockImplementation(async () => {
      uploadCount += 1;
      return { id: `new-${uploadCount}`, type: 'media-objects', attributes: {} } as never;
    });
    vi.mocked(mediaService.confirm).mockImplementation(
      async (id: string) => ({ id, type: 'media-objects', attributes: {} }) as never,
    );
    vi.mocked(maintenanceRecordService.appendDocumentMedia).mockImplementation(
      async (_id: string, mediaId: string) => {
        if (mediaId === 'new-1') throw new Error('boom');
        return {} as never;
      },
    );

    const user = userEvent.setup();
    renderDrawer(maintenanceRow, true);

    await user.click(await screen.findByRole('button', { name: /^edit$/i }));

    const input = document.querySelector('input[type="file"]') as HTMLInputElement;
    await user.upload(input, [
      new File(['a'], 'a.pdf', { type: 'application/pdf' }),
      new File(['b'], 'b.pdf', { type: 'application/pdf' }),
    ]);

    const submit = await screen.findByRole('button', { name: /log record|log modification/i });
    await waitFor(() => expect(submit).toBeEnabled());
    await user.click(submit);

    await waitFor(() =>
      expect(maintenanceRecordService.appendDocumentMedia).toHaveBeenCalledTimes(2),
    );
    await waitFor(() =>
      expect(toast.error).toHaveBeenCalledWith(expect.stringContaining('1 attachment')),
    );
    expect(
      vi
        .mocked(maintenanceRecordService.appendDocumentMedia)
        .mock.calls.some(([, mediaId]) => mediaId === 'new-2'),
    ).toBe(true);
  });

  // Removal destroys the underlying media object and cannot be undone, so a
  // row click must open the confirmation without touching the network.
  it('opens the removal dialog without removing anything until confirmed', async () => {
    const user = userEvent.setup();
    renderDrawer(maintenanceRow, true);

    await user.click(await screen.findByRole('button', { name: /remove invoice\.pdf/i }));
    expect(await screen.findByText('Remove this attachment?')).toBeInTheDocument();
    await expectNoCall(
      vi.mocked(maintenanceRecordService.removeDocumentMedia),
      'maintenanceRecordService.removeDocumentMedia',
    );
  });

  // Confirming removes the fleet-service reference, then best-effort deletes
  // the underlying media object (lib/hooks/api/maintenance.ts's
  // useRemoveMaintenanceRecordDocument), then reports success.
  it('removes the reference, then the media object, then confirms on success', async () => {
    const order: string[] = [];
    vi.mocked(maintenanceRecordService.removeDocumentMedia).mockImplementation(async () => {
      order.push('removeDocumentMedia');
    });
    vi.mocked(mediaService.remove).mockImplementation(async () => {
      order.push('mediaRemove');
    });

    const user = userEvent.setup();
    renderDrawer(maintenanceRow, true);

    await user.click(await screen.findByRole('button', { name: /remove invoice\.pdf/i }));
    await user.click(await screen.findByRole('button', { name: /^remove$/i }));

    await waitFor(() => expect(order).toEqual(['removeDocumentMedia', 'mediaRemove']));
    expect(maintenanceRecordService.removeDocumentMedia).toHaveBeenCalledWith('m1', 'm1');
    await waitFor(() => expect(toast.success).toHaveBeenCalledWith('Attachment removed'));
  });

  // canWrite=false is the viewer role: no remove control may render on any
  // row, or a viewer could trigger a destructive, non-undoable delete.
  it('renders no remove control on any row when canWrite is false', async () => {
    renderDrawer(maintenanceRow, false);
    await screen.findByText('invoice.pdf');
    expect(screen.queryByRole('button', { name: /remove invoice\.pdf/i })).not.toBeInTheDocument();
  });

  // Finding 1: closing the dialog before the mutation resolves leaves a user
  // who hit a failure staring at an already-dismissed dialog with no way to
  // retry. The dialog must stay mounted in flight and remain open on failure.
  it('keeps the removal dialog open while the mutation is in flight and on failure', async () => {
    let rejectRemove: ((err: Error) => void) | undefined;
    vi.mocked(maintenanceRecordService.removeDocumentMedia).mockImplementation(
      () =>
        new Promise<void>((_resolve, reject) => {
          rejectRemove = reject;
        }),
    );

    const user = userEvent.setup();
    renderDrawer(maintenanceRow, true);

    await user.click(await screen.findByRole('button', { name: /remove invoice\.pdf/i }));
    await user.click(await screen.findByRole('button', { name: /^remove$/i }));

    // Still in flight: the dialog must still be mounted.
    await waitFor(() => expect(maintenanceRecordService.removeDocumentMedia).toHaveBeenCalled());
    expect(screen.getByText('Remove this attachment?')).toBeInTheDocument();

    rejectRemove?.(new Error('boom'));
    // Rejected: the dialog must remain open so the user can retry, not close
    // out from under the error toast.
    await waitFor(() => expect(toast.error).toHaveBeenCalled());
    expect(screen.getByText('Remove this attachment?')).toBeInTheDocument();
  });
});
