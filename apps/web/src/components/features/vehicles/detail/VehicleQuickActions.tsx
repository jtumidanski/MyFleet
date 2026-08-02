import { Card, CardContent } from '../../../ui/card';
import { Button } from '../../../ui/button';

export type QuickAction =
  'mileage' | 'fuel' | 'maintenance' | 'modification' | 'schedule' | 'upload' | 'delete';

interface VehicleQuickActionsProps {
  canWrite: boolean;
  canDelete: boolean;
  onAction: (action: QuickAction) => void;
}

const WRITE_ACTIONS: ReadonlyArray<{ action: QuickAction; label: string }> = [
  { action: 'mileage', label: 'Log Mileage' },
  { action: 'fuel', label: 'Log Fuel' },
  { action: 'maintenance', label: 'Log Maintenance' },
  { action: 'modification', label: 'Log Modification' },
  { action: 'schedule', label: 'Add Schedule' },
  { action: 'upload', label: 'Upload Photo' },
];

/**
 * One-tap entry points into every mutation the detail page offers, gated on
 * role. Stacks full-width in the identity rail column; below `lg` (where the
 * rail collapses above the content instead of beside it) it becomes a
 * wrapping row so the actions don't eat a whole screen of vertical space.
 */
export function VehicleQuickActions({ canWrite, canDelete, onAction }: VehicleQuickActionsProps) {
  const actions = [
    ...(canWrite ? WRITE_ACTIONS : []),
    ...(canDelete ? [{ action: 'delete' as const, label: 'Delete Vehicle' }] : []),
  ];

  if (actions.length === 0) {
    return null;
  }

  return (
    <Card>
      <CardContent className="flex flex-row flex-wrap gap-1 pt-6 lg:flex-col">
        {actions.map(({ action, label }) => (
          <Button
            key={action}
            type="button"
            variant={action === 'delete' ? 'destructive' : 'outline'}
            className="flex-1 basis-32 lg:w-full lg:flex-none"
            onClick={() => onAction(action)}
          >
            {label}
          </Button>
        ))}
      </CardContent>
    </Card>
  );
}
