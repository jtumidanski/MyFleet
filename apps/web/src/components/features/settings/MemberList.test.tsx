import { describe, it, expect, vi, beforeEach, beforeAll } from 'vitest';
import { screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { renderWithProviders } from '../../../test/renderWithProviders';
import { memberService } from '../../../services/api/MemberService';
import { userService } from '../../../services/api/UserService';
import { MemberList } from './MemberList';
import type { Membership } from '../../../types/models/membership';

// jsdom lacks the PointerEvent APIs Radix's Select relies on for open/close
// handling; without these it throws instead of opening.
beforeAll(() => {
  window.HTMLElement.prototype.scrollIntoView = vi.fn();
  window.HTMLElement.prototype.hasPointerCapture = vi.fn();
  window.HTMLElement.prototype.releasePointerCapture = vi.fn();
});

vi.mock('../../../services/api/MemberService', () => ({
  memberService: {
    listByFleet: vi.fn(),
    removeMember: vi.fn().mockResolvedValue(undefined),
    updateRole: vi.fn(),
  },
}));

vi.mock('../../../services/api/UserService', () => ({
  userService: { listByIds: vi.fn() },
}));

// Both exports: the API client imports refreshAccessToken from this module, so
// a partial mock would break its import.
vi.mock('../../../lib/api/refresh', () => ({
  mintAccessToken: vi.fn().mockResolvedValue('fresh-token'),
  refreshAccessToken: vi.fn().mockResolvedValue('fresh-token'),
}));

vi.mock('sonner', () => ({ toast: { success: vi.fn(), error: vi.fn() } }));

// useAuth supplies the caller's identity. 'me' is the authenticated user in
// every test below. MemberList reads only `user.id` — every role decision it
// makes comes from the members LIST, not from this claim (see the component's
// myRole comment) — but the object is filled out so the mock type-checks.
vi.mock('../../../context/AuthContext', () => ({
  useAuth: () => ({
    user: {
      id: 'me',
      type: 'users',
      attributes: {
        email: 'me@example.com',
        displayName: 'Me',
        avatarUrl: '',
        themePreference: 'system',
      },
    },
    activeFleetId: 'f1',
    role: 'owner',
    isAuthenticated: true,
    isLoading: false,
  }),
}));

function membership(userId: string, role: string): Membership {
  return {
    type: 'memberships',
    id: 'm-' + userId,
    attributes: { fleetId: 'f1', userId, role, status: 'active' },
  };
}

function userRow(id: string, displayName: string, email: string) {
  return {
    type: 'users' as const,
    id,
    attributes: { displayName, email, avatarUrl: '', themePreference: 'system' as const },
  };
}

function seed(members: Membership[], users: ReturnType<typeof userRow>[] = []) {
  vi.mocked(memberService.listByFleet).mockResolvedValue({ data: members, meta: undefined });
  vi.mocked(userService.listByIds).mockResolvedValue({ data: users, meta: undefined });
}

beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(memberService.removeMember).mockResolvedValue(undefined);
  vi.mocked(memberService.updateRole).mockResolvedValue(membership('other', 'owner'));
});

describe('MemberList — names', () => {
  // FR-1.5. Before this task the card showed three indistinguishable UUIDs and
  // nobody could tell who they were about to remove.
  it('renders displayName, then email, then a shortened id', async () => {
    seed(
      [
        membership('me', 'owner'),
        membership('has-email-only-0000', 'member'),
        membership('has-nothing-0000', 'viewer'),
      ],
      [
        userRow('me', 'Jane Doe', 'jane@example.com'),
        userRow('has-email-only-0000', '', 'sam@example.com'),
      ],
    );

    renderWithProviders(<MemberList fleetId="f1" isOwner />);

    expect(await screen.findByText(/Jane Doe/)).toBeInTheDocument();
    expect(screen.getByText('sam@example.com')).toBeInTheDocument();
    // Neither name nor email: the first 8 characters of the id.
    expect(screen.getByText('has-noth')).toBeInTheDocument();
  });

  // An unset displayName arrives as "" from Go, not null. `??` would let the
  // empty string through and render a blank row.
  it('falls through an empty-string displayName to the email', async () => {
    seed([membership('u1', 'member')], [userRow('u1', '', 'blank@example.com')]);

    renderWithProviders(<MemberList fleetId="f1" isOwner />);

    expect(await screen.findByText('blank@example.com')).toBeInTheDocument();
  });

  // FR-1.7. A name-service failure must not blank the members card.
  it('still renders the list with id fallbacks when the name lookup fails', async () => {
    vi.mocked(memberService.listByFleet).mockResolvedValue({
      data: [membership('abcdefgh-1234', 'member')],
      meta: undefined,
    });
    vi.mocked(userService.listByIds).mockRejectedValue(new Error('auth-service down'));

    renderWithProviders(<MemberList fleetId="f1" isOwner />);

    expect(await screen.findByText('abcdefgh')).toBeInTheDocument();
  });

  // FR-1.6.
  it('marks the authenticated user with (you)', async () => {
    seed(
      [membership('me', 'owner'), membership('other', 'member')],
      [userRow('me', 'Jane Doe', 'jane@example.com'), userRow('other', 'Sam Ito', 'sam@example.com')],
    );

    renderWithProviders(<MemberList fleetId="f1" isOwner />);

    expect(await screen.findByText(/Jane Doe \(you\)/)).toBeInTheDocument();
    expect(screen.getByText('Sam Ito')).toBeInTheDocument();
  });
});

