import { matchPath } from 'react-router-dom';

/**
 * One crumb in a trail.
 *
 * `static` crumbs carry their own label and target. The two dynamic kinds stand
 * for a `:id` segment and are rendered by their own component, which resolves
 * the object's name (FR-CRUMBNAME-1) — they carry no label because they do not
 * know it until a query lands.
 */
export type Crumb =
  { kind: 'static'; label: string; to: string } | { kind: 'vehicle' } | { kind: 'fleet' };

export interface Trail {
  pattern: string;
  crumbs: readonly Crumb[];
}

const HOME: Crumb = { kind: 'static', label: 'Home', to: '/' };
const VEHICLES: Crumb = { kind: 'static', label: 'Vehicles', to: '/vehicles' };
const ACTIVITY: Crumb = { kind: 'static', label: 'Activity', to: '/activity' };
const NOTIFICATIONS: Crumb = { kind: 'static', label: 'Notifications', to: '/notifications' };
const SETTINGS: Crumb = { kind: 'static', label: 'Settings', to: '/settings' };
const ADMIN: Crumb = { kind: 'static', label: 'Admin', to: '/admin' };
const FLEETS: Crumb = { kind: 'static', label: 'Fleets', to: '/admin/fleets' };
const USERS: Crumb = { kind: 'static', label: 'Users', to: '/admin/users' };
const PURGES: Crumb = { kind: 'static', label: 'Purges', to: '/admin/purges' };
const AUDIT: Crumb = { kind: 'static', label: 'Audit log', to: '/admin/audit' };

/**
 * Every authenticated route's trail, spelled out (FR-CRUMB-4, FR-CRUMB-8).
 *
 * Explicit trails rather than a walk over pathname prefixes: the prefix walk is
 * fewer lines but implicit — /admin/fleets/:id would only produce
 * Admin / Fleets / «name» if every ancestor happened to be a real route, and
 * FR-CRUMB-2's rule that the console's trail is rooted at Admin rather than
 * Home would become a special case instead of a row. Here, adding a route means
 * adding a row, and the whole requirement is testable as data.
 *
 * ONE table, not one per shell: the pattern determines the root crumb, so the
 * two shells cannot disagree about where a trail starts.
 */
export const TRAILS: readonly Trail[] = [
  { pattern: '/', crumbs: [HOME] },
  { pattern: '/vehicles', crumbs: [HOME, VEHICLES] },
  { pattern: '/vehicles/:id', crumbs: [HOME, VEHICLES, { kind: 'vehicle' }] },
  { pattern: '/activity', crumbs: [HOME, ACTIVITY] },
  { pattern: '/notifications', crumbs: [HOME, NOTIFICATIONS] },
  { pattern: '/settings', crumbs: [HOME, SETTINGS] },
  { pattern: '/admin', crumbs: [ADMIN] },
  { pattern: '/admin/fleets', crumbs: [ADMIN, FLEETS] },
  { pattern: '/admin/fleets/:id', crumbs: [ADMIN, FLEETS, { kind: 'fleet' }] },
  { pattern: '/admin/users', crumbs: [ADMIN, USERS] },
  { pattern: '/admin/purges', crumbs: [ADMIN, PURGES] },
  { pattern: '/admin/audit', crumbs: [ADMIN, AUDIT] },
];

export interface ResolvedTrail {
  crumbs: readonly Crumb[];
  /** The `:id` the matched pattern captured, if it has one. */
  id: string | undefined;
}

/**
 * The trail for a pathname, or null when the path is not one of ours.
 *
 * `end: true` means no two patterns can match the same path, so first-hit-wins
 * is order-independent. Null is the correct answer for /login, /onboarding and
 * /invites/:token/accept (FR-CRUMB-7): they render no shell, so there is no
 * breadcrumb region to suppress.
 */
export function resolveTrail(pathname: string): ResolvedTrail | null {
  for (const trail of TRAILS) {
    const match = matchPath({ path: trail.pattern, end: true }, pathname);
    if (match) return { crumbs: trail.crumbs, id: match.params.id };
  }
  return null;
}
