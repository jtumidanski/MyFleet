import type { JsonApiDocument, JsonApiResource, PageMeta } from '@myfleet/shared-ts';
import { apiClient } from '../../lib/api/client';
import type { ActivityEventAttributes } from '../../types/models/activity';

/**
 * Activity service — wraps the fleet-service activity endpoints.
 *
 * Routes (apps/fleet-service/internal/activity/resource.go, gateway-prefixed):
 *   GET  /api/fleet/fleets/{id}/activity   — fleet activity feed (paged, newest first)
 *   GET  /api/fleet/vehicles/{id}/activity — per-vehicle timeline (paged, newest first)
 *
 * Both routes accept page[number] and page[size] query params and return a
 * JSON:API document with a meta.totalPages / meta.total envelope.
 */
class ActivityService {
  /**
   * GET /api/fleet/fleets/{fleetId}/activity
   */
  async listByFleet(
    fleetId: string,
    params?: { page?: number; pageSize?: number },
  ): Promise<{ data: Array<JsonApiResource<ActivityEventAttributes>>; meta?: PageMeta }> {
    const url = new URL(`/api/fleet/fleets/${fleetId}/activity`, 'http://localhost');
    if (params?.page != null) url.searchParams.set('page[number]', String(params.page));
    if (params?.pageSize != null) url.searchParams.set('page[size]', String(params.pageSize));

    const doc = await apiClient.request<
      JsonApiDocument<Array<JsonApiResource<ActivityEventAttributes>>>
    >(`/api/fleet/fleets/${fleetId}/activity${url.search}`);
    return { data: doc.data, meta: doc.meta };
  }

  /**
   * GET /api/fleet/vehicles/{vehicleId}/activity
   */
  async listByVehicle(
    vehicleId: string,
    params?: { page?: number; pageSize?: number },
  ): Promise<{ data: Array<JsonApiResource<ActivityEventAttributes>>; meta?: PageMeta }> {
    const url = new URL(`/api/fleet/vehicles/${vehicleId}/activity`, 'http://localhost');
    if (params?.page != null) url.searchParams.set('page[number]', String(params.page));
    if (params?.pageSize != null) url.searchParams.set('page[size]', String(params.pageSize));

    const doc = await apiClient.request<
      JsonApiDocument<Array<JsonApiResource<ActivityEventAttributes>>>
    >(`/api/fleet/vehicles/${vehicleId}/activity${url.search}`);
    return { data: doc.data, meta: doc.meta };
  }
}

export const activityService = new ActivityService();