describe('MemberList — removing another member', () => {
  // The bug this closes: one click fired DELETE with no confirmation at all.
  it('does not fire the DELETE until the dialog is confirmed', async () => {
    seed(
      [membership('me', 'owner'), membership('other', 'member')],
      [userRow('other', 'Sam Ito', 'sam@example.com')],
    );

    renderWithProviders(<MemberList fleetId="f1" isOwner />);

    await userEvent.click(await screen.findByRole('button', { name: /remove sam ito/i }));

    expect(await screen.findByText(/Remove Sam Ito from this fleet\?/i)).toBeInTheDocument();
    expect(memberService.removeMember).not.toHaveBeenCalled();

    await userEvent.click(screen.getByRole('button', { name: /^remove$/i }));

    await waitFor(() => expect(memberService.removeMember).toHaveBeenCalledWith('f1', 'other'));
  });

  it('fires nothing when the dialog is cancelled', async () => {
    seed(
      [membership('me', 'owner'), membership('other', 'member')],
      [userRow('other', 'Sam Ito', 'sam@example.com')],
    );

    renderWithProviders(<MemberList fleetId="f1" isOwner />);

    await userEvent.click(await screen.findByRole('button', { name: /remove sam ito/i }));
    await userEvent.click(await screen.findByRole('button', { name: /cancel/i }));

    await waitFor(() =>
      expect(screen.queryByText(/Remove Sam Ito from this fleet\?/i)).not.toBeInTheDocument(),
    );
    expect(memberService.removeMember).not.toHaveBeenCalled();
  });
});

describe('MemberList — Make owner', () => {
  // FR-2.8.
  it('is offered to owners on non-owner rows and confirms before PATCHing', async () => {
    seed(
      [membership('me', 'owner'), membership('other', 'member')],
      [userRow('other', 'Sam Ito', 'sam@example.com')],
    );

    renderWithProviders(<MemberList fleetId="f1" isOwner />);

    await userEvent.click(await screen.findByRole('button', { name: /make sam ito an owner/i }));

    expect(await screen.findByText(/Make Sam Ito an owner\?/i)).toBeInTheDocument();
    expect(memberService.updateRole).not.toHaveBeenCalled();

    await userEvent.click(screen.getByRole('button', { name: /^make owner$/i }));

    await waitFor(() => expect(memberService.updateRole).toHaveBeenCalledWith('f1', 'other', 'owner'));
  });

  it('is hidden from non-owners and never offered on an owner row', async () => {
    seed(
      [membership('me', 'member'), membership('boss', 'owner')],
      [userRow('boss', 'Jane Doe', 'jane@example.com')],
    );

    renderWithProviders(<MemberList fleetId="f1" isOwner={false} />);

    await screen.findByText('Jane Doe');
    expect(screen.queryByRole('button', { name: /make .* an owner/i })).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /^remove /i })).not.toBeInTheDocument();
  });
});

