import { Monitor, Moon, Sun, type LucideIcon } from 'lucide-react';
import { toast } from 'sonner';
import { useTheme } from '../context/ThemeContext';
import { useUpdateTheme } from '../lib/hooks/api/auth';
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

/**
 * The header theme control — the only place in the app where a theme change and
 * a network mutation fire together, so there is exactly one file to read to
 * understand when theming touches the network.
 *
 * The visual change is applied optimistically and is never rolled back on a
 * failed save (FR-PERSIST-4, FR-PERSIST-5): reverting the theme under the
 * user's cursor is more jarring than a preference that fails to stick, so the
 * toast explains the outcome instead.
 */
export function ThemeToggle() {
  const { preference, setPreference } = useTheme();
  const updateTheme = useUpdateTheme();

  const next = NEXT[preference];
  const { Icon, label } = META[preference];

  const onClick = () => {
    setPreference(next);
    updateTheme.mutate(next, {
      onError: () => {
        toast.error("Couldn't save your theme preference. It'll reset next time you reload.");
      },
    });
  };

  return (
    <Button
      type="button"
      variant="ghost"
      size="icon"
      onClick={onClick}
      title={`Theme: ${label}`}
      aria-label={`Theme: ${label}. Switch to ${META[next].label}.`}
    >
      <Icon className="h-4 w-4" aria-hidden="true" />
    </Button>
  );
}
