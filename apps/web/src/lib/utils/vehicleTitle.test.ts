import { describe, it, expect } from 'vitest';
import { vehicleTitle } from './vehicleTitle';
import type { VehicleAttributes } from '../../types/models/vehicle';

function attrs(overrides: Partial<VehicleAttributes> = {}): VehicleAttributes {
  return { fleetId: 'f1', make: 'Ford', model: 'F-150', year: 2019, ...overrides };
}

describe('vehicleTitle', () => {
  it('prefers the nickname', () => {
    expect(vehicleTitle(attrs({ nickname: 'Weekend Truck' }))).toBe('Weekend Truck');
  });

  it('trims the nickname it returns', () => {
    expect(vehicleTitle(attrs({ nickname: '  Weekend Truck  ' }))).toBe('Weekend Truck');
  });

  // Load-bearing (FR-SHARED-2): fleet-service marshals an unset nickname as "",
  // and a whitespace-only nickname is user-enterable. `??` would let both
  // through and render a blank title.
  it('falls through a blank nickname to year make model', () => {
    expect(vehicleTitle(attrs({ nickname: '   ' }))).toBe('2019 Ford F-150');
  });

  it('falls through an empty-string nickname to year make model', () => {
    expect(vehicleTitle(attrs({ nickname: '' }))).toBe('2019 Ford F-150');
  });

  it('does not throw when the nickname is absent', () => {
    expect(vehicleTitle(attrs())).toBe('2019 Ford F-150');
  });
});
