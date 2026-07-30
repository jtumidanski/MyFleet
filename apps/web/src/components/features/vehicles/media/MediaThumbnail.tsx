import { Skeleton } from '../../../ui/skeleton';
import { useMediaContentUrl, useMediaObject } from '../../../../lib/hooks/api/media';

interface MediaThumbnailProps {
  mediaId: string;
  isPrimary?: boolean;
  className?: string;
}

/**
 * Renders a media object's bytes, fetched through the API and held as an object
 * URL. Metadata comes from the separate detail query (both are cached by React
 * Query, so a gallery re-render costs no extra requests).
 */
export function MediaThumbnail({ mediaId, isPrimary, className }: MediaThumbnailProps) {
  const { url, isLoading } = useMediaContentUrl(mediaId);
  const { data: meta } = useMediaObject(mediaId);

  if (isLoading) {
    return <Skeleton className={className ?? 'h-24 w-24 rounded'} />;
  }

  if (!url) {
    return (
      <div
        className={`flex items-center justify-center rounded bg-muted text-xs text-muted-foreground ${className ?? 'h-24 w-24'}`}
      >
        No image
      </div>
    );
  }

  return (
    <div className="relative">
      <img
        src={url}
        alt={meta?.attributes.originalFilename ?? 'Vehicle photo'}
        className={`rounded object-cover ${className ?? 'h-24 w-24'}`}
      />
      {isPrimary && (
        <span className="absolute bottom-1 left-1 rounded bg-primary px-1 py-0.5 text-[10px] font-medium text-primary-foreground">
          Primary
        </span>
      )}
    </div>
  );
}
