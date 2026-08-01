import { Link } from 'react-router-dom';
import { ChevronRight, History } from 'lucide-react';
import { StatusBadge, formatMileage, type VehicleStatus } from '@myfleet/ui-components';
import { Button } from '../../ui/button';
import { Card } from '../../ui/card';
import { VehiclePhotoThumbnail } from './VehiclePhotoThumbnail';
import { buildCarfaxUrl } from '../../../lib/carfax';
import { useRuntimeConfig } from '../../../lib/hooks/useRuntimeConfig';
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
  // Read through the hook, not the module getter: the tree mounts before the
  // runtime config fetch resolves, so a card rendered in that window has to
  // re-render when the real template lands or a ConfigMap override would never
  // reach the user.
  const { carfaxUrlTemplate } = useRuntimeConfig();
  // null means "render no button": no VIN, a template that ignores {vin}, or a
  // template whose scheme is not https:. Nothing contacts Carfax until a click.
  const carfaxUrl = buildCarfaxUrl(carfaxUrlTemplate, attributes.vin);

  return (
    <Card className="p-4">
      <div className="flex items-start gap-4">
        <VehiclePhotoThumbnail mediaId={attributes.primaryImageMediaId} vehicleLabel={title} />
        {/* min-w-0 on BOTH flex children is what lets `truncate` work inside a
            flex row; without it the text sets a minimum content width and the
            card overflows horizontally at the single-column breakpoint. */}
        <div className="min-w-0 flex-1">
          <div className="flex items-start justify-between gap-2">
            <div className="min-w-0">
              <div className="truncate font-medium text-foreground">{title}</div>
              <div className="truncate text-sm text-muted-foreground">
                {attributes.year} {attributes.make} {attributes.model}
                {attributes.trim ? ` ${attributes.trim}` : ''}
              </div>
            </div>
            {status && <StatusBadge status={status} />}
          </div>
          {typeof attributes.currentMileage === 'number' && (
            <div className="mt-2 text-sm text-muted-foreground">
              {formatMileage(attributes.currentMileage)}
            </div>
          )}
        </div>
      </div>

      {/* Actions live in their own row so they stay clear of the status badge
          and hold the same position whether or not mileage renders. Detail
          first, Carfax second — always. */}
      <div className="mt-3 flex items-center justify-end gap-1">
        {/* asChild renders through a Radix Slot, so the element IS the router's
            <a>: middle-click, cmd/ctrl-click, and the link context menu all
            work, which an onClick handler would not preserve. Shown for every
            role including viewer — nothing here is gated on write permission. */}
        <Button asChild variant="ghost" size="icon">
          <Link to={`/vehicles/${vehicle.id}`} aria-label={`Open details for ${title}`}>
            <ChevronRight className="h-4 w-4" aria-hidden="true" />
          </Link>
        </Button>
        {carfaxUrl && (
          // A plain <a>, not a react-router <Link> — this leaves the SPA.
          // rel="noopener noreferrer" stops the opened page reaching back
          // through window.opener and suppresses the referrer, which matters
          // because the URL carries the VIN.
          <Button asChild variant="ghost" size="icon">
            <a
              href={carfaxUrl}
              target="_blank"
              rel="noopener noreferrer"
              aria-label={`View Carfax report for ${title} (opens in a new tab)`}
            >
              <History className="h-4 w-4" aria-hidden="true" />
            </a>
          </Button>
        )}
      </div>
    </Card>
  );
}
