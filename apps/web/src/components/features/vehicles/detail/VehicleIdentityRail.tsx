import { formatMileage } from '@myfleet/ui-components';
import { Card, CardContent } from '../../../ui/card';
import { Button } from '../../../ui/button';
import { VehiclePhotoThumbnail } from '../VehiclePhotoThumbnail';
import { useVehicleMedia } from '../../../../lib/hooks/api/media';
import type { Vehicle } from '../../../../types/models/vehicle';

/** How many tiles the thumbnail strip shows before collapsing the rest into "+N". */
const STRIP_SIZE = 3;

interface VehicleIdentityRailProps {
  vehicle: Vehicle;
  odometer?: number;
  canWrite: boolean;
  onEdit: () => void;
  onViewGallery: () => void;
}

/**
 * The vehicle's identity card: primary photo, key facts, a thumbnail strip
 * into the gallery, and an Edit affordance for writers. Title, status and spec
 * line sit in the page's PageHeader, not here (see VehicleDetailPage).
 */
export function VehicleIdentityRail({
  vehicle,
  odometer,
  canWrite,
  onEdit,
  onViewGallery,
}: VehicleIdentityRailProps) {
  const { attributes } = vehicle;
  const { data: mediaRefs } = useVehicleMedia(vehicle.id);

  // Title and status moved to the page's PageHeader (task-015). This is kept
  // only as the photo's alt text.
  const title =
    attributes.nickname?.trim() || `${attributes.year} ${attributes.make} ${attributes.model}`;

  // The primary photo comes from the media list's flagged resource, not
  // vehicle.attributes.primaryImageMediaId — the two are set by the same
  // mutation but this keeps the rail and the thumbnail strip reading from one
  // source instead of two that could momentarily disagree.
  const primaryRef = (mediaRefs ?? []).find((r) => r.attributes.isPrimary);

  const stripRefs = (mediaRefs ?? []).slice(0, STRIP_SIZE);
  const remaining = Math.max((mediaRefs?.length ?? 0) - STRIP_SIZE, 0);

  return (
    <Card>
      <CardContent className="space-y-4 pt-6">
        <VehiclePhotoThumbnail
          mediaId={primaryRef?.attributes.mediaId}
          vehicleLabel={title}
          className="h-40 w-full rounded-md"
        />

        <dl className="grid grid-cols-2 gap-3 text-sm">
          <div>
            <dt className="text-muted-foreground">Odometer</dt>
            <dd className="text-foreground">
              {typeof odometer === 'number' ? formatMileage(odometer) : '—'}
            </dd>
          </div>
          <div>
            <dt className="text-muted-foreground">VIN</dt>
            <dd className="text-foreground">{attributes.vin || '—'}</dd>
          </div>
          <div className="col-span-2">
            <dt className="text-muted-foreground">Notes</dt>
            <dd className="whitespace-pre-wrap text-foreground">{attributes.notes || '—'}</dd>
          </div>
        </dl>

        {stripRefs.length > 0 && (
          <div className="flex gap-2">
            {stripRefs.map((ref, index) => {
              const isLastTile = index === stripRefs.length - 1;
              return (
                <button
                  key={ref.id}
                  type="button"
                  onClick={onViewGallery}
                  aria-label={
                    isLastTile && remaining > 0
                      ? `View all photos (${remaining} more)`
                      : 'View photos'
                  }
                  className="relative h-16 w-16 shrink-0 overflow-hidden rounded-md"
                >
                  <VehiclePhotoThumbnail
                    mediaId={ref.attributes.mediaId}
                    vehicleLabel={title}
                    className="h-16 w-16 rounded-md"
                  />
                  {isLastTile && remaining > 0 && (
                    <span className="absolute inset-0 flex items-center justify-center bg-background/70 text-sm font-medium text-foreground">
                      +{remaining}
                    </span>
                  )}
                </button>
              );
            })}
          </div>
        )}

        {canWrite && (
          <Button type="button" variant="outline" className="w-full" onClick={onEdit}>
            Edit
          </Button>
        )}
      </CardContent>
    </Card>
  );
}
