import { describe, it, expect, vi, beforeEach } from 'vitest';
import { adminService } from './AdminService';
import { apiClient } from '../../lib/api/client';

vi.mock('../../lib/api/client', () => ({
  apiClient: { request: vi.fn() },
}));

const request = vi.mocked(apiClient.request);

beforeEach(() => {
  request.mockReset();
  request.mockResolvedValue({ data: { id: 'v1', type: 'x', attributes: {} } });
});

describe('previewVehicleTransfer', () => {
  it('omits the destination when none is given', async () => {
    await adminService.previewVehicleTransfer('v1');
    expect(request).toHaveBeenCalledWith('/api/fleet/admin/vehicles/v1/transfer-preview');
  });

  it('passes the destination as a query parameter', async () => {
    await adminService.previewVehicleTransfer('v1', 'fleet-b');
    expect(request).toHaveBeenCalledWith(
      '/api/fleet/admin/vehicles/v1/transfer-preview?destination_fleet_id=fleet-b',
    );
  });

  it('encodes an id containing URL-significant characters', async () => {
    await adminService.previewVehicleTransfer('v/1', 'a b');
    const path = request.mock.calls[0]?.[0];
    expect(path).toContain('v%2F1');
    expect(path).toContain('destination_fleet_id=a+b');
  });

  it('returns the preview resource unwrapped from the JSON:API document', async () => {
    request.mockResolvedValue({
      data: {
        id: 'v1',
        type: 'vehicle-transfer-previews',
        attributes: {
          vehicle_label: '2019 Subaru Outback (The Green Bean)',
          source_fleet_id: 'fleet-a',
          source_fleet_name: 'Fleet A',
          destination_fleet_id: 'fleet-b',
          destination_fleet_name: 'Fleet B',
          counts: { maintenance_records: 3, media_objects: 2 },
          categories_to_create: [{ name: 'Oil Change', kind: 'maintenance' }],
          warnings: ['media-service unavailable'],
        },
      },
    });

    const resource = await adminService.previewVehicleTransfer('v1', 'fleet-b');

    expect(resource.type).toBe('vehicle-transfer-previews');
    expect(resource.id).toBe('v1');
    // The label is the confirmation phrase; it must survive byte for byte.
    expect(resource.attributes.vehicle_label).toBe('2019 Subaru Outback (The Green Bean)');
    expect(resource.attributes.counts.maintenance_records).toBe(3);
    expect(resource.attributes.categories_to_create).toEqual([
      { name: 'Oil Change', kind: 'maintenance' },
    ]);
    expect(resource.attributes.warnings).toEqual(['media-service unavailable']);
  });

  it('propagates the rejection so the caller can read status and detail', async () => {
    request.mockRejectedValue(Object.assign(new Error('nope'), { status: 403 }));
    await expect(adminService.previewVehicleTransfer('v1')).rejects.toThrow('nope');
  });
});

describe('transferVehicle', () => {
  it('posts a JSON:API document with the vehicle-transfers type', async () => {
    await adminService.transferVehicle('v1', {
      destination_fleet_id: 'fleet-b',
      confirmation: 'The Green Bean',
    });
    expect(request).toHaveBeenCalledWith('/api/fleet/admin/vehicles/v1/transfer', {
      method: 'POST',
      body: JSON.stringify({
        data: {
          type: 'vehicle-transfers',
          attributes: { destination_fleet_id: 'fleet-b', confirmation: 'The Green Bean' },
        },
      }),
    });
  });

  it('encodes the vehicle id in the path', async () => {
    await adminService.transferVehicle('v/1', {
      destination_fleet_id: 'fleet-b',
      confirmation: 'x',
    });
    const path = request.mock.calls[0]?.[0];
    expect(path).toBe('/api/fleet/admin/vehicles/v%2F1/transfer');
  });

  it('sends the confirmation exactly as typed, untrimmed and unfolded', async () => {
    await adminService.transferVehicle('v1', {
      destination_fleet_id: 'fleet-b',
      confirmation: '  the green BEAN  ',
    });
    const body = request.mock.calls[0]?.[1]?.body;
    const sent = JSON.parse(String(body)) as {
      data: { attributes: { confirmation: string } };
    };
    expect(sent.data.attributes.confirmation).toBe('  the green BEAN  ');
  });

  it('returns the transfer resource and its count_semantics meta', async () => {
    request.mockResolvedValue({
      data: {
        id: 'v1',
        type: 'vehicle-transfers',
        attributes: {
          vehicle_id: 'v1',
          source_fleet_id: 'fleet-a',
          destination_fleet_id: 'fleet-b',
          transferred_at: '2026-08-25T12:00:00Z',
          affected_counts: { maintenance_records: 3, media_objects: 2, notifications: 5 },
        },
      },
      meta: {
        count_semantics: {
          media_objects: 'live rows now in the destination fleet',
          notifications: 'live rows now in the destination fleet',
        },
      },
    });

    const result = await adminService.transferVehicle('v1', {
      destination_fleet_id: 'fleet-b',
      confirmation: 'The Green Bean',
    });

    expect(result.data.type).toBe('vehicle-transfers');
    expect(result.data.id).toBe('v1');
    expect(result.data.attributes.transferred_at).toBe('2026-08-25T12:00:00Z');
    expect(result.data.attributes.affected_counts).toEqual({
      maintenance_records: 3,
      media_objects: 2,
      notifications: 5,
    });
    // The annotation is the only place the response admits that these two
    // numbers are not "rows moved". Dropping it here would end the chain.
    expect(result.meta?.count_semantics).toEqual({
      media_objects: 'live rows now in the destination fleet',
      notifications: 'live rows now in the destination fleet',
    });
  });

  it('leaves meta undefined when the server omits it', async () => {
    request.mockResolvedValue({
      data: {
        id: 'v1',
        type: 'vehicle-transfers',
        attributes: {
          vehicle_id: 'v1',
          source_fleet_id: 'fleet-a',
          destination_fleet_id: 'fleet-b',
          transferred_at: '2026-08-25T12:00:00Z',
          affected_counts: { maintenance_records: 3 },
        },
      },
    });

    const result = await adminService.transferVehicle('v1', {
      destination_fleet_id: 'fleet-b',
      confirmation: 'The Green Bean',
    });

    expect(result.meta).toBeUndefined();
    expect(result.data.attributes.affected_counts).toEqual({ maintenance_records: 3 });
  });

  it('propagates the rejection so the caller can read status and detail', async () => {
    request.mockRejectedValue(
      Object.assign(new Error('confirmation did not match'), { status: 409 }),
    );
    await expect(
      adminService.transferVehicle('v1', {
        destination_fleet_id: 'fleet-b',
        confirmation: 'wrong',
      }),
    ).rejects.toThrow('confirmation did not match');
  });
});
