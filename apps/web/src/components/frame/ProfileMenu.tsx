import { useState } from 'react';
import { CircleUser } from 'lucide-react';
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
        <DropdownMenuItem onSelect={() => void logout()}>Sign out</DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
