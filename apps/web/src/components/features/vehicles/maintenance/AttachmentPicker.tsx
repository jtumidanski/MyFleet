import { useRef } from 'react';
import { FileText, ImageIcon, Loader2, X } from 'lucide-react';
import { Button } from '../../../ui/button';
import {
  ACCEPTED_UPLOAD_TYPES,
  MEDIA_MAX_UPLOAD_BYTES,
  formatUploadSize,
} from '../../../../lib/hooks/api/media';
import {
  MAX_ATTACHMENTS,
  type PendingAttachment,
} from '../../../../lib/hooks/usePendingAttachments';

interface AttachmentPickerProps {
  items: PendingAttachment[];
  onAdd: (files: FileList | File[]) => void;
  onRemove: (localId: string) => void;
  /** Hides the picker for viewers (PRD FR-VIEW-5). */
  disabled?: boolean;
  /**
   * Attachments the record already holds. The cap is per record, so on the
   * edit path the picker's room is what is left after them. Defaults to 0, so
   * the create-flow call site is unaffected.
   *
   * A prop rather than a query: this is a presentational control inside a
   * form, and giving it a useMaintenanceRecord call would couple it to the
   * server cache and make it untestable without a QueryClient. The drawer
   * already holds the record.
   */
  existingCount?: number;
}

/**
 * Receipt picker for the log form: choose files, watch them upload, remove any
 * one before saving.
 *
 * The `accept` attribute mirrors the server allowlist as a convenience only —
 * the server answers 415 regardless of what a client offers.
 */
export function AttachmentPicker({
  items,
  onAdd,
  onRemove,
  disabled,
  existingCount = 0,
}: AttachmentPickerProps) {
  const inputRef = useRef<HTMLInputElement>(null);
  const remaining = Math.max(MAX_ATTACHMENTS - existingCount - items.length, 0);
  const isFull = remaining === 0;

  // The server cap is authoritative (a client that ignores all of this still
  // gets a 422); this copy exists so the user finds out before picking rather
  // than after saving.
  let helperText: string;
  if (isFull) {
    helperText =
      existingCount > 0
        ? `This record is at the ${MAX_ATTACHMENTS}-attachment limit.`
        : `Maximum ${MAX_ATTACHMENTS} attachments per record.`;
  } else if (existingCount > 0) {
    helperText = `${existingCount} of ${MAX_ATTACHMENTS} attached. You can add ${remaining} more.`;
  } else {
    helperText = `PDF, image, Word, Excel or CSV. Up to ${formatUploadSize(MEDIA_MAX_UPLOAD_BYTES)} each, ${MAX_ATTACHMENTS} per record.`;
  }

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

      <p className="text-xs text-muted-foreground">{helperText}</p>

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
