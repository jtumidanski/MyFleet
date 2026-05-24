/**
 * Dashboard domain models.
 * Mirrors apps/fleet-service/internal/dashboard/rest.go.
 */
import type { JsonApiResource } from '@myfleet/shared-ts';

// ---------------------------------------------------------------------------
// Widget
// ---------------------------------------------------------------------------

export interface WidgetAttributes {
  type: string;
  positionX: number;
  positionY: number;
  width: number;
  height: number;
  config?: Record<string, unknown>;
}

export type Widget = JsonApiResource<WidgetAttributes>;

// ---------------------------------------------------------------------------
// Dashboard
// ---------------------------------------------------------------------------

export interface WidgetResource {
  type: 'dashboardWidgets';
  id: string;
  attributes: WidgetAttributes;
}

export interface DashboardAttributes {
  fleetId: string;
  userId: string;
  widgets: WidgetResource[];
  createdAt: string;
  updatedAt: string;
}

export type Dashboard = JsonApiResource<DashboardAttributes>;

// ---------------------------------------------------------------------------
// Aggregation response shapes
// ---------------------------------------------------------------------------

export interface SpendRow {
  vehicleId: string;
  maintenanceCost: number;
  fuelCost: number;
  totalCost: number;
}

export interface MileagePoint {
  recordedAt: string;
  mileage: number;
  source: string;
}

export interface OverviewCounts {
  healthy: number;
  upcomingMaintenance: number;
  overdue: number;
  inactive: number;
  totalVehicles: number;
  upcomingSchedules: number;
  overdueSchedules: number;
}

// ---------------------------------------------------------------------------
// PUT /fleets/{id}/dashboard request body
// ---------------------------------------------------------------------------

/**
 * WidgetInput mirrors the backend WidgetInput struct
 * (apps/fleet-service/internal/dashboard/processor.go).
 *
 * The PUT body is a JSON:API envelope:
 * {
 *   data: {
 *     type: "dashboards",
 *     attributes: {
 *       widgets: WidgetInput[]
 *     }
 *   }
 * }
 */
export interface WidgetInput {
  type: string;
  positionX: number;
  positionY: number;
  width: number;
  height: number;
  config?: Record<string, unknown>;
}
