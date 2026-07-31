import { useEffect, useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { mediaService } from '../../../services/api/MediaService';
import { vehicleMediaService } from '../../../services/api/VehicleMediaService';
import { ApiError, type JsonApiResource } from '@myfleet/shared-ts';
import type { MediaObjectAttributes, InitMediaUploadAttributes } from '../../../types/models/media';

// Hierarchical query-key factory.
// all                       -> ['media']
// detail('m1')              -> ['media', 'detail', 'm1']
// content('m1')             -> ['media', 'content', 'm1']
// vehicleMedia(vehicleId)   -> ['media', 'vehicle', vehicleId]
export const mediaKeys = {
  all: ['media'] as const,
  details: () => [...mediaKeys.all, 'detail'] as const,
  detail: (id: string) => [...mediaKeys.details(), id] as const,
  contents: () => [...mediaKeys.all, 'content'] as const,
  content: (id: string) => [...mediaKeys.contents(), id] as const,
  vehicleMediaAll: () => [...mediaKeys.all, 'vehicle'] as const,
  vehicleMedia: (vehicleId: string) => [...mediaKeys.vehicleMediaAll(), vehicleId] as const,
};

// ---------------------------------------------------------------------------
// Upload helper (pure function — injectable for testing)
// ---------------------------------------------------------------------------

/**
 * Client-side mirror of the media-service `MEDIA_MAX_UPLOAD_BYTES` config key
 * (`deploy/k8s/base/media-service/configmap.yaml`, default in
 * `apps/media-service/cmd/main.go`). 26214400 bytes = 25 MiB.
 *
 * This is a UX affordance, NOT a security control: the server enforces the real
 * cap with `http.MaxBytesReader` and will still reject an oversized body if this
 * constant ever drifts above the deployed value. Its only job is to fail fast
 * with a message that names the limit, because the server's 413 closes the
 * connection without draining the in-flight body — which a browser mid-upload
 * usually surfaces as an opaque `TypeError: Failed to fetch`.
 */
export const MEDIA_MAX_UPLOAD_BYTES = 26214400;

/** JSON:API-style code used for the client-side size rejection. */
export const MEDIA_TOO_LARGE_CODE = 'payload_too_large';

/**
 * Client-side mirror of the media-service `MEDIA_ALLOWED_CONTENT_TYPES` config
 * key, formatted for a file input's `accept` attribute.
 *
 * Like `MEDIA_MAX_UPLOAD_BYTES` this is a UX affordance, NOT a security
 * control: the server validates the content type against its own allowlist and
 * answers 415 regardless of what this string says.
 */
export const ACCEPTED_UPLOAD_TYPES = [
  'image/jpeg',
  'image/png',
  'application/pdf',
  'application/vnd.openxmlformats-officedocument.wordprocessingml.document',
  'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet',
  'text/csv',
].join(',');

/** Bytes → a human-readable MB string ("25 MB", "31.5 MB"). */
export function formatUploadSize(bytes: number): string {
  const mb = bytes / (1024 * 1024);
  return `${Number.isInteger(mb) ? mb : mb.toFixed(1)} MB`;
}

export interface UploadDeps {
  initUpload: (attrs: InitMediaUploadAttributes) => Promise<JsonApiResource<MediaObjectAttributes>>;
  putContent: (id: string, file: File) => Promise<JsonApiResource<MediaObjectAttributes>>;
  confirm: (id: string) => Promise<JsonApiResource<MediaObjectAttributes>>;
}

/**
 * Orchestrates the three-step upload sequence:
 *  1. Init — creates the media row in the uploaded state.
 *  2. PUT the bytes to /api/media/{id}/content (proxied to MinIO by the service).
 *  3. Confirm — transitions the row from uploaded → processing.
 *
 * After confirm, the caller should poll GET /media/{id} until status === 'ready'.
 *
 * Oversized files are rejected here, before step 1, so nothing reaches the
 * network and no orphaned media row is created.
 */
export async function performMediaUpload(
  file: File,
  deps: UploadDeps,
): Promise<JsonApiResource<MediaObjectAttributes>> {
  if (file.size > MEDIA_MAX_UPLOAD_BYTES) {
    throw new ApiError(
      413,
      MEDIA_TOO_LARGE_CODE,
      `"${file.name}" is ${formatUploadSize(file.size)}. The maximum upload size is ${formatUploadSize(MEDIA_MAX_UPLOAD_BYTES)}.`,
    );
  }

  const media = await deps.initUpload({
    contentType: file.type,
    originalFilename: file.name,
  });

  await deps.putContent(media.id, file);

  return deps.confirm(media.id);
}

// ---------------------------------------------------------------------------
// Queries
// ---------------------------------------------------------------------------

/** GET /api/media/{id} — poll for status. */
export function useMediaObject(id: string | null | undefined) {
  return useQuery({
    queryKey: mediaKeys.detail(id ?? ''),
    queryFn: () => mediaService.get(id as string),
    enabled: !!id,
    staleTime: 30 * 1000,
    gcTime: 5 * 60 * 1000,
  });
}

/**
 * GET /api/media/{id}/content — fetches the bytes and exposes them as an object
 * URL suitable for an <img src>. A plain src cannot be used: the API needs an
 * Authorization header, which the browser does not send for image requests.
 *
 * Create and revoke are a matched pair inside a single effect keyed on
 * `data`: React StrictMode's dev-only mount→cleanup→mount replay does
 * create → revoke → create and leaks nothing, with no module-level cache,
 * refcounting, or timers required. The tradeoff is deliberate: the URL is
 * NOT available on the same render `data` arrives (the effect only runs
 * after that render commits), so this hook keeps reporting `isLoading` for
 * that one frame instead. That's what lets `MediaThumbnail` hold its
 * skeleton rather than flash the "No image" placeholder — the user-visible
 * defect this hook exists to avoid — at the cost of one extra skeleton
 * frame whenever the blob changes (id switch or refetch), which is accepted.
 */
export function useMediaContentUrl(id: string | null | undefined) {
  const { data, isLoading, isError, error } = useQuery({
    queryKey: mediaKeys.content(id ?? ''),
    queryFn: () => mediaService.getContentBlob(id as string),
    enabled: !!id,
    staleTime: 5 * 60 * 1000,
    gcTime: 6 * 60 * 1000,
  });

  const [entry, setEntry] = useState<{ blob: Blob; url: string } | null>(null);

  useEffect(() => {
    if (!data) {
      setEntry(null);
      return;
    }
    const objectUrl = URL.createObjectURL(data);
    setEntry({ blob: data, url: objectUrl });
    return () => URL.revokeObjectURL(objectUrl);
  }, [data]);

  // `entry` is one render behind `data` right after a change (state set in
  // the effect hasn't committed yet); only trust it once it matches the blob
  // currently in hand, so a stale (possibly already-revoked) URL is never
  // handed back.
  const url = entry && entry.blob === data ? entry.url : null;

  // `isError`/`error` are passed straight through from the query so callers can
  // tell a real failure (403/404/5xx) from "this media has no bytes"; without
  // them both render identically. Note `isLoading` stays true for the one frame
  // where `data` has arrived but the object URL has not been created yet.
  return { url, isLoading: isLoading || (!!data && url === null), isError, error };
}

/** GET /api/fleet/vehicles/{vehicleId}/media — list media refs for a vehicle. */
export function useVehicleMedia(vehicleId: string | null | undefined) {
  return useQuery({
    queryKey: mediaKeys.vehicleMedia(vehicleId ?? ''),
    queryFn: () => vehicleMediaService.listByVehicle(vehicleId as string),
    enabled: !!vehicleId,
    staleTime: 60 * 1000,
    gcTime: 5 * 60 * 1000,
  });
}

// ---------------------------------------------------------------------------
// Mutations
// ---------------------------------------------------------------------------

export interface MediaUploadOptions {
  /**
   * Query keys to invalidate once the upload settles. Empty by default: a
   * receipt attached to a maintenance record has no gallery to refresh, and the
   * previous hard-coded vehicle-media invalidation was exactly what made the
   * old hook unusable for one.
   */
  invalidateKeys?: ReadonlyArray<readonly unknown[]>;
}

/**
 * Full upload flow: init → putContent → confirm. The mutation variable is a
 * File; the result is the confirmed media resource.
 *
 * Documents come back `ready` from confirm; images come back `processing` and
 * finish asynchronously. Callers do NOT need to wait for `ready` — the server
 * validates attachment ownership, not readiness (design D8).
 */
export function useMediaUpload(options: MediaUploadOptions = {}) {
  const queryClient = useQueryClient();
  const { invalidateKeys } = options;
  return useMutation({
    mutationFn: (file: File) =>
      performMediaUpload(file, {
        initUpload: (attrs) => mediaService.initUpload(attrs),
        putContent: (id, f) => mediaService.putContent(id, f),
        confirm: (id) => mediaService.confirm(id),
      }),
    onSettled: () => {
      for (const key of invalidateKeys ?? []) {
        void queryClient.invalidateQueries({ queryKey: key });
      }
    },
  });
}

/** Upload a file and refresh a vehicle's media gallery. */
export function useUploadMedia(vehicleId: string) {
  return useMediaUpload({ invalidateKeys: [mediaKeys.vehicleMedia(vehicleId)] });
}

/**
 * Attach an already-uploaded (ready) media object to a vehicle.
 * Invalidates the vehicle's media list.
 */
export function useAddVehicleMedia(vehicleId: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (mediaId: string) => vehicleMediaService.addMedia(vehicleId, { mediaId }),
    onSettled: () => {
      void queryClient.invalidateQueries({
        queryKey: mediaKeys.vehicleMedia(vehicleId),
      });
    },
  });
}

