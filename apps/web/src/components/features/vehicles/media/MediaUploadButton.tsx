import { useRef } from 'react';
import { toast } from 'sonner';
import { createErrorFromUnknown } from '@myfleet/shared-ts';
import { Loader2 } from 'lucide-react';
import { Button } from '../../../ui/button';
import {
  useUploadMedia,
  useAddVehicleMedia,
  MEDIA_TOO_LARGE_CODE,
} from '../../../../lib/hooks/api/media';

interface MediaUploadButtonProps {
  vehicleId: string;
}

/**
 * The client-side guard in `performMediaUpload` already produces a message that
 * names the limit, so pass it through. A 413 that still reaches the server means
 * the client constant has drifted above the deployed `MEDIA_MAX_UPLOAD_BYTES`;
 * the raw Go string ("request entity too large") never mentions a number, so
 * don't quote a limit we now know to be wrong.
 */
function uploadErrorMessage(err: unknown): string {
  const apiError = createErrorFromUnknown(err);
  if (apiError.code === MEDIA_TOO_LARGE_CODE) return apiError.message;
  if (apiError.status === 413) {
    return 'That photo is too large to upload. Please choose a smaller file.';
  }
  return apiError.message || 'Upload failed';
}

/**
 * File-picker button that drives the three-step upload flow:
 *  1. POST /api/media (init) → media row
 *  2. PUT file bytes to /api/media/{id}/content (proxied to MinIO)
 *  3. POST /api/media/{id}/confirm
 *  4. POST /api/fleet/vehicles/{id}/media to attach the media ref
 *
 * Oversized files are rejected by `performMediaUpload` before step 1, so the
 * common case never reaches the network. Shows a spinner while uploading and
 * toasts on success/error.
 */
export function MediaUploadButton({ vehicleId }: MediaUploadButtonProps) {
  const inputRef = useRef<HTMLInputElement>(null);
  const upload = useUploadMedia(vehicleId);
  const addMedia = useAddVehicleMedia(vehicleId);

  const isPending = upload.isPending || addMedia.isPending;

  const handleFileChange = async (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    if (!file) return;

    // Reset so the same file can be re-selected
    e.target.value = '';

    try {
      const media = await upload.mutateAsync(file);
      await addMedia.mutateAsync(media.id);
      toast.success('Photo uploaded');
    } catch (err) {
      toast.error(uploadErrorMessage(err));
    }
  };

  return (
    <>
      <input
        ref={inputRef}
        type="file"
        accept="image/*"
        className="hidden"
        onChange={(e) => void handleFileChange(e)}
      />
      <Button
        type="button"
        variant="outline"
        size="sm"
        disabled={isPending}
        onClick={() => inputRef.current?.click()}
      >
        {isPending && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
        {isPending ? 'Uploading…' : 'Upload Photo'}
      </Button>
    </>
  );
}
