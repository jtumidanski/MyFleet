import { Link } from 'react-router-dom';
import { StatusBadge, formatMileage, type VehicleStatus } from '@myfleet/ui-components';
import { Card } from '../../ui/card';
import type { Vehicle } from '../../../types/models/vehicle';

const KNOWN_STATUSES: readonly VehicleStatus[] = [
  'Healthy',
  'Upcoming Maintenance',
  'Overdue',
  'Inactive',
];

function asVehicleStatus(value: string | undefined): VehicleStatus | null {
  return value && (KNOWN_STATUSES as readonly string[]).includes(value)
    ? (value as VehicleStatus)
    : null;
}

export function VehicleCard({ vehicle }: { vehicle: Vehicle }) {
  const { attributes } = vehicle;
  const title =
    attributes.nickname?.trim() ||
    `${attributes.year} ${attributes.make} ${attributes.model}`.trim();
  const status = asVehicleStatus(attributes.status);

  return (
    <Link to={`/vehicles/${vehicle.id}`} className="block">
      <Card className="cursor-pointer p-4 transition hover:border-ring hover:shadow">
        <div className="flex items-start justify-between gap-2">
          <div>
            <div className="font-medium text-foreground">{title}</div>
            <div className="text-sm text-muted-foreground">
              {attributes.year} {attributes.make} {attributes.model}
              {attributes.trim ? ` ${attributes.trim}` : ''}
            </div>
          </div>
          {status && <StatusBadge status={status} />}
        </div>
        {typeof attributes.currentMileage === 'number' && (
          <div className="mt-3 text-sm text-muted-foreground">
            {formatMileage(attributes.currentMileage)}
          </div>
        )}
      </Card>
    </Link>
  );
}
