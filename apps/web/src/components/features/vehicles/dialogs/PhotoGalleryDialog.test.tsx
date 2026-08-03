import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { PhotoGalleryDialog } from './PhotoGalleryDialog';
import { renderWithProviders } from '../../../../test/renderWithProviders';
import { stubObjectUrl, unstubObjectUrl } from '../../../../test/objectUrl';
import { expectNoCall } from '../../../../test/expectNoCall';

const { listByVehicle, removeMedia, removeObject, setPrimaryImage, calls } = vi.hoisted(() => {
  const calls: string[] = [];
  return {
    calls,
    listByVehicle: vi.fn(),
    removeMedia: vi.fn(async () => {
      calls.push('detach-reference');
    }),
    removeObject: vi.fn(async () => {
      calls.push('delete-object');
    }),
    setPrimaryImage: vi.fn(async () => {}),
  };
});

vi.mock('../../../../services/api/VehicleMediaService', () => ({
  vehicleMediaService: { listByVehicle, removeMedia, setPrimaryImage, addMedia: vi.fn() },
}));

vi.mock('../../../../services/api/MediaService', () => ({
  mediaService: {
    remove: removeObject,
    getContentBlob: vi.fn(async () => new Blob(['x'])),
    get: vi.fn(async () => ({ id: 'm1', type: 'media', attributes: {} })),
    initUpload: vi.fn(),
    putContent: vi.fn(),
    confirm: vi.fn(),
  },
}));

vi.mock('sonner', () => ({
  toast: Object.assign(vi.fn(), { error: vi.fn(), success: vi.fn() }),
}));

function mediaRef(id: string, mediaId: string, isPrimary = false) {
  return { id, type: 'vehicleMedia', attributes: { vehicleId: 'v1', mediaId, isPrimary } };
}

beforeEach(() => {
  // The thumbnails go through useMediaContentUrl, which jsdom cannot serve.
  stubObjectUrl();
  calls.length = 0;
  removeMedia.mockClear();
  removeObject.mockClear();
  listByVehicle.mockResolvedValue([mediaRef('r1', 'm1', true), mediaRef('r2', 'm2')]);
});

afterEach(unstubObjectUrl);

function open() {
  return renderWithProviders(
    <PhotoGalleryDialog open onOpenChange={() => {}} vehicleId="v1" canWrite />,
  );
}

/**
 * Opens the confirmation for the SECOND photo (m2) — the non-primary one, so
 * the assertions are about removal itself rather than primary promotion.
 * Indexing is wrapped so a missing tile fails as a clear assertion instead of
 * an `undefined` passed into `user.click`.
 */
async function beginRemovingSecondPhoto(user: ReturnType<typeof userEvent.setup>) {
  const buttons = await screen.findAllByRole('button', { name: /remove photo/i });
  const second = buttons[1];
  if (!second) throw new Error(`expected 2 remove buttons, found ${buttons.length}`);
  await user.click(second);
}

describe('PhotoGalleryDialog — removing a photo', () => {
  it('detaches the vehicle reference, not just the media object', async () => {
    const user = userEvent.setup();
    open();

    await beginRemovingSecondPhoto(user);
    await user.click(await screen.findByRole('button', { name: /^remove$/i }));

    // The reference is what the gallery lists. Deleting only the media object
    // left the tile on screen, which is what "removing photos doesn't work"
    // looked like.
    await waitFor(() => expect(removeMedia).toHaveBeenCalledWith('v1', 'm2'));
    expect(removeObject).toHaveBeenCalledWith('m2');
  });

  it('drops the reference before the bytes', async () => {
    const user = userEvent.setup();
    open();

    await beginRemovingSecondPhoto(user);
    await user.click(await screen.findByRole('button', { name: /^remove$/i }));

    // Reverse this order and a failure on the second call leaves the gallery
    // listing a reference whose bytes are already gone.
    await waitFor(() => expect(calls).toEqual(['detach-reference', 'delete-object']));
  });

  it('still reports success when only the object cleanup fails', async () => {
    removeObject.mockRejectedValueOnce(new Error('media-service unavailable'));
    const { toast } = await import('sonner');
    const user = userEvent.setup();
    open();

    await beginRemovingSecondPhoto(user);
    await user.click(await screen.findByRole('button', { name: /^remove$/i }));

    // The user's photo IS gone from the vehicle; an orphaned object is
    // media-service's problem, not something to report as a failed removal.
    await waitFor(() => expect(toast.success).toHaveBeenCalledWith('Photo removed'));
    await expectNoCall(vi.mocked(toast.error), 'toast.error');
  });

  it('does not remove anything until the confirmation is accepted', async () => {
    const user = userEvent.setup();
    open();

    await beginRemovingSecondPhoto(user);
    await user.click(await screen.findByRole('button', { name: /cancel/i }));

    await expectNoCall(removeMedia, 'removeMedia');
    await expectNoCall(removeObject, 'removeObject');
  });
});

describe('PhotoGalleryDialog — layout', () => {
  it('keeps the upload control clear of the dialog close button', async () => {
    open();

    const upload = await screen.findByRole('button', { name: /upload photo/i });
    const close = screen.getByRole('button', { name: /^close$/i });

    // The close button is absolutely positioned at the top-right of the
    // dialog. Anything sharing the title row lands underneath it, so the
    // upload control must not be a descendant of the header.
    const header = close.closest('[role="dialog"]')?.querySelector('h2')?.parentElement;
    expect(header).not.toBeNull();
    expect(header?.contains(upload)).toBe(false);
  });

  it('offers no write controls to a viewer', async () => {
    renderWithProviders(
      <PhotoGalleryDialog open onOpenChange={() => {}} vehicleId="v1" canWrite={false} />,
    );

    await screen.findByText('Photos');
    expect(screen.queryByRole('button', { name: /remove photo/i })).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /upload photo/i })).not.toBeInTheDocument();
  });
});
