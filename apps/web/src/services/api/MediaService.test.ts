import { describe, it, expect, vi, beforeEach } from 'vitest';
import { apiClient } from '../../lib/api/client';
import { mediaService } from './MediaService';

vi.mock('../../lib/api/client', () => ({
  apiClient: { requestBlob: vi.fn() },
}));

beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(apiClient.requestBlob).mockResolvedValue(new Blob(['x']));
});

/**
 * The wire format of the content request is a contract, not an implementation
 * detail. The vehicle detail gallery was deliberately left untouched by the
 * variant work, and the ONLY thing keeping its requests byte-identical is that
 * `'original'` appends no query parameter. Nothing asserted that before.
 */
describe('mediaService.getContentBlob', () => {
  it('sends no query parameter at all when no variant is given', async () => {
    await mediaService.getContentBlob('m1');
    expect(apiClient.requestBlob).toHaveBeenCalledWith('/api/media/m1/content');
  });

  it("sends no query parameter for an explicit 'original' either", async () => {
    await mediaService.getContentBlob('m1', 'original');
    // Not '?variant=original': the pre-variant callers' requests must stay
    // byte-identical, and the backend treats absent and "original" the same.
    expect(apiClient.requestBlob).toHaveBeenCalledWith('/api/media/m1/content');
  });

  it("appends ?variant=thumbnail for the list card's request", async () => {
    await mediaService.getContentBlob('m1', 'thumbnail');
    expect(apiClient.requestBlob).toHaveBeenCalledWith('/api/media/m1/content?variant=thumbnail');
  });

  it('appends ?variant=display for the display rendition', async () => {
    await mediaService.getContentBlob('m1', 'display');
    expect(apiClient.requestBlob).toHaveBeenCalledWith('/api/media/m1/content?variant=display');
  });
});
