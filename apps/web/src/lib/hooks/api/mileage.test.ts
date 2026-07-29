import { describe, it, expect } from 'vitest';
import { mileageKeys, getLatestMileage } from './mileage';
import type { MileageRecord } from '../../../types/models/mileage';

describe('mileageKeys', () => {
  it('is hierarchical', () => {
    expect(mileageKeys.all).toEqual(['mileage']);
    expect(mileageKeys.lists()).toEqual(['mileage', 'list']);
    expect(mileageKeys.list({ vehicleId: 'v1' })).toEqual(['mileage', 'list', { vehicleId: 'v1' }]);
    expect(mileageKeys.list({ vehicleId: 'v1', from: '2024-01-01T00:00:00Z' })).toEqual([
      'mileage',
      'list',
      { vehicleId: 'v1', from: '2024-01-01T00:00:00Z' },
    ]);
  });
});

describe('getLatestMileage', () => {
  it('returns the mileage of the record with the latest recordedAt', () => {
    const records: MileageRecord[] = [
      {
        id: '1',
        type: 'mileageRecords',
        attributes: {
          vehicleId: 'v1',
          mileage: 10000,
          recordedAt: '2024-01-01T00:00:00Z',
          source: 'manual',
          createdAt: '2024-01-01T00:00:00Z',
          flagged: false,
        },
      },
      {
        id: '2',
        type: 'mileageRecords',
        attributes: {
          vehicleId: 'v1',
          mileage: 15000,
          recordedAt: '2024-06-01T00:00:00Z',
          source: 'manual',
          createdAt: '2024-06-01T00:00:00Z',
          flagged: false,
        },
      },
      {
        id: '3',
        type: 'mileageRecords',
        attributes: {
          vehicleId: 'v1',
          mileage: 12000,
          recordedAt: '2024-03-01T00:00:00Z',
          source: 'manual',
          createdAt: '2024-03-01T00:00:00Z',
          flagged: false,
        },
      },
    ];

    expect(getLatestMileage(records)).toBe(15000);
  });

  it('returns undefined for an empty array', () => {
    expect(getLatestMileage([])).toBeUndefined();
  });

  it('returns the single record mileage when array has one element', () => {
    const records: MileageRecord[] = [
      {
        id: '1',
        type: 'mileageRecords',
        attributes: {
          vehicleId: 'v1',
          mileage: 5000,
          recordedAt: '2024-01-01T00:00:00Z',
          source: 'manual',
          createdAt: '2024-01-01T00:00:00Z',
          flagged: false,
        },
      },
    ];
    expect(getLatestMileage(records)).toBe(5000);
  });
});
