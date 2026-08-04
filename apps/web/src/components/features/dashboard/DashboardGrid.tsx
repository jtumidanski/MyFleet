/**
 * DashboardGrid — renders the widget list. Add, remove and reorder controls
 * only; the list itself and its writers come from useDashboardWidgets, and the
 * page header (title + Add Widget) is DashboardPage's (task-015, design §3).
 *
 * No drag-and-drop library added — uses simple buttons per guidelines.
 */
import { Trash2, ChevronUp, ChevronDown } from 'lucide-react';
import { Skeleton } from '../../ui/skeleton';
import { Button } from '../../ui/button';
import { widgetRegistry } from './widgetRegistry';
import type { GridWidget } from './useDashboardWidgets';

interface DashboardGridProps {
  fleetId: string;
  isOwner: boolean;
  widgets: GridWidget[];
  isLoading: boolean;
  removeWidget: (id: string) => void;
  moveUp: (idx: number) => void;
  moveDown: (idx: number) => void;
}

export function DashboardGrid({
  fleetId,
  isOwner,
  widgets,
  isLoading,
  removeWidget,
  moveUp,
  moveDown,
}: DashboardGridProps) {
  if (isLoading) {
    // No title skeleton here: the page renders the real <h1> unconditionally,
    // so a shimmer above these cards would reintroduce the vertical jump this
    // task exists to remove.
    return (
      <div className="space-y-4">
        <Skeleton className="h-40 w-full" />
        <Skeleton className="h-40 w-full" />
      </div>
    );
  }

  if (widgets.length === 0) {
    return (
      <div className="rounded-lg border border-dashed p-12 text-center">
        <p className="text-sm text-muted-foreground">
          {isOwner
            ? 'No widgets yet. Click "Add Widget" to customize your dashboard.'
            : 'Dashboard is empty.'}
        </p>
      </div>
    );
  }

  return (
    <div className="grid grid-cols-1 gap-4 md:grid-cols-2">
      {widgets.map((widget, idx) => {
        const entry = widgetRegistry[widget.type];
        const WidgetComponent = entry.component;
        return (
          <div key={widget.id} className="relative group">
            <WidgetComponent fleetId={fleetId} />
            {isOwner && (
              <div className="absolute top-2 right-2 hidden group-hover:flex items-center gap-1 bg-background rounded-sm border shadow-xs p-0.5">
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
  );
}
