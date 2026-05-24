import { Skeleton } from '../../../ui/skeleton';
import { Card, CardContent } from '../../../ui/card';
import { useFleetOverview } from '../../../../lib/hooks/api/dashboard';

interface FleetOverviewWidgetProps {
  fleetId: string;
}

export function FleetOverviewWidget({ fleetId }: FleetOverviewWidgetProps) {
  const { data, isLoading } = useFleetOverview(fleetId);

  if (isLoading) {
    return (
      <Card>
        <CardContent className="pt-4 space-y-2">
          <Skeleton className="h-5 w-32" />
          <Skeleton className="h-16 w-full" />
        </CardContent>
      </Card>
    );
  }

  return (
    <Card>
      <CardContent className="pt-4">
        <h3 className="text-sm font-semibold mb-3">Fleet Overview</h3>
        {data ? (
          <div className="grid grid-cols-2 gap-2 text-sm">
            <div className="rounded border p-2 text-center">
              <div className="text-2xl font-bold text-green-600">{data.healthy}</div>
              <div className="text-xs text-muted-foreground">Healthy</div>
            </div>
            <div className="rounded border p-2 text-center">
              <div className="text-2xl font-bold text-amber-600">{data.upcomingMaintenance}</div>
              <div className="text-xs text-muted-foreground">Upcoming</div>
            </div>
            <div className="rounded border p-2 text-center">
              <div className="text-2xl font-bold text-red-600">{data.overdue}</div>
              <div className="text-xs text-muted-foreground">Overdue</div>
            </div>
            <div className="rounded border p-2 text-center">
              <div className="text-2xl font-bold text-gray-400">{data.inactive}</div>
              <div className="text-xs text-muted-foreground">Inactive</div>
            </div>
          </div>
        ) : (
          <p className="text-sm text-muted-foreground">No data available.</p>
        )}
      </CardContent>
    </Card>
  );
}
