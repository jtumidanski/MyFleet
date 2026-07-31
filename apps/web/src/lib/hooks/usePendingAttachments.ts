import { useCallback, useEffect, useRef, useState } from 'react';
import { createErrorFromUnknown } from '@myfleet/shared-ts';
import { mediaService } from '../../services/api/MediaService';
import { performMediaUpload } from './api/media';

/**
 * Hard cap on attachments per record, mirroring
 * apps/fleet-service/internal/maintenancerecord.MaxDocuments. It bounds the
 * ids= query string on media-service's internal endpoint, the fan-out when an
 * attachment list is expanded, and the insert loop (design D9).
 */
export const MAX_ATTACHMENTS = 10;

export interface PendingAttachment {
  /** Stable across re-renders; the file itself is not a usable key. */
  localId: string;
  file: File;
  /**
   * 'ready' means the three-step upload completed — NOT that the media object
   * reached status 'ready'. An image is still 'processing' server-side at that
   * point, which is fine: the server validates ownership, not readiness
   * (design D8).
   */
  status: 'uploading' | 'ready' | 'failed';
  mediaId?: string;
  error?: string;
}

/**
 * Owns the upload lifecycle for files a user picks while filling in a form,
 * before any record exists to attach them to.
 *
 * Cleanup has three layers, in order of reliability (design D17):
 *  1. remove() deletes an uploaded object explicitly.
 *  2. An unmount effect deletes everything uploaded but never committed.
 *  3. The 5-day purge_after sweep catches the rest.
 *
 * Deliberately no beforeunload handler: it cannot reliably issue authenticated
 * requests during teardown, and layer 3 already covers the case.
 */
export function usePendingAttachments() {
  const [items, setItems] = useState<PendingAttachment[]>([]);
  // A ref mirrors the list so async upload callbacks and the unmount cleanup
  // read the current value without re-subscribing on every state change.
  const itemsRef = useRef<PendingAttachment[]>([]);
  const committedRef = useRef(false);

  const write = useCallback((next: PendingAttachment[]) => {
    itemsRef.current = next;
    setItems(next);
  }, []);

  const patch = useCallback(
    (localId: string, changes: Partial<PendingAttachment>) => {
      write(itemsRef.current.map((i) => (i.localId === localId ? { ...i, ...changes } : i)));
    },
    [write],
  );

  const add = useCallback(
    (files: FileList | File[]) => {
      const room = Math.max(MAX_ATTACHMENTS - itemsRef.current.length, 0);
      const accepted = Array.from(files).slice(0, room);
      if (accepted.length === 0) {
        return;
      }

      const created: PendingAttachment[] = accepted.map((file) => ({
        localId: crypto.randomUUID(),
        file,
        status: 'uploading',
      }));
      write([...itemsRef.current, ...created]);

      for (const item of created) {
        void performMediaUpload(item.file, {
          initUpload: (attrs) => mediaService.initUpload(attrs),
          putContent: (id, f) => mediaService.putContent(id, f),
          confirm: (id) => mediaService.confirm(id),
        })
          .then((media) => patch(item.localId, { status: 'ready', mediaId: media.id }))
          .catch((err) =>
            patch(item.localId, {
              status: 'failed',
              // Reported by name with the reason; the rest of the form is
              // unaffected (PRD FR-DOC-4).
              error: createErrorFromUnknown(err).message || 'Upload failed',
            }),
          );
      }
    },
    [patch, write],
  );

  const remove = useCallback(
    (localId: string) => {
      const target = itemsRef.current.find((i) => i.localId === localId);
      write(itemsRef.current.filter((i) => i.localId !== localId));
      if (target?.mediaId) {
        void mediaService.remove(target.mediaId).catch(() => undefined);
      }
    },
    [write],
  );

  /**
   * Returns the media IDs to submit and disarms the unmount cleanup, so a
   * successful save never deletes the media it just attached.
   */
  const commit = useCallback((): string[] => {
    committedRef.current = true;
    return itemsRef.current
      .filter((i): i is PendingAttachment & { mediaId: string } => !!i.mediaId)
      .map((i) => i.mediaId);
  }, []);

  useEffect(
    () => () => {
      if (committedRef.current) {
        return;
      }
      for (const item of itemsRef.current) {
        if (item.mediaId) {
          void mediaService.remove(item.mediaId).catch(() => undefined);
        }
      }
    },
    [],
  );

  const mediaIds = items.filter((i) => !!i.mediaId).map((i) => i.mediaId as string);

  return {
    items,
    add,
    remove,
    commit,
    mediaIds,
    isUploading: items.some((i) => i.status === 'uploading'),
    isFull: items.length >= MAX_ATTACHMENTS,
  };
}
