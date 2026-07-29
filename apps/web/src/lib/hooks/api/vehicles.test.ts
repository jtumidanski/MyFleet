import { describe, it, expect } from 'vitest';
import { vehicleKeys } from './vehicles';

describe('vehicleKeys', () => {
  it('is hierarchical', () => {
    expect(vehicleKeys.all).toEqual(['vehicles']);
    expect(vehicleKeys.list({ fleetId: 'f1' })).toEqual(['vehicles', 'list', { fleetId: 'f1' }]);
    expect(vehicleKeys.detail('v1')).toEqual(['vehicles', 'detail', 'v1']);
  });
});
