/**
 * InviteList — displays pending fleet invites with revoke button (owner-only).
 */
import { Skeleton } from '../../ui/skeleton';
import { Button } from '../../ui/button';
import { useInvites, useRevokeInvite } from '../../../lib/hooks/api/invites';

interface InviteListProps {
  fleetId: string;
  isOwner: boolean;
}

export function InviteList({ fleetId, isOwner }: InviteListProps) {
  const { data: invites, isLoading } = useInvites(fleetId);
  const revokeInvite = useRevokeInvite();

  if (isLoading) {
    return (
      <div className="space-y-2">
        <Skeleton className="h-10 w-full" />
        <Skeleton className="h-10 w-full" />
      </div>
    );
  }

  // Show only pending invites (not yet accepted)
  const pending = (invites ?? []).filter((inv) => !inv.attributes.acceptedAt);

  if (pending.length === 0) {
    return <p className="text-sm text-muted-foreground">No pending invites.</p>;
  }

  return (
    <ul className="divide-y">
      {pending.map((inv) => (
        <li key={inv.id} className="flex items-center justify-between py-3">
          <div className="space-y-0.5">
            <div className="text-sm font-medium">{inv.attributes.email}</div>
            <div className="text-xs text-muted-foreground">
              Role: <span className="capitalize">{inv.attributes.role}</span>
              {' · '}
              Expires {new Date(inv.attributes.expiresAt).toLocaleDateString()}
            </div>
          </div>
          {isOwner && (
            <Button
              variant="outline"
              size="sm"
              disabled={revokeInvite.isPending}
              onClick={() => revokeInvite.mutate(inv.id)}
            >
              Revoke
            </Button>
          )}
        </li>
      ))}
    </ul>
  );
}
