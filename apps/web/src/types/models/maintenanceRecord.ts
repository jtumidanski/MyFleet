import type { JsonApiResource } from '@myfleet/shared-ts';

/**
 * Mirrors apps/fleet-service/internal/maintenancerecord/rest.go Attributes.
 */
export interface MaintenanceRecordAttributes {
  vehicleId: string;
  categoryId: string;
  /** RFC3339 */
  performedAt: string;
  mileage: number;
  cost: number;
  vendor?: string;
  notes?: string;
  createdByUserId?: string;
  /** RFC3339 */
  createdAt: string;
  documentMediaIds?: string[];
}

export type MaintenanceRecord = JsonApiResource<MaintenanceRecordAttributes>;

/** POST /api/fleet/vehicles/{id}/maintenance-records body attributes */
export interface CreateMaintenanceRecordAttributes {
  categoryId: string;
  /** RFC3339 */
  performedAt: string;
  mileage: number;
  cost: number;
  vendor?: string;
  notes?: string;
  documentMediaIds?: string[];
}

/** PATCH /api/fleet/maintenance-records/{id} body attributes */
export interface UpdateMaintenanceRecordAttributes {
  /** RFC3339 */
  performedAt?: string;
  mileage?: number;
  cost?: number;
  vendor?: string;
  notes?: string;
}
