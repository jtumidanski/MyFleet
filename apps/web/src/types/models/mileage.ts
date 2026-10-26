import type { JsonApiResource } from '@myfleet/shared-ts';

/** Mirrors the backend ?source= filter values (lowercase). */
export type MileageSource = 'fuel' | 'maintenance' | 'manual';

/**
 * Mirrors apps/fleet-service/internal/mileage/rest.go Attributes.
 */
export interface MileageRecordAttributes {
  vehicleId: string;
  mileage: number;
  /** RFC3339 */
  recordedAt: string;
  source: MileageSource;
  sourceRefId?: string;
  createdByUserId?: string;
  /** RFC3339 */
  createdAt: string;
  flagged: boolean;
}

export type MileageRecord = JsonApiResource<MileageRecordAttributes>;

/** POST /api/fleet/vehicles/{id}/mileage body attributes */
export interface CreateMileageAttributes {
  mileage: number;
  /** RFC3339 – omit to default to server time */
  recordedAt?: string;
}
