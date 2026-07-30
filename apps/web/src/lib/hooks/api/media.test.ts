import { describe, it, expect, vi, beforeEach } from 'vitest';
import { mediaKeys } from './media';
import { performMediaUpload } from './media';

describe('mediaKeys', () => {
  it('is hierarchical', () => {
    expect(mediaKeys.all).toEqual(['media']);
    expect(mediaKeys.detail('m1')).toEqual(['media', 'detail', 'm1']);
    expect(mediaKeys.content('m1')).toEqual(['media', 'content', 'm1']);
  });
});

describe('performMediaUpload', () => {
  beforeEach(() => vi.restoreAllMocks());

  it('calls init → putContent → confirm in order', async () => {
    const calls: string[] = [];

    const mockInit = vi.fn().mockImplementation(async () => {
      calls.push('init');
      return {
        id: 'm1',
        type: 'media-objects',
        attributes: {
          fleetId: 'f1',
          uploadedByUserId: 'u1',
          bucket: 'myfleet-media',
          objectKey: 'f1/m1/photo.jpg',
          status: 'uploaded',
        },
      };
    });

    const mockPut = vi.fn().mockImplementation(async () => {
      calls.push('put-content');
      return {
        id: 'm1',
        type: 'media-objects',
        attributes: { status: 'uploaded' },
      };
    });

    const mockConfirm = vi.fn().mockImplementation(async () => {
      calls.push('confirm');
      return {
        id: 'm1',
        type: 'media-objects',
        attributes: { status: 'processing' },
      };
    });

    const fakeFile = new File(['bytes'], 'photo.jpg', { type: 'image/jpeg' });

    const result = await performMediaUpload(fakeFile, {
      initUpload: mockInit,
      putContent: mockPut,
      confirm: mockConfirm,
    });

    expect(calls).toEqual(['init', 'put-content', 'confirm']);
    // The bytes go to the media row's own id — never to an external URL.
    expect(mockPut).toHaveBeenCalledWith('m1', fakeFile);
    expect(result.attributes.status).toBe('processing');
  });
});
