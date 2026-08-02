import type { JsonApiResource } from '@myfleet/shared-ts';

/** Discriminates repair/service work from modifications (PRD FR-KIND-1). */
export type MaintenanceCategoryKind = 'maintenance' | 'modification';

/**
 * Mirrors apps/fleet-service/internal/maintenancecategory/rest.go Attributes.
 */
export interface MaintenanceCategoryAttributes {
  name: string;
  description?: string;
  systemDefined: boolean;
  /** Always present: the column is NOT NULL, so it is never omitted or null. */
  kind: MaintenanceCategoryKind;
}

export type MaintenanceCategory = JsonApiResource<MaintenanceCategoryAttributes>;

/** Body for POST /api/fleet/maintenance-categories. Fleet comes from the JWT. */
export interface CreateMaintenanceCategoryAttributes {
  name: string;
  kind: MaintenanceCategoryKind;
}
