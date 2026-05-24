/**
 * Invite domain models.
 * Mirrors apps/fleet-service/internal/invite/rest.go.
 */
import type { JsonApiResource } from '@myfleet/shared-ts';

export interface InviteAttributes {
  fleetId: string;
  email: string;
  role: string;
  token: string;
  expiresAt: string;
  acceptedAt?: string;
  invitedByUserId: string;
}

export type Invite = JsonApiResource<InviteAttributes>;

// POST /fleets/{id}/invites attrs
export interface CreateInviteAttributes {
  email: string;
  role: string;
}
