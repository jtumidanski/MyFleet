import { useState } from 'react';
import { useVehicleActivity } from '../../../lib/hooks/api/activity';
import { ActivityFeed } from './ActivityFeed';

const PAGE_SIZE = 10;

interface VehicleActivityTimelineProps {
  vehicleId: string;
}

/**
 * Per-vehicle activity timeline embedded in VehicleDetailPage.
 * Fetches paginated activity events scoped to the given vehicle.
 */
export function VehicleActivityTimeline({ vehicleId }: VehicleActivityTimelineProps) {
  const [page, setPage] = useState(1);

  const { data, isLoading } = useVehicleActivity(vehicleId, page, PAGE_SIZE);

  const events = data?.data ?? [];
  const totalPages = data?.totalPages ?? 1;
  const hasNextPage = data?.hasNextPage ?? false;

  return (
    <ActivityFeed
      events={events}
      isLoading={isLoading}
      page={page}
      totalPages={totalPages}
      hasNextPage={hasNextPage}
      onPrev={() => setPage((p) => Math.max(1, p - 1))}
      onNext={() => setPage((p) => p + 1)}
      title="Vehicle Timeline"
      // Every row on this page is the same vehicle; naming it per row is noise,
      // and the events that carry no frozen name would show the raw id.
      showVehicle={false}
    />
  );
}
