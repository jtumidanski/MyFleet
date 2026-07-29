import { Skeleton } from '../../../ui/skeleton';
import { Card, CardContent } from '../../../ui/card';
import { SeverityChip } from '../../vehicles/maintenance/SeverityChip';
import { useUpcomingMaintenanceQueue } from '../../../../lib/hooks/api/maintenance';

interface UpcomingMaintenanceWidgetProps {
  fleetId: string;
}

export function UpcomingMaintenanceWidget({ fleetId }: UpcomingMaintenanceWidgetProps) {
  const { data: items, isLoading } = useUpcomingMaintenanceQueue(fleetId);

  if (isLoading) {
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
        <h3 className="text-sm font-semibold mb-3 text-amber-700">Upcoming Maintenance</h3>
        {!items || items.length === 0 ? (
          <p className="text-sm text-muted-foreground">No upcoming maintenance.</p>
        ) : (
          <ul className="space-y-2">
            {items.slice(0, 5).map((item) => (
              <li
                key={item.id}
                className="flex items-center justify-between text-sm border-b pb-2 last:border-0"
              >
                <span className="font-medium">{item.attributes.categoryId}</span>
                <SeverityChip severity={item.attributes.severity} />
              </li>
            ))}
          </ul>
        )}
      </CardContent>
    </Card>
  );
}
