/**
 * DashboardGrid — dependency-light widget grid.
 *
 * Supports: add, remove, reorder (up/down), size selector.
 * No drag-and-drop library added — uses simple buttons per guidelines.
 * Layout persisted via PUT /fleets/{id}/dashboard.
 */
import { Trash2, ChevronUp, ChevronDown } from 'lucide-react';
import { Skeleton } from '../../ui/skeleton';
import { Button } from '../../ui/button';
import { widgetRegistry } from './widgetRegistry';
import { useDashboardWidgets } from './useDashboardWidgets';
import { AddWidgetMenu } from './AddWidgetMenu';

interface DashboardGridProps {
  fleetId: string;
  isOwner: boolean;
}

export function DashboardGrid({ fleetId, isOwner }: DashboardGridProps) {
  const { widgets, isLoading, addWidget, removeWidget, moveUp, moveDown } =
    useDashboardWidgets(fleetId);

  if (isLoading) {
    return (
      <div className="space-y-4">
        <Skeleton className="h-8 w-48" />
        <Skeleton className="h-40 w-full" />
        <Skeleton className="h-40 w-full" />
      </div>
    );
  }

  return (
    <div className="space-y-4">
      {/* Toolbar */}
      {isOwner && (
        <div className="flex items-center justify-between">
          <h2 className="text-lg font-semibold">Dashboard</h2>
          <AddWidgetMenu placedTypes={widgets.map((w) => w.type)} onAdd={addWidget} />
        </div>
      )}

      {/* Widget list */}
      {widgets.length === 0 ? (
        <div className="rounded-lg border border-dashed p-12 text-center">
          <p className="text-sm text-muted-foreground">
            {isOwner
              ? 'No widgets yet. Click "Add Widget" to customize your dashboard.'
              : 'Dashboard is empty.'}
          </p>
        </div>
      ) : (
        <div className="grid grid-cols-1 gap-4 md:grid-cols-2">
          {widgets.map((widget, idx) => {
            const entry = widgetRegistry[widget.type];
            const WidgetComponent = entry.component;
            return (
              <div key={widget.id} className="relative group">
                <WidgetComponent fleetId={fleetId} />
                {isOwner && (
                  <div className="absolute top-2 right-2 hidden group-hover:flex items-center gap-1 bg-background rounded border shadow-sm p-0.5">
                    <Button
                      variant="ghost"
                      size="icon"
                      title="Move up"
                      disabled={idx === 0}
                      onClick={() => moveUp(idx)}
                      className="h-6 w-6"
                    >
                      <ChevronUp className="h-3 w-3" />
                    </Button>
                    <Button
                      variant="ghost"
                      size="icon"
                      title="Move down"
                      disabled={idx === widgets.length - 1}
                      onClick={() => moveDown(idx)}
                      className="h-6 w-6"
                    >
                      <ChevronDown className="h-3 w-3" />
                    </Button>
                    <Button
                      variant="ghost"
                      size="icon"
                      title="Remove widget"
                      onClick={() => removeWidget(widget.id)}
                      className="h-6 w-6 text-destructive hover:text-destructive hover:bg-destructive/10"
                    >
                      <Trash2 className="h-3 w-3" />
                    </Button>
                  </div>
                )}
              </div>
            );
          })}
        </div>
      )}
    </div>
  );
}
