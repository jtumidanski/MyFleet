/**
 * widgetCatalog.ts — pure TS (no React imports) listing the valid widget types.
 * Mirrors backend ValidCatalog (apps/fleet-service/internal/dashboard/processor.go).
 *
 * Imported by tests directly (no React component loading needed).
 */

export const WIDGET_CATALOG = [
  'fleet-overview',
  'vehicle-status',
  'upcoming-maintenance',
  'overdue-maintenance',
  'recent-activity',
  'spend-by-vehicle',
  'mileage-trends',
] as const;

export type WidgetType = (typeof WIDGET_CATALOG)[number];
