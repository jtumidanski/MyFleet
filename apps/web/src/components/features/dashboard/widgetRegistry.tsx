/**
 * Widget registry — maps every backend ValidCatalog type to a React component
 * and default size. (Task 15.7, design §12)
 *
 * Backend ValidCatalog (apps/fleet-service/internal/dashboard/processor.go):
 *   "fleet-overview", "vehicle-status", "upcoming-maintenance",
 *   "overdue-maintenance", "recent-activity", "spend-by-vehicle", "mileage-trends"
 *
 * REQUIRED: Every entry in WIDGET_CATALOG must have a registry entry (tested in
 * dashboard.test.ts).
 */
import type { ComponentType } from 'react';
import { FleetOverviewWidget } from './widgets/FleetOverviewWidget';
import { VehicleStatusWidget } from './widgets/VehicleStatusWidget';
import { UpcomingMaintenanceWidget } from './widgets/UpcomingMaintenanceWidget';
import { OverdueMaintenanceWidget } from './widgets/OverdueMaintenanceWidget';
import { RecentActivityWidget } from './widgets/RecentActivityWidget';
import { SpendByVehicleWidget } from './widgets/SpendByVehicleWidget';
import { MileageTrendsWidget } from './widgets/MileageTrendsWidget';

// Re-export from the pure catalog module so importers can use either file.
export { WIDGET_CATALOG, type WidgetType } from './widgetCatalog';

// ---------------------------------------------------------------------------
// Registry entry
// ---------------------------------------------------------------------------

export interface WidgetRegistryEntry {
  /** The React component that renders this widget. Receives { fleetId } prop. */
  component: ComponentType<{ fleetId: string }>;
  /** Human-readable label for the add-widget menu. */
  label: string;
  /** Default grid width in columns (used when adding a new widget). */
  defaultWidth: number;
  /** Default grid height in rows (used when adding a new widget). */
  defaultHeight: number;
}

// ---------------------------------------------------------------------------
// Registry — ONE entry per WIDGET_CATALOG entry (enforced by test)
// ---------------------------------------------------------------------------

export const widgetRegistry: Record<WidgetType, WidgetRegistryEntry> = {
  'fleet-overview': {
    component: FleetOverviewWidget,
    label: 'Fleet Overview',
    defaultWidth: 2,
    defaultHeight: 2,
  },
  'vehicle-status': {
    component: VehicleStatusWidget,
    label: 'Vehicle Status',
    defaultWidth: 2,
    defaultHeight: 3,
  },
  'upcoming-maintenance': {
    component: UpcomingMaintenanceWidget,
    label: 'Upcoming Maintenance',
    defaultWidth: 2,
    defaultHeight: 3,
  },
  'overdue-maintenance': {
    component: OverdueMaintenanceWidget,
    label: 'Overdue Maintenance',
    defaultWidth: 2,
    defaultHeight: 3,
  },
  'recent-activity': {
    component: RecentActivityWidget,
    label: 'Recent Activity',
    defaultWidth: 2,
    defaultHeight: 4,
  },
  'spend-by-vehicle': {
    component: SpendByVehicleWidget,
    label: 'Spend by Vehicle',
    defaultWidth: 2,
    defaultHeight: 3,
  },
  'mileage-trends': {
    component: MileageTrendsWidget,
    label: 'Mileage Trends',
    defaultWidth: 2,
    defaultHeight: 3,
  },
};
