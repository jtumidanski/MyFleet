import { Skeleton } from '../../ui/skeleton';
import { Card, CardContent } from '../../ui/card';
import { Switch } from '../../ui/switch';
import { Label } from '../../ui/label';
import {
  useNotificationPreferences,
  useUpsertNotificationPreference,
} from '../../../lib/hooks/api/notifications';

/** Human-readable labels for known notification types. */
const TYPE_LABELS: Record<string, string> = {
  'vehicle.maintenance_due': 'Maintenance due reminders',
  'vehicle.created': 'Vehicle added',
  'fuel.logged': 'Fuel log entries',
  'mileage.recorded': 'Mileage updates',
  'maintenance.completed': 'Maintenance completed',
};

function getTypeLabel(type: string): string {
  return TYPE_LABELS[type] ?? type;
}

/**
 * Notification preferences page — displays per-type in-app toggle controls.
 * PUT updates are fire-and-forget via mutation with error toast on failure.
 */
export function NotificationPreferences() {
  const { data: preferences, isLoading } = useNotificationPreferences();
  const upsert = useUpsertNotificationPreference();

  if (isLoading) {
    return (
      <div className="space-y-3">
        <Skeleton className="h-6 w-56" />
        {Array.from({ length: 4 }).map((_, i) => (
          <Skeleton key={i} className="h-10 w-full" />
        ))}
      </div>
    );
  }

  const prefs = preferences ?? [];

  return (
    <Card>
      <CardContent className="pt-6">
        <h2 className="mb-4 text-base font-semibold">Notification Preferences</h2>

        {prefs.length === 0 ? (
          <p className="rounded-lg border border-dashed border-gray-300 p-6 text-center text-sm text-gray-500">
            No notification preferences configured yet.
          </p>
        ) : (
          <ul className="space-y-4">
            {prefs.map((pref) => {
              const { type, inAppEnabled } = pref.attributes;
              const labelId = `pref-${pref.id}`;
              return (
                <li key={pref.id} className="flex items-center justify-between gap-4">
                  <Label htmlFor={labelId} className="flex-1 cursor-pointer text-sm text-gray-700">
                    {getTypeLabel(type)}
                  </Label>
                  <Switch
                    id={labelId}
                    checked={inAppEnabled}
                    disabled={upsert.isPending}
                    onCheckedChange={(checked) => {
                      void upsert.mutate({ type, inAppEnabled: checked });
                    }}
                  />
                </li>
              );
            })}
          </ul>
        )}
      </CardContent>
    </Card>
  );
}
