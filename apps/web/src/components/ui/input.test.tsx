import { describe, it, expect, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { Input } from './input';

/**
 * jsdom implements neither `showPicker` nor the vendor spin-button
 * pseudo-elements, so both behaviours are asserted at the seam this component
 * actually controls: the method it calls, and the classes it emits.
 */
function stubShowPicker() {
  const showPicker = vi.fn();
  (HTMLInputElement.prototype as unknown as { showPicker: () => void }).showPicker = showPicker;
  return showPicker;
}

describe('Input', () => {
  it('suppresses the spin buttons on number fields', () => {
    render(<Input type="number" aria-label="Mileage" />);

    const input = screen.getByLabelText('Mileage');
    expect(input.className).toContain('[appearance:textfield]');
    expect(input.className).toContain('[&::-webkit-inner-spin-button]:appearance-none');
  });

  it('leaves other field types alone', () => {
    render(<Input type="text" aria-label="Vendor" />);

    expect(screen.getByLabelText('Vendor').className).not.toContain('[appearance:textfield]');
  });

  it('opens the picker when a date field is clicked', async () => {
    const showPicker = stubShowPicker();
    const user = userEvent.setup();
    render(<Input type="datetime-local" aria-label="Date Performed" />);

    await user.click(screen.getByLabelText('Date Performed'));

    expect(showPicker).toHaveBeenCalled();
  });

  it('opens the picker when a date field merely receives focus', async () => {
    const showPicker = stubShowPicker();
    const user = userEvent.setup();
    render(
      <>
        <button type="button">before</button>
        <Input type="date" aria-label="Date" />
      </>,
    );

    // Tab, not click: focusing via the keyboard is the case the click-only
    // affordance left with no way to reach the picker at all.
    await user.click(screen.getByRole('button', { name: 'before' }));
    await user.tab();

    expect(screen.getByLabelText('Date')).toHaveFocus();
    expect(showPicker).toHaveBeenCalled();
  });

  it('does not reach for a picker on non-picker types', async () => {
    const showPicker = stubShowPicker();
    const user = userEvent.setup();
    render(<Input type="number" aria-label="Cost" />);

    await user.click(screen.getByLabelText('Cost'));

    expect(showPicker).not.toHaveBeenCalled();
  });

  it('still calls a consumer-supplied onClick', async () => {
    stubShowPicker();
    const onClick = vi.fn();
    const user = userEvent.setup();
    render(<Input type="date" aria-label="Date" onClick={onClick} />);

    await user.click(screen.getByLabelText('Date'));

    expect(onClick).toHaveBeenCalled();
  });

  it('survives a browser that does not implement showPicker', async () => {
    // Deleting rather than stubbing: this is the Safari/older-Firefox shape,
    // where the property is simply absent.
    delete (HTMLInputElement.prototype as unknown as { showPicker?: () => void }).showPicker;
    const user = userEvent.setup();
    render(<Input type="date" aria-label="Date" />);

    await expect(user.click(screen.getByLabelText('Date'))).resolves.not.toThrow();
  });

  it('swallows the NotAllowedError a gesture-less focus produces', async () => {
    (HTMLInputElement.prototype as unknown as { showPicker: () => void }).showPicker = () => {
      throw new DOMException('not allowed', 'NotAllowedError');
    };
    const user = userEvent.setup();
    render(<Input type="date" aria-label="Date" />);

    await expect(user.click(screen.getByLabelText('Date'))).resolves.not.toThrow();
  });
});
