import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { screen, waitFor } from '@testing-library/react';
import { ApiError } from '@myfleet/shared-ts';
import { renderWithProviders } from '../../../test/renderWithProviders';
import { stubObjectUrl } from '../../../test/objectUrl';
import { mediaService } from '../../../services/api/MediaService';
import { VehiclePhotoThumbnail } from './VehiclePhotoThumbnail';

vi.mock('../../../services/api/MediaService', () => ({
  mediaService: { getContentBlob: vi.fn() },
}));

beforeEach(() => {
  stubObjectUrl();
});

afterEach(() => {
  vi.unstubAllGlobals();
  vi.clearAllMocks();
});

describe('VehiclePhotoThumbnail', () => {
  it('renders the photo once the bytes arrive, labelled with the vehicle', async () => {
    vi.mocked(mediaService.getContentBlob).mockResolvedValue(new Blob(['x']));

    renderWithProviders(<VehiclePhotoThumbnail mediaId="m1" vehicleLabel="2019 Honda Civic" />);

    const img = await screen.findByAltText('Photo of 2019 Honda Civic');
    expect(img).toHaveAttribute('src', expect.stringContaining('blob:'));
  });

  it('requests the thumbnail variant, not the original', async () => {
    // The whole point of the backend half of this task: a card must cost
    // kilobytes, not the full-size upload.
    vi.mocked(mediaService.getContentBlob).mockResolvedValue(new Blob(['x']));

    renderWithProviders(<VehiclePhotoThumbnail mediaId="m1" vehicleLabel="2019 Honda Civic" />);

    await waitFor(() => {
      expect(mediaService.getContentBlob).toHaveBeenCalledWith('m1', 'thumbnail');
    });
  });

  it('shows the "No photo" placeholder when there is no media id, and fetches nothing', () => {
    renderWithProviders(<VehiclePhotoThumbnail vehicleLabel="2019 Honda Civic" />);

    expect(screen.getByRole('img', { name: 'No photo' })).toBeInTheDocument();
    expect(mediaService.getContentBlob).not.toHaveBeenCalled();
  });

  it('falls back to a placeholder on a failed load, with a distinct label', async () => {
    // Visually identical to the no-photo case on purpose — a broken thumbnail
    // on a list card is not actionable — but the label keeps that from erasing
    // the information for assistive technology.
    vi.mocked(mediaService.getContentBlob).mockRejectedValue(
      new ApiError(404, 'not_found', 'gone'),
    );

    renderWithProviders(<VehiclePhotoThumbnail mediaId="m1" vehicleLabel="2019 Honda Civic" />);

    expect(await screen.findByRole('img', { name: 'Photo unavailable' })).toBeInTheDocument();
    expect(screen.queryByAltText('Photo of 2019 Honda Civic')).not.toBeInTheDocument();
  });

  it('holds a skeleton while loading rather than flashing the placeholder', () => {
    vi.mocked(mediaService.getContentBlob).mockReturnValue(new Promise(() => {}));

    const { container } = renderWithProviders(
      <VehiclePhotoThumbnail mediaId="m1" vehicleLabel="2019 Honda Civic" />,
    );

    expect(screen.queryByRole('img')).not.toBeInTheDocument();
    // The skeleton occupies exactly the thumbnail's box so the card does not
    // reflow when the image arrives.
    expect(container.querySelector('.h-20.w-20')).toBeInTheDocument();
  });
});
