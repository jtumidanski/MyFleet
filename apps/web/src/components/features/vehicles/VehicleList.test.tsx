import { describe, it, expect } from 'vitest';
import { screen } from '@testing-library/react';
import { renderWithProviders } from '../../../test/renderWithProviders';
import { VehicleList } from './VehicleList';
import type { Vehicle } from '../../../types/models/vehicle';

function makeVehicle(id: string, model: string): Vehicle {
  return {
    type: 'vehicles',
    id,
    attributes: { fleetId: 'f1', make: 'Honda', model, year: 2019 },
  };
}

describe('VehicleList — empty state', () => {
  it('invites the user to add one and renders the action it was given', () => {
    renderWithProviders(
      <VehicleList
        vehicles={[]}
        isLoading={false}
        emptyAction={<button type="button">Add Vehicle</button>}
      />,
    );

    expect(
      screen.getByText('No vehicles yet. Add your first one to get started.'),
    ).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Add Vehicle' })).toBeInTheDocument();
  });

  it('states the fact without a call to action when it has no action to offer', () => {
    // A viewer cannot add a vehicle, so "add your first one" would be an
    // instruction they cannot follow. Keying the copy off the prop rather than
    // off a role makes it impossible to promise an action that is not rendered.
    renderWithProviders(<VehicleList vehicles={[]} isLoading={false} />);

    expect(screen.getByText('No vehicles yet.')).toBeInTheDocument();
    expect(screen.queryByText(/Add your first one/)).not.toBeInTheDocument();
    expect(screen.queryByRole('button')).not.toBeInTheDocument();
  });
});

describe('VehicleList — populated', () => {
  it('renders a card per vehicle and no empty-state copy', () => {
    renderWithProviders(
      <VehicleList
        vehicles={[makeVehicle('v1', 'Civic'), makeVehicle('v2', 'Accord')]}
        isLoading={false}
        emptyAction={<button type="button">Add Vehicle</button>}
      />,
    );

    expect(screen.getByRole('link', { name: '2019 Honda Civic' })).toBeInTheDocument();
    expect(screen.getByRole('link', { name: '2019 Honda Accord' })).toBeInTheDocument();
    expect(screen.queryByText(/No vehicles yet/)).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Add Vehicle' })).not.toBeInTheDocument();
  });
});

describe('VehicleList — loading', () => {
  it('shows skeletons and no empty-state copy while loading', () => {
    // The empty-state branch must stay behind the loading branch: an empty
    // array during the first fetch is "not known yet", not "none exist".
    renderWithProviders(
      <VehicleList
        vehicles={[]}
        isLoading
        emptyAction={<button type="button">Add Vehicle</button>}
      />,
    );

    expect(screen.queryByText(/No vehicles yet/)).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Add Vehicle' })).not.toBeInTheDocument();
  });
});
