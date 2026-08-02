import { useMemo } from 'react';
import { Skeleton } from '../../ui/skeleton';
import { Button } from '../../ui/button';
import { Card, CardContent } from '../../ui/card';
import { ActivityEventIcon } from './ActivityEventIcon';
import {
  collectActivityUserIds,
  describeActivityEvent,
  getActivityEventLabel,
  SYSTEM_ACTOR,
} from './activityEventMeta';
import { useUsers } from '../../../lib/hooks/api/users';
import { displayFor } from '../../../lib/utils/displayName';
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
  /**
   * Vehicle id → display label, for the feed's vehicle line. Optional because
   * a vehicle-scoped timeline already knows what vehicle it is showing; without
   * it the line falls back to the name frozen in the event payload.
   */
  vehicleNames?: Record<string, string>;
}

/**
 * Paginated activity feed — renders a timeline of activity events with per-
 * event-type icons, the actor who caused each one, and the payload fields worth
 * showing.
 *
 * User names are resolved here through their own query rather than being passed
 * in, mirroring MemberList: "events loaded, names failed" then stays a
 * renderable state and the feed degrades to shortened ids instead of blanking.
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
  vehicleNames,
}: ActivityFeedProps) {
  const userIds = useMemo(() => collectActivityUserIds(events), [events]);
  const { data: users } = useUsers(userIds);

  const resolveUser = (userId: string) =>
    userId === SYSTEM_ACTOR ? 'MyFleet' : displayFor(userId, users);
  const resolveVehicle = (vehicleId: string) => vehicleNames?.[vehicleId];

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
            {events.map((event) => {
              const details = describeActivityEvent(event, { resolveUser, resolveVehicle });
              const actor = resolveUser(event.attributes.actorUserId);
              return (
                <li key={event.id} className="mb-6 ml-6">
                  <span className="absolute -left-4 flex items-center justify-center">
                    <ActivityEventIcon type={event.attributes.type} />
                  </span>
                  <div className="rounded-md border border-border bg-card p-3 shadow-sm">
                    <p className="text-sm font-medium text-foreground">
                      {getActivityEventLabel(event.attributes.type)}
                    </p>
                    <p className="text-xs text-muted-foreground">
                      {actor} ·{' '}
                      <time dateTime={event.attributes.createdAt}>
                        {new Date(event.attributes.createdAt).toLocaleString()}
                      </time>
                    </p>
                    {details.length > 0 && (
                      <dl className="mt-2 grid grid-cols-[auto,1fr] gap-x-3 gap-y-1 text-xs">
                        {details.map((detail) => (
                          <div key={detail.term} className="contents">
                            <dt className="text-muted-foreground">{detail.term}</dt>
                            <dd className="break-words text-foreground">{detail.value}</dd>
                          </div>
                        ))}
                      </dl>
                    )}
                  </div>
                </li>
              );
            })}
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
