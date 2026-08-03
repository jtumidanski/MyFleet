import { describe, it, expect, vi } from 'vitest';
import { render, screen, within } from '@testing-library/react';
import { BlastRadiusPanel } from './BlastRadiusPanel';

describe('BlastRadiusPanel', () => {
  it('lists what a purge would delete, per domain', () => {
    render(
      <BlastRadiusPanel
        counts={{ vehicles: 4, fuel_logs: 130 }}
        fleetName="Test"
        onPurge={vi.fn()}
      />,
    );
    expect(screen.getByText('130')).toBeInTheDocument();
    expect(screen.getByText('4')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /purge this fleet/i })).toBeEnabled();
  });

  // FR-ADMIN-UI-9. A live destructive button above numbers nobody could compute
  // is the worst state this screen can be in.
  it('withholds the purge control when the counts could not be computed', () => {
    render(<BlastRadiusPanel counts={undefined} error fleetName="Test" onPurge={vi.fn()} />);
    expect(screen.getByRole('alert')).toHaveTextContent(/could not/i);
    expect(screen.queryByRole('button', { name: /purge this fleet/i })).not.toBeInTheDocument();
  });

  // A domain this build does not know about must still be counted: omitting it
  // would understate the blast radius, which is the one error that matters here.
  it('renders an unknown count key rather than dropping it', () => {
    render(
      <BlastRadiusPanel
        counts={{ vehicles: 1, something_new: 9 }}
        fleetName="T"
        onPurge={vi.fn()}
      />,
    );
    expect(screen.getByTestId('radius-something_new')).toHaveTextContent('9');
  });

  // Every key the purge manifest can return needs a label. Two were missing and
  // fell through to the fallback, rendering "dashboard widgets" and "maintenance
  // record documents" in lower case beside properly-cased siblings.
  it('sentence-cases every domain the manifest can return', () => {
    const counts = {
      vehicles: 1,
      maintenance_records: 1,
      maintenance_record_documents: 1,
      maintenance_schedules: 1,
      fuel_logs: 1,
      mileage_records: 1,
      activity_events: 1,
      memberships: 1,
      invites: 1,
      dashboards: 1,
      dashboard_widgets: 1,
      vehicle_media: 1,
      fleets: 1,
    };
    render(<BlastRadiusPanel counts={counts} fleetName="T" onPurge={vi.fn()} />);
    for (const key of Object.keys(counts)) {
      // The <dt> specifically: the row also holds a <dd> with the count.
      const label = screen.getByTestId(`radius-${key}`).querySelector('dt')?.textContent ?? '';
      expect(label, `label for ${key}`).not.toBe('');
      expect(label[0], `label for ${key} is "${label}"`).toBe(label[0]?.toUpperCase());
    }
  });

  // The fallback is a safety net for a key this build does not know. It must
  // still look like the rest of the list.
  it('sentence-cases an unknown key too', () => {
    render(<BlastRadiusPanel counts={{ something_new: 9 }} fleetName="T" onPurge={vi.fn()} />);
    expect(
      within(screen.getByTestId('radius-something_new')).getByText('Something new'),
    ).toBeInTheDocument();
  });
});
