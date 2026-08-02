/**
 * InviteList — displays pending fleet invites with revoke button (owner-only).
 *
 * Each pending invite shows its accept link. Nothing delivers invites yet
 * (`task-009-smtp-invite-delivery` is specced but unimplemented), so this link
 * is the only way an invite reaches the person it names — without it, creating
 * an invite produced a row nobody could act on.
 */
import { useState } from 'react';
import { Check, Copy } from 'lucide-react';
import { toast } from 'sonner';
import { Skeleton } from '../../ui/skeleton';
import { Button } from '../../ui/button';
import { useInvites, useRevokeInvite } from '../../../lib/hooks/api/invites';
import { inviteAcceptUrl } from '../../../lib/invites/acceptUrl';

interface InviteListProps {
  fleetId: string;
  isOwner: boolean;
}

/**
 * The accept link plus a copy button.
 *
 * `navigator.clipboard` is undefined on insecure origins and can reject when
 * the document is not focused, so a failure keeps the URL on screen and says to
 * copy it by hand rather than leaving the owner believing they hold a link.
 */
function InviteLink({ token }: { token: string }) {
  const [copied, setCopied] = useState(false);
  const url = inviteAcceptUrl(token);

  const copy = async () => {
    try {
      await navigator.clipboard.writeText(url);
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    } catch {
      toast.error('Could not copy the link — select it and copy manually.');
    }
  };

  return (
    <div className="flex items-center gap-2">
      <code className="min-w-0 flex-1 truncate rounded bg-muted px-2 py-1 text-xs">{url}</code>
      <Button
        type="button"
        variant="ghost"
        size="sm"
        aria-label="Copy invite link"
        onClick={() => void copy()}
      >
        {copied ? <Check className="h-4 w-4" /> : <Copy className="h-4 w-4" />}
      </Button>
    </div>
  );
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
        <li key={inv.id} className="space-y-2 py-3">
          <div className="flex items-center justify-between gap-2">
            <div className="min-w-0 space-y-0.5">
              <div className="truncate text-sm font-medium">{inv.attributes.email}</div>
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
          </div>
          <InviteLink token={inv.attributes.token} />
        </li>
      ))}
    </ul>
  );
}
