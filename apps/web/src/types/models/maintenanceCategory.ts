import type { JsonApiResource } from '@myfleet/shared-ts';

/**
 * Mirrors apps/fleet-service/internal/maintenancecategory/rest.go Attributes.
 */
export interface MaintenanceCategoryAttributes {
  name: string;
  description?: string;
  systemDefined: boolean;
}

export type MaintenanceCategory = JsonApiResource<MaintenanceCategoryAttributes>;
