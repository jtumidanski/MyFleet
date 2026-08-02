/**
 * Invite domain models.
 * Mirrors apps/fleet-service/internal/invite/rest.go.
 */
import type { JsonApiResource } from '@myfleet/shared-ts';

export interface InviteAttributes {
  fleetId: string;
  /**
   * Present only on `GET /invites/pending`. The recipient holds no membership
   * anywhere yet, so they cannot read `/fleets/{id}` to resolve the id — the
   * server sends the name with the invite or they are asked to join a uuid.
   */
  fleetName?: string;
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
