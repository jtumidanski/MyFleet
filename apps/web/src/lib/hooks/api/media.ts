import { useEffect, useMemo } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { mediaService } from '../../../services/api/MediaService';
import { vehicleMediaService } from '../../../services/api/VehicleMediaService';
import type { JsonApiResource } from '@myfleet/shared-ts';
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
 */
export async function performMediaUpload(
  file: File,
  deps: UploadDeps,
): Promise<JsonApiResource<MediaObjectAttributes>> {
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

// ---------------------------------------------------------------------------
// Object URL cache for useMediaContentUrl
//
// Deriving the object URL directly in render (rather than via a post-paint
// effect) is what avoids the one-frame "no image" flash between the blob
// arriving and the <img> picking it up. But React (in development, under
// StrictMode) intentionally double-invokes the render body and any function
// passed to useMemo/useState/useReducer to surface impure code — so a naive
// `useMemo(() => URL.createObjectURL(data), [data])` can call
// `createObjectURL` twice for the same logical render, and the discarded
// invocation's URL would never be revoked (a real native leak, not just a
// wasted JS reference). Keying creation off this cache makes repeated calls
// for the same Blob idempotent: only the first call per Blob ever creates a
// URL, so a duplicate invocation just re-reads the cache.
//
// Revocation is refcounted and kept out of the (unreliable-call-count)
// render path entirely — it only happens from useEffect setup/cleanup, whose
// mount → cleanup → mount sequence (StrictMode's *other* dev-only check, for
// missing cleanup) is a matched pair and nets out correctly. The one hazard
// that pairing introduces is the interim `cleanup` call revoking a URL the
// still-rendered <img> is using, a heartbeat before the synchronous
// `remount` re-acquires it — so release defers the actual revoke to a
// macrotask, which the immediate re-`retain` cancels before it ever fires.
interface ObjectUrlCacheEntry {
  url: string;
  refCount: number;
  pendingRevoke: ReturnType<typeof setTimeout> | null;
}

const objectUrlCache = new Map<Blob, ObjectUrlCacheEntry>();

/**
 * Idempotent: returns the cached object URL for `blob`, creating one only on
 * the first call for that exact Blob instance. Safe to call more than once
 * for the same Blob (e.g. a duplicated render) — never allocates twice.
 */
function getOrCreateObjectUrl(blob: Blob): string {
  const existing = objectUrlCache.get(blob);
  if (existing) return existing.url;
  const entry: ObjectUrlCacheEntry = {
    url: URL.createObjectURL(blob),
    refCount: 0,
    pendingRevoke: null,
  };
  objectUrlCache.set(blob, entry);
  return entry.url;
}

/** Registers one more live consumer of `blob`'s URL; cancels a pending revoke. */
function retainObjectUrl(blob: Blob): void {
  const entry = objectUrlCache.get(blob);
  if (!entry) return;
  if (entry.pendingRevoke !== null) {
    clearTimeout(entry.pendingRevoke);
    entry.pendingRevoke = null;
  }
  entry.refCount += 1;
}

/**
 * Unregisters one consumer of `blob`'s URL. When the last consumer releases,
 * the revoke is scheduled for the next macrotask rather than run inline, so
 * that StrictMode's synchronous cleanup→remount can cancel it via
 * `retainObjectUrl` instead of revoking a URL still bound to the DOM.
 */
function releaseObjectUrl(blob: Blob): void {
  const entry = objectUrlCache.get(blob);
  if (!entry) return;
  entry.refCount -= 1;
  if (entry.refCount <= 0 && entry.pendingRevoke === null) {
    entry.pendingRevoke = setTimeout(() => {
      URL.revokeObjectURL(entry.url);
      objectUrlCache.delete(blob);
    }, 0);
  }
}

/**
 * GET /api/media/{id}/content — fetches the bytes and exposes them as an object
 * URL suitable for an <img src>. A plain src cannot be used: the API needs an
 * Authorization header, which the browser does not send for image requests.
 *
 * The URL is derived synchronously from `data` (via `useMemo`) so it is
 * available on the very same render the blob arrives on — no null-then-value
 * transition, so `MediaThumbnail` never flashes its "No image" placeholder
 * before the real image. Revocation is refcounted through
 * `retainObjectUrl`/`releaseObjectUrl` in an effect keyed on `data`; see the
 * comment above the cache for why that split (idempotent create in render,
 * refcounted revoke in effect) is necessary rather than a single
 * state+effect pair.
 */
export function useMediaContentUrl(id: string | null | undefined) {
  const { data, isLoading } = useQuery({
    queryKey: mediaKeys.content(id ?? ''),
    queryFn: () => mediaService.getContentBlob(id as string),
    enabled: !!id,
    staleTime: 5 * 60 * 1000,
    gcTime: 6 * 60 * 1000,
  });

  const url = useMemo(() => (data ? getOrCreateObjectUrl(data) : null), [data]);

  useEffect(() => {
    if (!data) return;
    retainObjectUrl(data);
    return () => releaseObjectUrl(data);
  }, [data]);

  return { url, isLoading };
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

/**
 * Full upload flow: init → putContent → confirm.
 * The mutation variable is a File. Invalidates vehicle-media list on success.
 */
export function useUploadMedia(vehicleId: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (file: File) =>
      performMediaUpload(file, {
        initUpload: (attrs) => mediaService.initUpload(attrs),
        putContent: (id, f) => mediaService.putContent(id, f),
        confirm: (id) => mediaService.confirm(id),
      }),
    onSettled: () => {
      void queryClient.invalidateQueries({
        queryKey: mediaKeys.vehicleMedia(vehicleId),
      });
    },
  });
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
