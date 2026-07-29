import { Skeleton } from '../../../ui/skeleton';
import { Card, CardContent } from '../../../ui/card';
import { SeverityChip } from './SeverityChip';
import {
  useUpcomingMaintenanceQueue,
  useOverdueMaintenanceQueue,
} from '../../../../lib/hooks/api/maintenance';

interface MaintenanceQueueViewProps {
  fleetId: string;
}

/**
 * Shows upcoming and overdue maintenance queue for a fleet.
 * Uses live-computed severity from the backend.
 */
export function MaintenanceQueueView({ fleetId }: MaintenanceQueueViewProps) {
  const { data: upcoming, isLoading: upcomingLoading } = useUpcomingMaintenanceQueue(fleetId);
  const { data: overdue, isLoading: overdueLoading } = useOverdueMaintenanceQueue(fleetId);

  const isLoading = upcomingLoading || overdueLoading;

  if (isLoading) {
    return (
      <div className="space-y-4">
        <Skeleton className="h-8 w-48" />
        <Skeleton className="h-24 w-full" />
        <Skeleton className="h-24 w-full" />
      </div>
    );
  }

  return (
    <div className="space-y-6">
      {/* Overdue — intentional status colors: red indicates urgency */}
      <Card>
        <CardContent className="pt-6">
          <h2 className="mb-4 text-base font-semibold text-red-700">Overdue Maintenance</h2>
          {!overdue || overdue.length === 0 ? (
            <p className="text-sm text-muted-foreground">No overdue maintenance items.</p>
          ) : (
            <div className="space-y-3">
              {overdue.map((item) => (
                <div
                  key={item.id}
                  className="flex items-center justify-between rounded-md border border-red-200 bg-red-50 p-3"
                >
                  <div className="min-w-0 flex-1 space-y-1">
                    <div className="flex items-center gap-2">
                      <span className="text-sm font-medium">{item.attributes.categoryId}</span>
                      <SeverityChip severity={item.attributes.severity} />
                    </div>
                    {item.attributes.nextDueDate && (
                      <p className="text-xs text-muted-foreground">
                        Was due {new Date(item.attributes.nextDueDate).toLocaleDateString()}
                      </p>
                    )}
                    {item.attributes.nextDueMileage ? (
                      <p className="text-xs text-muted-foreground">
                        At {item.attributes.nextDueMileage.toLocaleString()} miles
                      </p>
                    ) : null}
                  </div>
                </div>
              ))}
            </div>
          )}
        </CardContent>
      </Card>

      {/* Upcoming — intentional status color: amber indicates upcoming attention needed */}
      <Card>
        <CardContent className="pt-6">
          <h2 className="mb-4 text-base font-semibold text-amber-700">Upcoming Maintenance</h2>
          {!upcoming || upcoming.length === 0 ? (
            <p className="text-sm text-muted-foreground">No upcoming maintenance items.</p>
          ) : (
            <div className="space-y-3">
              {upcoming.map((item) => (
                <div
                  key={item.id}
                  className="flex items-center justify-between rounded-md border p-3"
                >
                  <div className="min-w-0 flex-1 space-y-1">
                    <div className="flex items-center gap-2">
                      <span className="text-sm font-medium">{item.attributes.categoryId}</span>
                      <SeverityChip severity={item.attributes.severity} />
                    </div>
                    {item.attributes.nextDueDate && (
                      <p className="text-xs text-muted-foreground">
                        Due {new Date(item.attributes.nextDueDate).toLocaleDateString()}
                      </p>
                    )}
                    {item.attributes.nextDueMileage ? (
                      <p className="text-xs text-muted-foreground">
                        At {item.attributes.nextDueMileage.toLocaleString()} miles
                      </p>
                    ) : null}
                  </div>
                </div>
              ))}
            </div>
          )}
        </CardContent>
      </Card>
    </div>
  );
}
