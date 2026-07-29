import { describe, it, expect } from 'vitest';
import { activityKeys, selectActivityPage } from './activity';
import type { ActivityEvent } from '../../../types/models/activity';
import type { PageMeta } from '@myfleet/shared-ts';

describe('activityKeys', () => {
  it('is hierarchical', () => {
    expect(activityKeys.all).toEqual(['activityEvents']);
    expect(activityKeys.lists()).toEqual(['activityEvents', 'list']);
    expect(activityKeys.fleetFeed({ fleetId: 'f1', page: 1, pageSize: 25 })).toEqual([
      'activityEvents',
      'list',
      { fleetId: 'f1', page: 1, pageSize: 25 },
    ]);
    expect(activityKeys.vehicleTimeline({ vehicleId: 'v1', page: 1, pageSize: 25 })).toEqual([
      'activityEvents',
      'list',
      { vehicleId: 'v1', page: 1, pageSize: 25 },
    ]);
  });
});

describe('selectActivityPage', () => {
  const makeEvent = (id: string): ActivityEvent => ({
    id,
    type: 'activityEvents',
    attributes: {
      fleetId: 'f1',
      actorUserId: 'u1',
      type: 'vehicle.created',
      createdAt: '2024-01-01T00:00:00Z',
    },
  });

  const meta: PageMeta = {
    total: 50,
    totalPages: 2,
    number: 1,
    size: 25,
  };

  it('derives total, totalPages, and hasNextPage from meta', () => {
    const data = [makeEvent('1'), makeEvent('2')];
    const result = selectActivityPage({ data, meta });

    expect(result.total).toBe(50);
    expect(result.totalPages).toBe(2);
    expect(result.hasNextPage).toBe(true);
    expect(result.data).toHaveLength(2);
  });

  it('returns hasNextPage false when on the last page', () => {
    const lastPageMeta: PageMeta = {
      total: 50,
      totalPages: 2,
      number: 2,
      size: 25,
    };
    const result = selectActivityPage({ data: [makeEvent('1')], meta: lastPageMeta });
    expect(result.hasNextPage).toBe(false);
  });

  it('returns hasNextPage false when meta is undefined', () => {
    const result = selectActivityPage({ data: [makeEvent('1')], meta: undefined });
    expect(result.hasNextPage).toBe(false);
    expect(result.total).toBe(0);
    expect(result.totalPages).toBe(1);
  });

  it('returns hasNextPage false when there is only one page', () => {
    const singlePageMeta: PageMeta = {
      total: 5,
      totalPages: 1,
      number: 1,
      size: 25,
    };
    const result = selectActivityPage({ data: [makeEvent('1')], meta: singlePageMeta });
    expect(result.hasNextPage).toBe(false);
  });
});
