import { describe, it, expect, vi, beforeEach } from 'vitest';
import { screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { toast } from 'sonner';
import { renderWithProviders } from '../../../test/renderWithProviders';
import { inviteService } from '../../../services/api/InviteService';
import { InviteForm } from './InviteForm';

vi.mock('../../../services/api/InviteService', () => ({
  inviteService: { createInvite: vi.fn() },
}));

vi.mock('sonner', () => ({
  toast: { success: vi.fn(), error: vi.fn() },
}));

beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(inviteService.createInvite).mockResolvedValue({
    type: 'invites',
    id: 'i1',
    attributes: {
      fleetId: 'f1',
      email: 'jane@example.com',
      role: 'member',
      token: 'tok-abc',
      expiresAt: '2099-01-01T00:00:00Z',
      invitedByUserId: 'u1',
    },
  });
});

describe('InviteForm', () => {
  // The form used to toast "Invite sent to <email>". Nothing sends anything —
  // there is no mail path in this service — so the owner believed the invitee
  // had been contacted and the invite sat unredeemed. The confirmation must
  // describe what actually happened and point at the next step.
  it('does not claim the invite was sent anywhere', async () => {
    renderWithProviders(<InviteForm fleetId="f1" />);

    await userEvent.type(screen.getByLabelText(/email/i), 'jane@example.com');
    await userEvent.click(screen.getByRole('button', { name: /invite/i }));

    await waitFor(() => expect(toast.success).toHaveBeenCalled());
    const message = vi.mocked(toast.success).mock.calls[0]?.[0] as string;
    expect(message).not.toMatch(/sent/i);
    expect(message).toMatch(/link/i);
  });

  // Role is required by the schema even though it opens pre-filled with
  // "Member" — required-ness is a property of the field, not of its initial
  // value. This is also the SelectTrigger slot case.
  it('marks both fields required', () => {
    renderWithProviders(<InviteForm fleetId="f1" />);

    expect(screen.getByLabelText(/email/i)).toHaveAttribute('aria-required', 'true');
    expect(screen.getByRole('combobox', { name: /role/i })).toHaveAttribute(
      'aria-required',
      'true',
    );
  });
});
