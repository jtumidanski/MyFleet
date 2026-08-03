import { useState } from 'react';
import { toast } from 'sonner';
import { createErrorFromUnknown } from '@myfleet/shared-ts';
import { useAuth } from '../../context/AuthContext';
import { Button } from '../ui/button';
import { accountLabel } from './accountLabel';

/**
 * The way out of an account from the fleetless pages (FR-SHARED-1).
 *
 * `/onboarding` and `/invites/:token/accept` are routed OUTSIDE AppLayout
 * (App.tsx), so neither renders FrameHeader and neither renders ProfileMenu —
 * which was, until this component, the only sign-out control in the product. A
 * user who signed in with the wrong Google account had no exit but creating a
 * fleet they did not want or hand-clearing localStorage.
 *
 * It performs NO navigation (FR-SIGNOUT-3). `logout()` flips `hasToken` in
 * AuthContext, the provider re-renders, and RequireAuth's own
 * `<Navigate to="/login" replace>` branch does the rest. Adding a useNavigate
 * here would duplicate a redirect that already happens.
 *
 * It also never clears the token itself (FR-SIGNOUT-1 / NFR-1): the refresh
 * token is an HttpOnly cookie only the server can invalidate, and a control
 * that merely cleared local state would say "signed out" over a session that is
 * still resumable — precisely the gap that matters on the shared machine where
 * "I signed in with the wrong account" most often happens.
 */
export function SignedInFooter() {
  const { user, logout } = useAuth();
  const [signingOut, setSigningOut] = useState(false);

  if (!user) return null;

  const label = accountLabel(user);

  const signOut = async () => {
    setSigningOut(true);
    try {
      await logout();
      // Deliberately no setSigningOut(false) and no navigation on success: the
      // component is about to be unmounted by RequireAuth's redirect, so a
      // reset would be either a no-op or a post-unmount write, and it would
      // briefly re-enable the button mid-redirect. The flag stays latched from
      // the click until the component ceases to exist.
    } catch (err) {
      toast.error(createErrorFromUnknown(err).message || 'Could not sign out');
      setSigningOut(false);
    }
  };

  return (
    <div className="flex w-full max-w-md flex-col items-center gap-1 text-sm">
      {label !== null && (
        <p className="max-w-full truncate text-muted-foreground">Signed in as {label}</p>
      )}
      <p className="text-muted-foreground">
        {/* `Not you?` is a sibling text node, NOT inside the button, so the
            accessible name stays exactly "Sign out" (FR-A11Y-2). A <button> is
            phrasing content, so nesting it in a <p> is valid HTML. */}
        Not you?{' '}
        <Button
          type="button"
          variant="link"
          size="sm"
          // link + h-auto p-0 gives a real <button> with no filled surface and
          // no button-sized box, so it reads as subordinate to "Create Fleet"
          // (FR-ONBOARD-3). text-muted-foreground overrides the variant's
          // text-primary for the same reason.
          className="h-auto p-0 align-baseline text-muted-foreground underline"
          disabled={signingOut}
          // `void` on the handler only because a DOM handler returning a
          // promise is a lint smell. The promise from logout() IS awaited, one
          // frame down inside signOut — which is what makes the rejection
          // catchable at all (FR-SIGNOUT-2).
          onClick={() => void signOut()}
        >
          Sign out
        </Button>
      </p>
    </div>
  );
}
