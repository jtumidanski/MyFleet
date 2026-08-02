/**
 * AddWidgetMenu — the dashboard's "Add Widget" button and its dropdown.
 *
 * Split out of DashboardGrid (task-015, design §3.2) so it can be rendered as
 * PageHeader's `actions` from DashboardPage. It owns only its own open/close
 * state; the widget list lives in useDashboardWidgets.
 *
 * Already-placed types are dimmed rather than disabled — adding a second copy
 * of a widget is legal, it just usually is not what you meant.
 */
import { useState } from 'react';
import { Plus } from 'lucide-react';
import { Button } from '../../ui/button';
import { cn } from '../../../lib/utils';
import { widgetRegistry } from './widgetRegistry';
import { WIDGET_CATALOG, type WidgetType } from './widgetCatalog';

export interface AddWidgetMenuProps {
  /** Types already on the board — rendered dimmed in the menu. */
  placedTypes: WidgetType[];
  onAdd: (type: WidgetType) => void;
}

export function AddWidgetMenu({ placedTypes, onAdd }: AddWidgetMenuProps) {
  const [showAddMenu, setShowAddMenu] = useState(false);

  const handleAdd = (type: WidgetType) => {
    onAdd(type);
    setShowAddMenu(false);
  };

  return (
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
                    placedTypes.includes(type) && 'text-muted-foreground',
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
  );
}
