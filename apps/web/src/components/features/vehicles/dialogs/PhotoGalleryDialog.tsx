import { toast } from 'sonner';
import { createErrorFromUnknown } from '@myfleet/shared-ts';
import { Dialog, DialogContent, DialogHeader, DialogTitle } from '../../../ui/dialog';
import { Skeleton } from '../../../ui/skeleton';
import { Button } from '../../../ui/button';
import {
  useVehicleMedia,
  useSetPrimaryImage,
  useDeleteMedia,
} from '../../../../lib/hooks/api/media';
import { MediaThumbnail } from '../media/MediaThumbnail';
import { MediaUploadButton } from '../media/MediaUploadButton';

interface PhotoGalleryDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  vehicleId: string;
  canWrite: boolean;
}

/**
 * The body of VehicleMediaGallery, lifted into a dialog. Behavior and hooks
 * are unchanged from the section it replaces — only the container differs
 * (a modal instead of an always-visible page section).
 */
export function PhotoGalleryDialog({
  open,
  onOpenChange,
  vehicleId,
  canWrite,
}: PhotoGalleryDialogProps) {
  const { data: mediaRefs, isLoading } = useVehicleMedia(vehicleId);
  const setPrimary = useSetPrimaryImage(vehicleId);
  const deleteMedia = useDeleteMedia(vehicleId);

  const handleSetPrimary = async (mediaId: string) => {
    try {
      await setPrimary.mutateAsync(mediaId);
      toast.success('Primary image updated');
    } catch (err) {
      const apiError = createErrorFromUnknown(err);
      toast.error(apiError.message || 'Could not set primary image');
    }
  };

  const handleDelete = async (mediaId: string) => {
    try {
      await deleteMedia.mutateAsync(mediaId);
      toast.success('Photo removed');
    } catch (err) {
      const apiError = createErrorFromUnknown(err);
      toast.error(apiError.message || 'Could not remove photo');
    }
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-2xl">
        <DialogHeader>
          <div className="flex items-center justify-between gap-2">
            <DialogTitle>Photos</DialogTitle>
            {canWrite && <MediaUploadButton vehicleId={vehicleId} />}
          </div>
        </DialogHeader>

        {isLoading ? (
          <div className="flex flex-wrap gap-3">
            {[1, 2, 3].map((i) => (
              <Skeleton key={i} className="h-24 w-24 rounded" />
            ))}
          </div>
        ) : !mediaRefs || mediaRefs.length === 0 ? (
          <p className="text-sm text-muted-foreground">No photos yet.</p>
        ) : (
          <div className="flex flex-wrap gap-3">
            {mediaRefs.map((ref) => (
              <div key={ref.id} className="flex flex-col items-center gap-1">
                <MediaThumbnail
                  mediaId={ref.attributes.mediaId}
                  isPrimary={ref.attributes.isPrimary}
                  className="h-24 w-24"
                />
                {canWrite && (
                  <div className="flex gap-1">
                    {!ref.attributes.isPrimary && (
                      <Button
                        type="button"
                        variant="ghost"
                        size="sm"
                        className="h-6 px-2 text-xs"
                        disabled={setPrimary.isPending}
                        onClick={() => void handleSetPrimary(ref.attributes.mediaId)}
                      >
                        Set primary
                      </Button>
                    )}
                    <Button
                      type="button"
                      variant="ghost"
                      size="sm"
                      className="h-6 px-2 text-xs text-destructive hover:text-destructive"
                      disabled={deleteMedia.isPending}
                      onClick={() => void handleDelete(ref.attributes.mediaId)}
                    >
                      Remove
                    </Button>
                  </div>
                )}
              </div>
            ))}
          </div>
        )}
      </DialogContent>
    </Dialog>
  );
}
