/**
 * MemberService — fleet membership endpoints.
 *
 * Backend routes (apps/fleet-service/internal/membership/resource.go, gateway-prefixed):
 *   GET    /api/fleet/fleets/{id}/members          — list members
 *   DELETE /api/fleet/fleets/{id}/members/{userId} — remove member (owner-only)
 *   PATCH  /api/fleet/fleets/{id}/members/{userId} — change a member's role (owner-only)
 *
 * Sole-owner guard returns HTTP 409 when the last owner tries to remove themselves.
 */
import type { JsonApiDocument, JsonApiResource } from '@myfleet/shared-ts';
import { apiClient } from '../../lib/api/client';
import { BaseService, type ListResult } from './BaseService';
import type { MembershipAttributes } from '../../types/models/membership';
import type { FleetRole } from '../../types/models/user';

class MemberService extends BaseService<MembershipAttributes> {
  protected readonly resourceType = 'memberships';
  protected readonly basePath = '/api/fleet/memberships'; // not used directly

  /**
   * GET /api/fleet/fleets/{fleetId}/members
   */
  async listByFleet(fleetId: string): Promise<ListResult<MembershipAttributes>> {
    return this.listAt(`/api/fleet/fleets/${fleetId}/members`);
  }

  /**
   * DELETE /api/fleet/fleets/{fleetId}/members/{userId}
   * Returns 204 on success, 409 on sole-owner guard violation.
   */
  async removeMember(fleetId: string, userId: string): Promise<void> {
    await apiClient.request<null>(`/api/fleet/fleets/${fleetId}/members/${userId}`, {
      method: 'DELETE',
    });
  }

  /**
   * PATCH /api/fleet/fleets/{fleetId}/members/{userId}
   *
   * Written out rather than routed through BaseService.patch: `basePath` is the
   * placeholder this class already documents as "not used directly" — every
   * real membership route is nested under a fleet.
   *
   * Returns 409 when the change would leave the fleet with zero owners.
   */
  async updateRole(
    fleetId: string,
    userId: string,
    role: FleetRole,
  ): Promise<JsonApiResource<MembershipAttributes>> {
    const doc = await apiClient.request<JsonApiDocument<JsonApiResource<MembershipAttributes>>>(
      `/api/fleet/fleets/${fleetId}/members/${userId}`,
      {
        method: 'PATCH',
        body: JSON.stringify({ data: { type: this.resourceType, attributes: { role } } }),
      },
    );
    return doc.data;
  }
}

export const memberService = new MemberService();
