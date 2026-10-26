import { describe, it, expect, vi, beforeEach } from 'vitest';
import { apiClient } from '../../lib/api/client';
import { mileageService } from './MileageService';

vi.mock('../../lib/api/client', () => ({
  apiClient: { request: vi.fn() },
}));

beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(apiClient.request).mockResolvedValue({ data: [] });
});

function requestedUrl(): URL {
  const call = vi.mocked(apiClient.request).mock.calls[0];
  if (!call) throw new Error('apiClient.request was not called');
  return new URL(call[0], 'http://localhost');
}

describe('mileageService.listByVehicle source filter', () => {
  it('serializes sources as a comma-separated source param', async () => {
    await mileageService.listByVehicle('v1', { sources: ['manual', 'fuel'] });
    expect(requestedUrl().searchParams.get('source')).toBe('manual,fuel');
  });

  it('sends no source param for an empty sources array', async () => {
    await mileageService.listByVehicle('v1', { sources: [] });
    expect(requestedUrl().searchParams.has('source')).toBe(false);
  });

  it('sends no source param when sources is absent', async () => {
    await mileageService.listByVehicle('v1');
    expect(requestedUrl().searchParams.has('source')).toBe(false);
  });
});
