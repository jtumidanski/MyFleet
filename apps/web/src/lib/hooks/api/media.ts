import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { mediaService } from '../../../services/api/MediaService';
import { vehicleMediaService } from '../../../services/api/VehicleMediaService';
import type { JsonApiResource } from '@myfleet/shared-ts';
import type {
  MediaObjectAttributes,
  InitMediaUploadAttributes,
} from '../../../types/models/media';

// Hierarchical query-key factory.
// all                       -> ['media']
// detail('m1')              -> ['media', 'detail', 'm1']
// download('m1')            -> ['media', 'download', 'm1']
// vehicleMedia(vehicleId)   -> ['media', 'vehicle', vehicleId]
export const mediaKeys = {
  all: ['media'] as const,
  details: () => [...mediaKeys.all, 'detail'] as const,
  detail: (id: string) => [...mediaKeys.details(), id] as const,
  downloads: () => [...mediaKeys.all, 'download'] as const,
  download: (id: string) => [...mediaKeys.downloads(), id] as const,
  vehicleMediaAll: () => [...mediaKeys.all, 'vehicle'] as const,
  vehicleMedia: (vehicleId: string) => [...mediaKeys.vehicleMediaAll(), vehicleId] as const,
};

// ---------------------------------------------------------------------------
// Upload helper (pure function — injectable for testing)
// ---------------------------------------------------------------------------

export interface UploadDeps {
  initUpload: (attrs: InitMediaUploadAttributes) => Promise<JsonApiResource<MediaObjectAttributes>>;
  putToPresignedUrl: (url: string, file: File) => Promise<void>;
  confirm: (id: string) => Promise<JsonApiResource<MediaObjectAttributes>>;
}

/**
 * Orchestrates the three-step upload sequence:
 *  1. Init — creates the media row and returns a presigned PUT URL.
 *  2. PUT the file bytes directly to MinIO (no auth header — raw presigned URL).
 *  3. Confirm — transitions the row from uploaded → processing.
 *
 * After confirm, the caller should poll GET /media/{id} until status === 'ready'.
 */
export async function performMediaUpload(
  file: File,
  deps: UploadDeps,
): Promise<JsonApiResource<MediaObjectAttributes>> {
  // Step 1: init
  const media = await deps.initUpload({
    contentType: file.type,
    originalFilename: file.name,
  });

  const uploadUrl = media.attributes.uploadUrl;
  if (!uploadUrl) {
    throw new Error('No upload URL returned from init');
  }

  // Step 2: presigned PUT (raw MinIO — no apiClient)
  await deps.putToPresignedUrl(uploadUrl, file);

  // Step 3: confirm
  const confirmed = await deps.confirm(media.id);
  return confirmed;
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

/** GET /api/media/{id}/download — returns downloadUrl in attributes. */
export function useMediaDownloadUrl(id: string | null | undefined) {
  return useQuery({
    queryKey: mediaKeys.download(id ?? ''),
    queryFn: () => mediaService.getDownloadUrl(id as string),
    enabled: !!id,
    staleTime: 5 * 60 * 1000, // presigned URLs are short-lived; keep gcTime short
    gcTime: 6 * 60 * 1000,
  });
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
 * Full upload flow: init → presigned PUT → confirm.
 * The mutation variable is a File. Invalidates vehicle-media list on success.
 */
export function useUploadMedia(vehicleId: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (file: File) =>
      performMediaUpload(file, {
        initUpload: (attrs) => mediaService.initUpload(attrs),
        putToPresignedUrl: (url, f) => mediaService.putToPresignedUrl(url, f),
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
    mutationFn: (mediaId: string) =>
      vehicleMediaService.addMedia(vehicleId, { mediaId }),
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
    mutationFn: (mediaId: string) =>
      vehicleMediaService.setPrimaryImage(vehicleId, mediaId),
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
