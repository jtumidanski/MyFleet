import type { JsonApiDocument, JsonApiResource, PageMeta } from '@myfleet/shared-ts';
import { apiClient } from '../../lib/api/client';
import type {
  NotificationAttributes,
  NotificationPreferenceAttributes,
  UpsertNotificationPreferenceAttributes,
} from '../../types/models/notification';

/**
 * Notification service — wraps the notification-service endpoints.
 *
 * Gateway strips /api/notifications → service receives bare path.
 * Traefik labels (deploy/compose/docker-compose.yml):
 *   PathPrefix(`/api/notifications`) + stripprefix.prefixes=/api/notifications
 *
 * Service-registered paths (apps/notification-service/internal/notification/resource.go):
 *   GET  /notifications              → full gateway path: /api/notifications/notifications
 *   POST /notifications/{id}/read   → full gateway path: /api/notifications/notifications/{id}/read
 *   POST /notifications/read-all    → full gateway path: /api/notifications/notifications/read-all
 *
 * Service-registered paths (apps/notification-service/internal/preferences/resource.go):
 *   GET  /notification-preferences  → full gateway path: /api/notifications/notification-preferences
 *   PUT  /notification-preferences  → full gateway path: /api/notifications/notification-preferences
 */
class NotificationService {
  private readonly notificationsPath = '/api/notifications/notifications';
  private readonly preferencesPath = '/api/notifications/notification-preferences';

  /**
   * GET /api/notifications/notifications?read=&type=
   * Supports optional read (boolean) and type filters, plus pagination.
   */
  async list(params?: {
    read?: boolean;
    type?: string;
    page?: number;
    pageSize?: number;
  }): Promise<{ data: Array<JsonApiResource<NotificationAttributes>>; meta?: PageMeta }> {
    const url = new URL(this.notificationsPath, 'http://localhost');
    if (params?.read !== undefined) url.searchParams.set('read', String(params.read));
    if (params?.type) url.searchParams.set('type', params.type);
    if (params?.page != null) url.searchParams.set('page[number]', String(params.page));
    if (params?.pageSize != null) url.searchParams.set('page[size]', String(params.pageSize));

    const doc = await apiClient.request<
      JsonApiDocument<Array<JsonApiResource<NotificationAttributes>>>
    >(`${this.notificationsPath}${url.search}`);
    return { data: doc.data, meta: doc.meta };
  }

  /**
   * POST /api/notifications/notifications/{id}/read — mark a single notification read.
   */
  async markRead(id: string): Promise<void> {
    await apiClient.request<null>(`${this.notificationsPath}/${id}/read`, { method: 'POST' });
  }

  /**
   * POST /api/notifications/notifications/read-all — mark all unread as read.
   */
  async markAllRead(): Promise<void> {
    await apiClient.request<null>(`${this.notificationsPath}/read-all`, { method: 'POST' });
  }

  /**
   * GET /api/notifications/notification-preferences — list preference rows.
   */
  async listPreferences(): Promise<{
    data: Array<JsonApiResource<NotificationPreferenceAttributes>>;
  }> {
    const doc = await apiClient.request<
      JsonApiDocument<Array<JsonApiResource<NotificationPreferenceAttributes>>>
    >(this.preferencesPath);
    return { data: doc.data };
  }

  /**
   * PUT /api/notifications/notification-preferences — upsert one preference toggle.
   */
  async upsertPreference(
    attrs: UpsertNotificationPreferenceAttributes,
  ): Promise<JsonApiResource<NotificationPreferenceAttributes>> {
    const doc = await apiClient.request<
      JsonApiDocument<JsonApiResource<NotificationPreferenceAttributes>>
    >(this.preferencesPath, {
      method: 'PUT',
      body: JSON.stringify({ data: { type: 'notification-preferences', attributes: attrs } }),
    });
    return doc.data;
  }
}

export const notificationService = new NotificationService();
