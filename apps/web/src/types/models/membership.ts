/**
 * Membership domain models.
 * Mirrors apps/fleet-service/internal/membership/rest.go.
 */
import type { JsonApiResource } from '@myfleet/shared-ts';

export interface MembershipAttributes {
  fleetId: string;
  userId: string;
  role: string;
  status: string;
}

export type Membership = JsonApiResource<MembershipAttributes>;
