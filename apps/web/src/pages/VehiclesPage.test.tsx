import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { screen, waitFor, within, fireEvent } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { renderWithProviders } from '../test/renderWithProviders';
import { vehicleService } from '../services/api/VehicleService';
import { toast } from 'sonner';
import type { AuthContextValue } from '../context/AuthContext';
import { VehiclesPage } from './VehiclesPage';
import type { Vehicle } from '../types/models/vehicle';

// Mock auth so role and fleet can be varied per test without standing up the
// provider stack — the pattern AppLayout.test.tsx established.
const mockAuth = vi.fn<() => AuthContextValue>();
vi.mock('../context/AuthContext', () => ({
  useAuth: () => mockAuth(),
}));

// Mock at the service boundary, as VehicleCard.test.tsx does, so the real
// query/mutation wiring (keys, invalidation, error normalisation) is exercised.
vi.mock('../services/api/VehicleService', () => ({
  vehicleService: { listByFleet: vi.fn(), createInFleet: vi.fn() },
}));

vi.mock('sonner', () => ({
  toast: { success: vi.fn(), error: vi.fn() },
}));

function makeVehicle(id = 'v1'): Vehicle {
  return {
    type: 'vehicles',
    id,
    attributes: { fleetId: 'f1', make: 'Toyota', model: 'Corolla', year: 2020 },
  };
}

function setRole(role: AuthContextValue['role']): void {
  mockAuth.mockReturnValue({
    user: null,
    activeFleetId: 'f1',
    role,
    isAuthenticated: true,
    isLoading: false,
    login: vi.fn(),
    logout: vi.fn(),
  });
}

/**
 * The page's two triggers and the form's submit button all read "Add Vehicle".
 * This returns only the triggers — the ones outside the dialog — so a test can
 * never accidentally drive the submit button when it means to open the dialog.
 * DOM order puts the header trigger first.
 */
function triggers(): HTMLElement[] {
  return screen
    .getAllByRole('button', { name: 'Add Vehicle' })
    .filter((button) => !button.closest('[role="dialog"]'));
}

const headerTrigger = () => triggers()[0];
const submitButton = () =>
  within(screen.getByRole('dialog')).getByRole('button', { name: 'Add Vehicle' });

/** Fills the three required fields. */
async function fillRequired(): Promise<void> {
  const dialog = within(screen.getByRole('dialog'));
  await userEvent.type(dialog.getByLabelText('Make'), 'Toyota');
  await userEvent.type(dialog.getByLabelText('Model'), 'Corolla');
  await userEvent.type(dialog.getByLabelText('Year'), '2020');
}

beforeEach(() => {
  setRole('owner');
  vi.mocked(vehicleService.listByFleet).mockResolvedValue({ data: [] });
  vi.mocked(vehicleService.createInFleet).mockResolvedValue(makeVehicle());
});

afterEach(() => {
  vi.clearAllMocks();
});

describe('VehiclesPage — opening the dialog', () => {
  it('opens a titled, described dialog containing the create form from the header', async () => {
    renderWithProviders(<VehiclesPage />);
    await userEvent.click(headerTrigger());

    const dialog = screen.getByRole('dialog');
    expect(dialog).toHaveAccessibleName('Add Vehicle');
    expect(dialog).toHaveAccessibleDescription('Make, model, and year are required.');
    expect(within(dialog).getByLabelText('Make')).toBeInTheDocument();
  });

  it('opens the same dialog from the empty state', async () => {
    renderWithProviders(<VehiclesPage />);
    await waitFor(() => expect(triggers()).toHaveLength(2));

    await userEvent.click(triggers()[1]);
    expect(within(screen.getByRole('dialog')).getByLabelText('Make')).toBeInTheDocument();
  });

  it('keeps the header trigger rendered while the dialog is open', async () => {
    // The old inline form unmounted it; the dialog has no reason to. Radix
    // aria-hides the page behind an open modal, which is correct and is why
    // this asserts on the node itself rather than re-querying by role — an
    // accessibility-tree query skips it while the dialog is up.
    renderWithProviders(<VehiclesPage />);
    const trigger = headerTrigger();
    await userEvent.click(trigger);

    expect(trigger).toBeInTheDocument();
    expect(trigger.closest('[aria-hidden="true"]')).not.toBeNull();
  });

  it('leaves no inline card form on the page', async () => {
    renderWithProviders(<VehiclesPage />);
    expect(screen.queryByText('New Vehicle')).not.toBeInTheDocument();
    expect(screen.queryByLabelText('Make')).not.toBeInTheDocument();
  });

  it('offers a viewer neither trigger', async () => {
    setRole('viewer');
    renderWithProviders(<VehiclesPage />);
    await waitFor(() => expect(screen.getByText('No vehicles yet.')).toBeInTheDocument());
    expect(screen.queryByRole('button', { name: 'Add Vehicle' })).not.toBeInTheDocument();
  });
});

