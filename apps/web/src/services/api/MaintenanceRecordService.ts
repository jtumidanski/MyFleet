import type { JsonApiDocument, JsonApiResource } from '@myfleet/shared-ts';
import { apiClient } from '../../lib/api/client';
import type {
  MaintenanceRecordAttributes,
  CreateMaintenanceRecordAttributes,
  UpdateMaintenanceRecordAttributes,
} from '../../types/models/maintenanceRecord';
import { BaseService } from './BaseService';

/**
 * Maintenance Record service.
 *
 * Routes (apps/fleet-service/internal/maintenancerecord/resource.go, gateway-prefixed):
 *   GET    /api/fleet/vehicles/{id}/maintenance-records  — list (paged, newest first)
 *   POST   /api/fleet/vehicles/{id}/maintenance-records  — log a record
 *   GET    /api/fleet/maintenance-records/{id}           — get single
 *   PATCH  /api/fleet/maintenance-records/{id}           — partial update
 *   DELETE /api/fleet/maintenance-records/{id}           — soft delete
 */
class MaintenanceRecordService extends BaseService<
  MaintenanceRecordAttributes,
  CreateMaintenanceRecordAttributes,
  UpdateMaintenanceRecordAttributes
> {
  protected readonly resourceType = 'maintenanceRecords';
  protected readonly basePath = '/api/fleet/maintenance-records';

  /** GET /api/fleet/vehicles/{vehicleId}/maintenance-records */
  listByVehicle(vehicleId: string) {
    return this.listAt(`/api/fleet/vehicles/${vehicleId}/maintenance-records`);
  }

  /** POST /api/fleet/vehicles/{vehicleId}/maintenance-records */
  createForVehicle(vehicleId: string, attributes: CreateMaintenanceRecordAttributes) {
    return this.createAt(
      `/api/fleet/vehicles/${vehicleId}/maintenance-records`,
      attributes,
    );
  }

  /** POST /api/fleet/maintenance-records/{id} with documentMediaIds append */
  async appendDocumentMedia(id: string, mediaId: string): Promise<JsonApiResource<MaintenanceRecordAttributes>> {
    const doc = await apiClient.request<JsonApiDocument<JsonApiResource<MaintenanceRecordAttributes>>>(
      `${this.basePath}/${id}/document-media`,
      {
        method: 'POST',
        body: JSON.stringify({ data: { type: 'mediaRefs', attributes: { mediaId } } }),
      },
    );
    return doc.data;
  }
}

export const maintenanceRecordService = new MaintenanceRecordService();
