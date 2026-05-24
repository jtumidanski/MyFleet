/**
 * widgetManifest.ts — pure TS (no React imports) mapping widget types to metadata.
 * Tests import this to verify all catalog types have registry entries.
 *
 * widgetRegistry.tsx (React-importing) re-exports from here plus adds the component refs.
 */
import { WIDGET_CATALOG, type WidgetType } from './widgetCatalog';

export interface WidgetMeta {
  label: string;
  defaultWidth: number;
  defaultHeight: number;
}

export const widgetManifest: Record<WidgetType, WidgetMeta> = {
  'fleet-overview': { label: 'Fleet Overview', defaultWidth: 2, defaultHeight: 2 },
  'vehicle-status': { label: 'Vehicle Status', defaultWidth: 2, defaultHeight: 3 },
  'upcoming-maintenance': { label: 'Upcoming Maintenance', defaultWidth: 2, defaultHeight: 3 },
  'overdue-maintenance': { label: 'Overdue Maintenance', defaultWidth: 2, defaultHeight: 3 },
  'recent-activity': { label: 'Recent Activity', defaultWidth: 2, defaultHeight: 4 },
  'spend-by-vehicle': { label: 'Spend by Vehicle', defaultWidth: 2, defaultHeight: 3 },
  'mileage-trends': { label: 'Mileage Trends', defaultWidth: 2, defaultHeight: 3 },
};

// Compile-time safety: TypeScript will error if WIDGET_CATALOG has an entry
// not in widgetManifest, or vice versa.
const _exhaustiveCheck: Record<WidgetType, unknown> = widgetManifest;
void _exhaustiveCheck;

// Re-export catalog so tests can import from a single location.
export { WIDGET_CATALOG, type WidgetType };
