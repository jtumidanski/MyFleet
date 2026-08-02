import type { BadgeProps } from '../../components/ui/badge';
import type { PurgeStatus } from '../../types/models/admin';

/**
 * THE mapping from API vocabulary to user language (FR-ADMIN-UI-12).
 *
 * Nothing else in the console reads a raw status string. Two reasons: the words
 * an operator needs ("Recoverable", "Deleted for good") are not the words the
 * state machine uses, and keeping the translation in one place lets the API and
 * the UI diverge later without drift.
 */

const SERVICE_LABELS: Record<string, string> = {
  media: 'Media',
  notification: 'Notifications',
};

/**
 * A human sentence for a purge's state.
 *
 * `partial` is special: the word tells an operator nothing actionable, so the
 * label names the service that did not finish. That is the difference between
 * "something went wrong" and "press retry, it is media-service".
 */
export function purgeStatusLabel(status: PurgeStatus, failedServices: string[]): string {
  switch (status) {
    case 'pending':
      return 'Recoverable';
    case 'reaped':
      return 'Deleted for good';
    case 'cancelled':
      return 'Restored';
    case 'partial': {
      const names = failedServices.map((s) => SERVICE_LABELS[s] ?? s);
      const [first] = names;
      if (first === undefined) return 'Partly deleted';
      if (names.length === 1) return `${first} not deleted`;
      const last = (names[names.length - 1] ?? '').toLowerCase();
      return `${names.slice(0, -1).join(', ')} and ${last} not deleted`;
    }
    default:
      // A status this build does not know about must still render something a
      // human can read, rather than an empty chip.
      return 'Unknown';
  }
}

/** Badge variant for a purge status. */
export function purgeStatusVariant(status: PurgeStatus): NonNullable<BadgeProps['variant']> {
  switch (status) {
    case 'pending':
      return 'info';
    case 'partial':
      return 'warning';
    case 'reaped':
      return 'danger';
    case 'cancelled':
      return 'success';
    default:
      return 'secondary';
  }
}
