import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { renderWithProviders } from '../../../test/renderWithProviders';
import { inviteService } from '../../../services/api/InviteService';
import { inviteAcceptUrl } from '../../../lib/invites/acceptUrl';
import { InviteList } from './InviteList';
import type { Invite } from '../../../types/models/invite';

vi.mock('../../../services/api/InviteService', () => ({
  inviteService: { listByFleet: vi.fn(), revokeInvite: vi.fn(), resendInvite: vi.fn() },
}));

function makeInvite(overrides: Partial<Invite['attributes']> = {}): Invite {
  return {
    type: 'invites',
    id: 'i1',
    attributes: {
      fleetId: 'f1',
      email: 'jane@example.com',
      role: 'member',
      token: 'tok-abc',
      expiresAt: '2099-01-01T00:00:00Z',
      invitedByUserId: 'u1',
      ...overrides,
    },
  };
}

beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(inviteService.listByFleet).mockResolvedValue({ data: [makeInvite()], meta: undefined });
});

afterEach(() => vi.unstubAllGlobals());

describe('InviteList', () => {
  // The gap that made invites useless: fleet-service returns the token, the UI
  // rendered everything except it, and nothing emails the invitee. Without the
  // link on screen the invite cannot reach the person it names.
  it('shows the accept link for a pending invite', async () => {
    renderWithProviders(<InviteList fleetId="f1" isOwner />);

    expect(await screen.findByText(inviteAcceptUrl('tok-abc'))).toBeInTheDocument();
  });

  it('copies the accept link to the clipboard', async () => {
    const writeText = vi.fn().mockResolvedValue(undefined);
    vi.stubGlobal('navigator', { ...navigator, clipboard: { writeText } });

    renderWithProviders(<InviteList fleetId="f1" isOwner />);
    const copy = await screen.findByRole('button', { name: /copy invite link/i });
    await userEvent.click(copy);

    await waitFor(() => expect(writeText).toHaveBeenCalledWith(inviteAcceptUrl('tok-abc')));
  });

  // An accepted invite is spent — its link would only produce a 409. Surfacing
  // it would invite the owner to send a link that cannot work.
  it('does not list an accepted invite', async () => {
    vi.mocked(inviteService.listByFleet).mockResolvedValue({
      data: [makeInvite({ acceptedAt: '2026-01-01T00:00:00Z' })],
      meta: undefined,
    });

    renderWithProviders(<InviteList fleetId="f1" isOwner />);

    expect(await screen.findByText(/no pending invites/i)).toBeInTheDocument();
    expect(screen.queryByText(inviteAcceptUrl('tok-abc'))).not.toBeInTheDocument();
  });

  // Resend rotates the token server-side, so the invitee gets a fresh email and
  // the previously copied link dies. The id, not the token, addresses the row.
  it('resends a pending invite by id', async () => {
    vi.mocked(inviteService.resendInvite).mockResolvedValue(makeInvite({ token: 'tok-new' }));

    renderWithProviders(<InviteList fleetId="f1" isOwner />);
    await userEvent.click(await screen.findByRole('button', { name: /resend/i }));

    await waitFor(() => expect(inviteService.resendInvite).toHaveBeenCalledWith('f1', 'i1'));
  });

  // Resend and Revoke are owner-only, matching the server-side gate. The accept
  // link itself is not gated — a non-owner member seeing it changes nothing,
  // since the token is already in the list response they just read.
  it('hides the mutating controls from a non-owner', async () => {
    renderWithProviders(<InviteList fleetId="f1" isOwner={false} />);

    expect(await screen.findByText(inviteAcceptUrl('tok-abc'))).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /resend/i })).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /revoke/i })).not.toBeInTheDocument();
  });
});
