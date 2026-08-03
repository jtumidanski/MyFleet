import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen } from '@testing-library/react';
import { FleetNameCrumb } from './FleetNameCrumb';

const mockUseAdminFleet = vi.fn();
vi.mock('../../../lib/hooks/api/admin', () => ({
  useAdminFleet: (id: string | undefined) => mockUseAdminFleet(id),
}));

const ID = '3fa85f64-5717-4562-b3fc-2c963f66afa6';

describe('FleetNameCrumb', () => {
  beforeEach(() => {
    mockUseAdminFleet.mockReset();
  });

  it('shows a skeleton while the lookup is in flight', () => {
    mockUseAdminFleet.mockReturnValue({ data: undefined, isLoading: true });
    const { container } = render(<FleetNameCrumb id={ID} />);

    expect(container.querySelector('.animate-pulse')).toBeInTheDocument();
    expect(screen.queryByText(ID)).not.toBeInTheDocument();
  });

  // AdminService.getFleet returns doc.data — a JsonApiResource — so the name is
  // on .attributes, not on the resource itself (design §6.4).
  it('shows the fleet name from the resource attributes', () => {
    mockUseAdminFleet.mockReturnValue({
      data: { id: ID, type: 'fleets', attributes: { name: 'The Lovelace Household' } },
      isLoading: false,
    });
    render(<FleetNameCrumb id={ID} />);

    expect(screen.getByText('The Lovelace Household')).toBeInTheDocument();
  });

  it('falls back to the raw id when the lookup fails', () => {
    mockUseAdminFleet.mockReturnValue({ data: undefined, isLoading: false });
    render(<FleetNameCrumb id={ID} />);

    expect(screen.getByText(ID)).toBeInTheDocument();
  });
});
