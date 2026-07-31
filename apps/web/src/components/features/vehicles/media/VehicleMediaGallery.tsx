import { toast } from 'sonner';
import { createErrorFromUnknown } from '@myfleet/shared-ts';
import { Skeleton } from '../../../ui/skeleton';
import { Button } from '../../../ui/button';
import { Card, CardContent } from '../../../ui/card';
import {
  useVehicleMedia,
  useSetPrimaryImage,
  useDeleteMedia,
} from '../../../../lib/hooks/api/media';
import { MediaThumbnail } from './MediaThumbnail';
import { MediaUploadButton } from './MediaUploadButton';

interface VehicleMediaGalleryProps {
  vehicleId: string;
  /** Hides write actions (upload, set-primary, delete) for viewers. */
  canWrite: boolean;
}

/**
 * Gallery section shown on the vehicle detail page.
 * Lists all media refs for the vehicle, renders thumbnails via proxied content URLs,
 * and exposes upload + primary-image selection for member/owner roles.
 */
export function VehicleMediaGallery({ vehicleId, canWrite }: VehicleMediaGalleryProps) {
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
    <Card>
      <CardContent className="pt-6">
        <div className="mb-4 flex items-center justify-between">
          <h2 className="text-base font-semibold">Photos</h2>
          {canWrite && <MediaUploadButton vehicleId={vehicleId} />}
        </div>

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
      </CardContent>
    </Card>
  );
}
