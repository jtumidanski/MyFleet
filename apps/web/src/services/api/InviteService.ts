/**
 * InviteService — fleet invite endpoints.
 *
 * Backend routes (apps/fleet-service/internal/invite/resource.go, gateway-prefixed):
 *   POST   /api/fleet/fleets/{id}/invites               — create invite (owner-only)
 *   GET    /api/fleet/fleets/{id}/invites               — list invites
 *   DELETE /api/fleet/invites/{id}                      — revoke invite (owner-only)
 *   POST   /api/fleet/invites/{token}/accept            — accept invite (no body needed)
 *   POST   /api/fleet/fleets/{id}/invites/{id}/resend   — resend invite (owner-only)
 */
import type { JsonApiDocument, JsonApiResource } from '@myfleet/shared-ts';
import { apiClient } from '../../lib/api/client';
import { BaseService, type ListResult } from './BaseService';
import type { InviteAttributes, Invite, CreateInviteAttributes } from '../../types/models/invite';

class InviteService extends BaseService<InviteAttributes, CreateInviteAttributes> {
  protected readonly resourceType = 'invites';
  protected readonly basePath = '/api/fleet/invites';

  /**
   * GET /api/fleet/fleets/{fleetId}/invites
   */
  async listByFleet(fleetId: string): Promise<ListResult<InviteAttributes>> {
    return this.listAt(`/api/fleet/fleets/${fleetId}/invites`);
  }

  /**
   * GET /api/fleet/invites/pending — the invites waiting for the CALLER.
   *
   * Takes no argument by design: the server scopes the result to the validated
   * token's `email` claim, so there is no identifier here for a caller to
   * tamper with.
   */
  async listPending(): Promise<ListResult<InviteAttributes>> {
    return this.listAt('/api/fleet/invites/pending');
  }

  /**
   * POST /api/fleet/fleets/{fleetId}/invites
   * attrs: { email, role }
   */
  async createInvite(fleetId: string, attrs: CreateInviteAttributes): Promise<Invite> {
    return this.createAt(`/api/fleet/fleets/${fleetId}/invites`, attrs);
  }

  /**
   * DELETE /api/fleet/invites/{id}
   */
  async revokeInvite(id: string): Promise<void> {
    return this.remove(id);
  }

  /**
   * POST /api/fleet/invites/{token}/accept
   * No body required — the backend looks up by token and validates identity.
   */
  async acceptInvite(token: string): Promise<Invite> {
    const doc = await apiClient.request<JsonApiDocument<JsonApiResource<InviteAttributes>>>(
      `/api/fleet/invites/${token}/accept`,
      { method: 'POST' },
    );
    return doc.data;
  }

  /**
   * POST /api/fleet/fleets/{fleetId}/invites/{inviteId}/resend
   * No body required. Rotates the token, so the response carries a NEW token
   * and any previously copied link is dead.
   */
  async resendInvite(fleetId: string, inviteId: string): Promise<Invite> {
    const doc = await apiClient.request<JsonApiDocument<JsonApiResource<InviteAttributes>>>(
      `/api/fleet/fleets/${fleetId}/invites/${inviteId}/resend`,
      { method: 'POST' },
    );
    return doc.data;
  }
}

export const inviteService = new InviteService();
