import { Car } from 'lucide-react';
import { Skeleton } from '../../ui/skeleton';
import { cn } from '../../../lib/utils';
import { useMediaContentUrl } from '../../../lib/hooks/api/media';

/**
 * The default box: an 80x80 square. Every state occupies exactly the box in
 * force, so cards with and without photos align within a grid row and the card
 * does not reflow when an image arrives. `shrink-0` is what stops a long title
 * from squeezing it.
 */
const DEFAULT_BOX = 'h-20 w-20 shrink-0 rounded-md';

interface VehiclePhotoThumbnailProps {
  mediaId?: string;
  /** Used for the image's alt text — the card already knows it, so no metadata request is needed. */
  vehicleLabel: string;
  /**
   * Overrides the default 80x80 square — the list card passes a 16:9 hero box.
   * Applied to ALL four states (image, skeleton, and both placeholders), so
   * "identical dimensions in every state" is structurally guaranteed here rather
   * than restated at four call sites.
   */
  boxClassName?: string;
  className?: string;
}

/**
 * The neutral fallback for both "no photo uploaded" and "the photo failed to
 * load". The two cases look identical on purpose — a wall of red error tiles on
 * a list page is not something a user can act on — so the accessible label is
 * what keeps them distinguishable.
 */
function PhotoPlaceholder({
  label,
  boxClassName,
  className,
}: {
  label: string;
  boxClassName: string;
  className?: string;
}) {
  return (
    <div
      role="img"
      aria-label={label}
      className={cn(
        boxClassName,
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
  boxClassName = DEFAULT_BOX,
  className,
}: VehiclePhotoThumbnailProps) {
  const { url, isLoading, isError } = useMediaContentUrl(mediaId, 'thumbnail');

  if (!mediaId) {
    return <PhotoPlaceholder label="No photo" boxClassName={boxClassName} className={className} />;
  }
  if (isLoading) {
    return <Skeleton className={cn(boxClassName, className)} />;
  }
  if (isError || !url) {
    // "No photo" is reserved for the `!mediaId` branch above. Reaching here
    // means the vehicle DOES have a photo we could not show — a real error, or
    // React Query pausing the query offline (isLoading false, isError false,
    // data undefined), which would otherwise tell the user their vehicle has no
    // photo at all.
    return (
      <PhotoPlaceholder
        label="Photo unavailable"
        boxClassName={boxClassName}
        className={className}
      />
    );
  }
  return (
    <img
      src={url}
      alt={`Photo of ${vehicleLabel}`}
      className={cn(boxClassName, 'object-cover', className)}
    />
  );
}
