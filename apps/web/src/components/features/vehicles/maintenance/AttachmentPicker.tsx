import { useRef } from 'react';
import { FileText, ImageIcon, Loader2, X } from 'lucide-react';
import { Button } from '../../../ui/button';
import {
  ACCEPTED_UPLOAD_TYPES,
  MEDIA_MAX_UPLOAD_BYTES,
  formatUploadSize,
} from '../../../../lib/hooks/api/media';
import { MAX_ATTACHMENTS, type PendingAttachment } from '../../../../lib/hooks/usePendingAttachments';

interface AttachmentPickerProps {
  items: PendingAttachment[];
  onAdd: (files: FileList | File[]) => void;
  onRemove: (localId: string) => void;
  /** Hides the picker for viewers (PRD FR-VIEW-5). */
  disabled?: boolean;
}

/**
 * Receipt picker for the log form: choose files, watch them upload, remove any
 * one before saving.
 *
 * The `accept` attribute mirrors the server allowlist as a convenience only —
 * the server answers 415 regardless of what a client offers.
 */
export function AttachmentPicker({ items, onAdd, onRemove, disabled }: AttachmentPickerProps) {
  const inputRef = useRef<HTMLInputElement>(null);
  const isFull = items.length >= MAX_ATTACHMENTS;

  return (
    <div className="space-y-2">
      <div className="flex items-center justify-between">
        <span className="text-sm font-medium">Receipts &amp; documents</span>
        <Button
          type="button"
          size="sm"
          variant="outline"
          disabled={disabled || isFull}
          onClick={() => inputRef.current?.click()}
        >
          Add files
        </Button>
      </div>

      <input
        ref={inputRef}
        type="file"
        multiple
        className="hidden"
        accept={ACCEPTED_UPLOAD_TYPES}
        onChange={(e) => {
          if (e.target.files) {
            onAdd(e.target.files);
          }
          // Reset so re-picking the same file fires change again.
          e.target.value = '';
        }}
      />

      <p className="text-xs text-muted-foreground">
        {isFull
          ? `Maximum ${MAX_ATTACHMENTS} attachments per record.`
          : `PDF, image, Word, Excel or CSV. Up to ${formatUploadSize(MEDIA_MAX_UPLOAD_BYTES)} each, ${MAX_ATTACHMENTS} per record.`}
      </p>

      {items.length > 0 && (
        <ul className="space-y-1">
          {items.map((item) => (
            <li
              key={item.localId}
              className="flex items-center gap-2 rounded-md border px-2 py-1.5 text-sm"
            >
              {item.file.type.startsWith('image/') ? (
                <ImageIcon className="h-4 w-4 shrink-0 text-muted-foreground" aria-hidden="true" />
              ) : (
                <FileText className="h-4 w-4 shrink-0 text-muted-foreground" aria-hidden="true" />
              )}

              <span className="min-w-0 flex-1 truncate">{item.file.name}</span>

              {item.status === 'uploading' && (
                <Loader2 className="h-4 w-4 animate-spin" aria-label="Uploading" />
              )}
              {item.status === 'failed' && (
                <span className="truncate text-xs text-destructive" title={item.error}>
                  {item.error ?? 'Upload failed'}
                </span>
              )}

              <Button
                type="button"
                size="sm"
                variant="ghost"
                className="h-6 w-6 p-0"
                disabled={disabled}
                aria-label={`Remove ${item.file.name}`}
                onClick={() => onRemove(item.localId)}
              >
                <X className="h-4 w-4" aria-hidden="true" />
              </Button>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}
