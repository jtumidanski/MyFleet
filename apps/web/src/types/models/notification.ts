import type { JsonApiResource } from '@myfleet/shared-ts';

/**
 * Mirrors apps/notification-service/internal/notification/rest.go Attributes.
 */
export interface NotificationAttributes {
  userId: string;
  /** Domain notification type string, e.g. 'vehicle.maintenance_due'. */
  type: string;
  title: string;
  body?: string;
  vehicleId?: string;
  fleetId: string;
  read: boolean;
  /** RFC3339 — present when read is true. */
  readAt?: string;
  /** RFC3339 */
  createdAt: string;
}

export type Notification = JsonApiResource<NotificationAttributes>;

/**
 * Mirrors apps/notification-service/internal/preferences/rest.go Attributes.
 */
export interface NotificationPreferenceAttributes {
  userId: string;
  /** Domain notification type string. */
  type: string;
  inAppEnabled: boolean;
}

export type NotificationPreference = JsonApiResource<NotificationPreferenceAttributes>;

/** PUT /api/notifications/notification-preferences body attributes */
export interface UpsertNotificationPreferenceAttributes {
  type: string;
  inAppEnabled: boolean;
}
