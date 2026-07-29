export type VehicleStatus = 'Healthy' | 'Upcoming Maintenance' | 'Overdue' | 'Inactive';

const VARIANT: Record<VehicleStatus, string> = {
  Healthy: 'bg-green-100 text-green-800',
  'Upcoming Maintenance': 'bg-amber-100 text-amber-800',
  Overdue: 'bg-red-100 text-red-800',
  Inactive: 'bg-gray-100 text-gray-700',
};

export function StatusBadge({ status }: { status: VehicleStatus }) {
  return (
    <span className={`inline-flex rounded px-2 py-0.5 text-xs font-medium ${VARIANT[status]}`}>
      {status}
    </span>
  );
}
