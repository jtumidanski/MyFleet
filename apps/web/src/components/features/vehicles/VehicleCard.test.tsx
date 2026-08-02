import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { screen, act } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { useLocation } from 'react-router-dom';
import { renderWithProviders } from '../../../test/renderWithProviders';
import { stubObjectUrl, unstubObjectUrl } from '../../../test/objectUrl';
import { mediaService } from '../../../services/api/MediaService';
import { DEFAULT_RUNTIME_CONFIG, loadRuntimeConfig } from '../../../lib/config/runtimeConfig';
import { VehicleCard } from './VehicleCard';
import type { Vehicle } from '../../../types/models/vehicle';

vi.mock('../../../services/api/MediaService', () => ({
  mediaService: { getContentBlob: vi.fn() },
}));

// The runtime-config module is deliberately NOT mocked. Its observability is the
// behaviour under test — a mocked getter would happily satisfy every assertion
// here while the real app silently ignored its ConfigMap.
const TEMPLATE = DEFAULT_RUNTIME_CONFIG.carfaxUrlTemplate;

/** Serves one config document and latches it into the real module store. */
async function latchConfig(carfaxUrlTemplate: string): Promise<void> {
  vi.stubGlobal(
    'fetch',
    vi.fn(async () => new Response(JSON.stringify({ carfaxUrlTemplate }), { status: 200 })),
  );
  await loadRuntimeConfig();
}

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

beforeEach(async () => {
  stubObjectUrl();
  // The latch is module state and survives between tests, so reset it to a
  // known value rather than depending on what ran before.
  await latchConfig(TEMPLATE);
  vi.mocked(mediaService.getContentBlob).mockResolvedValue(new Blob(['x']));
});

afterEach(() => {
  unstubObjectUrl();
  vi.clearAllMocks();
});

/**
 * Renders the router's current pathname so a test can assert navigation.
 *
 * Assert on `.textContent` with `toBe`, never `toHaveTextContent('/')` — that
 * matcher does a substring match, and '/vehicles/v1' contains '/', so the
 * "did NOT navigate" assertion would pass even when the bug is present.
 */
function LocationProbe() {
  return <span data-testid="pathname">{useLocation().pathname}</span>;
}

describe('VehicleCard — photo', () => {
  it('renders the vehicle photo when one is set', async () => {
    renderWithProviders(<VehicleCard vehicle={makeVehicle({ primaryImageMediaId: 'm1' })} />);
    expect(await screen.findByAltText('Photo of 2019 Honda Civic')).toBeInTheDocument();
  });

  it('renders the hero at a 16:9 aspect ratio', async () => {
    renderWithProviders(<VehicleCard vehicle={makeVehicle({ primaryImageMediaId: 'm1' })} />);
    expect(await screen.findByAltText('Photo of 2019 Honda Civic')).toHaveClass(
      'aspect-[16/9]',
      'w-full',
    );
  });

  it('renders the placeholder at identical dimensions when no photo is set', () => {
    renderWithProviders(<VehicleCard vehicle={makeVehicle()} />);
    const placeholder = screen.getByRole('img', { name: 'No photo' });
    expect(placeholder).toHaveClass('aspect-[16/9]', 'w-full');
    expect(mediaService.getContentBlob).not.toHaveBeenCalled();
  });

  it('renders the placeholder with no error tile and no toast when the photo fails', async () => {
    vi.mocked(mediaService.getContentBlob).mockRejectedValue(new Error('nope'));
    renderWithProviders(<VehicleCard vehicle={makeVehicle({ primaryImageMediaId: 'm1' })} />);

    expect(await screen.findByRole('img', { name: 'Photo unavailable' })).toBeInTheDocument();
    expect(screen.queryByText(/failed/i)).not.toBeInTheDocument();
  });
});

