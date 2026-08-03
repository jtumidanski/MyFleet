import type {
  FuelLogAttributes,
  CreateFuelLogAttributes,
  UpdateFuelLogAttributes,
} from '../../types/models/fuelLog';
import { BaseService } from './BaseService';

/**
 * Fuel Log service.
 *
 * Routes (apps/fleet-service/internal/fuel/resource.go, gateway-prefixed):
 *   GET    /api/fleet/vehicles/{id}/fuel-logs  — list (newest first, paged)
 *   POST   /api/fleet/vehicles/{id}/fuel-logs  — log a fuel entry
 *   GET    /api/fleet/fuel-logs/{id}           — get single
 *   PATCH  /api/fleet/fuel-logs/{id}           — partial update
 *   DELETE /api/fleet/fuel-logs/{id}           — soft delete
 */
class FuelService extends BaseService<
  FuelLogAttributes,
  CreateFuelLogAttributes,
  UpdateFuelLogAttributes
> {
  protected readonly resourceType = 'fuelLogs';
  protected readonly basePath = '/api/fleet/fuel-logs';

  /** GET /api/fleet/vehicles/{vehicleId}/fuel-logs */
  listByVehicle(vehicleId: string, params?: { page?: number; pageSize?: number }) {
    const search = new URLSearchParams();
    if (params?.page != null) search.set('page[number]', String(params.page));
    if (params?.pageSize != null) search.set('page[size]', String(params.pageSize));
    const qs = search.toString();
    const path = `/api/fleet/vehicles/${vehicleId}/fuel-logs`;
    return this.listAt(qs ? `${path}?${qs}` : path);
  }

  /** POST /api/fleet/vehicles/{vehicleId}/fuel-logs */
  createForVehicle(vehicleId: string, attributes: CreateFuelLogAttributes) {
    return this.createAt(`/api/fleet/vehicles/${vehicleId}/fuel-logs`, attributes);
  }
}

export const fuelService = new FuelService();
