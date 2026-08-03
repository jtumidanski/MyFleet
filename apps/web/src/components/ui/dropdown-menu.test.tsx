import { describe, it, expect } from 'vitest';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from './dropdown-menu';

function renderMenu() {
  return render(
    <DropdownMenu>
      <DropdownMenuTrigger aria-label="Account menu">Open</DropdownMenuTrigger>
      <DropdownMenuContent>
        <DropdownMenuLabel>Ada Lovelace</DropdownMenuLabel>
        <DropdownMenuSeparator />
        <DropdownMenuItem>Sign out</DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>,
  );
}

/**
 * A smoke test for the vendored primitive AND for the jsdom stubs it needs.
 * Radix's dropdown reaches for scrollIntoView and pointer capture, neither of
 * which jsdom implements; without the stubs in src/test/setup.ts every test
 * that opens one of these throws before reaching an assertion (design §8.3).
 */
describe('DropdownMenu', () => {
  it('opens on click and exposes its items as menuitems', async () => {
    const user = userEvent.setup();
    renderMenu();

    expect(screen.queryByRole('menuitem')).not.toBeInTheDocument();

    await user.click(screen.getByRole('button', { name: 'Account menu' }));

    expect(screen.getByRole('menuitem', { name: 'Sign out' })).toBeInTheDocument();
    expect(screen.getByText('Ada Lovelace')).toBeInTheDocument();
  });

  // FR-PROFILE-5, at the primitive level: Escape closes and focus comes back,
  // which is Radix's default and must not be defeated by an onCloseAutoFocus
  // handler downstream.
  it('closes on Escape and returns focus to the trigger', async () => {
    const user = userEvent.setup();
    renderMenu();

    const trigger = screen.getByRole('button', { name: 'Account menu' });
    await user.click(trigger);
    await user.keyboard('{Escape}');

    expect(screen.queryByRole('menuitem')).not.toBeInTheDocument();
    expect(trigger).toHaveFocus();
  });

  it('opens with Enter from the keyboard', async () => {
    const user = userEvent.setup();
    renderMenu();

    await user.tab();
    expect(screen.getByRole('button', { name: 'Account menu' })).toHaveFocus();

    await user.keyboard('{Enter}');

    expect(screen.getByRole('menuitem', { name: 'Sign out' })).toBeInTheDocument();
  });

  // Untested Radix default, credited by the plan's Verification Checklist as
  // "moves with arrows" — this closes that gap. ArrowDown from the trigger
  // moves focus to the first (and here, only) menuitem.
  it('moves focus to the menu item with ArrowDown', async () => {
    const user = userEvent.setup();
    renderMenu();

    await user.click(screen.getByRole('button', { name: 'Account menu' }));
    await user.keyboard('{ArrowDown}');

    expect(screen.getByRole('menuitem', { name: 'Sign out' })).toHaveFocus();
  });
});