describe('VehicleCard — banner', () => {
  const banner = () => screen.getByTestId('vehicle-card-banner');

  it('tints an overdue vehicle in danger and states the overrun', () => {
    renderWithProviders(
      <VehicleCard
        vehicle={makeVehicle({
          status: 'Overdue',
          nextDue: { state: 'overdue', axis: 'mileage', miles: 1120 },
        })}
      />,
    );
    expect(screen.getByText('Service overdue by 1,120 mi')).toBeInTheDocument();
    expect(banner()).toHaveClass('bg-danger-subtle', 'text-danger-subtle-foreground');
  });

  it('tints an upcoming vehicle in warning and states the remaining distance', () => {
    renderWithProviders(
      <VehicleCard
        vehicle={makeVehicle({
          status: 'Upcoming Maintenance',
          nextDue: { state: 'upcoming', axis: 'mileage', miles: 310 },
        })}
      />,
    );
    expect(screen.getByText('Service due in 310 mi')).toBeInTheDocument();
    expect(banner()).toHaveClass('bg-warning-subtle', 'text-warning-subtle-foreground');
  });

  it('leaves a healthy vehicle untinted', () => {
    renderWithProviders(<VehicleCard vehicle={makeVehicle({ status: 'Healthy' })} />);
    expect(screen.getByText('Up to date')).toBeInTheDocument();
    expect(banner()).toHaveClass('text-muted-foreground');
    expect(banner().className).not.toMatch(/bg-(danger|warning|success)-subtle/);
  });

  it('leaves an inactive vehicle untinted and states the dormancy', () => {
    const longAgo = new Date(Date.now() - 400 * 24 * 60 * 60 * 1000).toISOString();
    renderWithProviders(
      <VehicleCard vehicle={makeVehicle({ status: 'Inactive', lastActivityAt: longAgo })} />,
    );
    expect(screen.getByText(/No activity in \d+ months/)).toBeInTheDocument();
    expect(banner().className).not.toMatch(/bg-(danger|warning|success)-subtle/);
  });

  it('renders a time-axis schedule as a day count, never as mileage', () => {
    renderWithProviders(
      <VehicleCard
        vehicle={makeVehicle({
          status: 'Overdue',
          nextDue: { state: 'overdue', axis: 'time', days: 12 },
        })}
      />,
    );
    expect(screen.getByText('Service overdue by 12 days')).toBeInTheDocument();
    expect(screen.queryByText(/mi$/)).not.toBeInTheDocument();
  });

  it('renders the quiet banner when status is absent', () => {
    renderWithProviders(<VehicleCard vehicle={makeVehicle()} />);
    expect(screen.getByText('Status unavailable')).toBeInTheDocument();
    expect(banner().className).not.toMatch(/bg-(danger|warning)-subtle/);
  });

  it('renders the quiet banner for an unrecognised status rather than crashing', () => {
    renderWithProviders(<VehicleCard vehicle={makeVehicle({ status: 'Exploded' })} />);
    expect(screen.getByText('Status unavailable')).toBeInTheDocument();
    expect(screen.queryByText('Exploded')).not.toBeInTheDocument();
  });

  it('stays tinted with generic copy when an overdue vehicle has no due detail', () => {
    // Urgency has to survive missing detail — this is the case a naive
    // "render nothing without nextDue" would get silently wrong.
    renderWithProviders(<VehicleCard vehicle={makeVehicle({ status: 'Overdue' })} />);
    expect(screen.getByText('Maintenance overdue')).toBeInTheDocument();
    expect(banner()).toHaveClass('bg-danger-subtle');
  });

  it('carries an icon alongside the text so colour is never the only signal', () => {
    renderWithProviders(<VehicleCard vehicle={makeVehicle({ status: 'Overdue' })} />);
    expect(banner().querySelector('svg')).toBeInTheDocument();
  });

  it('truncates long banner text instead of wrapping', () => {
    renderWithProviders(<VehicleCard vehicle={makeVehicle({ status: 'Healthy' })} />);
    expect(screen.getByText('Up to date')).toHaveClass('truncate');
  });

  it('does not render a StatusBadge anywhere on the card', () => {
    // The banner replaces it. A badge alongside would restate the status without
    // the reason and reintroduce colour on healthy cards.
    renderWithProviders(<VehicleCard vehicle={makeVehicle({ status: 'Healthy' })} />);
    expect(screen.queryByText('Healthy')).not.toBeInTheDocument();
  });
});

