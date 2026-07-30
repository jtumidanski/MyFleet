import { useEffect, useState } from 'react';
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

/**
 * GET /api/media/{id}/content — fetches the bytes and exposes them as an object
 * URL suitable for an <img src>. A plain src cannot be used: the API needs an
 * Authorization header, which the browser does not send for image requests.
 */
export function useMediaContentUrl(id: string | null | undefined) {
  const { data, isLoading } = useQuery({
    queryKey: mediaKeys.content(id ?? ''),
    queryFn: () => mediaService.getContentBlob(id as string),
    enabled: !!id,
    staleTime: 5 * 60 * 1000,
    gcTime: 6 * 60 * 1000,
  });

  const [url, setUrl] = useState<string | null>(null);

  useEffect(() => {
    if (!data) {
      setUrl(null);
      return;
    }
    const objectUrl = URL.createObjectURL(data);
    setUrl(objectUrl);
    return () => URL.revokeObjectURL(objectUrl);
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
