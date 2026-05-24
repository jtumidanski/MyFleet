import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { toast } from 'sonner';
import { createErrorFromUnknown } from '@myfleet/shared-ts';
import { notificationService } from '../../../services/api/NotificationService';
import type { Notification } from '../../../types/models/notification';
import type { UpsertNotificationPreferenceAttributes } from '../../../types/models/notification';

// ---------------------------------------------------------------------------
// Query key factory
// ---------------------------------------------------------------------------

/**
 * Hierarchical query-key factory for notifications.
 *
 * all                                            -> ['notifications']
 * lists()                                        -> ['notifications', 'list']
 * list({ read, type, page, pageSize })           -> ['notifications', 'list', { ... }]
 * preferences()                                  -> ['notifications', 'preferences']
 */
export const notificationKeys = {
  all: ['notifications'] as const,
  lists: () => [...notificationKeys.all, 'list'] as const,
  list: (params: { read?: boolean; type?: string; page: number; pageSize: number }) =>
    [...notificationKeys.lists(), params] as const,
  preferences: () => [...notificationKeys.all, 'preferences'] as const,
};

// ---------------------------------------------------------------------------
// Unread-count selector (pure — injectable/testable)
// ---------------------------------------------------------------------------

/**
 * Returns the number of notifications that have not been read yet
 * (readAt is absent/null/undefined).
 */
export function selectUnreadCount(notifications: Notification[]): number {
  return notifications.filter((n) => !n.attributes.readAt).length;
}

// ---------------------------------------------------------------------------
// Queries
// ---------------------------------------------------------------------------

const DEFAULT_PAGE = 1;
const DEFAULT_PAGE_SIZE = 25;

export interface UseNotificationsParams {
  read?: boolean;
  type?: string;
  page?: number;
  pageSize?: number;
}

/**
 * GET /api/notifications/notifications — the caller's own notifications (paged).
 * Supports optional `read` and `type` filters.
 */
export function useNotifications(params: UseNotificationsParams = {}) {
  const { read, type, page = DEFAULT_PAGE, pageSize = DEFAULT_PAGE_SIZE } = params;
  return useQuery({
    queryKey: notificationKeys.list({ read, type, page, pageSize }),
    queryFn: () => notificationService.list({ read, type, page, pageSize }),
    staleTime: 30 * 1000,
    gcTime: 5 * 60 * 1000,
    select: (result) => ({ data: result.data, meta: result.meta }),
  });
}

/**
 * Derived hook — returns the unread notification count by fetching the unread
 * list (read=false). Suitable for the notification bell badge.
 */
export function useUnreadNotificationCount() {
  return useQuery({
    queryKey: notificationKeys.list({ read: false, page: DEFAULT_PAGE, pageSize: 100 }),
    queryFn: () => notificationService.list({ read: false, page: DEFAULT_PAGE, pageSize: 100 }),
    staleTime: 30 * 1000,
    gcTime: 5 * 60 * 1000,
    select: (result) => selectUnreadCount(result.data),
  });
}

/**
 * GET /api/notifications/notification-preferences — list preference rows.
 */
export function useNotificationPreferences() {
  return useQuery({
    queryKey: notificationKeys.preferences(),
    queryFn: () => notificationService.listPreferences(),
    staleTime: 5 * 60 * 1000,
    gcTime: 10 * 60 * 1000,
    select: (result) => result.data,
  });
}

// ---------------------------------------------------------------------------
// Mutations
// ---------------------------------------------------------------------------

/**
 * POST /api/notifications/notifications/{id}/read — mark a single notification read.
 * Invalidates all notification lists on settle.
 */
export function useMarkNotificationRead() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => notificationService.markRead(id),
    onSettled: () => {
      void queryClient.invalidateQueries({ queryKey: notificationKeys.lists() });
    },
    onError: (err) => {
      const apiError = createErrorFromUnknown(err);
      toast.error(apiError.message || 'Could not mark notification as read');
    },
  });
}

/**
 * POST /api/notifications/notifications/read-all — mark all unread as read.
 * Invalidates all notification lists on settle.
 */
export function useMarkAllNotificationsRead() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: () => notificationService.markAllRead(),
    onSettled: () => {
      void queryClient.invalidateQueries({ queryKey: notificationKeys.lists() });
    },
    onError: (err) => {
      const apiError = createErrorFromUnknown(err);
      toast.error(apiError.message || 'Could not mark all notifications as read');
    },
  });
}

/**
 * PUT /api/notifications/notification-preferences — upsert one preference toggle.
 * Invalidates preferences key on settle.
 */
export function useUpsertNotificationPreference() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (attrs: UpsertNotificationPreferenceAttributes) =>
      notificationService.upsertPreference(attrs),
    onSettled: () => {
      void queryClient.invalidateQueries({ queryKey: notificationKeys.preferences() });
    },
    onError: (err) => {
      const apiError = createErrorFromUnknown(err);
      toast.error(apiError.message || 'Could not save notification preference');
    },
  });
}
