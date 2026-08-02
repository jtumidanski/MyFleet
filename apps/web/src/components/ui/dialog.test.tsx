import * as React from 'react';
import { describe, it, expect } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from './dialog';

/**
 * Drives the dialog the way the app does: controlled `open`, opened from an
 * ordinary button rather than a DialogTrigger. That combination is precisely
 * what defeats Radix's built-in focus restoration (it only ever restores to a
 * registered trigger), so it is the configuration the tests must exercise.
 */
function Harness({ dismissible = true, modal }: { dismissible?: boolean; modal?: boolean }) {
  const [open, setOpen] = React.useState(false);
  return (
    <div>
      <button type="button" onClick={() => setOpen(true)}>
        Open
      </button>
      <Dialog open={open} onOpenChange={setOpen} modal={modal}>
        <DialogContent dismissible={dismissible}>
          <DialogHeader>
            <DialogTitle>Panel</DialogTitle>
            <DialogDescription>What this panel is for.</DialogDescription>
          </DialogHeader>
          <input aria-label="First field" />
          <button type="button" onClick={() => setOpen(false)}>
            Done
          </button>
        </DialogContent>
      </Dialog>
    </div>
  );
}

/** Clicks the harness trigger and returns the opened dialog element. */
async function open(): Promise<HTMLElement> {
  await userEvent.click(screen.getByRole('button', { name: 'Open' }));
  return screen.getByRole('dialog');
}

describe('Dialog — announcement', () => {
  it('is a modal dialog labelled by its title and described by its description', async () => {
    render(<Harness />);
    const dialog = await open();
    expect(dialog).toHaveAttribute('aria-modal', 'true');
    expect(dialog).toHaveAccessibleName('Panel');
    expect(dialog).toHaveAccessibleDescription('What this panel is for.');
  });

  it('drops the modal claim when the dialog is not modal', async () => {
    // Radix routes a non-modal Root to DialogContentNonModal: no focus trap and
    // no aria-hiding of the page behind. Announcing aria-modal there would
    // promise assistive tech a containment the dialog does not have, so the
    // attribute has to follow the Root rather than be hard-coded.
    render(<Harness modal={false} />);
    const dialog = await open();
    expect(dialog).not.toHaveAttribute('aria-modal');
  });

  it('names the close button "Close"', async () => {
    render(<Harness />);
    await open();
    expect(screen.getByRole('button', { name: 'Close' })).toBeInTheDocument();
  });
});

describe('Dialog — focus', () => {
  it('moves focus to the first control in the body, not to the close button', async () => {
    // Guaranteed by DOM order: the close button is rendered after `children`.
    // Reorder them and this is the test that fails.
    render(<Harness />);
    await open();
    expect(document.activeElement).toBe(screen.getByLabelText('First field'));
  });

  it('returns focus to the plain button that opened it', async () => {
    // Radix restores focus only to a registered DialogTrigger. This dialog is
    // opened from an ordinary button in controlled mode, so without the
    // primitive's own capture-and-restore, focus would land on <body>.
    render(<Harness />);
    const trigger = screen.getByRole('button', { name: 'Open' });
    await userEvent.click(trigger);
    await userEvent.keyboard('{Escape}');
    expect(document.activeElement).toBe(trigger);
  });

  it('lets a consumer redirect focus by preventing the close-autofocus default', async () => {
    function Redirecting() {
      const [isOpen, setIsOpen] = React.useState(false);
      const elsewhereRef = React.useRef<HTMLButtonElement>(null);
      return (
        <div>
          <button type="button" onClick={() => setIsOpen(true)}>
            Open
          </button>
          <button type="button" ref={elsewhereRef}>
            Elsewhere
          </button>
          <Dialog open={isOpen} onOpenChange={setIsOpen}>
            <DialogContent
              onCloseAutoFocus={(event) => {
                event.preventDefault();
                elsewhereRef.current?.focus();
              }}
            >
              <DialogTitle>Panel</DialogTitle>
              <DialogDescription>Body.</DialogDescription>
            </DialogContent>
          </Dialog>
        </div>
      );
    }

    render(<Redirecting />);
    await userEvent.click(screen.getByRole('button', { name: 'Open' }));
    await userEvent.keyboard('{Escape}');
    expect(document.activeElement).toBe(screen.getByRole('button', { name: 'Elsewhere' }));
  });
});

describe('Dialog — dismissal', () => {
  it('closes on Escape', async () => {
    render(<Harness />);
    await open();
    await userEvent.keyboard('{Escape}');
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
  });

  it('closes on a pointer-down outside the content', async () => {
    // This is the path a real overlay click takes through DismissableLayer.
    // userEvent cannot drive the overlay itself under jsdom.
    render(<Harness />);
    await open();
    fireEvent.pointerDown(document.body);
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
  });

  it('closes on the close button', async () => {
    render(<Harness />);
    await open();
    await userEvent.click(screen.getByRole('button', { name: 'Close' }));
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
  });
});

describe('Dialog — dismissible={false}', () => {
  // All three routes are asserted independently. Wiring only two of them is the
  // exact regression this block exists to catch.
  it('ignores Escape', async () => {
    render(<Harness dismissible={false} />);
    await open();
    await userEvent.keyboard('{Escape}');
    expect(screen.getByRole('dialog')).toBeInTheDocument();
  });

  it('ignores a pointer-down outside the content', async () => {
    render(<Harness dismissible={false} />);
    await open();
    fireEvent.pointerDown(document.body);
    expect(screen.getByRole('dialog')).toBeInTheDocument();
  });

  it('disables the close button rather than hiding it', async () => {
    // Hiding it would reflow the header for the duration of the request.
    render(<Harness dismissible={false} />);
    await open();
    expect(screen.getByRole('button', { name: 'Close' })).toBeDisabled();
  });
});

describe('Dialog — content lifecycle', () => {
  it('does not mount its children before the first open', () => {
    render(<Harness />);
    expect(screen.queryByLabelText('First field')).not.toBeInTheDocument();
  });

  it('unmounts its children on close so nothing survives to the next open', async () => {
    render(<Harness />);
    await userEvent.click(screen.getByRole('button', { name: 'Open' }));
    await userEvent.type(screen.getByLabelText('First field'), 'typed');
    await userEvent.keyboard('{Escape}');
    await userEvent.click(screen.getByRole('button', { name: 'Open' }));
    expect(screen.getByLabelText('First field')).toHaveValue('');
  });
});
