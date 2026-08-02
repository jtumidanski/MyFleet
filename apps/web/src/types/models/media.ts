import type { JsonApiResource } from '@myfleet/shared-ts';

/**
 * Lifecycle states of a media object, mirroring
 * apps/media-service/internal/mediaobject/model.go.
 *
 * 'failed' is terminal: it means the bytes could not be processed and never
 * will be. Documents skip 'processing' entirely and go straight to 'ready'.
 */
export type MediaStatus = 'uploaded' | 'processing' | 'ready' | 'failed';

/**
 * Renditions served by GET /api/media/{id}/content?variant=…
 * (apps/media-service/internal/mediaobject/contentvariant.go). Omitting the
 * parameter means 'original'.
 *
 * Sizes are 320 (thumbnail), 768 (card) and 1280 (display) on the longest edge.
 * The vehicles list asks for 'card': its hero is a full-width 16:9 box, which a
 * 320px thumbnail visibly softens, while the full-size upload would cost
 * megabytes per card.
 */
export type MediaVariant = 'original' | 'thumbnail' | 'display' | 'card';

/**
 * Mirrors apps/media-service/internal/mediaobject/rest.go Attributes.
 * Bytes are fetched from /api/media/{id}/content, not from a URL in the
 * payload — see MediaService.
 */
export interface MediaObjectAttributes {
  fleetId: string;
  uploadedByUserId: string;
  bucket: string;
  objectKey: string;
  contentType?: string;
  size?: number;
  originalFilename?: string;
  status: MediaStatus;
}

export type MediaObject = JsonApiResource<MediaObjectAttributes>;

/** POST /api/media body attributes */
export interface InitMediaUploadAttributes {
  contentType: string;
  originalFilename: string;
}

/**
 * Mirrors apps/fleet-service/internal/vehiclemedia/rest.go Attributes.
 */
export interface VehicleMediaAttributes {
  vehicleId: string;
  mediaId: string;
  isPrimary: boolean;
  sortOrder: number;
}

export type VehicleMedia = JsonApiResource<VehicleMediaAttributes>;

/** POST /api/fleet/vehicles/{id}/media body attributes */
export interface AddVehicleMediaAttributes {
  mediaId: string;
  sortOrder?: number;
}
