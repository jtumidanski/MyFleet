/**
 * DashboardGrid — dependency-light widget grid.
 *
 * Supports: add, remove, reorder (up/down), size selector.
 * No drag-and-drop library added — uses simple buttons per guidelines.
 * Layout persisted via PUT /fleets/{id}/dashboard.
 */
import { useState } from 'react';
import { Plus, Trash2, ChevronUp, ChevronDown } from 'lucide-react';
import { Skeleton } from '../../ui/skeleton';
import { Button } from '../../ui/button';
import { cn } from '../../../lib/utils';
import { widgetRegistry } from './widgetRegistry';
import { WIDGET_CATALOG, type WidgetType } from './widgetCatalog';
import { useDashboardWidgets } from './useDashboardWidgets';

interface DashboardGridProps {
  fleetId: string;
  isOwner: boolean;
}

export function DashboardGrid({ fleetId, isOwner }: DashboardGridProps) {
  const { widgets, isLoading, addWidget, removeWidget, moveUp, moveDown } =
    useDashboardWidgets(fleetId);
  const [showAddMenu, setShowAddMenu] = useState(false);

  const handleAdd = (type: WidgetType) => {
    addWidget(type);
    setShowAddMenu(false);
  };

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
          <div className="relative">
            <Button variant="outline" size="sm" onClick={() => setShowAddMenu((v) => !v)}>
              <Plus className="mr-1 h-4 w-4" />
              Add Widget
            </Button>
            {showAddMenu && (
              <div className="absolute right-0 z-10 mt-1 w-52 rounded-md border bg-popover shadow-lg">
                <ul className="py-1">
                  {WIDGET_CATALOG.map((type) => (
                    <li key={type}>
                      <Button
                        variant="ghost"
                        size="sm"
                        className={cn(
                          'w-full justify-start px-4 py-2 text-sm font-normal',
                          widgets.some((w) => w.type === type) && 'text-muted-foreground',
                        )}
                        onClick={() => handleAdd(type)}
                      >
                        {widgetRegistry[type].label}
                      </Button>
                    </li>
                  ))}
                </ul>
              </div>
            )}
          </div>
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
