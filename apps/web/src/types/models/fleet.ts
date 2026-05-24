import type { JsonApiResource } from '@myfleet/shared-ts';

// Mirrors fleet-service fleet resource (apps/fleet-service/internal/fleet/rest.go).
export interface FleetAttributes {
  name: string;
  createdByUserId: string;
}

export type Fleet = JsonApiResource<FleetAttributes>;