describe('MemberList — leaving', () => {
  // ux-flow state 1. Before this task a member had no way to leave at all.
  it('offers a plain leave confirmation to a member', async () => {
    seed([membership('me', 'member'), membership('boss', 'owner')]);

    renderWithProviders(<MemberList fleetId="f1" isOwner={false} />);

    await userEvent.click(await screen.findByRole('button', { name: /^leave$/i }));

    expect(await screen.findByText(/Leave this fleet\?/i)).toBeInTheDocument();
    expect(screen.queryByRole('combobox')).not.toBeInTheDocument();

    // Scoped to the dialog: the row button is ALSO named "Leave", so an
    // unscoped query matches two elements and throws.
    const dialog = within(screen.getByRole('alertdialog'));
    await userEvent.click(dialog.getByRole('button', { name: /^leave$/i }));

    await waitFor(() => expect(memberService.removeMember).toHaveBeenCalledWith('f1', 'me'));
    expect(memberService.updateRole).not.toHaveBeenCalled();
  });

  // ux-flow state 2: an owner with a co-owner leaves without picking anyone.
  it('offers a plain leave confirmation to one of two owners', async () => {
    seed([membership('me', 'owner'), membership('co', 'owner')]);

    renderWithProviders(<MemberList fleetId="f1" isOwner />);

    await userEvent.click(await screen.findByRole('button', { name: /^leave$/i }));

    expect(await screen.findByText(/Leave this fleet\?/i)).toBeInTheDocument();
    expect(screen.queryByText(/only owner/i)).not.toBeInTheDocument();
    expect(screen.queryByRole('combobox')).not.toBeInTheDocument();
  });

  // ux-flow state 3 / FR-3.7. Without this the sole owner hits a 409 with
  // nothing they can do about it — the dead end this task exists to close.
  it('requires a successor before a sole owner can leave', async () => {
    seed(
      [membership('me', 'owner'), membership('other', 'member')],
      [userRow('other', 'Sam Ito', 'sam@example.com')],
    );

    renderWithProviders(<MemberList fleetId="f1" isOwner />);

    await userEvent.click(await screen.findByRole('button', { name: /^leave$/i }));

    expect(await screen.findByText(/only owner/i)).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /transfer & leave/i })).toBeDisabled();
  });

  // FR-3.8 ordering: promote first, then delete.
  it('promotes the successor and then removes the leaver', async () => {
    seed(
      [membership('me', 'owner'), membership('other', 'member')],
      [userRow('other', 'Sam Ito', 'sam@example.com')],
    );
    const order: string[] = [];
    vi.mocked(memberService.updateRole).mockImplementation(async () => {
      order.push('patch');
      return membership('other', 'owner');
    });
    vi.mocked(memberService.removeMember).mockImplementation(async () => {
      order.push('delete');
    });

    renderWithProviders(<MemberList fleetId="f1" isOwner />);

    await userEvent.click(await screen.findByRole('button', { name: /^leave$/i }));
    await userEvent.click(await screen.findByRole('combobox'));
    await userEvent.click(await screen.findByRole('option', { name: /Sam Ito/ }));
    await userEvent.click(screen.getByRole('button', { name: /transfer & leave/i }));

    await waitFor(() => expect(order).toEqual(['patch', 'delete']));
    expect(memberService.updateRole).toHaveBeenCalledWith('f1', 'other', 'owner');
    expect(memberService.removeMember).toHaveBeenCalledWith('f1', 'me');
  });

  // FR-3.8, the half that matters: a failed promote must NOT be followed by the
  // delete, or the fleet is orphaned. Sequencing this with awaits inside one
  // function is what makes that a property of control flow rather than of
  // callback wiring nobody re-reads.
  it('does not remove the leaver when the promote fails', async () => {
    seed(
      [membership('me', 'owner'), membership('other', 'member')],
      [userRow('other', 'Sam Ito', 'sam@example.com')],
    );
    vi.mocked(memberService.updateRole).mockRejectedValue(new Error('boom'));

    renderWithProviders(<MemberList fleetId="f1" isOwner />);

    await userEvent.click(await screen.findByRole('button', { name: /^leave$/i }));
    await userEvent.click(await screen.findByRole('combobox'));
    await userEvent.click(await screen.findByRole('option', { name: /Sam Ito/ }));
    await userEvent.click(screen.getByRole('button', { name: /transfer & leave/i }));

    await waitFor(() => expect(memberService.updateRole).toHaveBeenCalled());
    expect(memberService.removeMember).not.toHaveBeenCalled();
  });

  // D8: the picker offers viewers too. Excluding them would create a SECOND
  // dead end — an owner whose only companion is a viewer would see an enabled
  // Leave button, an empty picker and a permanently disabled confirm.
  it('offers viewers as successors', async () => {
    seed(
      [membership('me', 'owner'), membership('watcher', 'viewer')],
      [userRow('watcher', 'Val Watcher', 'val@example.com')],
    );

    renderWithProviders(<MemberList fleetId="f1" isOwner />);

    await userEvent.click(await screen.findByRole('button', { name: /^leave$/i }));
    await userEvent.click(await screen.findByRole('combobox'));

    expect(await screen.findByRole('option', { name: /Val Watcher/ })).toBeInTheDocument();
  });

  // ux-flow state 4 / FR-3.10. The one case with no path forward: there is
  // nobody to hand the fleet to, and deleting a fleet is out of scope.
  it('disables Leave for a sole owner who is the only member, with an explanation', async () => {
    seed([membership('me', 'owner')], [userRow('me', 'Jane Doe', 'jane@example.com')]);

    renderWithProviders(<MemberList fleetId="f1" isOwner />);

    const leave = await screen.findByRole('button', { name: /^leave$/i });
    expect(leave).toBeDisabled();
    expect(screen.getByText(/only member of this fleet/i)).toBeInTheDocument();

    await userEvent.click(leave);
    expect(screen.queryByText(/Leave this fleet\?/i)).not.toBeInTheDocument();
  });
});
