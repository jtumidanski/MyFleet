import { describe, it, expect, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { PurgeConfirmDialog, type PurgeConfirmDialogProps } from './PurgeConfirmDialog';

function props(over: Partial<PurgeConfirmDialogProps> = {}): PurgeConfirmDialogProps {
  return {
    open: true,
    onOpenChange: vi.fn(),
    scope: 'fleet',
    confirmationPhrase: 'The Tumidanski Fleet',
    counts: { vehicles: 4 },
    peopleCount: 3,
    recoveryDeadline: '2026-08-07T14:03:11Z',
    onConfirm: vi.fn(),
    isPending: false,
    ...over,
  };
}

describe('PurgeConfirmDialog', () => {
  it('keeps confirm unavailable until the phrase matches exactly', async () => {
    const user = userEvent.setup();
    render(<PurgeConfirmDialog {...props()} />);
    const confirm = screen.getByRole('button', { name: /purge this fleet/i });
    expect(confirm).toBeDisabled();

    const box = screen.getByLabelText(/type the fleet name/i);
    await user.type(box, 'the tumidanski fleet');
    expect(confirm).toBeDisabled();

    await user.clear(box);
    await user.type(box, 'The Tumidanski Fleet');
    expect(confirm).toBeEnabled();
  });

  // Trailing whitespace is the classic near-miss. The server does not trim, so
  // neither may the client — otherwise the button goes live on a phrase the
  // server will reject with a 409.
  it('does not accept a phrase with trailing whitespace', async () => {
    const user = userEvent.setup();
    render(<PurgeConfirmDialog {...props()} />);
    await user.type(screen.getByLabelText(/type the fleet name/i), 'The Tumidanski Fleet ');
    expect(screen.getByRole('button', { name: /purge this fleet/i })).toBeDisabled();
  });

  it('states the blast radius in people as well as rows', () => {
    render(<PurgeConfirmDialog {...props({ peopleCount: 3, counts: { vehicles: 4 } })} />);
    expect(screen.getByText(/3 people/i)).toBeInTheDocument();
    expect(screen.getByText(/4 vehicles/i)).toBeInTheDocument();
  });

  // A duration ("recoverable for 5 days") makes an operator do arithmetic under
  // pressure and get it wrong. An absolute deadline does not.
  it('gives the recovery deadline as an absolute date and time', () => {
    render(<PurgeConfirmDialog {...props({ recoveryDeadline: '2026-08-07T14:03:11Z' })} />);
    expect(screen.getByText(/august 7, 2026/i)).toBeInTheDocument();
    expect(screen.queryByText(/5 days/i)).not.toBeInTheDocument();
  });

  it('names what survives a system purge', () => {
    render(
      <PurgeConfirmDialog
        {...props({ scope: 'system', confirmationPhrase: 'PURGE EVERYTHING' })}
      />,
    );
    expect(screen.getByText(/user accounts/i)).toBeInTheDocument();
    expect(screen.getByText(/sign-ins/i)).toBeInTheDocument();
    expect(screen.getByText(/maintenance categories/i)).toBeInTheDocument();
  });

  it('does not name survivors for a fleet purge', () => {
    render(<PurgeConfirmDialog {...props({ scope: 'fleet' })} />);
    expect(screen.queryByText(/what survives/i)).not.toBeInTheDocument();
  });

  // The second purge of a session must not start with the confirm button
  // already live. The reset is done during render off a remembered `open`
  // rather than from an effect, so the reopened dialog's FIRST frame is the
  // empty box — this asserts on the frame the operator actually sees.
  it('clears a previously matching phrase when reopened', async () => {
    const user = userEvent.setup();
    const { rerender } = render(<PurgeConfirmDialog {...props()} />);
    await user.type(screen.getByLabelText(/type the fleet name/i), 'The Tumidanski Fleet');
    expect(screen.getByRole('button', { name: /purge this fleet/i })).toBeEnabled();

    rerender(<PurgeConfirmDialog {...props({ open: false })} />);
    rerender(<PurgeConfirmDialog {...props({ open: true })} />);

    expect(screen.getByLabelText(/type the fleet name/i)).toHaveValue('');
    expect(screen.getByRole('button', { name: /purge this fleet/i })).toBeDisabled();
  });

  // Not a react-hook-form surface either: typing the phrase is mandatory, so
  // the marker and aria-required are applied by hand.
  it('marks the confirmation input required', () => {
    render(<PurgeConfirmDialog {...props()} />);

    expect(screen.getByLabelText(/type the fleet name/i)).toHaveAttribute(
      'aria-required',
      'true',
    );
  });
});

describe('PurgeConfirmDialog transmission', () => {
  // The finding that matters most: the server's exact comparison is the real
  // control, and it can only work if the operator's ACTUAL keystrokes reach it.
  // Handing back the expected phrase would make the disabled button the only
  // gate and the server's 409 unreachable from the UI.
  it('hands the caller what was typed, not the expected phrase', async () => {
    const user = userEvent.setup();
    const onConfirm = vi.fn();
    render(<PurgeConfirmDialog {...props({ onConfirm })} />);
    await user.type(screen.getByLabelText(/type the fleet name/i), 'The Tumidanski Fleet');
    await user.click(screen.getByRole('button', { name: /purge this fleet/i }));
    expect(onConfirm).toHaveBeenCalledWith('The Tumidanski Fleet');
  });

  // FR-ADMIN-UI-6 at the most consequential render site there is.
  it('says the people count is unknown rather than zero when it is unavailable', () => {
    render(<PurgeConfirmDialog {...props({ peopleCount: null })} />);
    expect(screen.getByText(/unknown number of people/i)).toBeInTheDocument();
    expect(screen.queryByText(/affects 0 people/i)).not.toBeInTheDocument();
  });
});
