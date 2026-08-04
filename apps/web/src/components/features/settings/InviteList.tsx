/**
 * InviteList — pending fleet invites with the accept link, resend and revoke
 * (owner-only).
 *
 * Invites are delivered by email as of task-009, but the visible link and its
 * copy button stay: they are the documented recovery path when a message is
 * spam-filtered, or when a relay outage outlives the sender's bounded retry
 * budget and the email is dropped (design §5.2). The token is already in the
 * fleet-scoped list response, so this needs no API call.
 */
import { useState } from 'react';
import { Check, Copy } from 'lucide-react';
import { toast } from 'sonner';
import { Skeleton } from '../../ui/skeleton';
import { Button } from '../../ui/button';
import { useInvites, useResendInvite, useRevokeInvite } from '../../../lib/hooks/api/invites';
import { inviteAcceptUrl } from '../../../lib/invites/acceptUrl';
import { copyToClipboard } from '../../../lib/utils/clipboard';

interface InviteListProps {
  fleetId: string;
  isOwner: boolean;
}

/**
 * The accept link plus a copy button.
 *
 * Copying goes through `copyToClipboard`, not `navigator.clipboard` directly:
 * that API is undefined on insecure origins, and local dev runs over plain HTTP
 * on `myfleet.home` — the exact environment where this button is tried first.
 * A failure keeps the URL on screen and says to copy it by hand rather than
 * leaving the owner believing they hold a link.
 */
function InviteLink({ token }: { token: string }) {
  const [copied, setCopied] = useState(false);
  const url = inviteAcceptUrl(token);

  const copy = async () => {
    const ok = await copyToClipboard(url);
    if (ok) {
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    } else {
      toast.error('Could not copy the link — select it and copy manually.');
    }
  };

  return (
    <div className="flex items-center gap-2">
      <code className="min-w-0 flex-1 truncate rounded-sm bg-muted px-2 py-1 text-xs">{url}</code>
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
              <div className="flex shrink-0 items-center gap-2">
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
          </div>
          <InviteLink token={inv.attributes.token} />
        </li>
      ))}
    </ul>
  );
}
