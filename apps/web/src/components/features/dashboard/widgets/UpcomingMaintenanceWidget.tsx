import { Skeleton } from '../../../ui/skeleton';
import { Card, CardContent } from '../../../ui/card';
import { SeverityChip } from '../../vehicles/maintenance/SeverityChip';
import { useUpcomingMaintenanceQueue } from '../../../../lib/hooks/api/maintenance';
import {
  categoryLabel,
  useCategoryNameMap,
  useVehicleTitleMap,
  vehicleLabel,
} from '../../../../lib/hooks/api/labels';

interface UpcomingMaintenanceWidgetProps {
  fleetId: string;
}

export function UpcomingMaintenanceWidget({ fleetId }: UpcomingMaintenanceWidgetProps) {
  const { data: items, isLoading: queueLoading } = useUpcomingMaintenanceQueue(fleetId);
  const { names, isLoading: categoriesLoading } = useCategoryNameMap();
  const { titles, isLoading: vehiclesLoading } = useVehicleTitleMap(fleetId);

  // Every query the rows read, ORed. React Query v5's isLoading is
  // `isPending && isFetching`: an in-flight supporting query holds the skeleton
  // so no frame can show a UUID (FR-LOAD-1), while a failed one settles to false
  // and lets rows through with the Unknown fallbacks (FR-LOAD-3). A disabled
  // query reports isLoading false, so a null fleetId never wedges the skeleton.
  if (queueLoading || categoriesLoading || vehiclesLoading) {
    return (
      <Card>
        <CardContent className="pt-4 space-y-2">
          <Skeleton className="h-5 w-40" />
          <Skeleton className="h-12 w-full" />
          <Skeleton className="h-12 w-full" />
        </CardContent>
      </Card>
    );
  }

  return (
    <Card>
      <CardContent className="pt-4">
        <h3 className="text-sm font-semibold mb-3 text-warning">Upcoming Maintenance</h3>
        {!items || items.length === 0 ? (
          <p className="text-sm text-muted-foreground">No upcoming maintenance.</p>
        ) : (
          <ul className="space-y-2">
            {items.slice(0, 5).map((item) => (
              // min-w-0 is load-bearing: a flex child defaults to
              // min-width:auto, so without it `truncate` does nothing and a long
              // category name pushes the chip out of the card.
              <li
                key={item.id}
                className="flex items-start justify-between gap-2 text-sm border-b pb-2 last:border-0"
              >
                <div className="min-w-0 flex-1 space-y-0.5">
                  <p className="truncate font-medium">
                    {categoryLabel(names, item.attributes.categoryId)}
                  </p>
                  <p className="truncate text-xs text-muted-foreground">
                    {vehicleLabel(titles, item.attributes.vehicleId)}
                  </p>
                  {item.attributes.nextDueDate && (
                    <p className="text-xs text-muted-foreground">
                      Due {new Date(item.attributes.nextDueDate).toLocaleDateString()}
                    </p>
                  )}
                  {/* `? :` not `&&`: nextDueMileage is a Go int with omitempty,
                      and `&&` on a number renders a literal 0 into the row. */}
                  {item.attributes.nextDueMileage ? (
                    <p className="text-xs text-muted-foreground">
                      At {item.attributes.nextDueMileage.toLocaleString()} miles
                    </p>
                  ) : null}
                </div>
                <SeverityChip severity={item.attributes.severity} className="shrink-0" />
              </li>
            ))}
          </ul>
        )}
      </CardContent>
    </Card>
  );
}
