/**
 * MemberList — fleet members by name, with guarded membership actions.
 *
 * Three actions, each behind its own confirmation:
 *   - Remove {name}  — owners only, on other members' rows
 *   - Make {name} an owner — owners only, on non-owner rows
 *   - Leave — every member, on their own row
 *
 * The Leave action has four states (ux-flow.md). A sole owner with company must
 * name a successor before going; a sole owner who is ALSO the only member has
 * nobody to hand the fleet to and the button is disabled with an explanation.
 */
import { useMemo, useState } from 'react';
import { Skeleton } from '../../ui/skeleton';
import { Button } from '../../ui/button';
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '../../ui/alert-dialog';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '../../ui/select';
import { useMembers, useRemoveMember, useUpdateMemberRole } from '../../../lib/hooks/api/members';
import { useUsers } from '../../../lib/hooks/api/users';
import { useAuth } from '../../../context/AuthContext';
import { displayFor } from '../../../lib/utils/displayName';

interface MemberListProps {
  fleetId: string;
  isOwner: boolean;
}

/**
 * One dialog at a time, by construction: two dialogs cannot be open at once
 * because there is only one piece of state to hold them.
 */
type PendingAction =
  | { kind: 'remove'; userId: string; name: string }
  | { kind: 'promote'; userId: string; name: string }
  | { kind: 'leave' }
  | null;

