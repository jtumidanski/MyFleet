import type { JsonApiResource } from '@myfleet/shared-ts';

/**
 * Mirrors apps/media-service/internal/mediaobject/rest.go Attributes.
 * uploadUrl is present only on the init-upload response.
 * downloadUrl is present only on the /download endpoint response.
 */
export interface MediaObjectAttributes {
  fleetId: string;
  uploadedByUserId: string;
  bucket: string;
  objectKey: string;
  contentType?: string;
  size?: number;
  originalFilename?: string;
  /** 'uploaded' | 'processing' | 'ready' | 'error' */
  status: string;
  uploadUrl?: string;
  downloadUrl?: string;
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
