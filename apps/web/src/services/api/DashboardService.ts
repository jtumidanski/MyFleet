/**
 * DashboardService — layout persistence + aggregation endpoints.
 *
 * Backend routes (apps/fleet-service/internal/dashboard/resource.go, gateway-prefixed):
 *   GET /api/fleet/fleets/{id}/dashboard
 *   PUT /api/fleet/fleets/{id}/dashboard        (replace layout)
 *   GET /api/fleet/fleets/{id}/dashboard/spend-by-vehicle?from=&to=
 *   GET /api/fleet/fleets/{id}/dashboard/overview
 *   GET /api/fleet/vehicles/{id}/dashboard/mileage-trends?from=&to=
 */
import type { JsonApiDocument, JsonApiResource } from '@myfleet/shared-ts';
import { apiClient } from '../../lib/api/client';
import { BaseService } from './BaseService';
import type {
  Dashboard,
  DashboardAttributes,
  OverviewCounts,
  SpendRow,
  MileagePoint,
  WidgetInput,
} from '../../types/models/dashboard';

export interface DateRange {
  from?: string;
  to?: string;
}

class DashboardService extends BaseService<DashboardAttributes> {
  protected readonly resourceType = 'dashboards';
  protected readonly basePath = '/api/fleet/dashboards'; // not used directly — all routes are nested

  /**
   * GET /api/fleet/fleets/{fleetId}/dashboard
   * Returns the current user's widget layout for the fleet.
   */
  async getLayout(fleetId: string): Promise<Dashboard> {
    const doc = await apiClient.request<JsonApiDocument<JsonApiResource<DashboardAttributes>>>(
      `/api/fleet/fleets/${fleetId}/dashboard`,
    );
    return doc.data;
  }

  /**
   * PUT /api/fleet/fleets/{fleetId}/dashboard
   * Replaces the widget layout. Body is a JSON:API envelope with widgets array.
   *
   * Backend WidgetInput (apps/fleet-service/internal/dashboard/processor.go):
   *   { type, positionX, positionY, width, height, config? }
   *
   * The handler uses RegisterInputHandler so it parses the JSON:API envelope.
   * The struct it expects has field `widgets []WidgetInput`, meaning the
   * JSON:API attributes block is `{ "widgets": [...] }`.
   */
  async saveLayout(fleetId: string, widgets: WidgetInput[]): Promise<Dashboard> {
    const doc = await apiClient.request<JsonApiDocument<JsonApiResource<DashboardAttributes>>>(
      `/api/fleet/fleets/${fleetId}/dashboard`,
      {
        method: 'PUT',
        body: JSON.stringify({
          data: {
            type: 'dashboards',
            attributes: { widgets },
          },
        }),
      },
    );
    return doc.data;
  }

  /**
   * GET /api/fleet/fleets/{fleetId}/dashboard/overview
   */
  async getOverview(fleetId: string): Promise<OverviewCounts> {
    const doc = await apiClient.request<JsonApiDocument<OverviewCounts>>(
      `/api/fleet/fleets/${fleetId}/dashboard/overview`,
    );
    return doc.data;
  }

  /**
   * GET /api/fleet/fleets/{fleetId}/dashboard/spend-by-vehicle?from=&to=
   */
  async getSpendByVehicle(fleetId: string, range?: DateRange): Promise<SpendRow[]> {
    const params = buildRangeParams(range);
    const url = `/api/fleet/fleets/${fleetId}/dashboard/spend-by-vehicle${params}`;
    const doc = await apiClient.request<JsonApiDocument<SpendRow[]>>(url);
    return doc.data;
  }

  /**
   * GET /api/fleet/vehicles/{vehicleId}/dashboard/mileage-trends?from=&to=
   */
  async getMileageTrends(vehicleId: string, range?: DateRange): Promise<MileagePoint[]> {
    const params = buildRangeParams(range);
    const url = `/api/fleet/vehicles/${vehicleId}/dashboard/mileage-trends${params}`;
    const doc = await apiClient.request<JsonApiDocument<MileagePoint[]>>(url);
    return doc.data;
  }
}

function buildRangeParams(range?: DateRange): string {
  if (!range) return '';
  const parts: string[] = [];
  if (range.from) parts.push(`from=${encodeURIComponent(range.from)}`);
  if (range.to) parts.push(`to=${encodeURIComponent(range.to)}`);
  return parts.length > 0 ? `?${parts.join('&')}` : '';
}

export const dashboardService = new DashboardService();