/**
 * PUT /api/fleet/vehicles/{vehicleId}/primary-image.
 * Invalidates both the vehicle-media list and the vehicle detail.
 */
export function useSetPrimaryImage(vehicleId: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (mediaId: string) => vehicleMediaService.setPrimaryImage(vehicleId, mediaId),
    onSettled: () => {
      void queryClient.invalidateQueries({
        queryKey: mediaKeys.vehicleMedia(vehicleId),
      });
      // Also invalidate vehicle detail so primaryImageMediaId is refreshed
      void queryClient.invalidateQueries({ queryKey: ['vehicles', 'detail', vehicleId] });
    },
  });
}

/** DELETE /api/media/{id} — soft delete. Invalidates vehicle media list. */
export function useDeleteMedia(vehicleId: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (mediaId: string) => mediaService.remove(mediaId),
    onSettled: () => {
      void queryClient.invalidateQueries({
        queryKey: mediaKeys.vehicleMedia(vehicleId),
      });
    },
  });
}

/**
 * DELETE /api/media/{id} — soft delete, with no gallery coupling. Used to clean
 * up an attachment that was uploaded but never attached to a saved record
 * (PRD FR-DOC-2/FR-DOC-3). Best-effort: the 5-day purge_after sweep is the
 * authoritative backstop.
 */
export function useDeleteMediaObject() {
  return useMutation({
    mutationFn: (mediaId: string) => mediaService.remove(mediaId),
  });
}
