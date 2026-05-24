/**
 * Dashboard page — customizable widget grid (Task 15.7).
 * Route: / (index)
 */
import { useAuth } from '../context/AuthContext';
import { DashboardGrid } from '../components/features/dashboard/DashboardGrid';

export function DashboardPage() {
  const { activeFleetId, role } = useAuth();
  const isOwner = role === 'owner';

  if (!activeFleetId) {
    return (
      <div className="space-y-4">
        <h1 className="text-2xl font-semibold">Dashboard</h1>
        <p className="text-sm text-muted-foreground">
          No fleet selected. Complete onboarding to get started.
        </p>
      </div>
    );
  }

  return (
    <div className="space-y-4">
      <DashboardGrid fleetId={activeFleetId} isOwner={isOwner} />
    </div>
  );
}
