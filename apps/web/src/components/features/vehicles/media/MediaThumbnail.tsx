import { ImageOff } from 'lucide-react';
import { createErrorFromUnknown } from '@myfleet/shared-ts';
import { Skeleton } from '../../../ui/skeleton';
import { cn } from '../../../../lib/utils';
import { useMediaContentUrl, useMediaObject } from '../../../../lib/hooks/api/media';

interface MediaThumbnailProps {
  mediaId: string;
  isPrimary?: boolean;
  className?: string;
}

/**
 * Maps a failed content fetch to something a 96×96 tile can actually say.
 * `short` is what fits in the tile; `label` is the accessible description.
 */
function describeError(err: unknown): { short: string; label: string } {
  const apiError = createErrorFromUnknown(err);
  if (apiError.status === 401 || apiError.status === 403) {
    return { short: 'No access', label: 'Photo unavailable: you do not have access to it' };
  }
  if (apiError.status === 404) {
    return { short: 'Missing', label: 'Photo unavailable: it no longer exists' };
  }
  return {
    short: 'Load failed',
    label: 'Photo could not be loaded. Reload the page to try again.',
  };
}

/**
 * Renders a media object's bytes, fetched through the API and held as an object
 * URL. Metadata comes from the separate detail query (both are cached by React
 * Query, so a gallery re-render costs no extra requests).
 *
 * Four render states, deliberately distinct: loading skeleton, load failure
 * (403/404/5xx), no bytes at all, and the image. Failures get a visual
 * affordance plus an accessible label rather than a toast — a gallery of N
 * broken thumbnails would otherwise fire N toasts.
 */
export function MediaThumbnail({ mediaId, isPrimary, className }: MediaThumbnailProps) {
  const { url, isLoading, isError, error } = useMediaContentUrl(mediaId);
  const { data: meta } = useMediaObject(mediaId);

  if (isLoading) {
    return <Skeleton className={cn('h-24 w-24 rounded', className)} />;
  }

  if (isError) {
    const { short, label } = describeError(error);
    return (
      <div
        role="img"
        aria-label={label}
        title={label}
        className={cn(
          'flex h-24 w-24 flex-col items-center justify-center gap-1 rounded border border-destructive/40 bg-destructive/10 px-1 text-center text-[10px] text-destructive',
          className,
        )}
      >
        <ImageOff className="h-4 w-4" aria-hidden="true" />
        <span>{short}</span>
      </div>
    );
  }

  if (!url) {
    return (
      <div
        className={cn(
          'flex h-24 w-24 items-center justify-center rounded bg-muted text-xs text-muted-foreground',
          className,
        )}
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
        className={cn('h-24 w-24 rounded object-cover', className)}
      />
      {isPrimary && (
        <span className="absolute bottom-1 left-1 rounded bg-primary px-1 py-0.5 text-[10px] font-medium text-primary-foreground">
          Primary
        </span>
      )}
    </div>
  );
}
