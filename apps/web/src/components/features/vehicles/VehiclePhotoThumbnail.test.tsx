import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { screen, waitFor } from '@testing-library/react';
import { toast } from 'sonner';
import { onlineManager } from '@tanstack/react-query';
import { ApiError } from '@myfleet/shared-ts';
import { renderWithProviders } from '../../../test/renderWithProviders';
import { stubObjectUrl, unstubObjectUrl } from '../../../test/objectUrl';
import { mediaService } from '../../../services/api/MediaService';
import { VehiclePhotoThumbnail } from './VehiclePhotoThumbnail';

vi.mock('../../../services/api/MediaService', () => ({
  mediaService: { getContentBlob: vi.fn() },
}));

// Spied rather than stubbed out: the assertion is "nothing fires a toast", so a
// real-looking module that records calls is what the test needs.
vi.mock('sonner', () => ({
  toast: Object.assign(vi.fn(), {
    error: vi.fn(),
    success: vi.fn(),
    warning: vi.fn(),
    info: vi.fn(),
  }),
}));

beforeEach(() => {
  stubObjectUrl();
});

afterEach(() => {
  // The offline test flips this globally; leaving it false would pause every
  // query in every later test file.
  onlineManager.setOnline(true);
  unstubObjectUrl();
  vi.clearAllMocks();
});

describe('VehiclePhotoThumbnail', () => {
  it('renders the photo once the bytes arrive, labelled with the vehicle', async () => {
    vi.mocked(mediaService.getContentBlob).mockResolvedValue(new Blob(['x']));

    renderWithProviders(<VehiclePhotoThumbnail mediaId="m1" vehicleLabel="2019 Honda Civic" />);

    const img = await screen.findByAltText('Photo of 2019 Honda Civic');
    expect(img).toHaveAttribute('src', expect.stringContaining('blob:'));
  });

  it('requests the card variant, not the original', async () => {
    // The whole point of the backend half of this task: a card must cost
    // kilobytes, not the full-size upload — and at a resolution that matches the
    // 16:9 hero it is rendered into, which `thumbnail` (320px) did not.
    vi.mocked(mediaService.getContentBlob).mockResolvedValue(new Blob(['x']));

    renderWithProviders(<VehiclePhotoThumbnail mediaId="m1" vehicleLabel="2019 Honda Civic" />);

    await waitFor(() => {
      expect(mediaService.getContentBlob).toHaveBeenCalledWith('m1', 'card');
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

  it('fires no toast when a thumbnail fails to load', async () => {
    // The component's contract is "N broken thumbnails produce N placeholders
    // and ZERO notifications" — a list of 20 vehicles with a wedged media
    // service must not stack 20 toasts. Nothing pinned that before, so adding
    // toast.error(...) to the error branch kept every test green.
    vi.mocked(mediaService.getContentBlob).mockRejectedValue(new ApiError(500, 'internal', 'boom'));

    renderWithProviders(<VehiclePhotoThumbnail mediaId="m1" vehicleLabel="2019 Honda Civic" />);

    await screen.findByRole('img', { name: 'Photo unavailable' });

    expect(toast.error).not.toHaveBeenCalled();
    expect(toast).not.toHaveBeenCalled();
    expect(toast.warning).not.toHaveBeenCalled();
    expect(toast.info).not.toHaveBeenCalled();
  });

  it('says "Photo unavailable", not "No photo", when a known photo cannot be fetched', async () => {
    // React Query PAUSES a query while the browser is offline: status pending,
    // fetchStatus 'paused' — so isLoading is false, isError is false, and data
    // is undefined all at once. The vehicle demonstrably HAS a photo (mediaId is
    // set and non-empty), so falling through to "No photo" here tells the user
    // something that is simply untrue.
    vi.mocked(mediaService.getContentBlob).mockResolvedValue(new Blob(['x']));
    onlineManager.setOnline(false);

    renderWithProviders(<VehiclePhotoThumbnail mediaId="m1" vehicleLabel="2019 Honda Civic" />);

    expect(await screen.findByRole('img', { name: 'Photo unavailable' })).toBeInTheDocument();
    expect(screen.queryByRole('img', { name: 'No photo' })).not.toBeInTheDocument();
    // And the request really was never made — this is the paused state, not a
    // fetch that failed.
    expect(mediaService.getContentBlob).not.toHaveBeenCalled();
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

  describe('boxClassName override', () => {
    const BOX = 'aspect-[16/9] w-full rounded-none';

    it('reaches the no-photo placeholder', () => {
      renderWithProviders(<VehiclePhotoThumbnail vehicleLabel="Civic" boxClassName={BOX} />);
      expect(screen.getByRole('img', { name: 'No photo' })).toHaveClass('aspect-[16/9]', 'w-full');
    });

    it('reaches the loading skeleton', () => {
      // The blob promise is left pending, so the component stays in isLoading.
      vi.mocked(mediaService.getContentBlob).mockReturnValue(new Promise(() => {}));
      const { container } = renderWithProviders(
        <VehiclePhotoThumbnail mediaId="m1" vehicleLabel="Civic" boxClassName={BOX} />,
      );
      expect(container.querySelector('.aspect-\\[16\\/9\\]')).toBeInTheDocument();
    });

    it('reaches the failed-photo placeholder', async () => {
      vi.mocked(mediaService.getContentBlob).mockRejectedValue(new Error('nope'));
      renderWithProviders(
        <VehiclePhotoThumbnail mediaId="m1" vehicleLabel="Civic" boxClassName={BOX} />,
      );
      const placeholder = await screen.findByRole('img', { name: 'Photo unavailable' });
      expect(placeholder).toHaveClass('aspect-[16/9]', 'w-full');
    });

    it('reaches the loaded image', async () => {
      vi.mocked(mediaService.getContentBlob).mockResolvedValue(new Blob(['x']));
      renderWithProviders(
        <VehiclePhotoThumbnail mediaId="m1" vehicleLabel="Civic" boxClassName={BOX} />,
      );
      const img = await screen.findByAltText('Photo of Civic');
      expect(img).toHaveClass('aspect-[16/9]', 'w-full', 'object-cover');
    });

    it('keeps the 80x80 square when no override is given', () => {
      renderWithProviders(<VehiclePhotoThumbnail vehicleLabel="Civic" />);
      expect(screen.getByRole('img', { name: 'No photo' })).toHaveClass('h-20', 'w-20');
    });
  });
});