describe('VehiclesPage — submitting', () => {
  it('omits blank optionals from the payload, closes, and reports success', async () => {
    // The call arguments are the assertion that matters: toCreateAttributes
    // strips empty-string optionals, and that is exactly the behaviour a
    // refactor drops silently.
    renderWithProviders(<VehiclesPage />);
    await userEvent.click(headerTrigger());
    await fillRequired();
    await userEvent.click(submitButton());

    await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument());
    expect(vehicleService.createInFleet).toHaveBeenCalledWith('f1', {
      make: 'Toyota',
      model: 'Corolla',
      year: 2020,
      nickname: undefined,
      trim: undefined,
      vin: undefined,
      notes: undefined,
      currentMileage: undefined,
    });
    expect(toast.success).toHaveBeenCalledWith('Vehicle added');
  });

  it('shows the created vehicle in the list', async () => {
    vi.mocked(vehicleService.listByFleet)
      .mockResolvedValueOnce({ data: [] })
      .mockResolvedValue({ data: [makeVehicle()] });

    renderWithProviders(<VehiclesPage />);
    await userEvent.click(headerTrigger());
    await fillRequired();
    await userEvent.click(submitButton());

    expect(await screen.findByRole('link', { name: '2020 Toyota Corolla' })).toBeInTheDocument();
  });

  it('keeps the dialog open with inline errors when required fields are blank', async () => {
    renderWithProviders(<VehiclesPage />);
    await userEvent.click(headerTrigger());
    await userEvent.click(submitButton());

    expect(await screen.findByText('Make is required')).toBeInTheDocument();
    expect(screen.getByText('Model is required')).toBeInTheDocument();
    expect(screen.getByRole('dialog')).toBeInTheDocument();
    expect(vehicleService.createInFleet).not.toHaveBeenCalled();
  });

  it('keeps the dialog open with the typed values when the request fails', async () => {
    vi.mocked(vehicleService.createInFleet).mockRejectedValue(new Error('boom'));

    renderWithProviders(<VehiclesPage />);
    await userEvent.click(headerTrigger());
    await fillRequired();
    await userEvent.click(submitButton());

    await waitFor(() => expect(toast.error).toHaveBeenCalled());
    expect(screen.getByRole('dialog')).toBeInTheDocument();
    expect(within(screen.getByRole('dialog')).getByLabelText('Make')).toHaveValue('Toyota');
  });
});

describe('VehiclesPage — dismissing', () => {
  // Typed explicitly: a bare array literal infers a union element type and the
  // callback's `dismiss` parameter stops being callable.
  const dismissals: Array<[string, () => Promise<void>]> = [
    ['Escape', () => userEvent.keyboard('{Escape}')],
    ['the close button', () => userEvent.click(screen.getByRole('button', { name: 'Close' }))],
    [
      'Cancel',
      () =>
        userEvent.click(within(screen.getByRole('dialog')).getByRole('button', { name: 'Cancel' })),
    ],
  ];

  it.each(dismissals)('closes on %s without creating a vehicle', async (_label, dismiss) => {
    renderWithProviders(<VehiclesPage />);
    await userEvent.click(headerTrigger());
    await dismiss();

    await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument());
    expect(vehicleService.createInFleet).not.toHaveBeenCalled();
  });

  it('closes on an outside pointer-down without creating a vehicle', async () => {
    // The path a real overlay click takes; userEvent cannot drive the overlay.
    renderWithProviders(<VehiclesPage />);
    await userEvent.click(headerTrigger());

    fireEvent.pointerDown(document.body);

    await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument());
    expect(vehicleService.createInFleet).not.toHaveBeenCalled();
  });

  it('presents a blank form on reopen', async () => {
    renderWithProviders(<VehiclesPage />);
    await userEvent.click(headerTrigger());
    await userEvent.type(within(screen.getByRole('dialog')).getByLabelText('Nickname'), 'Scratch');
    await userEvent.keyboard('{Escape}');
    await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument());

    await userEvent.click(headerTrigger());
    expect(within(screen.getByRole('dialog')).getByLabelText('Nickname')).toHaveValue('');
  });

  it('returns focus to the header trigger it was opened from', async () => {
    renderWithProviders(<VehiclesPage />);
    const trigger = headerTrigger();
    await userEvent.click(trigger);
    await userEvent.keyboard('{Escape}');

    await waitFor(() => expect(document.activeElement).toBe(trigger));
  });
});
