import { describe, it, expect, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
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
});
