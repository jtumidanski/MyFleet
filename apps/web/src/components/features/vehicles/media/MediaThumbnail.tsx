import { Skeleton } from '../../../ui/skeleton';
import { useMediaDownloadUrl } from '../../../../lib/hooks/api/media';

interface MediaThumbnailProps {
  mediaId: string;
  isPrimary?: boolean;
  className?: string;
}

/**
 * Fetches the presigned GET URL for a media object and renders it as an <img>.
 * While loading, shows a skeleton placeholder.
 */
export function MediaThumbnail({ mediaId, isPrimary, className }: MediaThumbnailProps) {
  const { data, isLoading } = useMediaDownloadUrl(mediaId);

  if (isLoading) {
    return <Skeleton className={className ?? 'h-24 w-24 rounded'} />;
  }

  const src = data?.attributes.downloadUrl;
  if (!src) {
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
        src={src}
        alt={data?.attributes.originalFilename ?? 'Vehicle photo'}
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
