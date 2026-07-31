import type { JsonApiResource } from '@myfleet/shared-ts';

/**
 * Mirrors apps/fleet-service/internal/maintenancerecord/rest.go Attributes.
 */
export interface MaintenanceRecordAttributes {
  vehicleId: string;
  categoryId: string;
  /** RFC3339 */
  performedAt: string;
  /**
   * Short summary. Absent when empty (the server emits it with omitempty), so
   * callers fall back to the category name (PRD FR-REC-2).
   */
  description?: string;
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
  /** RFC3339. Required — the server no longer defaults it to now. */
  performedAt: string;
  description?: string;
  mileage?: number;
  cost?: number;
  vendor?: string;
  notes?: string;
  documentMediaIds?: string[];
}

/** PATCH /api/fleet/maintenance-records/{id} body attributes */
export interface UpdateMaintenanceRecordAttributes {
  /** RFC3339 */
  performedAt?: string;
  description?: string;
  mileage?: number;
  cost?: number;
  vendor?: string;
  notes?: string;
}
