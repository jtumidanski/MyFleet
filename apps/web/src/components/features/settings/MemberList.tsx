/**
 * MemberList — displays fleet members with remove button (owner-only).
 * Sole-owner guard 409 is surfaced as a toast (in useRemoveMember).
 */
import { Skeleton } from '../../ui/skeleton';
import { Button } from '../../ui/button';
import { useMembers, useRemoveMember } from '../../../lib/hooks/api/members';
import { useAuth } from '../../../context/AuthContext';

interface MemberListProps {
  fleetId: string;
  isOwner: boolean;
}

export function MemberList({ fleetId, isOwner }: MemberListProps) {
  const { user } = useAuth();
  const { data: members, isLoading } = useMembers(fleetId);
  const removeMember = useRemoveMember(fleetId);

  if (isLoading) {
    return (
      <div className="space-y-2">
        <Skeleton className="h-10 w-full" />
        <Skeleton className="h-10 w-full" />
        <Skeleton className="h-10 w-full" />
      </div>
    );
  }

  if (!members || members.length === 0) {
    return <p className="text-sm text-muted-foreground">No members found.</p>;
  }

  return (
    <ul className="divide-y">
      {members.map((m) => {
        const isSelf = m.attributes.userId === user?.id;
        return (
          <li key={m.id} className="flex items-center justify-between py-3">
            <div className="space-y-0.5">
              <div className="text-sm font-medium">{m.attributes.userId}</div>
              <div className="text-xs text-muted-foreground capitalize">{m.attributes.role}</div>
            </div>
            {isOwner && (
              <Button
                variant="destructive"
                size="sm"
                disabled={removeMember.isPending}
                onClick={() => removeMember.mutate(m.attributes.userId)}
              >
                {isSelf ? 'Leave' : 'Remove'}
              </Button>
            )}
          </li>
        );
      })}
    </ul>
  );
}
