/**
 * MemberService — fleet membership endpoints.
 *
 * Backend routes (apps/fleet-service/internal/membership/resource.go, gateway-prefixed):
 *   GET    /api/fleet/fleets/{id}/members          — list members
 *   DELETE /api/fleet/fleets/{id}/members/{userId} — remove member (owner-only)
 *
 * Sole-owner guard returns HTTP 409 when the last owner tries to remove themselves.
 */
import { apiClient } from '../../lib/api/client';
import { BaseService, type ListResult } from './BaseService';
import type { MembershipAttributes } from '../../types/models/membership';

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
}

export const memberService = new MemberService();
