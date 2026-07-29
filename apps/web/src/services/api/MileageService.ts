import type { JsonApiDocument, JsonApiResource, PageMeta } from '@myfleet/shared-ts';
import { apiClient } from '../../lib/api/client';
import type { MileageRecordAttributes, CreateMileageAttributes } from '../../types/models/mileage';

/**
 * Mileage service — wraps the fleet-service mileage endpoints.
 * Backend routes (apps/fleet-service/internal/mileage/resource.go, gateway-prefixed):
 *   GET  /api/fleet/vehicles/{id}/mileage  — list mileage records (paged, ?from=&to=)
 *   POST /api/fleet/vehicles/{id}/mileage  — append a manual mileage record
 */
class MileageService {
  private readonly basePath = '/api/fleet/vehicles';

  /**
   * GET /api/fleet/vehicles/{vehicleId}/mileage
   * Supports ?from= and ?to= as RFC3339 range filters on recorded_at.
   */
  async listByVehicle(
    vehicleId: string,
    params?: { from?: string; to?: string; page?: number; pageSize?: number },
  ): Promise<{ data: Array<JsonApiResource<MileageRecordAttributes>>; meta?: PageMeta }> {
    const url = new URL(`${this.basePath}/${vehicleId}/mileage`, 'http://localhost');
    if (params?.from) url.searchParams.set('from', params.from);
    if (params?.to) url.searchParams.set('to', params.to);
    if (params?.page != null) url.searchParams.set('page[number]', String(params.page));
    if (params?.pageSize != null) url.searchParams.set('page[size]', String(params.pageSize));

    const doc = await apiClient.request<
      JsonApiDocument<Array<JsonApiResource<MileageRecordAttributes>>>
    >(`${this.basePath}/${vehicleId}/mileage${url.search}`);
    return { data: doc.data, meta: doc.meta };
  }

  /** POST /api/fleet/vehicles/{vehicleId}/mileage — append a manual mileage record. */
  async create(
    vehicleId: string,
    attrs: CreateMileageAttributes,
  ): Promise<JsonApiResource<MileageRecordAttributes>> {
    const doc = await apiClient.request<JsonApiDocument<JsonApiResource<MileageRecordAttributes>>>(
      `${this.basePath}/${vehicleId}/mileage`,
      {
        method: 'POST',
        body: JSON.stringify({ data: { type: 'mileageRecords', attributes: attrs } }),
      },
    );
    return doc.data;
  }
}

export const mileageService = new MileageService();
