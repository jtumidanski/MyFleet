import { useMemo, useState } from 'react';
import { useAuth } from '../context/AuthContext';
import { useFleetActivity } from '../lib/hooks/api/activity';
import { useVehicles } from '../lib/hooks/api/vehicles';
import { ActivityFeed } from '../components/features/activity/ActivityFeed';
import { PageHeader } from '../components/PageHeader';

const PAGE_SIZE = 25;

/**
 * Fleet-level activity feed page.
 * Fetches the current fleet's activity feed and renders a paginated timeline.
 */
export function ActivityPage() {
  const { activeFleetId } = useAuth();
  const [page, setPage] = useState(1);

  const { data, isLoading } = useFleetActivity(activeFleetId, page, PAGE_SIZE);
  // The fleet's vehicles are already cached by the vehicles page, so this is
  // usually free. It only exists to turn the feed's vehicle ids into names;
  // a miss degrades to the name frozen in the event payload.
  const { data: vehicles } = useVehicles(activeFleetId);

  const vehicleNames = useMemo(
    () =>
      Object.fromEntries(
        (vehicles?.data ?? []).map((v) => [
          v.id,
          v.attributes.nickname?.trim() ||
            `${v.attributes.year} ${v.attributes.make} ${v.attributes.model}`,
        ]),
      ),
    [vehicles],
  );

  const events = data?.data ?? [];
  const totalPages = data?.totalPages ?? 1;
  const hasNextPage = data?.hasNextPage ?? false;

  return (
    <div className="space-y-6">
      <PageHeader title="Activity" />
      <ActivityFeed
        events={events}
        isLoading={isLoading}
        page={page}
        totalPages={totalPages}
        hasNextPage={hasNextPage}
        onPrev={() => setPage((p) => Math.max(1, p - 1))}
        onNext={() => setPage((p) => p + 1)}
        title="Fleet Activity"
        vehicleNames={vehicleNames}
      />
    </div>
  );
}
