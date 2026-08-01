import { useState } from 'react';
import { toast } from 'sonner';
import { Download, FileText, Loader2 } from 'lucide-react';
import { createErrorFromUnknown } from '@myfleet/shared-ts';
import { Button } from '../../../ui/button';
import { Skeleton } from '../../../ui/skeleton';
import { useMediaObject } from '../../../../lib/hooks/api/media';
import { mediaService } from '../../../../services/api/MediaService';
import { downloadBlob } from '../../../../lib/utils/download';
import { MediaThumbnail } from '../media/MediaThumbnail';

/**
 * The two types media-service classifies as renderable images (its
 * mediaobject.renderableImages set). Everything else on the allowlist is a
 * document and is served as an attachment.
 */
function isRenderableImage(contentType?: string): boolean {
  return contentType === 'image/jpeg' || contentType === 'image/png';
}

function AttachmentRow({ mediaId }: { mediaId: string }) {
  const { data, isLoading, isError } = useMediaObject(mediaId);
  const [downloading, setDownloading] = useState(false);

  if (isLoading) {
    return <Skeleton className="h-10 w-full" />;
  }

  // Missing, soft-deleted, cross-fleet, or a terminal processing failure all
  // render the same explicit row rather than a broken control (PRD FR-VIEW-4).
  if (isError || !data || data.attributes.status === 'failed') {
    return (
      <div className="rounded-md border border-dashed px-2 py-1.5 text-xs text-muted-foreground">
        Attachment unavailable
      </div>
    );
  }

  const filename = data.attributes.originalFilename || mediaId;

  if (isRenderableImage(data.attributes.contentType)) {
    return (
      <div className="flex items-center gap-2 rounded-md border px-2 py-1.5">
        <MediaThumbnail mediaId={mediaId} className="h-12 w-12" />
        <span className="min-w-0 flex-1 truncate text-sm">{filename}</span>
      </div>
    );
  }

  const handleDownload = async () => {
    setDownloading(true);
    try {
      // Fetched through the authenticated API client: GET
      // /api/media/{id}/content needs an Authorization header, so a plain
      // <a href> cannot be used (PRD FR-VIEW-3).
      const blob = await mediaService.getContentBlob(mediaId);
      downloadBlob(blob, filename);
    } catch (err) {
      toast.error(createErrorFromUnknown(err).message || 'Could not download attachment');
    } finally {
      setDownloading(false);
    }
  };

  return (
    <Button
      type="button"
      variant="outline"
      className="w-full justify-start gap-2"
      disabled={downloading}
      onClick={() => void handleDownload()}
    >
      {downloading ? (
        <Loader2 className="h-4 w-4 animate-spin" aria-hidden="true" />
      ) : (
        <FileText className="h-4 w-4" aria-hidden="true" />
      )}
      <span className="min-w-0 flex-1 truncate text-left">{filename}</span>
      <Download className="h-4 w-4 shrink-0" aria-hidden="true" />
    </Button>
  );
}

/**
 * The attachments of one record. Rendered only for the expanded record, which
 * is what keeps a 25-record page from issuing 25 × N metadata requests.
 *
 * Everything here renders for viewers too — only uploading, attaching and
 * deleting are gated on write access (PRD FR-VIEW-5).
 */
export function RecordAttachmentList({ mediaIds }: { mediaIds: string[] }) {
  if (mediaIds.length === 0) {
    return <p className="text-xs text-muted-foreground">No attachments.</p>;
  }
  return (
    <div className="space-y-1.5">
      {mediaIds.map((id) => (
        <AttachmentRow key={id} mediaId={id} />
      ))}
    </div>
  );
}
