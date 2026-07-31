import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { screen } from '@testing-library/react';
import { renderWithProviders } from '../../../test/renderWithProviders';
import { stubObjectUrl } from '../../../test/objectUrl';
import { mediaService } from '../../../services/api/MediaService';
import { getRuntimeConfig } from '../../../lib/config/runtimeConfig';
import { VehicleCard } from './VehicleCard';
import type { Vehicle } from '../../../types/models/vehicle';

vi.mock('../../../services/api/MediaService', () => ({
  mediaService: { getContentBlob: vi.fn() },
}));

vi.mock('../../../lib/config/runtimeConfig', () => ({
  getRuntimeConfig: vi.fn(),
}));

const TEMPLATE = 'https://www.carfax.com/VehicleHistory/p/Report.cfx?vin={vin}';

function makeVehicle(attrs: Partial<Vehicle['attributes']> = {}): Vehicle {
  return {
    type: 'vehicles',
    id: 'v1',
    attributes: {
      fleetId: 'f1',
      make: 'Honda',
      model: 'Civic',
      year: 2019,
      ...attrs,
    },
  };
}

beforeEach(() => {
  stubObjectUrl();
  vi.mocked(getRuntimeConfig).mockReturnValue({ carfaxUrlTemplate: TEMPLATE });
  vi.mocked(mediaService.getContentBlob).mockResolvedValue(new Blob(['x']));
});

afterEach(() => {
  vi.unstubAllGlobals();
  vi.clearAllMocks();
});

describe('VehicleCard', () => {
  it('renders the vehicle photo when one is set', async () => {
    renderWithProviders(<VehicleCard vehicle={makeVehicle({ primaryImageMediaId: 'm1' })} />);
    expect(await screen.findByAltText('Photo of 2019 Honda Civic')).toBeInTheDocument();
  });

  it('renders the placeholder when no photo is set', () => {
    renderWithProviders(<VehicleCard vehicle={makeVehicle()} />);
    expect(screen.getByRole('img', { name: 'No photo' })).toBeInTheDocument();
    expect(mediaService.getContentBlob).not.toHaveBeenCalled();
  });

  it('navigates to the detail page through a real anchor named for the vehicle', () => {
    // A real <a href> is what preserves middle-click and cmd/ctrl-click; an
    // onClick handler on a <button> would not. And the label must name the
    // vehicle — a grid of "Open details" buttons is unusable with a screen
    // reader.
    renderWithProviders(<VehicleCard vehicle={makeVehicle()} />);

    const link = screen.getByRole('link', { name: 'Open details for 2019 Honda Civic' });
    expect(link).toHaveAttribute('href', '/vehicles/v1');
  });

  it('uses the nickname in the labels when the vehicle has one', () => {
    renderWithProviders(<VehicleCard vehicle={makeVehicle({ nickname: 'Daily Driver' })} />);
    expect(screen.getByRole('link', { name: 'Open details for Daily Driver' })).toBeInTheDocument();
  });

  it('does not make the card body or the thumbnail clickable', () => {
    renderWithProviders(<VehicleCard vehicle={makeVehicle()} />);

    // Exactly one link: the detail button. No wrapping card link, and no VIN so
    // no Carfax link either.
    expect(screen.getAllByRole('link')).toHaveLength(1);
    expect(screen.getByRole('img', { name: 'No photo' }).closest('a')).toBeNull();
  });

  it('renders a Carfax link with the VIN substituted when a VIN is present', () => {
    renderWithProviders(<VehicleCard vehicle={makeVehicle({ vin: '1HGCM82633A004352' })} />);

    const carfax = screen.getByRole('link', {
      name: 'View Carfax report for 2019 Honda Civic (opens in a new tab)',
    });
    expect(carfax).toHaveAttribute(
      'href',
      'https://www.carfax.com/VehicleHistory/p/Report.cfx?vin=1HGCM82633A004352',
    );
    expect(carfax).toHaveAttribute('target', '_blank');
    // noopener blocks window.opener access from the opened page; noreferrer
    // stops MyFleet being sent as the referrer alongside the VIN.
    expect(carfax).toHaveAttribute('rel', 'noopener noreferrer');
  });

  it('omits the Carfax button entirely when there is no VIN', () => {
    // Omitted, not disabled: a disabled button would occupy space and attract
    // focus for no reason.
    renderWithProviders(<VehicleCard vehicle={makeVehicle({ vin: '   ' })} />);
    expect(screen.queryByRole('link', { name: /Carfax/ })).not.toBeInTheDocument();
    expect(screen.getAllByRole('link')).toHaveLength(1);
  });

  it('omits the Carfax button when the configured template ignores the VIN', () => {
    vi.mocked(getRuntimeConfig).mockReturnValue({ carfaxUrlTemplate: 'https://www.carfax.com/' });

    renderWithProviders(<VehicleCard vehicle={makeVehicle({ vin: '1HGCM82633A004352' })} />);
    expect(screen.queryByRole('link', { name: /Carfax/ })).not.toBeInTheDocument();
  });

  it('keeps the detail button first and in the same place with or without a VIN', () => {
    const { unmount } = renderWithProviders(<VehicleCard vehicle={makeVehicle()} />);
    expect(screen.getAllByRole('link')[0]).toHaveAttribute('href', '/vehicles/v1');
    unmount();

    renderWithProviders(<VehicleCard vehicle={makeVehicle({ vin: 'ABC' })} />);
    const links = screen.getAllByRole('link');
    expect(links).toHaveLength(2);
    expect(links[0]).toHaveAttribute('href', '/vehicles/v1');
    expect(links[1]).toHaveAttribute('href', expect.stringContaining('carfax.com'));
  });

  it('still shows the title, subtitle, status, and mileage', () => {
    renderWithProviders(
      <VehicleCard
        vehicle={makeVehicle({ trim: 'Sport', currentMileage: 42000, status: 'Healthy' })}
      />,
    );

    expect(screen.getByText('2019 Honda Civic Sport')).toBeInTheDocument();
    expect(screen.getByText('Healthy')).toBeInTheDocument();
    expect(screen.getByText(/42,000/)).toBeInTheDocument();
  });
});
