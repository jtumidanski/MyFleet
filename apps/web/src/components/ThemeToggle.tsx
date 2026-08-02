import { toast } from 'sonner';
import { useTheme } from '../context/ThemeContext';
import { useUpdateTheme } from '../lib/hooks/api/auth';
import { ThemeToggleButton } from './ThemeToggleButton';

/**
 * The header theme control — the only place in the app where a theme change and
 * a network mutation fire together, so there is exactly one file to read to
 * understand when theming touches the network.
 *
 * The visual change is applied optimistically and is never rolled back on a
 * failed save (FR-PERSIST-4, FR-PERSIST-5): reverting the theme under the
 * user's cursor is more jarring than a preference that fails to stick, so the
 * toast explains the outcome instead.
 *
 * The presentation lives in ThemeToggleButton, which the signed-out login page
 * uses directly — there is no session there, so the mutation must not come
 * along (FR-PRETOGGLE-3).
 */
export function ThemeToggle() {
  const { preference, setPreference } = useTheme();
  const updateTheme = useUpdateTheme();

  return (
    <ThemeToggleButton
      preference={preference}
      onSelect={(next) => {
        setPreference(next);
        updateTheme.mutate(next, {
          onError: () => {
            toast.error("Couldn't save your theme preference. It'll reset next time you reload.");
          },
        });
      }}
    />
  );
}
