export type VehicleStatus = 'Healthy' | 'Upcoming Maintenance' | 'Overdue' | 'Inactive';

// Semantic status families from apps/web/src/index.css. This package is already
// in the web app's Tailwind content globs (apps/web/tailwind.config.ts:9), so
// the classes are picked up with no config change. Each badge shows its status
// as text, so colour is never the only signal (FR-A11Y-2).
const VARIANT: Record<VehicleStatus, string> = {
  Healthy: 'bg-success-subtle text-success-subtle-foreground',
  'Upcoming Maintenance': 'bg-warning-subtle text-warning-subtle-foreground',
  Overdue: 'bg-danger-subtle text-danger-subtle-foreground',
  Inactive: 'bg-muted text-muted-foreground',
};

export function StatusBadge({ status }: { status: VehicleStatus }) {
  return (
    <span className={`inline-flex rounded px-2 py-0.5 text-xs font-medium ${VARIANT[status]}`}>
      {status}
    </span>
  );
}