describe('VehicleCard — stat strip', () => {
  it('shows odometer and last activity with tabular figures', () => {
    const sixDaysAgo = new Date(Date.now() - 6 * 24 * 60 * 60 * 1000).toISOString();
    renderWithProviders(
      <VehicleCard vehicle={makeVehicle({ currentMileage: 42000, lastActivityAt: sixDaysAgo })} />,
    );
    expect(screen.getByText('Odometer')).toBeInTheDocument();
    expect(screen.getByText('Last activity')).toBeInTheDocument();

    const odometer = screen.getByText('42,000 mi');
    expect(odometer).toHaveClass('tabular-nums');
    expect(screen.getByText('6 days ago')).toBeInTheDocument();
  });

  it('renders an em-dash and keeps the slot when mileage is missing', () => {
    renderWithProviders(<VehicleCard vehicle={makeVehicle({ lastActivityAt: undefined })} />);
    expect(screen.getByText('Odometer')).toBeInTheDocument();
    expect(screen.getAllByText('—')).toHaveLength(2);
  });

  it('renders an em-dash when last activity is missing', () => {
    renderWithProviders(<VehicleCard vehicle={makeVehicle({ currentMileage: 42000 })} />);
    expect(screen.getByText('42,000 mi')).toBeInTheDocument();
    expect(screen.getAllByText('—')).toHaveLength(1);
  });

  it('renders an em-dash when last activity is unparseable', () => {
    renderWithProviders(
      <VehicleCard vehicle={makeVehicle({ currentMileage: 1, lastActivityAt: 'nope' })} />,
    );
    expect(screen.getAllByText('—')).toHaveLength(1);
  });

  it('does not show a next-service figure in the strip', () => {
    // The banner already states it where it matters; a third slot would read
    // "—" on every healthy card.
    renderWithProviders(
      <VehicleCard
        vehicle={makeVehicle({
          status: 'Overdue',
          nextDue: { state: 'overdue', axis: 'mileage', miles: 1120 },
        })}
      />,
    );
    expect(screen.queryByText('Next service')).not.toBeInTheDocument();
  });
});

describe('VehicleCard — navigation', () => {
  it('makes the title a real anchor to the detail page', () => {
    // A real <a href> is what preserves middle-click, cmd/ctrl-click, and the
    // link context menu; an onClick handler on a div would not.
    renderWithProviders(<VehicleCard vehicle={makeVehicle()} />);
    const link = screen.getByRole('link', { name: '2019 Honda Civic' });
    expect(link).toHaveAttribute('href', '/vehicles/v1');
  });

  it('names the card link with the nickname when the vehicle has one', () => {
    renderWithProviders(<VehicleCard vehicle={makeVehicle({ nickname: 'Daily Driver' })} />);
    expect(screen.getByRole('link', { name: 'Daily Driver' })).toBeInTheDocument();
  });

  it('spans the whole card with an overlay pseudo-element rather than wrapping it', () => {
    // FR-6.1/6.2: the anchor must never be a DOM ancestor of another interactive
    // element. The overlay is what makes the whole card clickable without that.
    renderWithProviders(<VehicleCard vehicle={makeVehicle()} />);
    const link = screen.getByRole('link', { name: '2019 Honda Civic' });
    expect(link.className).toContain('after:absolute');
    expect(link.className).toContain('after:inset-0');
    expect(screen.getByRole('img', { name: 'No photo' }).closest('a')).toBeNull();
  });

  it('navigates to the detail page when the card link is activated', async () => {
    renderWithProviders(
      <>
        <VehicleCard vehicle={makeVehicle()} />
        <LocationProbe />
      </>,
    );
    expect(screen.getByTestId('pathname').textContent).toBe('/');

    await userEvent.click(screen.getByRole('link', { name: '2019 Honda Civic' }));
    expect(screen.getByTestId('pathname').textContent).toBe('/vehicles/v1');
  });

  it('no longer renders a separate chevron detail button', () => {
    renderWithProviders(<VehicleCard vehicle={makeVehicle()} />);
    expect(screen.queryByRole('link', { name: /Open details/ })).not.toBeInTheDocument();
    expect(screen.getAllByRole('link')).toHaveLength(1);
  });

  it('puts the card link before Carfax in DOM order', () => {
    renderWithProviders(<VehicleCard vehicle={makeVehicle({ vin: '1HGCM82633A004352' })} />);
    const links = screen.getAllByRole('link');
    expect(links).toHaveLength(2);
    expect(links[0]).toHaveAttribute('href', '/vehicles/v1');
    expect(links[1]).toHaveAttribute('href', expect.stringContaining('carfax.com'));
  });
});

