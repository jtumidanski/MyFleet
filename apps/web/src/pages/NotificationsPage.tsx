import { NotificationList } from '../components/features/notifications/NotificationList';
import { NotificationPreferences } from '../components/features/notifications/NotificationPreferences';

/**
 * Notifications page — shows the user's notifications and their per-type
 * in-app notification preferences.
 */
export function NotificationsPage() {
  return (
    <div className="space-y-6">
      <h1 className="text-2xl font-semibold">Notifications</h1>
      <div className="grid grid-cols-1 gap-6 lg:grid-cols-2">
        <NotificationList />
        <NotificationPreferences />
      </div>
    </div>
  );
}
