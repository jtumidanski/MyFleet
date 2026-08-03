import { describe, it, expect, vi, beforeEach } from 'vitest';
import { screen, waitFor } from '@testing-library/react';
import { ActivityFeed } from './ActivityFeed';
import { renderWithProviders } from '../../../test/renderWithProviders';
import type { ActivityEvent } from '../../../types/models/activity';

const { listByIds } = vi.hoisted(() => ({ listByIds: vi.fn() }));

vi.mock('../../../services/api/UserService', () => ({
  userService: { listByIds },
}));

function event(
  type: string,
  attrs: Partial<ActivityEvent['attributes']> = {},
  payload?: Record<string, unknown>,
): ActivityEvent {
  return {
    id: `e-${type}-${attrs.actorUserId ?? 'a'}`,
    type: 'activityEvents',
    attributes: {
      fleetId: 'f1',
      actorUserId: 'u-actor',
      type,
      payload,
      createdAt: '2026-08-01T12:00:00Z',
      ...attrs,
    },
  };
}

const feedProps = {
  isLoading: false,
  page: 1,
  totalPages: 1,
  hasNextPage: false,
  onPrev: vi.fn(),
  onNext: vi.fn(),
};

beforeEach(() => {
  // Cleared, not just re-stubbed: the "asked for no names" assertions below
  // read call counts, which would otherwise accumulate across tests.
  listByIds.mockReset();
  listByIds.mockResolvedValue({
    data: [
      { id: 'u-actor', type: 'users', attributes: { displayName: 'Dana Reed', email: 'd@x.co' } },
    ],
  });
});

describe('ActivityFeed', () => {
  it('names the actor once the lookup resolves', async () => {
    renderWithProviders(
      <ActivityFeed {...feedProps} events={[event('member.left', {}, { role: 'member' })]} />,
    );

    expect(await screen.findByText(/Dana Reed/)).toBeInTheDocument();
  });

  it('degrades to a shortened id when the name lookup fails', async () => {
    // FR-1.7's whole point: "events loaded, names failed" must stay renderable.
    listByIds.mockRejectedValue(new Error('users unavailable'));

    renderWithProviders(
      <ActivityFeed {...feedProps} events={[event('member.left', {}, { role: 'member' })]} />,
    );

    expect(await screen.findByText(/Member left the fleet/)).toBeInTheDocument();
    await waitFor(() => expect(screen.getByText(/u-actor/)).toBeInTheDocument());
  });

  it('renders the system actor as MyFleet without looking it up', async () => {
    renderWithProviders(
      <ActivityFeed
        {...feedProps}
        events={[
          event(
            'schedule.overdue',
            { actorUserId: 'system', vehicleId: 'v1' },
            { severity: 'overdue' },
          ),
        ]}
        vehicleNames={{ v1: 'The Wagon' }}
      />,
    );

    expect(await screen.findByText(/MyFleet/)).toBeInTheDocument();
    // `system` is not a user id; sending it would put a bogus id in the request.
    const asked = listByIds.mock.calls.flatMap((c) => c[0] as string[]);
    expect(asked).not.toContain('system');
  });

  it('shows the vehicle name rather than its id', async () => {
    renderWithProviders(
      <ActivityFeed
        {...feedProps}
        events={[event('fuel.logged', { vehicleId: 'v1' }, { mileage: 84210, total_cost: 52.4 })]}
        vehicleNames={{ v1: 'The Wagon' }}
      />,
    );

    expect(await screen.findByText('The Wagon')).toBeInTheDocument();
    expect(screen.getByText('84,210 mi')).toBeInTheDocument();
    expect(screen.queryByText('v1')).not.toBeInTheDocument();
  });

  // A vehicle-scoped timeline passes no vehicleNames, so an unsuppressed line
  // would fall through to the raw uuid on every fuel/mileage row.
  it('omits the vehicle line entirely when scoped to one vehicle', async () => {
    renderWithProviders(
      <ActivityFeed
        {...feedProps}
        showVehicle={false}
        events={[
          event(
            'fuel.logged',
            { vehicleId: 'a3f1c0de-0000-4000-8000-000000000001' },
            { mileage: 100 },
          ),
        ]}
      />,
    );

    expect(await screen.findByText(/Fuel logged/)).toBeInTheDocument();
    expect(screen.queryByText('Vehicle')).not.toBeInTheDocument();
    expect(screen.queryByText(/a3f1c0de-/)).not.toBeInTheDocument();
  });

  it('renders the empty state without asking for any names', async () => {
    renderWithProviders(<ActivityFeed {...feedProps} events={[]} />);

    expect(screen.getByText(/no activity yet/i)).toBeInTheDocument();
    expect(listByIds).not.toHaveBeenCalled();
  });
});
