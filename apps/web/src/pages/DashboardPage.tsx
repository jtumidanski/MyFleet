/**
 * Dashboard page — customizable widget grid.
 * Route: / (index)
 *
 * The title is unconditional (FR-13): it used to live inside `isOwner &&` in
 * DashboardGrid, which left members and viewers on an untitled page. Only the
 * Add Widget control is owner-gated.
 */
import { useAuth } from '../context/AuthContext';
import { PageHeader } from '../components/PageHeader';
import { DashboardGrid } from '../components/features/dashboard/DashboardGrid';
import { AddWidgetMenu } from '../components/features/dashboard/AddWidgetMenu';
import { useDashboardWidgets } from '../components/features/dashboard/useDashboardWidgets';

export function DashboardPage() {
  const { activeFleetId, role } = useAuth();
  const isOwner = role === 'owner';
  const { widgets, isLoading, addWidget, removeWidget, moveUp, moveDown } = useDashboardWidgets(
    activeFleetId ?? '',
  );

  if (!activeFleetId) {
    return (
      <div className="space-y-6">
        <PageHeader title="Dashboard" />
        <p className="text-sm text-muted-foreground">
          No fleet selected. Complete onboarding to get started.
        </p>
      </div>
    );
  }

  return (
    <div className="space-y-6">
      <PageHeader
        title="Dashboard"
        actions={
          isOwner && (
            <AddWidgetMenu placedTypes={widgets.map((w) => w.type)} onAdd={addWidget} />
          )
        }
      />
      <DashboardGrid
        fleetId={activeFleetId}
        isOwner={isOwner}
        widgets={widgets}
        isLoading={isLoading}
        removeWidget={removeWidget}
        moveUp={moveUp}
        moveDown={moveDown}
      />
    </div>
  );
}
