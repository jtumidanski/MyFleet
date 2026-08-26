import { describe, it, expect } from 'vitest';
import {
  collectActivityUserIds,
  describeActivityEvent,
  getActivityEventLabel,
} from './activityEventMeta';
import type { ActivityEvent } from '../../../types/models/activity';

function event(
  type: string,
  payload?: Record<string, unknown>,
  extra: Partial<ActivityEvent['attributes']> = {},
): ActivityEvent {
  return {
    id: `e-${type}`,
    type: 'activityEvents',
    attributes: {
      fleetId: 'f1',
      actorUserId: 'u-actor',
      type,
      payload,
      createdAt: '2026-08-01T12:00:00Z',
      ...extra,
    },
  };
}

const ctx = {
  resolveUser: (id: string) => (id === 'u-target' ? 'Dana Reed' : id),
  resolveVehicle: (id: string) => (id === 'v1' ? 'The Wagon' : undefined),
};

describe('getActivityEventLabel', () => {
  // These four are the entire membership vocabulary fleet-service records.
  // Missing any of them is what made the feed a wall of undifferentiated
  // "Event" rows.
  it.each([
    ['member.invited', 'Member joined by invite'],
    ['member.role_changed', 'Member role changed'],
    ['member.left', 'Member left the fleet'],
    ['member.removed', 'Member removed from the fleet'],
    ['schedule.overdue', 'Maintenance overdue'],
    // Both halves of an admin vehicle transfer. FR-XFER-SRC-4 puts these in
    // both fleets' user-facing feeds; the source fleet's row is the only
    // record that the car left, and it used to read "Event".
    ['vehicle.transferred_out', 'Vehicle transferred out'],
    ['vehicle.transferred_in', 'Vehicle transferred in'],
  ])('labels %s', (type, expected) => {
    expect(getActivityEventLabel(type)).toBe(expected);
  });

  it('falls back for a type it has never seen', () => {
    expect(getActivityEventLabel('something.new')).toBe('Event');
  });
});

describe('collectActivityUserIds', () => {
  it('collects actors and payload targets', () => {
    const ids = collectActivityUserIds([
      event('member.removed', { target_user_id: 'u-target' }),
      event('member.left', { role: 'member' }),
    ]);

    expect(ids).toContain('u-actor');
    expect(ids).toContain('u-target');
  });

  it('never asks the user endpoint about the system actor', () => {
    const ids = collectActivityUserIds([
      event('schedule.overdue', { severity: 'overdue' }, { actorUserId: 'system' }),
    ]);

    expect(ids).toEqual([]);
  });
});

describe('describeActivityEvent', () => {
  it('names the member and both roles on a role change', () => {
    const details = describeActivityEvent(
      event('member.role_changed', {
        target_user_id: 'u-target',
        from_role: 'member',
        to_role: 'owner',
      }),
      ctx,
    );

    expect(details).toEqual([
      { term: 'Member', value: 'Dana Reed' },
      { term: 'Role', value: 'member → owner' },
    ]);
  });

  it('surfaces the invited address and role', () => {
    const details = describeActivityEvent(
      event('member.invited', {
        invite_id: 'i1',
        email: 'dana@example.com',
        role: 'member',
      }),
      ctx,
    );

    expect(details).toEqual([
      { term: 'Invited', value: 'dana@example.com' },
      { term: 'Role', value: 'member' },
    ]);
    // The invite id is an identifier, not information — it must not take up a
    // line of the timeline.
    expect(JSON.stringify(details)).not.toContain('i1');
  });

  it('formats fuel amounts rather than dumping the raw payload', () => {
    const details = describeActivityEvent(
      event(
        'fuel.logged',
        { fuel_log_id: 'x', mileage: 84210, total_cost: 52.4 },
        { vehicleId: 'v1' },
      ),
      ctx,
    );

    expect(details).toContainEqual({ term: 'Odometer', value: '84,210 mi' });
    expect(details).toContainEqual({ term: 'Cost', value: '$52.40' });
  });

  it('prefers a live vehicle name over the id', () => {
    const details = describeActivityEvent(event('mileage.recorded', {}, { vehicleId: 'v1' }), ctx);

    expect(details).toContainEqual({ term: 'Vehicle', value: 'The Wagon' });
  });

  it('falls back to the name frozen in the payload for a vehicle that is gone', () => {
    const details = describeActivityEvent(
      event(
        'vehicle.created',
        { nickname: 'Old Truck', make: 'Ford', model: 'F-150' },
        {
          vehicleId: 'v-deleted',
        },
      ),
      ctx,
    );

    expect(details).toContainEqual({ term: 'Vehicle', value: 'Old Truck' });
    expect(details).toContainEqual({ term: 'Make & model', value: 'Ford F-150' });
  });

  it('uses the raw id only when nothing else is known', () => {
    const details = describeActivityEvent(
      event('mileage.recorded', {}, { vehicleId: 'v-gone' }),
      ctx,
    );

    expect(details).toContainEqual({ term: 'Vehicle', value: 'v-gone' });
  });

  // The source fleet's transferred_out row names a vehicle that is no longer in
  // that fleet, so resolveVehicle cannot help and the frozen vehicle_label is
  // the only readable name there is.
  it('names the vehicle from the transfer payload once it has left the fleet', () => {
    const details = describeActivityEvent(
      event(
        'vehicle.transferred_out',
        { counterpart_fleet_id: 'f-other', vehicle_label: '2020 Toyota Corolla' },
        { vehicleId: 'v-moved' },
      ),
      ctx,
    );

    expect(details).toContainEqual({ term: 'Vehicle', value: '2020 Toyota Corolla' });
    // The counterpart fleet id is a UUID, and a UUID on a timeline tells the
    // reader nothing while displacing what does.
    expect(JSON.stringify(details)).not.toContain('f-other');
  });

  it('prefers the live vehicle name over the frozen transfer label', () => {
    const details = describeActivityEvent(
      event(
        'vehicle.transferred_in',
        { counterpart_fleet_id: 'f-other', vehicle_label: '2020 Toyota Corolla' },
        { vehicleId: 'v1' },
      ),
      ctx,
    );

    expect(details).toContainEqual({ term: 'Vehicle', value: 'The Wagon' });
  });

  it('renders nothing extra for an event with no payload', () => {
    expect(describeActivityEvent(event('member.left'), ctx)).toEqual([]);
  });
});
