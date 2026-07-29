import { Skeleton } from '../../../ui/skeleton';
import { Card, CardContent } from '../../../ui/card';
import { getActivityEventLabel } from '../../activity/activityEventMeta';
import { useFleetActivity } from '../../../../lib/hooks/api/activity';

interface RecentActivityWidgetProps {
  fleetId: string;
}

export function RecentActivityWidget({ fleetId }: RecentActivityWidgetProps) {
  const { data, isLoading } = useFleetActivity(fleetId, 1, 5);

  if (isLoading) {
    return (
      <Card>
        <CardContent className="pt-4 space-y-2">
          <Skeleton className="h-5 w-36" />
          {Array.from({ length: 3 }).map((_, i) => (
            <Skeleton key={i} className="h-10 w-full" />
          ))}
        </CardContent>
      </Card>
    );
  }

  const events = data?.data ?? [];

  return (
    <Card>
      <CardContent className="pt-4">
        <h3 className="text-sm font-semibold mb-3">Recent Activity</h3>
        {events.length === 0 ? (
          <p className="text-sm text-muted-foreground">No recent activity.</p>
        ) : (
          <ul className="space-y-2">
            {events.map((event) => (
              <li key={event.id} className="text-sm border-b pb-2 last:border-0">
                <div className="font-medium">{getActivityEventLabel(event.attributes.type)}</div>
                <time className="text-xs text-muted-foreground">
                  {new Date(event.attributes.createdAt).toLocaleString()}
                </time>
              </li>
            ))}
          </ul>
        )}
      </CardContent>
    </Card>
  );
}
