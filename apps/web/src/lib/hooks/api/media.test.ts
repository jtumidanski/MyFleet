import { describe, it, expect, vi, beforeEach } from 'vitest';
import { mediaKeys } from './media';
import { performMediaUpload } from './media';

describe('mediaKeys', () => {
  it('is hierarchical', () => {
    expect(mediaKeys.all).toEqual(['media']);
    expect(mediaKeys.detail('m1')).toEqual(['media', 'detail', 'm1']);
    expect(mediaKeys.download('m1')).toEqual(['media', 'download', 'm1']);
  });
});

describe('performMediaUpload', () => {
  beforeEach(() => vi.restoreAllMocks());

  it('calls init → presigned PUT → confirm in order', async () => {
    const calls: string[] = [];

    const mockInit = vi.fn().mockImplementation(async () => {
      calls.push('init');
      return {
        id: 'media-001',
        type: 'media-objects',
        attributes: {
          fleetId: 'f1',
          uploadedByUserId: 'u1',
          bucket: 'bucket',
          objectKey: 'key',
          status: 'uploaded',
          uploadUrl: 'https://minio.example.com/presigned-put',
          contentType: 'image/jpeg',
          originalFilename: 'photo.jpg',
        },
      };
    });

    const mockPut = vi.fn().mockImplementation(async () => {
      calls.push('presigned-put');
    });

    const mockConfirm = vi.fn().mockImplementation(async () => {
      calls.push('confirm');
      return {
        id: 'media-001',
        type: 'media-objects',
        attributes: {
          fleetId: 'f1',
          uploadedByUserId: 'u1',
          bucket: 'bucket',
          objectKey: 'key',
          status: 'processing',
          contentType: 'image/jpeg',
          originalFilename: 'photo.jpg',
        },
      };
    });

    const fakeFile = new File(['data'], 'photo.jpg', { type: 'image/jpeg' });

    const result = await performMediaUpload(fakeFile, {
      initUpload: mockInit,
      putToPresignedUrl: mockPut,
      confirm: mockConfirm,
    });

    expect(calls).toEqual(['init', 'presigned-put', 'confirm']);
    expect(mockInit).toHaveBeenCalledWith({
      contentType: 'image/jpeg',
      originalFilename: 'photo.jpg',
    });
    expect(mockPut).toHaveBeenCalledWith('https://minio.example.com/presigned-put', fakeFile);
    expect(mockConfirm).toHaveBeenCalledWith('media-001');
    expect(result.id).toBe('media-001');
  });

  it('throws if init returns no uploadUrl', async () => {
    const mockInit = vi.fn().mockResolvedValue({
      id: 'media-001',
      type: 'media-objects',
      attributes: {
        fleetId: 'f1',
        uploadedByUserId: 'u1',
        bucket: 'bucket',
        objectKey: 'key',
        status: 'uploaded',
        // uploadUrl is missing
      },
    });
    const fakeFile = new File(['data'], 'photo.jpg', { type: 'image/jpeg' });

    await expect(
      performMediaUpload(fakeFile, {
        initUpload: mockInit,
        putToPresignedUrl: vi.fn(),
        confirm: vi.fn(),
      }),
    ).rejects.toThrow('No upload URL returned from init');
  });
});
