import { useUnreadNotificationCount } from '../../../lib/hooks/api/notifications';
import { cn } from '../../../lib/utils';

interface NotificationBellProps {
  className?: string;
}

/**
 * Notification bell icon with an unread-count badge.
 * Fetches the unread count via the notifications query (read=false filter).
 */
export function NotificationBell({ className }: NotificationBellProps) {
  const { data: unreadCount = 0 } = useUnreadNotificationCount();

  return (
    <span className={cn('relative inline-flex items-center', className)} aria-label="Notifications">
      {/* Bell icon */}
      <svg
        xmlns="http://www.w3.org/2000/svg"
        className="h-6 w-6 text-gray-600"
        fill="none"
        viewBox="0 0 24 24"
        stroke="currentColor"
        strokeWidth={1.5}
        aria-hidden="true"
      >
        <path
          strokeLinecap="round"
          strokeLinejoin="round"
          d="M14.857 17.082a23.848 23.848 0 0 0 5.454-1.31A8.967 8.967 0 0 1 18 9.75V9A6 6 0 0 0 6 9v.75a8.967 8.967 0 0 1-2.312 6.022c1.733.64 3.56 1.085 5.455 1.31m5.714 0a24.255 24.255 0 0 1-5.714 0m5.714 0a3 3 0 1 1-5.714 0"
        />
      </svg>

      {/* Unread badge */}
      {unreadCount > 0 && (
        <span
          className="absolute -right-1 -top-1 flex h-4 min-w-[1rem] items-center justify-center rounded-full bg-red-500 px-1 text-[10px] font-bold leading-none text-white"
          aria-live="polite"
          aria-label={`${unreadCount} unread notifications`}
        >
          {unreadCount > 99 ? '99+' : unreadCount}
        </span>
      )}
    </span>
  );
}
