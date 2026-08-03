import { useState } from 'react';
import { CircleUser } from 'lucide-react';
import { toast } from 'sonner';
import { createErrorFromUnknown } from '@myfleet/shared-ts';
import { useAuth } from '../../context/AuthContext';
import { Button } from '../ui/button';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '../ui/dropdown-menu';
import { identityLines } from './identityLines';

/**
 * The identity control in the header (FR-PROFILE-1).
 *
 * It absorbs both the loose display-name text and the standalone "Sign out"
 * button the header used to carry, so the right side reads as one row of icon
 * buttons. Defined once and imported by both shells (FR-PROFILE-6).
 *
 * Keyboard behaviour is Radix's default and is deliberately not overridden —
 * notably there is no onCloseAutoFocus handler, which is the usual way
 * focus-return gets broken (FR-PROFILE-5).
 */
export function ProfileMenu() {
  const { user, logout } = useAuth();
  const [avatarFailed, setAvatarFailed] = useState(false);

  const { primary, secondary } = identityLines(user);
  const avatarUrl = (user?.attributes.avatarUrl ?? '').trim();
  const showAvatar = avatarUrl !== '' && !avatarFailed;

  // logout() signs out locally in every case and rejects only to report that
  // the server may not have revoked the refresh-token family. Hence no success
  // toast, and copy that says what is still true rather than "sign-out failed".
  //
  // The message is FIXED rather than apiError.message: WriteError redacts the
  // title of every 5xx to "internal server error", and 500 is the only status
  // this path produces, so the house `message || fallback` pattern would show
  // the user that string and never reach the fallback. `apiError.detail` is
  // undefined on this path today: createErrorFromUnknown is handed an
  // already-built ApiError, which has no `.body`, so it falls through to the
  // generic-Error branch and rebuilds a fresh ApiError with status/code/detail
  // discarded; separately, WriteError only ever sets Detail below 500. sonner
  // omits the description when it is undefined. The field is passed anyway so
  // a future non-5xx failure mode would surface its detail instead of being
  // silently dropped.
  //
  // .catch rather than an async handler because onSelect is Radix's
  // synchronous callback. Raising the toast after RequireAuth has already
  // redirected and unmounted this menu is safe: sonner's toast is a
  // module-level imperative API rendered by the app-root <Toaster>, not
  // component state.
  const handleSignOut = () => {
    logout().catch((err: unknown) => {
      const apiError = createErrorFromUnknown(err);
      toast.error('Signed out on this device, but the server may still have an active session.', {
        description: apiError.detail,
      });
    });
  };

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button type="button" variant="ghost" size="icon" aria-label="Account menu">
          {showAvatar ? (
            // alt="": the button's aria-label already names this control, so
            // the image is decorative. onError flips a broken URL back to the
            // icon rather than leaving a broken frame.
            <img
              src={avatarUrl}
              alt=""
              className="h-6 w-6 rounded-full object-cover"
              onError={() => setAvatarFailed(true)}
            />
          ) : (
            <CircleUser className="h-5 w-5" aria-hidden="true" />
          )}
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="w-56">
        {/* DropdownMenuLabel is non-interactive by construction, so
            FR-PROFILE-3's "non-interactive header" needs no extra work. Both
            lines truncate rather than widening the menu. */}
        <DropdownMenuLabel className="font-normal">
          <p className="truncate text-sm font-medium">{primary}</p>
          {secondary !== '' && (
            <p className="truncate text-xs text-muted-foreground">{secondary}</p>
          )}
        </DropdownMenuLabel>
        <DropdownMenuSeparator />
        <DropdownMenuItem onSelect={handleSignOut}>Sign out</DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
