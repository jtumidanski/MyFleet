import type { JsonApiDocument, JsonApiResource } from '@myfleet/shared-ts';
import { apiClient } from '../../lib/api/client';
import { BaseService, type ListResult } from './BaseService';
import type {
  CreateVehicleAttributes,
  UpdateVehicleAttributes,
  Vehicle,
  VehicleAttributes,
} from '../../types/models/vehicle';

/**
 * Vehicles service (design §12, canonical feature). Backend routes
 * (apps/fleet-service/internal/vehicle/resource.go, gateway-prefixed):
 *   GET    /api/fleet/fleets/{fleetId}/vehicles   (list — fleet-scoped)
 *   POST   /api/fleet/fleets/{fleetId}/vehicles   (create — fleet-scoped)
 *   GET    /api/fleet/vehicles/{id}
 *   PATCH  /api/fleet/vehicles/{id}
 *   DELETE /api/fleet/vehicles/{id}               (soft-delete)
 *   POST   /api/fleet/vehicles/{id}/restore       (owner-only)
 */
class VehicleService extends BaseService<
  VehicleAttributes,
  CreateVehicleAttributes,
  UpdateVehicleAttributes
> {
  protected readonly resourceType = 'vehicles';
  protected readonly basePath = '/api/fleet/vehicles';

  // List and create are nested under the fleet, so override to the fleet path.
  listByFleet(fleetId: string): Promise<ListResult<VehicleAttributes>> {
    return this.listAt(`/api/fleet/fleets/${fleetId}/vehicles`);
  }

  createInFleet(fleetId: string, attributes: CreateVehicleAttributes): Promise<Vehicle> {
    return this.createAt(`/api/fleet/fleets/${fleetId}/vehicles`, attributes);
  }

  // POST /vehicles/{id}/restore — JSON:API envelope with empty attributes
  // (the route runs through RegisterInputHandler and parses the body first).
  async restore(id: string): Promise<Vehicle> {
    const doc = await apiClient.request<JsonApiDocument<JsonApiResource<VehicleAttributes>>>(
      `${this.basePath}/${id}/restore`,
      {
        method: 'POST',
        body: JSON.stringify({ data: { type: this.resourceType, id, attributes: {} } }),
      },
    );
    return doc.data;
  }
}

export const vehicleService = new VehicleService();
