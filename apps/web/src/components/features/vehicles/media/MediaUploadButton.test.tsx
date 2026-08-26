import { describe, it, expect, vi, afterEach } from 'vitest';
import { waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { toast } from 'sonner';
import { ApiError } from '@myfleet/shared-ts';
import { renderWithProviders } from '../../../../test/renderWithProviders';
import { expectNoCall, expectNoCallWith } from '../../../../test/expectNoCall';
import { mediaService } from '../../../../services/api/MediaService';
import { MEDIA_TOO_LARGE_CODE } from '../../../../lib/hooks/api/media';
import { MediaUploadButton } from './MediaUploadButton';

// Only the service boundary is mocked, not createErrorFromUnknown: initUpload
// rejects with a real ApiError, exactly like apiClient.request does on a
// failed response, so these tests exercise the real conversion path added in
// task-031 (the `e instanceof ApiError` short circuit), not a hand-built
// stand-in for it.
vi.mock('../../../../services/api/MediaService', () => ({
  mediaService: {
    initUpload: vi.fn(),
    putContent: vi.fn(),
    confirm: vi.fn(),
  },
}));

vi.mock('../../../../services/api/VehicleMediaService', () => ({
  vehicleMediaService: { addMedia: vi.fn() },
}));

vi.mock('sonner', () => ({
  toast: Object.assign(vi.fn(), {
    error: vi.fn(),
    success: vi.fn(),
    warning: vi.fn(),
    info: vi.fn(),
  }),
}));

afterEach(() => {
  vi.clearAllMocks();
});

async function selectFile(file: File) {
  renderWithProviders(<MediaUploadButton vehicleId="v1" />);
  const input = document.querySelector<HTMLInputElement>('input[type="file"]');
  if (!input) throw new Error('file input not found');
  await userEvent.upload(input, file);
}

describe('MediaUploadButton error mapping', () => {
  it('shows the canned "too large" message for a server 413 that is not the client-side code', async () => {
    // A real backend 413 answers with its own error code (e.g. an internal
    // "request entity too large" string), NOT the client-only
    // MEDIA_TOO_LARGE_CODE — that constant only exists for the client-side
    // pre-flight guard in performMediaUpload. Before task-031's fix, status
    // was rebuilt as 0, so this fell through to the raw Go message instead of
    // the canned copy.
    vi.mocked(mediaService.initUpload).mockRejectedValue(
      new ApiError(413, 'request_entity_too_large', 'request entity too large'),
    );

    const file = new File(['x'], 'photo.jpg', { type: 'image/jpeg' });
    await selectFile(file);

    await waitFor(() => {
      expect(toast.error).toHaveBeenCalledWith(
        'That photo is too large to upload. Please choose a smaller file.',
      );
    });
    await expectNoCallWith(vi.mocked(toast.error), ['request entity too large'], 'toast.error');
  });

  it('passes through the ApiError message for the MEDIA_TOO_LARGE_CODE branch, even when it is empty', async () => {
    // This is the branch that checks `code === MEDIA_TOO_LARGE_CODE` ahead of
    // the status check. For any non-empty message it happens to read the same
    // text the generic fallback (`apiError.message || 'Upload failed'`) would
    // also produce, so an empty message is the only fixture that actually
    // distinguishes "the code branch ran" from "the fallback ran": the code
    // branch returns the empty string verbatim, the fallback substitutes
    // 'Upload failed'. Before the fix, code was rebuilt as 'unknown', so this
    // branch was unreachable and the fallback's 'Upload failed' would show
    // instead.
    vi.mocked(mediaService.initUpload).mockRejectedValue(
      new ApiError(413, MEDIA_TOO_LARGE_CODE, ''),
    );

    const file = new File(['x'], 'photo.jpg', { type: 'image/jpeg' });
    await selectFile(file);

    await waitFor(() => {
      expect(toast.error).toHaveBeenCalledWith('');
    });
    await expectNoCallWith(vi.mocked(toast.error), ['Upload failed'], 'toast.error');
  });

  it('shows the client-side size-limit message for an oversized file (MEDIA_TOO_LARGE_CODE, no network call)', async () => {
    // performMediaUpload's own pre-flight guard throws a real ApiError with
    // MEDIA_TOO_LARGE_CODE before any service call — this is the common case
    // the docstring on uploadErrorMessage describes ("pass it through").
    const big = new File([new Uint8Array(1)], 'huge.jpg', { type: 'image/jpeg' });
    Object.defineProperty(big, 'size', { value: 26214401 });

    await selectFile(big);

    await waitFor(() => {
      expect(toast.error).toHaveBeenCalledWith(
        expect.stringContaining('The maximum upload size is'),
      );
    });
    await expectNoCall(vi.mocked(mediaService.initUpload), 'mediaService.initUpload');
  });

  it('shows the raw upload error message for a status that is neither the too-large code nor 413', async () => {
    vi.mocked(mediaService.initUpload).mockRejectedValue(
      new ApiError(500, 'internal', 'server exploded'),
    );

    const file = new File(['x'], 'photo.jpg', { type: 'image/jpeg' });
    await selectFile(file);

    await waitFor(() => {
      expect(toast.error).toHaveBeenCalledWith('server exploded');
    });
  });
});
