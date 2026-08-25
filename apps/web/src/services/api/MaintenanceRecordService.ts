import type { JsonApiDocument, JsonApiResource } from '@myfleet/shared-ts';
import { apiClient } from '../../lib/api/client';
import type {
  MaintenanceRecordAttributes,
  CreateMaintenanceRecordAttributes,
  UpdateMaintenanceRecordAttributes,
} from '../../types/models/maintenanceRecord';
import type { MaintenanceCategoryKind } from '../../types/models/maintenanceCategory';
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
 *   POST   /api/fleet/maintenance-records/{id}/document-media            — attach one media object
 *   DELETE /api/fleet/maintenance-records/{id}/document-media/{mediaId}  — detach one media object
 */
class MaintenanceRecordService extends BaseService<
  MaintenanceRecordAttributes,
  CreateMaintenanceRecordAttributes,
  UpdateMaintenanceRecordAttributes
> {
  protected readonly resourceType = 'maintenanceRecords';
  protected readonly basePath = '/api/fleet/maintenance-records';

  /** GET /api/fleet/vehicles/{vehicleId}/maintenance-records[?kind=…] */
  listByVehicle(
    vehicleId: string,
    kind?: MaintenanceCategoryKind,
    params?: { page?: number; pageSize?: number },
  ) {
    const search = new URLSearchParams();
    if (kind) search.set('kind', kind);
    if (params?.page != null) search.set('page[number]', String(params.page));
    if (params?.pageSize != null) search.set('page[size]', String(params.pageSize));
    const qs = search.toString();
    const path = `/api/fleet/vehicles/${vehicleId}/maintenance-records`;
    return this.listAt(qs ? `${path}?${qs}` : path);
  }

  /** POST /api/fleet/vehicles/{vehicleId}/maintenance-records */
  createForVehicle(vehicleId: string, attributes: CreateMaintenanceRecordAttributes) {
    return this.createAt(`/api/fleet/vehicles/${vehicleId}/maintenance-records`, attributes);
  }

  /** POST /api/fleet/maintenance-records/{id}/document-media */
  async appendDocumentMedia(
    id: string,
    mediaId: string,
  ): Promise<JsonApiResource<MaintenanceRecordAttributes>> {
    const doc = await apiClient.request<
      JsonApiDocument<JsonApiResource<MaintenanceRecordAttributes>>
    >(`${this.basePath}/${id}/document-media`, {
      method: 'POST',
      body: JSON.stringify({ data: { type: 'mediaRefs', attributes: { mediaId } } }),
    });
    return doc.data;
  }

  /**
   * DELETE /api/fleet/maintenance-records/{id}/document-media/{mediaId}
   *
   * Removes the REFERENCE only. The media object itself is deleted separately
   * by the caller against media-service; fleet-service has no authority to do
   * it (PRD D3).
   */
  async removeDocumentMedia(id: string, mediaId: string): Promise<void> {
    await apiClient.request<null>(`${this.basePath}/${id}/document-media/${mediaId}`, {
      method: 'DELETE',
    });
  }
}

export const maintenanceRecordService = new MaintenanceRecordService();
