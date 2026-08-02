import { Monitor, Moon, Sun, type LucideIcon } from 'lucide-react';
import type { ThemePreference } from '../types/models/user';
import { Button } from './ui/button';

// FR-TOGGLE-2: a fixed cycle, so the control is predictable without a menu.
const NEXT: Record<ThemePreference, ThemePreference> = {
  light: 'dark',
  dark: 'system',
  system: 'light',
};

// FR-TOGGLE-3: keyed on the PREFERENCE, not the resolved theme — otherwise
// `system` would be indistinguishable from whichever theme it resolved to.
const META: Record<ThemePreference, { Icon: LucideIcon; label: string }> = {
  light: { Icon: Sun, label: 'light' },
  dark: { Icon: Moon, label: 'dark' },
  system: { Icon: Monitor, label: 'system' },
};

export interface ThemeToggleButtonProps {
  preference: ThemePreference;
  onSelect: (next: ThemePreference) => void;
}

/**
 * The theme cycle control, with no opinion about what happens next.
 *
 * It holds no state and reads no context, so it knows nothing about auth
 * (FR-PRETOGGLE-5) — which is what lets the signed-out login page use the same
 * control as the authenticated header without a request firing. The header's
 * `PATCH` lives one level up, in ThemeToggle.
 */
export function ThemeToggleButton({ preference, onSelect }: ThemeToggleButtonProps) {
  const next = NEXT[preference];
  const { Icon, label } = META[preference];

  return (
    <Button
      type="button"
      variant="ghost"
      size="icon"
      onClick={() => onSelect(next)}
      title={`Theme: ${label}`}
      aria-label={`Theme: ${label}. Switch to ${META[next].label}.`}
    >
      <Icon className="h-4 w-4" aria-hidden="true" />
    </Button>
  );
}
