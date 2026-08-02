import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { InviteList } from './InviteList';
import type { Invite } from '../../../types/models/invite';

const pendingInvite: Invite = {
  id: 'inv-1',
  type: 'invites',
  attributes: {
    fleetId: 'f1',
    email: 'a@b.com',
    role: 'member',
    token: 'deadbeef',
    expiresAt: '2026-08-09T12:00:00Z',
    invitedByUserId: 'u1',
  },
};

const resendMutate = vi.fn();
const revokeMutate = vi.fn();
let invites = [pendingInvite];

vi.mock('../../../lib/hooks/api/invites', () => ({
  useInvites: () => ({ data: invites, isLoading: false }),
  useRevokeInvite: () => ({ mutate: revokeMutate, isPending: false }),
  useResendInvite: () => ({ mutate: resendMutate, isPending: false }),
}));

const copyToClipboard = vi.fn().mockResolvedValue(true);
vi.mock('../../../lib/utils/clipboard', () => ({
  copyToClipboard: (text: string) => copyToClipboard(text),
}));

const toastSuccess = vi.fn();
const toastError = vi.fn();
vi.mock('sonner', () => ({
  toast: {
    success: (...args: unknown[]) => toastSuccess(...args),
    error: (...args: unknown[]) => toastError(...args),
  },
}));

describe('InviteList', () => {
  beforeEach(() => {
    invites = [pendingInvite];
    resendMutate.mockReset();
    revokeMutate.mockReset();
    copyToClipboard.mockClear();
    toastSuccess.mockReset();
    toastError.mockReset();
  });

  // FR-UI-1: the copied URL must be the one the SPA's accept route serves.
  it('copies the accept link for a pending invite', async () => {
    render(<InviteList fleetId="f1" isOwner />);
    await userEvent.click(screen.getByRole('button', { name: /copy link/i }));

    await waitFor(() => {
      expect(copyToClipboard).toHaveBeenCalledWith(
        `${window.location.origin}/invites/deadbeef/accept`,
      );
    });
    expect(toastSuccess).toHaveBeenCalledWith('Invite link copied');
  });

  it('tells the user when the copy fails instead of silently doing nothing', async () => {
    copyToClipboard.mockResolvedValueOnce(false);
    render(<InviteList fleetId="f1" isOwner />);
    await userEvent.click(screen.getByRole('button', { name: /copy link/i }));

    await waitFor(() => expect(toastError).toHaveBeenCalled());
    expect(toastSuccess).not.toHaveBeenCalled();
  });

  // FR-UI-2.
  it('resends a pending invite by id', async () => {
    render(<InviteList fleetId="f1" isOwner />);
    await userEvent.click(screen.getByRole('button', { name: /resend/i }));
    expect(resendMutate).toHaveBeenCalledWith('inv-1');
  });

  // FR-UI-3: an accepted invite is filtered out entirely, so neither control
  // can render on one.
  it('renders no controls for an accepted invite', () => {
    invites = [
      {
        ...pendingInvite,
        attributes: { ...pendingInvite.attributes, acceptedAt: '2026-08-03T00:00:00Z' },
      },
    ];
    render(<InviteList fleetId="f1" isOwner />);

    expect(screen.queryByRole('button', { name: /copy link/i })).toBeNull();
    expect(screen.queryByRole('button', { name: /resend/i })).toBeNull();
    expect(screen.getByText(/no pending invites/i)).toBeInTheDocument();
  });

  // The controls are owner-gated, matching Revoke.
  it('renders no controls for a non-owner', () => {
    render(<InviteList fleetId="f1" isOwner={false} />);
    expect(screen.queryByRole('button', { name: /copy link/i })).toBeNull();
    expect(screen.queryByRole('button', { name: /resend/i })).toBeNull();
  });
});
