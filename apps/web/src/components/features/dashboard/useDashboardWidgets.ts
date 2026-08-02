/**
 * useDashboardWidgets — the dashboard's widget list and its writers.
 *
 * Lifted out of DashboardGrid (task-015, design §3.2) so DashboardPage can own
 * both the page header's Add Widget control and the grid beneath it from a
 * single source of state. DashboardGrid became a props-driven renderer.
 *
 * Layout is persisted via PUT /fleets/{id}/dashboard on every mutation; the
 * local copy exists so the UI updates before the save settles.
 */
import { useCallback, useState } from 'react';
import { widgetRegistry } from './widgetRegistry';
import { WIDGET_CATALOG, type WidgetType } from './widgetCatalog';
import { useDashboardLayout, useSaveDashboardLayout } from '../../../lib/hooks/api/dashboard';
import type { WidgetInput } from '../../../types/models/dashboard';

export interface GridWidget {
  id: string;
  type: WidgetType;
  positionX: number;
  positionY: number;
  width: number;
  height: number;
}

export interface DashboardWidgets {
  widgets: GridWidget[];
  isLoading: boolean;
  addWidget: (type: WidgetType) => void;
  removeWidget: (id: string) => void;
  moveUp: (idx: number) => void;
  moveDown: (idx: number) => void;
}

function toGridWidget(w: {
  id: string;
  attributes: { type: string; positionX: number; positionY: number; width: number; height: number };
}): GridWidget | null {
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

export function useDashboardWidgets(fleetId: string): DashboardWidgets {
  const { data: dashboard, isLoading } = useDashboardLayout(fleetId);
  const saveLayout = useSaveDashboardLayout(fleetId);

  // Local widget list derived from the server layout on first load.
  // We keep a local copy for immediate UI updates before the save settles.
  const [localWidgets, setLocalWidgets] = useState<GridWidget[] | null>(null);

  const serverWidgets: GridWidget[] = (dashboard?.attributes.widgets ?? [])
    .map(toGridWidget)
    .filter((w): w is GridWidget => w !== null);

  const widgets = localWidgets ?? serverWidgets;

  const save = useCallback(
    (next: GridWidget[]) => {
      setLocalWidgets(next);
      saveLayout.mutate(toWidgetInputs(next));
    },
    [saveLayout],
  );

  const addWidget = (type: WidgetType) => {
    // Finding 1: while the initial GET is still in flight, `widgets` is `[]`
    // (server data hasn't landed and there's no local copy yet). Adding here
    // would `save([newWidget])`, and `save` is a full-replace PUT — it would
    // wipe out whatever the server actually has once it responds, and the
    // local copy would then mask the real GET result. No-op until we know
    // what's really there.
    if (isLoading) return;
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

  return { widgets, isLoading, addWidget, removeWidget, moveUp, moveDown };
}
