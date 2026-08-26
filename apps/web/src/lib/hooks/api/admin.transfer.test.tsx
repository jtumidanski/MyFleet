import { describe, it, expect, vi, beforeEach } from 'vitest';
import { renderHook, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import type { ReactNode } from 'react';
import { toast } from 'sonner';
import { ApiError } from '@myfleet/shared-ts';
import type { JsonApiResource } from '@myfleet/shared-ts';
import { adminService } from '../../../services/api/AdminService';
import type { VehicleTransferResult } from '../../../services/api/AdminService';
import type {
  VehicleTransferPreviewAttributes,
  VehicleTransferAttributes,
} from '../../../types/models/admin';
import { expectNoCall } from '../../../test/expectNoCall';
import { adminKeys, useTransferVehicle, useVehicleTransferPreview } from './admin';

vi.mock('../../../services/api/AdminService', () => ({
  adminService: { previewVehicleTransfer: vi.fn(), transferVehicle: vi.fn() },
}));
vi.mock('sonner', () => ({ toast: { success: vi.fn(), error: vi.fn() } }));

function wrapper(client: QueryClient) {
  return function Wrapper({ children }: { children: ReactNode }) {
    return <QueryClientProvider client={client}>{children}</QueryClientProvider>;
  };
}

function newClient() {
  return new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
}

function preview(
  overrides: Partial<VehicleTransferPreviewAttributes> = {},
): JsonApiResource<VehicleTransferPreviewAttributes> {
  return {
    id: 'v1',
    type: 'vehicle-transfer-previews',
    attributes: {
      vehicle_label: 'The Green Bean',
      source_fleet_id: 'fleet-a',
      source_fleet_name: 'Jones Household',
      destination_fleet_id: 'fleet-b',
      destination_fleet_name: 'Smith Household',
      counts: { maintenance_records: 3 },
      categories_to_create: [],
      warnings: [],
      ...overrides,
    },
  };
}

function transferResult(meta?: VehicleTransferResult['meta']): VehicleTransferResult {
  const data: JsonApiResource<VehicleTransferAttributes> = {
    id: 'v1',
    type: 'vehicle-transfers',
    attributes: {
      vehicle_id: 'v1',
      source_fleet_id: 'fleet-a',
      destination_fleet_id: 'fleet-b',
      transferred_at: '2026-08-25T00:00:00Z',
      affected_counts: { media_objects: 4, maintenance_records: 3 },
    },
  };
  return { data, meta };
}

const vars = {
  vehicleId: 'v1',
  attributes: { destination_fleet_id: 'fleet-b', confirmation: 'The Green Bean' },
  destinationName: 'Smith Household',
};

beforeEach(() => {
  vi.mocked(adminService.previewVehicleTransfer).mockReset();
  vi.mocked(adminService.transferVehicle).mockReset();
  vi.mocked(toast.error).mockReset();
  vi.mocked(toast.success).mockReset();
});

describe('adminKeys.transferPreview', () => {
  it('puts the destination in the key so a different choice is a different query', () => {
    expect(adminKeys.transferPreview('v1', 'fleet-b')).toEqual([
      'admin',
      'transfer-preview',
      'v1',
      'fleet-b',
    ]);
    expect(adminKeys.transferPreview('v1', 'fleet-b')).not.toEqual(
      adminKeys.transferPreview('v1', 'fleet-c'),
    );
  });
});

describe('useVehicleTransferPreview', () => {
  it('does not fetch while disabled', async () => {
    renderHook(() => useVehicleTransferPreview('v1', 'fleet-b', false), {
      wrapper: wrapper(newClient()),
    });
    await expectNoCall(vi.mocked(adminService.previewVehicleTransfer), 'previewVehicleTransfer');
  });

  // The endpoint treats an absent destination as a valid "not chosen yet" state
  // and answers with the source-side picture only, so an ungated query would
  // spend a request on a half-answer the dialog cannot use.
  it('does not fetch before a destination is chosen', async () => {
    renderHook(() => useVehicleTransferPreview('v1', '', true), {
      wrapper: wrapper(newClient()),
    });
    await expectNoCall(vi.mocked(adminService.previewVehicleTransfer), 'previewVehicleTransfer');
  });

  it('refetches when the destination changes', async () => {
    vi.mocked(adminService.previewVehicleTransfer).mockResolvedValue(preview());
    const client = newClient();
    const { rerender } = renderHook(
      ({ dest }: { dest: string }) => useVehicleTransferPreview('v1', dest, true),
      { wrapper: wrapper(client), initialProps: { dest: 'fleet-b' } },
    );
    await waitFor(() =>
      expect(adminService.previewVehicleTransfer).toHaveBeenCalledWith('v1', 'fleet-b'),
    );

    rerender({ dest: 'fleet-c' });
    await waitFor(() =>
      expect(adminService.previewVehicleTransfer).toHaveBeenCalledWith('v1', 'fleet-c'),
    );
  });
});

describe('useTransferVehicle', () => {
  it('invalidates the whole admin subtree on settle', async () => {
    vi.mocked(adminService.transferVehicle).mockResolvedValue(transferResult());
    const client = newClient();
    const spy = vi.spyOn(client, 'invalidateQueries');
    const { result } = renderHook(() => useTransferVehicle(), { wrapper: wrapper(client) });

    result.current.mutate(vars);

    await waitFor(() => expect(spy).toHaveBeenCalledWith({ queryKey: adminKeys.all }));
  });

  it('invalidates even when the transfer fails', async () => {
    vi.mocked(adminService.transferVehicle).mockRejectedValue(
      new ApiError(503, 'unavailable', 'unavailable', 'media-service refused; nothing was moved'),
    );
    const client = newClient();
    const spy = vi.spyOn(client, 'invalidateQueries');
    const { result } = renderHook(() => useTransferVehicle(), { wrapper: wrapper(client) });

    result.current.mutate(vars);

    await waitFor(() => expect(result.current.isError).toBe(true));
    expect(spy).toHaveBeenCalledWith({ queryKey: adminKeys.all });
  });

  it('names the destination in the success toast', async () => {
    vi.mocked(adminService.transferVehicle).mockResolvedValue(transferResult());
    const { result } = renderHook(() => useTransferVehicle(), { wrapper: wrapper(newClient()) });

    result.current.mutate(vars);

    await waitFor(() => expect(toast.success).toHaveBeenCalled());
    expect(vi.mocked(toast.success).mock.calls[0]?.[0]).toContain('Smith Household');
  });

  // meta.count_semantics is the only thing that says the media_objects figure
  // counts rows now ON the destination rather than rows this call moved. If the
  // hook resolved to the bare resource that correction would die here, one hop
  // short of the dialog that renders it.
  it('keeps meta.count_semantics on the mutation result', async () => {
    vi.mocked(adminService.transferVehicle).mockResolvedValue(
      transferResult({
        count_semantics: { media_objects: 'Media items now on the destination fleet.' },
      }),
    );
    const { result } = renderHook(() => useTransferVehicle(), { wrapper: wrapper(newClient()) });

    result.current.mutate(vars);

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data?.meta?.count_semantics).toEqual({
      media_objects: 'Media items now on the destination fleet.',
    });
    expect(result.current.data?.data.attributes.affected_counts.media_objects).toBe(4);
  });

  it('surfaces the server detail verbatim', async () => {
    vi.mocked(adminService.transferVehicle).mockRejectedValue(
      new ApiError(
        409,
        'conflict',
        'conflict',
        'vehicle is pending purge and cannot be transferred',
      ),
    );
    const { result } = renderHook(() => useTransferVehicle(), { wrapper: wrapper(newClient()) });

    result.current.mutate({ ...vars, attributes: { ...vars.attributes, confirmation: 'x' } });

    await waitFor(() => {
      expect(toast.error).toHaveBeenCalledWith(
        'vehicle is pending purge and cannot be transferred',
      );
    });
  });

  it('falls back to a generic message when there is no detail', async () => {
    vi.mocked(adminService.transferVehicle).mockRejectedValue(new Error('boom'));
    const { result } = renderHook(() => useTransferVehicle(), { wrapper: wrapper(newClient()) });

    result.current.mutate({ ...vars, attributes: { ...vars.attributes, confirmation: 'x' } });

    await waitFor(() => expect(toast.error).toHaveBeenCalledWith('boom'));
  });
});
