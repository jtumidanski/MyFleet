/**
 * DashboardGrid — dependency-light widget grid.
 *
 * Supports: add, remove, reorder (up/down), size selector.
 * No drag-and-drop library added — uses simple buttons per guidelines.
 * Layout persisted via PUT /fleets/{id}/dashboard.
 */
import { useState, useCallback } from 'react';
import { Plus, Trash2, ChevronUp, ChevronDown } from 'lucide-react';
import { Skeleton } from '../../ui/skeleton';
import { Button } from '../../ui/button';
import { cn } from '../../../lib/utils';
import { widgetRegistry } from './widgetRegistry';
import { WIDGET_CATALOG, type WidgetType } from './widgetCatalog';
import { useDashboardLayout, useSaveDashboardLayout } from '../../../lib/hooks/api/dashboard';
import type { WidgetInput } from '../../../types/models/dashboard';

interface DashboardGridProps {
  fleetId: string;
  isOwner: boolean;
}

interface GridWidget {
  id: string;
  type: WidgetType;
  positionX: number;
  positionY: number;
  width: number;
  height: number;
}

function toGridWidget(w: { id: string; attributes: { type: string; positionX: number; positionY: number; width: number; height: number } }): GridWidget | null {
  if (!WIDGET_CATALOG.includes(w.attributes.type as WidgetType)) return null;
  return {
    id: w.id,
    type: w.attributes.type as WidgetType,
    positionX: w.attributes.positionX,
    positionY: w.attributes.positionY,
    width: w.attributes.width,
    height: w.attributes.height,
  };
}

function toWidgetInputs(widgets: GridWidget[]): WidgetInput[] {
  return widgets.map((w, idx) => ({
    type: w.type,
    positionX: 0,
    positionY: idx,
    width: w.width,
    height: w.height,
  }));
}

export function DashboardGrid({ fleetId, isOwner }: DashboardGridProps) {
  const { data: dashboard, isLoading } = useDashboardLayout(fleetId);
  const saveLayout = useSaveDashboardLayout(fleetId);

  // Local widget list derived from the server layout on first load.
  // We keep a local copy for immediate UI updates before the save settles.
  const [localWidgets, setLocalWidgets] = useState<GridWidget[] | null>(null);

  const [showAddMenu, setShowAddMenu] = useState(false);

  // On load, initialize local state once.
  const serverWidgets: GridWidget[] = (dashboard?.attributes.widgets ?? [])
    .map(toGridWidget)
    .filter((w): w is GridWidget => w !== null);

  const widgets = localWidgets ?? serverWidgets;

  const save = useCallback((next: GridWidget[]) => {
    setLocalWidgets(next);
    saveLayout.mutate(toWidgetInputs(next));
  }, [saveLayout]);

  const addWidget = (type: WidgetType) => {
    const entry = widgetRegistry[type];
    const next: GridWidget[] = [
      ...widgets,
      {
        id: `new-${Date.now()}`,
        type,
        positionX: 0,
        positionY: widgets.length,
        width: entry.defaultWidth,
        height: entry.defaultHeight,
      },
    ];
    save(next);
    setShowAddMenu(false);
  };

  const removeWidget = (id: string) => {
    const next = widgets.filter((w) => w.id !== id);
    save(next);
  };

  const moveUp = (idx: number) => {
    if (idx === 0) return;
    const next = [...widgets];
    const above = next[idx - 1];
    const current = next[idx];
    if (!above || !current) return;
    next[idx - 1] = current;
    next[idx] = above;
    save(next);
  };

  const moveDown = (idx: number) => {
    if (idx === widgets.length - 1) return;
    const next = [...widgets];
    const current = next[idx];
    const below = next[idx + 1];
    if (!current || !below) return;
    next[idx] = below;
    next[idx + 1] = current;
    save(next);
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
            <Button
              variant="outline"
              size="sm"
              onClick={() => setShowAddMenu((v) => !v)}
            >
              <Plus className="mr-1 h-4 w-4" />
              Add Widget
            </Button>
            {showAddMenu && (
              <div className="absolute right-0 z-10 mt-1 w-52 rounded-md border bg-white shadow-lg">
                <ul className="py-1">
                  {WIDGET_CATALOG.map((type) => (
                    <li key={type}>
                      <button
                        className={cn(
                          'w-full px-4 py-2 text-left text-sm hover:bg-gray-100',
                          widgets.some((w) => w.type === type) && 'text-muted-foreground',
                        )}
                        onClick={() => addWidget(type)}
                      >
                        {widgetRegistry[type].label}
                      </button>
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
                  <div className="absolute top-2 right-2 hidden group-hover:flex items-center gap-1 bg-white rounded border shadow-sm p-0.5">
                    <button
                      title="Move up"
                      disabled={idx === 0}
                      onClick={() => moveUp(idx)}
                      className="p-1 rounded hover:bg-gray-100 disabled:opacity-40"
                    >
                      <ChevronUp className="h-3 w-3" />
                    </button>
                    <button
                      title="Move down"
                      disabled={idx === widgets.length - 1}
                      onClick={() => moveDown(idx)}
                      className="p-1 rounded hover:bg-gray-100 disabled:opacity-40"
                    >
                      <ChevronDown className="h-3 w-3" />
                    </button>
                    <button
                      title="Remove widget"
                      onClick={() => removeWidget(widget.id)}
                      className="p-1 rounded hover:bg-red-50 text-red-500"
                    >
                      <Trash2 className="h-3 w-3" />
                    </button>
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