describe('VehicleCard — Carfax', () => {
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
    renderWithProviders(<VehicleCard vehicle={makeVehicle({ vin: '   ' })} />);
    expect(screen.queryByRole('link', { name: /Carfax/ })).not.toBeInTheDocument();
    expect(screen.getAllByRole('link')).toHaveLength(1);
  });

  it('omits the Carfax button when the configured template ignores the VIN', async () => {
    await latchConfig('https://www.carfax.com/');
    renderWithProviders(<VehicleCard vehicle={makeVehicle({ vin: '1HGCM82633A004352' })} />);
    expect(screen.queryByRole('link', { name: /Carfax/ })).not.toBeInTheDocument();
  });

  it('picks up a runtime config that arrives AFTER the card has mounted', async () => {
    // main.tsx does not block the mount on the config fetch, so this is the
    // ordering that actually happens in production. If the card read the module
    // getter directly instead of subscribing, a ConfigMap override would
    // silently never take effect.
    renderWithProviders(<VehicleCard vehicle={makeVehicle({ vin: '1HGCM82633A004352' })} />);
    expect(screen.getByRole('link', { name: /Carfax/ })).toHaveAttribute(
      'href',
      'https://www.carfax.com/VehicleHistory/p/Report.cfx?vin=1HGCM82633A004352',
    );

    await act(async () => {
      await latchConfig('https://example.test/report?vin={vin}');
    });

    expect(screen.getByRole('link', { name: /Carfax/ })).toHaveAttribute(
      'href',
      'https://example.test/report?vin=1HGCM82633A004352',
    );
  });

  it('drops the Carfax button when a late config makes the template unusable', async () => {
    renderWithProviders(<VehicleCard vehicle={makeVehicle({ vin: '1HGCM82633A004352' })} />);
    expect(screen.getByRole('link', { name: /Carfax/ })).toBeInTheDocument();

    await act(async () => {
      await latchConfig('https://www.carfax.com/');
    });

    expect(screen.queryByRole('link', { name: /Carfax/ })).not.toBeInTheDocument();
  });

  it('is not nested inside the card link and sits above the overlay', () => {
    // The FR-6.6 regression this layout risks. Two structural guards: the
    // Carfax anchor has no ancestor anchor (so a click can never be the card
    // link's), and its wrapper is lifted above the overlay pseudo-element.
    //
    // jsdom does no layout, so it cannot prove the stacking actually works —
    // that check lives in manual-verification.md. What it CAN prove is that the
    // two structural preconditions are in place.
    renderWithProviders(<VehicleCard vehicle={makeVehicle({ vin: '1HGCM82633A004352' })} />);
    const carfax = screen.getByRole('link', { name: /Carfax/ });

    expect(carfax.parentElement?.closest('a')).toBeNull();
    expect(carfax.className).toContain('z-10');
  });

  it('does not navigate to the detail page when Carfax is clicked', async () => {
    renderWithProviders(
      <>
        <VehicleCard vehicle={makeVehicle({ vin: '1HGCM82633A004352' })} />
        <LocationProbe />
      </>,
    );

    await userEvent.click(screen.getByRole('link', { name: /Carfax/ }));
    expect(screen.getByTestId('pathname').textContent).toBe('/');
  });
});
