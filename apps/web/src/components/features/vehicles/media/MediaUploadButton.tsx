import { useRef } from 'react';
import { toast } from 'sonner';
import { createErrorFromUnknown } from '@myfleet/shared-ts';
import { Loader2 } from 'lucide-react';
import { Button } from '../../../ui/button';
import { useUploadMedia, useAddVehicleMedia } from '../../../../lib/hooks/api/media';

interface MediaUploadButtonProps {
  vehicleId: string;
}

/**
 * File-picker button that drives the three-step upload flow:
 *  1. POST /api/media (init) → presigned PUT URL
 *  2. PUT file bytes directly to MinIO presigned URL
 *  3. POST /api/media/{id}/confirm
 *  4. POST /api/fleet/vehicles/{id}/media to attach the media ref
 *
 * Shows a spinner while uploading. Toasts on success/error.
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
      const apiError = createErrorFromUnknown(err);
      toast.error(apiError.message || 'Upload failed');
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
