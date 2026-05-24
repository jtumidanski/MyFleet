import { useState } from 'react';
import { useAuth } from '../context/AuthContext';
import { useFleetActivity } from '../lib/hooks/api/activity';
import { ActivityFeed } from '../components/features/activity/ActivityFeed';

const PAGE_SIZE = 25;

/**
 * Fleet-level activity feed page.
 * Fetches the current fleet's activity feed and renders a paginated timeline.
 */
export function ActivityPage() {
  const { activeFleetId } = useAuth();
  const [page, setPage] = useState(1);

  const { data, isLoading } = useFleetActivity(activeFleetId, page, PAGE_SIZE);

  const events = data?.data ?? [];
  const totalPages = data?.totalPages ?? 1;
  const hasNextPage = data?.hasNextPage ?? false;

  return (
    <div className="space-y-6">
      <h1 className="text-2xl font-semibold">Activity</h1>
      <ActivityFeed
        events={events}
        isLoading={isLoading}
        page={page}
        totalPages={totalPages}
        hasNextPage={hasNextPage}
        onPrev={() => setPage((p) => Math.max(1, p - 1))}
        onNext={() => setPage((p) => p + 1)}
        title="Fleet Activity"
      />
    </div>
  );
}
