import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { screen } from '@testing-library/react';
import { ApiError, type JsonApiResource } from '@myfleet/shared-ts';
import { renderWithProviders } from '../../../../test/renderWithProviders';
import { stubObjectUrl, unstubObjectUrl } from '../../../../test/objectUrl';
import { mediaService } from '../../../../services/api/MediaService';
import type { MediaObjectAttributes } from '../../../../types/models/media';
import { MediaThumbnail } from './MediaThumbnail';

const readyMedia: JsonApiResource<MediaObjectAttributes> = {
  id: 'm1',
  type: 'media-objects',
  attributes: {
    fleetId: 'f1',
    uploadedByUserId: 'u1',
    bucket: 'b',
    objectKey: 'k',
    contentType: 'image/jpeg',
    originalFilename: 'photo.jpg',
    status: 'ready',
  },
};

// Only mocking the service boundary (mediaService), NOT createErrorFromUnknown
// itself. `mediaService.getContentBlob` rejects with a real `ApiError` because
// that is exactly what `apiClient.request`/`requestBlob` throw on a failed
// response — so these tests exercise the real conversion in
// `createErrorFromUnknown`, including the `e instanceof ApiError` short
// circuit added for task-031. Rebuilding the error (the pre-fix behavior)
// resets `status` to 0, which collapses every one of these cases onto the
// same generic branch — see the assertions below.
vi.mock('../../../../services/api/MediaService', () => ({
  mediaService: { getContentBlob: vi.fn(), get: vi.fn() },
}));

beforeEach(() => {
  stubObjectUrl();
  // The metadata query (useMediaObject) is independent of the content query
  // under test; give it a resolved value so it doesn't dangle as an unrelated
  // rejected promise.
  vi.mocked(mediaService.get).mockResolvedValue(readyMedia);
});

afterEach(() => {
  unstubObjectUrl();
  vi.clearAllMocks();
});

describe('MediaThumbnail error mapping', () => {
  it('shows "No access" for a 401', async () => {
    vi.mocked(mediaService.getContentBlob).mockRejectedValue(
      new ApiError(401, 'unauthorized', 'not authenticated'),
    );

    renderWithProviders(<MediaThumbnail mediaId="m1" />);

    expect(
      await screen.findByRole('img', { name: 'Photo unavailable: you do not have access to it' }),
    ).toBeInTheDocument();
    expect(screen.getByText('No access')).toBeInTheDocument();
  });

  it('shows "No access" for a 403', async () => {
    vi.mocked(mediaService.getContentBlob).mockRejectedValue(
      new ApiError(403, 'forbidden', 'not permitted'),
    );

    renderWithProviders(<MediaThumbnail mediaId="m1" />);

    expect(
      await screen.findByRole('img', { name: 'Photo unavailable: you do not have access to it' }),
    ).toBeInTheDocument();
    expect(screen.getByText('No access')).toBeInTheDocument();
  });

  it('shows "Missing" for a 404', async () => {
    vi.mocked(mediaService.getContentBlob).mockRejectedValue(
      new ApiError(404, 'not_found', 'no such media'),
    );

    renderWithProviders(<MediaThumbnail mediaId="m1" />);

    expect(
      await screen.findByRole('img', { name: 'Photo unavailable: it no longer exists' }),
    ).toBeInTheDocument();
    expect(screen.getByText('Missing')).toBeInTheDocument();
  });

  it('falls back to the generic "Load failed" label for a 500', async () => {
    // Distinguishes the specific mappings above from the catch-all: a status
    // that is neither 401/403 nor 404 must still land on the pre-existing
    // generic message, not "No access"/"Missing".
    vi.mocked(mediaService.getContentBlob).mockRejectedValue(new ApiError(500, 'internal', 'boom'));

    renderWithProviders(<MediaThumbnail mediaId="m1" />);

    expect(
      await screen.findByRole('img', {
        name: 'Photo could not be loaded. Reload the page to try again.',
      }),
    ).toBeInTheDocument();
    expect(screen.getByText('Load failed')).toBeInTheDocument();
  });
});
