import { describe, it, expect } from 'vitest';
import { notificationKeys, selectUnreadCount } from './notifications';
import type { Notification } from '../../../types/models/notification';

describe('notificationKeys', () => {
  it('is hierarchical', () => {
    expect(notificationKeys.all).toEqual(['notifications']);
    expect(notificationKeys.lists()).toEqual(['notifications', 'list']);
    expect(notificationKeys.list({ read: false, type: undefined, page: 1, pageSize: 25 })).toEqual([
      'notifications',
      'list',
      { read: false, type: undefined, page: 1, pageSize: 25 },
    ]);
    expect(notificationKeys.preferences()).toEqual(['notifications', 'preferences']);
  });
});

describe('selectUnreadCount', () => {
  const makeNotification = (id: string, readAt: string | null): Notification => ({
    id,
    type: 'notifications',
    attributes: {
      userId: 'u1',
      type: 'vehicle.maintenance_due',
      title: 'Maintenance due',
      fleetId: 'f1',
      read: readAt !== null,
      readAt: readAt ?? undefined,
      createdAt: '2024-01-01T00:00:00Z',
    },
  });

  it('counts notifications with readAt null as unread', () => {
    const notifications: Notification[] = [
      makeNotification('1', null),
      makeNotification('2', null),
      makeNotification('3', '2024-01-02T00:00:00Z'),
      makeNotification('4', null),
    ];
    expect(selectUnreadCount(notifications)).toBe(3);
  });

  it('returns 0 when all are read', () => {
    const notifications: Notification[] = [
      makeNotification('1', '2024-01-02T00:00:00Z'),
      makeNotification('2', '2024-01-03T00:00:00Z'),
    ];
    expect(selectUnreadCount(notifications)).toBe(0);
  });

  it('returns 0 for empty array', () => {
    expect(selectUnreadCount([])).toBe(0);
  });

  it('returns full count when all are unread', () => {
    const notifications: Notification[] = [
      makeNotification('1', null),
      makeNotification('2', null),
      makeNotification('3', null),
    ];
    expect(selectUnreadCount(notifications)).toBe(3);
  });
});
