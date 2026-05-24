import { toast } from 'sonner';
import { createErrorFromUnknown } from '@myfleet/shared-ts';
import { Button } from '../../ui/button';
import { Skeleton } from '../../ui/skeleton';
import { Card, CardContent } from '../../ui/card';
import { cn } from '../../../lib/utils';
import {
  useNotifications,
  useMarkNotificationRead,
  useMarkAllNotificationsRead,
} from '../../../lib/hooks/api/notifications';
import type { Notification } from '../../../types/models/notification';

interface NotificationItemProps {
  notification: Notification;
  onMarkRead: (id: string) => void;
  isMarkingRead: boolean;
}

function NotificationItem({ notification, onMarkRead, isMarkingRead }: NotificationItemProps) {
  const { title, body, read, createdAt } = notification.attributes;

  return (
    <li
      className={cn(
        'flex items-start gap-3 rounded-md border p-3',
        read ? 'border-border bg-card' : 'border-primary/20 bg-primary/5',
      )}
    >
      {!read && (
        <span className="mt-1.5 h-2 w-2 shrink-0 rounded-full bg-primary" aria-hidden="true" />
      )}
      <div className="flex-1 min-w-0">
        <p className="text-sm font-medium text-foreground">{title}</p>
        {body && <p className="mt-0.5 text-xs text-muted-foreground">{body}</p>}
        <time className="mt-1 text-xs text-muted-foreground">
          {new Date(createdAt).toLocaleString()}
        </time>
      </div>
      {!read && (
        <Button
          variant="ghost"
          size="sm"
          className="shrink-0 text-xs"
          onClick={() => onMarkRead(notification.id)}
          disabled={isMarkingRead}
        >
          Mark read
        </Button>
      )}
    </li>
  );
}

/**
 * Notification list — shows the current user's notifications with mark-read
 * actions. Includes a "Mark all read" button at the top.
 */
export function NotificationList() {
  const { data, isLoading } = useNotifications({ page: 1, pageSize: 50 });
  const markRead = useMarkNotificationRead();
  const markAllRead = useMarkAllNotificationsRead();

  const notifications = data?.data ?? [];

  const handleMarkRead = async (id: string) => {
    try {
      await markRead.mutateAsync(id);
    } catch (err) {
      const apiError = createErrorFromUnknown(err);
      toast.error(apiError.message || 'Could not mark notification as read');
    }
  };

  const handleMarkAllRead = async () => {
    try {
      await markAllRead.mutateAsync();
      toast.success('All notifications marked as read');
    } catch (err) {
      const apiError = createErrorFromUnknown(err);
      toast.error(apiError.message || 'Could not mark all notifications as read');
    }
  };

  if (isLoading) {
    return (
      <div className="space-y-3">
        <Skeleton className="h-6 w-40" />
        {Array.from({ length: 4 }).map((_, i) => (
          <Skeleton key={i} className="h-14 w-full" />
        ))}
      </div>
    );
  }

  const hasUnread = notifications.some((n) => !n.attributes.read);

  return (
    <Card>
      <CardContent className="pt-6">
        <div className="mb-4 flex items-center justify-between">
          <h2 className="text-base font-semibold">Notifications</h2>
          {hasUnread && (
            <Button
              variant="outline"
              size="sm"
              onClick={() => void handleMarkAllRead()}
              disabled={markAllRead.isPending}
            >
              Mark all read
            </Button>
          )}
        </div>

        {notifications.length === 0 ? (
          <p className="rounded-lg border border-dashed border-border p-6 text-center text-sm text-muted-foreground">
            No notifications yet.
          </p>
        ) : (
          <ul className="space-y-2">
            {notifications.map((n) => (
              <NotificationItem
                key={n.id}
                notification={n}
                onMarkRead={(id) => void handleMarkRead(id)}
                isMarkingRead={markRead.isPending}
              />
            ))}
          </ul>
        )}
      </CardContent>
    </Card>
  );
}
