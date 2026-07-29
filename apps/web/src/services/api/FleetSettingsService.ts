/**
 * FleetSettingsService — fleet rename (PATCH /fleets/{id}).
 *
 * Backend route (apps/fleet-service/internal/fleet/resource.go):
 *   PATCH /api/fleet/fleets/{id} — owner-only rename
 */
import type { JsonApiDocument, JsonApiResource } from '@myfleet/shared-ts';
import { apiClient } from '../../lib/api/client';
import { BaseService } from './BaseService';
import type { FleetAttributes } from '../../types/models/fleet';
import type { Fleet } from '../../types/models/fleet';

export interface RenameFleetAttributes {
  name: string;
}

class FleetSettingsService extends BaseService<
  FleetAttributes,
  FleetAttributes,
  RenameFleetAttributes
> {
  protected readonly resourceType = 'fleets';
  protected readonly basePath = '/api/fleet/fleets';

  /**
   * PATCH /api/fleet/fleets/{id} — rename the fleet (owner-only).
   */
  async rename(fleetId: string, name: string): Promise<Fleet> {
    const doc = await apiClient.request<JsonApiDocument<JsonApiResource<FleetAttributes>>>(
      `${this.basePath}/${fleetId}`,
      {
        method: 'PATCH',
        body: JSON.stringify({
          data: {
            type: this.resourceType,
            id: fleetId,
            attributes: { name },
          },
        }),
      },
    );
    return doc.data;
  }

  /**
   * GET /api/fleet/fleets/{id} — fetch fleet details.
   */
  async getFleet(fleetId: string): Promise<Fleet> {
    const doc = await apiClient.request<JsonApiDocument<JsonApiResource<FleetAttributes>>>(
      `${this.basePath}/${fleetId}`,
    );
    return doc.data;
  }
}

export const fleetSettingsService = new FleetSettingsService();
