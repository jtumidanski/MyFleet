import { useState } from 'react';
import { toast } from 'sonner';
import { Star, Trash2 } from 'lucide-react';
import { createErrorFromUnknown } from '@myfleet/shared-ts';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '../../../ui/dialog';
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '../../../ui/alert-dialog';
import { Skeleton } from '../../../ui/skeleton';
import { Button } from '../../../ui/button';
import {
  useVehicleMedia,
  useSetPrimaryImage,
  useRemoveVehiclePhoto,
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
 * A vehicle's photos, with per-photo primary/remove controls for writers.
 *
 * Layout note: the upload control is deliberately NOT in the header row. Every
 * DialogContent draws its own close button at `right-4 top-4`, so a
 * right-aligned control on the title line lands underneath it. The upload
 * action lives in the footer instead, where the dialog owns the full width.
 */
export function PhotoGalleryDialog({
  open,
  onOpenChange,
  vehicleId,
  canWrite,
}: PhotoGalleryDialogProps) {
  const { data: mediaRefs, isLoading } = useVehicleMedia(vehicleId);
  const setPrimary = useSetPrimaryImage(vehicleId);
  const removePhoto = useRemoveVehiclePhoto(vehicleId);

  // Removal is not undoable, so it is confirmed. Holding the pending media id
  // (rather than a boolean) is what keeps the confirmation bound to the tile
  // that opened it.
  const [pendingRemoval, setPendingRemoval] = useState<string | null>(null);

  const handleSetPrimary = async (mediaId: string) => {
    try {
      await setPrimary.mutateAsync(mediaId);
      toast.success('Primary image updated');
    } catch (err) {
      const apiError = createErrorFromUnknown(err);
      toast.error(apiError.message || 'Could not set primary image');
    }
  };

  const handleRemove = async (mediaId: string) => {
    setPendingRemoval(null);
    try {
      await removePhoto.mutateAsync(mediaId);
      toast.success('Photo removed');
    } catch (err) {
      const apiError = createErrorFromUnknown(err);
      toast.error(apiError.message || 'Could not remove photo');
    }
  };

  const photos = mediaRefs ?? [];

  return (
    <>
      <Dialog open={open} onOpenChange={onOpenChange}>
        <DialogContent className="max-w-2xl">
          <DialogHeader>
            <DialogTitle>Photos</DialogTitle>
            <DialogDescription>
              {canWrite
                ? 'Choose which photo represents this vehicle, or remove one you no longer want.'
                : 'Photos for this vehicle.'}
            </DialogDescription>
          </DialogHeader>

          {isLoading ? (
            <div className="grid grid-cols-2 gap-3 sm:grid-cols-3">
              {[1, 2, 3].map((i) => (
                <Skeleton key={i} className="aspect-square w-full rounded-md" />
              ))}
            </div>
          ) : photos.length === 0 ? (
            <p className="rounded-lg border border-dashed border-border p-6 text-center text-sm text-muted-foreground">
              No photos yet.
            </p>
          ) : (
            <ul className="grid grid-cols-2 gap-3 sm:grid-cols-3">
              {photos.map((ref) => (
                <li
                  key={ref.id}
                  className="overflow-hidden rounded-md border border-border bg-card"
                >
                  <MediaThumbnail
                    mediaId={ref.attributes.mediaId}
                    isPrimary={ref.attributes.isPrimary}
                    className="aspect-square h-auto w-full rounded-none"
                  />
                  {canWrite && (
                    /* Controls sit in a bar BELOW the image rather than
                       floating over its corners: overlaid icon buttons on a
                       photograph have no reliable contrast, and on a touch
                       target this size they collide with each other. */
                    <div className="flex items-center justify-between gap-1 border-t border-border px-1 py-1">
                      <Button
                        type="button"
                        variant="ghost"
                        size="sm"
                        className="h-7 gap-1 px-2 text-xs"
                        disabled={ref.attributes.isPrimary || setPrimary.isPending}
                        onClick={() => void handleSetPrimary(ref.attributes.mediaId)}
                      >
                        <Star
                          className={`h-3.5 w-3.5 ${ref.attributes.isPrimary ? 'fill-current' : ''}`}
                          aria-hidden="true"
                        />
                        {ref.attributes.isPrimary ? 'Primary' : 'Make primary'}
                      </Button>
                      <Button
                        type="button"
                        variant="ghost"
                        size="sm"
                        className="h-7 w-7 shrink-0 p-0 text-destructive hover:text-destructive"
                        disabled={removePhoto.isPending}
                        onClick={() => setPendingRemoval(ref.attributes.mediaId)}
                      >
                        <Trash2 className="h-3.5 w-3.5" aria-hidden="true" />
                        <span className="sr-only">Remove photo</span>
                      </Button>
                    </div>
                  )}
                </li>
              ))}
            </ul>
          )}

          {canWrite && (
            <DialogFooter>
              <MediaUploadButton vehicleId={vehicleId} />
            </DialogFooter>
          )}
        </DialogContent>
      </Dialog>

      {/* Sibling of the gallery, not a child: nesting it inside DialogContent
          would unmount the confirmation along with the dialog the moment the
          alert took focus away. */}
      <AlertDialog
        open={pendingRemoval !== null}
        onOpenChange={(next) => !next && setPendingRemoval(null)}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Remove this photo?</AlertDialogTitle>
            <AlertDialogDescription>
              The photo is removed from this vehicle and deleted. This cannot be undone.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>Cancel</AlertDialogCancel>
            <AlertDialogAction onClick={() => pendingRemoval && void handleRemove(pendingRemoval)}>
              Remove
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </>
  );
}
