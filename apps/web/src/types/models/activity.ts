import type { JsonApiResource } from '@myfleet/shared-ts';

/**
 * Mirrors apps/fleet-service/internal/activity/rest.go Attributes.
 */
export interface ActivityEventAttributes {
  fleetId: string;
  /** Omitted for fleet-level events. */
  vehicleId?: string;
  actorUserId: string;
  /** Domain event type string, e.g. 'vehicle.created', 'fuel.logged', etc. */
  type: string;
  /** Structured event payload — shape varies by type. */
  payload?: Record<string, unknown>;
  /** RFC3339 */
  createdAt: string;
}

export type ActivityEvent = JsonApiResource<ActivityEventAttributes>;
