import { Skeleton } from '../../ui/skeleton';
import { Button } from '../../ui/button';
import { Card, CardContent } from '../../ui/card';
import { ActivityEventIcon } from './ActivityEventIcon';
import { getActivityEventLabel } from './activityEventMeta';
import type { ActivityEvent } from '../../../types/models/activity';

interface ActivityFeedProps {
  events: ActivityEvent[];
  isLoading: boolean;
  /** Current page number (1-based). */
  page: number;
  totalPages: number;
  hasNextPage: boolean;
  onPrev: () => void;
  onNext: () => void;
  /** Optional heading shown above the feed. */
  title?: string;
}

/**
 * Paginated activity feed — renders a timeline of activity events with per-
 * event-type icons. Read-only; pagination controls are provided via callbacks.
 */
export function ActivityFeed({
  events,
  isLoading,
  page,
  totalPages,
  hasNextPage,
  onPrev,
  onNext,
  title = 'Activity',
}: ActivityFeedProps) {
  if (isLoading) {
    return (
      <div className="space-y-3">
        <Skeleton className="h-6 w-40" />
        {Array.from({ length: 5 }).map((_, i) => (
          <Skeleton key={i} className="h-14 w-full" />
        ))}
      </div>
    );
  }

  return (
    <Card>
      <CardContent className="pt-6">
        <h2 className="mb-4 text-base font-semibold">{title}</h2>

        {events.length === 0 ? (
          <p className="rounded-lg border border-dashed border-border p-6 text-center text-sm text-muted-foreground">
            No activity yet.
          </p>
        ) : (
          <ol className="relative border-l border-border">
            {events.map((event) => (
              <li key={event.id} className="mb-6 ml-6">
                <span className="absolute -left-4 flex items-center justify-center">
                  <ActivityEventIcon type={event.attributes.type} />
                </span>
                <div className="rounded-md border border-border bg-card p-3 shadow-sm">
                  <p className="text-sm font-medium text-foreground">
                    {getActivityEventLabel(event.attributes.type)}
                  </p>
                  <time className="text-xs text-muted-foreground">
                    {new Date(event.attributes.createdAt).toLocaleString()}
                  </time>
                  {event.attributes.vehicleId && (
                    <p className="mt-1 text-xs text-muted-foreground">
                      Vehicle: {event.attributes.vehicleId}
                    </p>
                  )}
                </div>
              </li>
            ))}
          </ol>
        )}

        {totalPages > 1 && (
          <div className="mt-4 flex items-center justify-between text-sm text-muted-foreground">
            <Button variant="outline" size="sm" onClick={onPrev} disabled={page <= 1}>
              Previous
            </Button>
            <span>
              Page {page} of {totalPages}
            </span>
            <Button variant="outline" size="sm" onClick={onNext} disabled={!hasNextPage}>
              Next
            </Button>
          </div>
        )}
      </CardContent>
    </Card>
  );
}
