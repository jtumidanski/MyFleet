import type { JsonApiDocument, JsonApiResource } from '@myfleet/shared-ts';
import { apiClient } from '../../lib/api/client';
import type {
  MaintenanceScheduleAttributes,
  CreateMaintenanceScheduleAttributes,
  UpdateMaintenanceScheduleAttributes,
  CompleteMaintenanceScheduleAttributes,
} from '../../types/models/maintenanceSchedule';
import { BaseService } from './BaseService';

/** Shape returned by POST .../complete */
export interface CompletionResult {
  maintenanceRecordId: string;
}

/**
 * Maintenance Schedule service.
 *
 * Routes (apps/fleet-service/internal/maintenanceschedule/resource.go, gateway-prefixed):
 *   GET    /api/fleet/vehicles/{id}/maintenance-schedules     — list
 *   POST   /api/fleet/vehicles/{id}/maintenance-schedules     — create
 *   GET    /api/fleet/maintenance-schedules/{id}              — get single
 *   PATCH  /api/fleet/maintenance-schedules/{id}              — partial update
 *   DELETE /api/fleet/maintenance-schedules/{id}              — delete
 *   POST   /api/fleet/maintenance-schedules/{id}/complete     — complete action
 *   GET    /api/fleet/fleets/{id}/maintenance/upcoming        — upcoming queue
 *   GET    /api/fleet/fleets/{id}/maintenance/overdue         — overdue queue
 */
class MaintenanceScheduleService extends BaseService<
  MaintenanceScheduleAttributes,
  CreateMaintenanceScheduleAttributes,
  UpdateMaintenanceScheduleAttributes
> {
  protected readonly resourceType = 'maintenanceSchedules';
  protected readonly basePath = '/api/fleet/maintenance-schedules';

  /** GET /api/fleet/vehicles/{vehicleId}/maintenance-schedules */
  listByVehicle(vehicleId: string) {
    return this.listAt(`/api/fleet/vehicles/${vehicleId}/maintenance-schedules`);
  }

  /** POST /api/fleet/vehicles/{vehicleId}/maintenance-schedules */
  createForVehicle(vehicleId: string, attributes: CreateMaintenanceScheduleAttributes) {
    return this.createAt(`/api/fleet/vehicles/${vehicleId}/maintenance-schedules`, attributes);
  }

  /** POST /api/fleet/maintenance-schedules/{id}/complete */
  async complete(
    id: string,
    attributes: CompleteMaintenanceScheduleAttributes,
  ): Promise<JsonApiResource<CompletionResult>> {
    const doc = await apiClient.request<JsonApiDocument<JsonApiResource<CompletionResult>>>(
      `${this.basePath}/${id}/complete`,
      {
        method: 'POST',
        body: JSON.stringify({
          data: { type: 'maintenanceCompletions', attributes },
        }),
      },
    );
    return doc.data;
  }

  /** GET /api/fleet/fleets/{fleetId}/maintenance/upcoming */
  listUpcoming(fleetId: string) {
    return this.listAt(`/api/fleet/fleets/${fleetId}/maintenance/upcoming`);
  }

  /** GET /api/fleet/fleets/{fleetId}/maintenance/overdue */
  listOverdue(fleetId: string) {
    return this.listAt(`/api/fleet/fleets/${fleetId}/maintenance/overdue`);
  }
}

export const maintenanceScheduleService = new MaintenanceScheduleService();
