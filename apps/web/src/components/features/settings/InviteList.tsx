/**
 * InviteList — pending fleet invites with copy-link, resend and revoke
 * (owner-only).
 *
 * The copy-link control is the documented recovery path when an invite email is
 * spam-filtered or a relay outage drops it (design §5.2). The token is already
 * in the fleet-scoped list response, so this needs no API change.
 */
import { toast } from 'sonner';
import { Skeleton } from '../../ui/skeleton';
import { Button } from '../../ui/button';
import { useInvites, useResendInvite, useRevokeInvite } from '../../../lib/hooks/api/invites';
import { copyToClipboard } from '../../../lib/utils/clipboard';

interface InviteListProps {
  fleetId: string;
  isOwner: boolean;
}

export function InviteList({ fleetId, isOwner }: InviteListProps) {
  const { data: invites, isLoading } = useInvites(fleetId);
  const revokeInvite = useRevokeInvite();
  const resendInvite = useResendInvite(fleetId);

  if (isLoading) {
    return (
      <div className="space-y-2">
        <Skeleton className="h-10 w-full" />
        <Skeleton className="h-10 w-full" />
      </div>
    );
  }

  // Show only pending invites (not yet accepted). This is also what satisfies
  // FR-UI-3: an accepted invite never renders, so no control can appear on one.
  const pending = (invites ?? []).filter((inv) => !inv.attributes.acceptedAt);

  if (pending.length === 0) {
    return <p className="text-sm text-muted-foreground">No pending invites.</p>;
  }

  const handleCopy = async (token: string) => {
    const ok = await copyToClipboard(`${window.location.origin}/invites/${token}/accept`);
    if (ok) {
      toast.success('Invite link copied');
    } else {
      toast.error('Could not copy the link. Select and copy it manually.');
    }
  };

  return (
    <ul className="divide-y">
      {pending.map((inv) => (
        <li key={inv.id} className="flex flex-wrap items-center justify-between gap-2 py-3">
          <div className="space-y-0.5">
            <div className="text-sm font-medium">{inv.attributes.email}</div>
            <div className="text-xs text-muted-foreground">
              Role: <span className="capitalize">{inv.attributes.role}</span>
              {' · '}
              Expires {new Date(inv.attributes.expiresAt).toLocaleDateString()}
            </div>
          </div>
          {isOwner && (
            <div className="flex items-center gap-2">
              <Button
                variant="outline"
                size="sm"
                onClick={() => void handleCopy(inv.attributes.token)}
              >
                Copy Link
              </Button>
              <Button
                variant="outline"
                size="sm"
                disabled={resendInvite.isPending}
                onClick={() => resendInvite.mutate(inv.id)}
              >
                Resend
              </Button>
              <Button
                variant="outline"
                size="sm"
                disabled={revokeInvite.isPending}
                onClick={() => revokeInvite.mutate(inv.id)}
              >
                Revoke
              </Button>
            </div>
          )}
        </li>
      ))}
    </ul>
  );
}