export function MemberList({ fleetId, isOwner }: MemberListProps) {
  const { user } = useAuth();
  const { data: members, isLoading } = useMembers(fleetId);
  const memberIds = useMemo(() => (members ?? []).map((m) => m.attributes.userId), [members]);
  // A SECOND, independent query — not a select over useMembers. That is what
  // makes "memberships loaded, names failed" renderable (FR-1.7).
  const { data: users } = useUsers(memberIds);

  const removeMember = useRemoveMember(fleetId);
  const updateRole = useUpdateMemberRole(fleetId);

  const [pending, setPending] = useState<PendingAction>(null);
  const [successorId, setSuccessorId] = useState('');

  const activeMembers = members ?? [];
  const ownerCount = activeMembers.filter((m) => m.attributes.role === 'owner').length;
  const memberCount = activeMembers.length;
  // myRole comes from the MEMBERS LIST, not useAuth().role. The list is the
  // database; the auth role is a token claim that can be stale. The leave flow
  // is only correct if myRole and ownerCount agree, and they only agree if both
  // come from the same response.
  const myRole = activeMembers.find((m) => m.attributes.userId === user?.id)?.attributes.role;

  const soleOwner = myRole === 'owner' && ownerCount === 1;
  const needsSuccessor = soleOwner && memberCount > 1; // ux-flow state 3
  const leaveBlocked = soleOwner && memberCount === 1; // ux-flow state 4

  const successorOptions = activeMembers.filter((m) => m.attributes.userId !== user?.id);
  const busy = removeMember.isPending || updateRole.isPending;

  const closeDialog = () => {
    setPending(null);
    setSuccessorId('');
  };

  const confirmRemove = async (userId: string) => {
    try {
      await removeMember.mutateAsync({ userId, isSelf: false });
      closeDialog();
    } catch {
      // useRemoveMember surfaces the failure as a toast; leave the dialog open
      // so the user can retry or cancel.
    }
  };

  const confirmPromote = async (userId: string) => {
    try {
      await updateRole.mutateAsync({ userId, role: 'owner' });
      closeDialog();
    } catch {
      // Toasted by useUpdateMemberRole.
    }
  };

  /**
   * FR-3.8. One function with sequential awaits, NOT two mutations wired to each
   * other's onSuccess: "if the promote fails, the delete is not attempted" is
   * then a property of control flow rather than of callback wiring nobody
   * re-reads.
   *
   * If the promote succeeds and the delete fails, the fleet has two owners and
   * the user is still a member — a valid state, not a corruption. Reopening the
   * dialog lands in ux-flow state 2 (plain leave, no picker) and the retry
   * completes it.
   */
  const confirmLeave = async () => {
    if (!user) return;
    try {
      if (needsSuccessor) {
        await updateRole.mutateAsync({ userId: successorId, role: 'owner' });
      }
      await removeMember.mutateAsync({ userId: user.id, isSelf: true });
      closeDialog();
    } catch {
      // Toasted by the hooks. Navigation onward is not done here: invalidating
      // authKeys refetches /auth/me, which now reports no active fleet, and
      // RequireAuth redirects to onboarding on its own.
    }
  };

  if (isLoading) {
    return (
      <div className="space-y-2">
        <Skeleton className="h-10 w-full" />
        <Skeleton className="h-10 w-full" />
        <Skeleton className="h-10 w-full" />
      </div>
    );
  }

  if (activeMembers.length === 0) {
    return <p className="text-sm text-muted-foreground">No members found.</p>;
  }

  return (
    <>
      <ul className="divide-y">
        {activeMembers.map((m) => {
          const userId = m.attributes.userId;
          const isSelf = userId === user?.id;
          const name = displayFor(userId, users);
          return (
            <li key={m.id} className="flex items-center justify-between gap-4 py-3">
              <div className="space-y-0.5">
                <div className="text-sm font-medium">
                  {name}
                  {isSelf && ' (you)'}
                </div>
                <div className="text-xs text-muted-foreground capitalize">{m.attributes.role}</div>
                {isSelf && leaveBlocked && (
                  <p className="text-sm text-muted-foreground">
                    You are the only member of this fleet, so there is nobody to hand it to.
                  </p>
                )}
              </div>

              <div className="flex shrink-0 items-center gap-2">
                {isSelf ? (
                  <Button
                    variant="destructive"
                    size="sm"
                    disabled={leaveBlocked || busy}
                    onClick={() => setPending({ kind: 'leave' })}
                  >
                    Leave
                  </Button>
                ) : (
                  isOwner && (
                    <>
                      {m.attributes.role !== 'owner' && (
                        <Button
                          variant="outline"
                          size="sm"
                          aria-label={`Make ${name} an owner`}
                          disabled={busy}
                          onClick={() => setPending({ kind: 'promote', userId, name })}
                        >
                          Make owner
                        </Button>
                      )}
                      <Button
                        variant="destructive"
                        size="sm"
                        aria-label={`Remove ${name}`}
                        disabled={busy}
                        onClick={() => setPending({ kind: 'remove', userId, name })}
                      >
                        Remove
                      </Button>
                    </>
                  )
                )}
              </div>
            </li>
          );
        })}
      </ul>

      <AlertDialog open={pending !== null} onOpenChange={(open) => !open && closeDialog()}>
        <AlertDialogContent>
          {pending?.kind === 'remove' && (
            <>
              <AlertDialogHeader>
                <AlertDialogTitle>Remove {pending.name} from this fleet?</AlertDialogTitle>
                <AlertDialogDescription>
                  They will immediately lose access to all of this fleet&apos;s vehicles,
                  maintenance records, and photos. You can invite them back later.
                </AlertDialogDescription>
              </AlertDialogHeader>
              <AlertDialogFooter>
                <AlertDialogCancel disabled={busy}>Cancel</AlertDialogCancel>
                <AlertDialogAction
                  className="bg-destructive text-destructive-foreground hover:bg-destructive/90"
                  disabled={busy}
                  onClick={(e) => {
                    // Keep the dialog mounted while the request is in flight so
                    // a failure can be retried from it.
                    e.preventDefault();
                    void confirmRemove(pending.userId);
                  }}
                >
                  Remove
                </AlertDialogAction>
              </AlertDialogFooter>
            </>
          )}

          {pending?.kind === 'promote' && (
            <>
              <AlertDialogHeader>
                <AlertDialogTitle>Make {pending.name} an owner?</AlertDialogTitle>
                <AlertDialogDescription>
                  They will be able to invite and remove members, rename the fleet, and grant
                  ownership to others. You remain an owner.
                </AlertDialogDescription>
              </AlertDialogHeader>
              <AlertDialogFooter>
                <AlertDialogCancel disabled={busy}>Cancel</AlertDialogCancel>
                <AlertDialogAction
                  disabled={busy}
                  onClick={(e) => {
                    e.preventDefault();
                    void confirmPromote(pending.userId);
                  }}
                >
                  Make owner
                </AlertDialogAction>
              </AlertDialogFooter>
            </>
          )}

          {pending?.kind === 'leave' && (
            <>
              <AlertDialogHeader>
                <AlertDialogTitle>Leave this fleet?</AlertDialogTitle>
                <AlertDialogDescription>
                  {needsSuccessor
                    ? 'You are the only owner. Choose who takes over before you go.'
                    : "You will lose access to all of this fleet's vehicles, maintenance records, and photos. Rejoining requires a new invite from an owner."}
                </AlertDialogDescription>
              </AlertDialogHeader>

              {needsSuccessor && (
                <div className="space-y-2">
                  <label className="text-sm font-medium" htmlFor="successor">
                    New owner
                  </label>
                  <Select value={successorId} onValueChange={setSuccessorId}>
                    <SelectTrigger id="successor">
                      <SelectValue placeholder="Select a member" />
                    </SelectTrigger>
                    <SelectContent>
                      {/* D8: viewers are offered too. Excluding them would leave
                          an owner whose only companion is a viewer with an empty
                          picker and no way out. */}
                      {successorOptions.map((m) => (
                        <SelectItem key={m.id} value={m.attributes.userId}>
                          {displayFor(m.attributes.userId, users)} — {m.attributes.role}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                  <p className="text-sm text-muted-foreground">
                    You will lose access to all of this fleet&apos;s vehicles, maintenance records,
                    and photos. Rejoining requires a new invite.
                  </p>
                </div>
              )}

              <AlertDialogFooter>
                <AlertDialogCancel disabled={busy}>Cancel</AlertDialogCancel>
                <AlertDialogAction
                  className="bg-destructive text-destructive-foreground hover:bg-destructive/90"
                  disabled={busy || (needsSuccessor && !successorId)}
                  onClick={(e) => {
                    e.preventDefault();
                    void confirmLeave();
                  }}
                >
                  {needsSuccessor ? 'Transfer & leave' : 'Leave'}
                </AlertDialogAction>
              </AlertDialogFooter>
            </>
          )}
        </AlertDialogContent>
      </AlertDialog>
    </>
  );
}
