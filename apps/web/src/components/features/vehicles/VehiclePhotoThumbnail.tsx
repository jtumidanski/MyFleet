import { Car } from 'lucide-react';
import { Skeleton } from '../../ui/skeleton';
import { cn } from '../../../lib/utils';
import { useMediaContentUrl } from '../../../lib/hooks/api/media';

/**
 * Every state occupies exactly this box, so cards with and without photos align
 * within a grid row and the card does not reflow when an image arrives.
 * `shrink-0` is what stops a long title from squeezing it.
 */
const BOX = 'h-20 w-20 shrink-0 rounded-md';

interface VehiclePhotoThumbnailProps {
  mediaId?: string;
  /** Used for the image's alt text — the card already knows it, so no metadata request is needed. */
  vehicleLabel: string;
  className?: string;
}

/**
 * The neutral fallback for both "no photo uploaded" and "the photo failed to
 * load". The two cases look identical on purpose — a wall of red error tiles on
 * a list page is not something a user can act on — so the accessible label is
 * what keeps them distinguishable.
 */
function PhotoPlaceholder({ label, className }: { label: string; className?: string }) {
  return (
    <div
      role="img"
      aria-label={label}
      className={cn(
        BOX,
        'flex items-center justify-center bg-muted text-muted-foreground',
        className,
      )}
    >
      <Car className="h-8 w-8" aria-hidden="true" />
    </div>
  );
}

/**
 * A vehicle's primary photo, sized for a list card.
 *
 * Bytes come through the authenticated API as an object URL — a bare <img src>
 * cannot be used because the API requires an Authorization header that the
 * browser will not send for image subresource requests. The thumbnail variant
 * is requested, so a card costs tens of kilobytes rather than the full-size
 * upload.
 *
 * Deliberately not MediaThumbnail: that component issues a second metadata
 * request per tile for its alt text (N avoidable requests on a list of N
 * vehicles), renders a red "Load failed" tile on error, and hardcodes a
 * different size. The shared piece is the hook, which is the correct seam.
 *
 * No toast on failure, by construction: N broken thumbnails produce N
 * placeholders and zero notifications.
 */
export function VehiclePhotoThumbnail({
  mediaId,
  vehicleLabel,
  className,
}: VehiclePhotoThumbnailProps) {
  const { url, isLoading, isError } = useMediaContentUrl(mediaId, 'thumbnail');

  if (!mediaId) {
    return <PhotoPlaceholder label="No photo" className={className} />;
  }
  if (isLoading) {
    return <Skeleton className={cn(BOX, className)} />;
  }
  if (isError || !url) {
    return (
      <PhotoPlaceholder label={isError ? 'Photo unavailable' : 'No photo'} className={className} />
    );
  }
  return (
    <img
      src={url}
      alt={`Photo of ${vehicleLabel}`}
      className={cn(BOX, 'object-cover', className)}
    />
  );
}
