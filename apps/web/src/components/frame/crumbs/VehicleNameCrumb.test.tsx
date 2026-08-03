import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, resolve } from 'node:path';
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen } from '@testing-library/react';
import { VehicleNameCrumb } from './VehicleNameCrumb';

// Mock the hook rather than standing up a QueryClient: it matches the pattern
// the layout tests already use, and it makes the loading state — otherwise a
// race — trivially reachable.
const mockUseVehicle = vi.fn();
vi.mock('../../../lib/hooks/api/vehicles', () => ({
  useVehicle: (id: string | null | undefined) => mockUseVehicle(id),
}));

const ID = '8f14e45f-ceea-467a-9f8e-1b2c3d4e5f60';

function vehicle(attributes: Record<string, unknown>) {
  return { id: ID, type: 'vehicles', attributes };
}

describe('VehicleNameCrumb', () => {
  beforeEach(() => {
    mockUseVehicle.mockReset();
  });

  // FR-CRUMBNAME-4: a skeleton, not the UUID and not an empty space that makes
  // the trail jump when the name lands.
  it('shows a skeleton while the lookup is in flight', () => {
    mockUseVehicle.mockReturnValue({ data: undefined, isLoading: true });
    const { container } = render(<VehicleNameCrumb id={ID} />);

    expect(container.querySelector('.animate-pulse')).toBeInTheDocument();
    expect(screen.queryByText(ID)).not.toBeInTheDocument();
  });

  it('prefers the nickname', () => {
    mockUseVehicle.mockReturnValue({
      data: vehicle({ nickname: 'Weekend Truck', year: 2019, make: 'Ford', model: 'F-150' }),
      isLoading: false,
    });
    render(<VehicleNameCrumb id={ID} />);

    expect(screen.getByText('Weekend Truck')).toBeInTheDocument();
  });

  it('falls back to year make model when the nickname is blank', () => {
    mockUseVehicle.mockReturnValue({
      data: vehicle({ nickname: '   ', year: 2019, make: 'Ford', model: 'F-150' }),
      isLoading: false,
    });
    render(<VehicleNameCrumb id={ID} />);

    expect(screen.getByText('2019 Ford F-150')).toBeInTheDocument();
  });

  // FR-CRUMBNAME-5: error, 404 and soft-deleted alike fall back to the raw id,
  // chosen deliberately over a generic "Vehicle" label — a crumb that degrades
  // to a category name hides that a lookup failed.
  it('falls back to the raw id when the lookup fails', () => {
    mockUseVehicle.mockReturnValue({ data: undefined, isLoading: false });
    render(<VehicleNameCrumb id={ID} />);

    expect(screen.getByText(ID)).toBeInTheDocument();
  });

  it('passes the id straight through to the hook', () => {
    mockUseVehicle.mockReturnValue({ data: undefined, isLoading: true });
    render(<VehicleNameCrumb id={ID} />);

    expect(mockUseVehicle).toHaveBeenCalledWith(ID);
  });
});

/**
 * FR-CRUMBNAME-2: the crumb's text must match VehicleDetailPage's <h1> exactly.
 * A page titled "Weekend Truck" under a breadcrumb reading "2019 Ford F-150" is
 * a defect.
 *
 * The rule is duplicated deliberately rather than extracted into a shared
 * helper: extracting it means editing VehicleDetailPage during a frame task,
 * and task-015 owns that page's title contract. This is what pays for the
 * duplication — a source-level pin, in the style of src/test/conventions.test.ts.
 */
describe('the vehicle title rule', () => {
  const CRUMBS_DIR = dirname(fileURLToPath(import.meta.url));
  const WEB_ROOT = resolve(CRUMBS_DIR, '../../../..');

  const RULE =
    'attributes.nickname?.trim() || `${attributes.year} ${attributes.make} ${attributes.model}`';

  /** Collapse whitespace so Prettier's line wrapping cannot break the match. */
  function normalized(path: string): string {
    return readFileSync(resolve(WEB_ROOT, path), 'utf8').replace(/\s+/g, ' ');
  }

  it('is byte-for-byte the same in the crumb and on the page', () => {
    expect(normalized('src/pages/VehicleDetailPage.tsx')).toContain(RULE);
    expect(normalized('src/components/frame/crumbs/VehicleNameCrumb.tsx')).toContain(RULE);
  });
});
