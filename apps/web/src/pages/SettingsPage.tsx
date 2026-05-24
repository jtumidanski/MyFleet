/**
 * SettingsPage — fleet settings, member management, invites (Task 15.8).
 * Route: /settings
 */
import { Skeleton } from '../components/ui/skeleton';
import { Card, CardContent } from '../components/ui/card';
import { useAuth } from '../context/AuthContext';
import { useFleet } from '../lib/hooks/api/fleetSettings';
import { FleetNameForm } from '../components/features/settings/FleetNameForm';
import { MemberList } from '../components/features/settings/MemberList';
import { InviteForm } from '../components/features/settings/InviteForm';
import { InviteList } from '../components/features/settings/InviteList';

export function SettingsPage() {
  const { activeFleetId, role } = useAuth();
  const isOwner = role === 'owner';
  const { data: fleet, isLoading } = useFleet(activeFleetId);

  if (!activeFleetId) {
    return (
      <div className="space-y-4">
        <h1 className="text-2xl font-semibold">Settings</h1>
        <p className="text-sm text-muted-foreground">
          No fleet selected. Complete onboarding to get started.
        </p>
      </div>
    );
  }

  return (
    <div className="space-y-8 max-w-2xl">
      <h1 className="text-2xl font-semibold">Fleet Settings</h1>

      {/* Fleet name — owner-only */}
      {isOwner && (
        <Card>
          <CardContent className="pt-6">
            <h2 className="text-base font-semibold mb-4">Fleet Name</h2>
            {isLoading ? (
              <Skeleton className="h-10 w-64" />
            ) : (
              <FleetNameForm
                fleetId={activeFleetId}
                currentName={fleet?.attributes.name ?? ''}
              />
            )}
          </CardContent>
        </Card>
      )}

      {/* Members */}
      <Card>
        <CardContent className="pt-6">
          <h2 className="text-base font-semibold mb-4">Members</h2>
          <MemberList fleetId={activeFleetId} isOwner={isOwner} />
        </CardContent>
      </Card>

      {/* Invites — owner-only */}
      {isOwner && (
        <>
          <Card>
            <CardContent className="pt-6">
              <h2 className="text-base font-semibold mb-4">Pending Invites</h2>
              <InviteList fleetId={activeFleetId} isOwner={isOwner} />
            </CardContent>
          </Card>

          <Card>
            <CardContent className="pt-6">
              <h2 className="text-base font-semibold mb-4">Invite a Member</h2>
              <InviteForm fleetId={activeFleetId} />
            </CardContent>
          </Card>
        </>
      )}
    </div>
  );
}
