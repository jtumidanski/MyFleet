import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, resolve } from 'node:path';
import { describe, it, expect } from 'vitest';
import { TRAILS, resolveTrail } from './breadcrumbTrails';

const VEHICLE_ID = '8f14e45f-ceea-467a-9f8e-1b2c3d4e5f60';
const FLEET_ID = '3fa85f64-5717-4562-b3fc-2c963f66afa6';

/**
 * A trail as a readable, checkable list: static crumbs as `[label, to]` pairs
 * (so a swapped target is caught, not just a swapped label), dynamic ones by
 * kind marker.
 */
type CrumbDescriptor = [label: string, to: string] | string;

function describeTrail(pathname: string): CrumbDescriptor[] {
  const resolved = resolveTrail(pathname);
  if (!resolved) return [];
  return resolved.crumbs.map((crumb) =>
    crumb.kind === 'static' ? [crumb.label, crumb.to] : `«${crumb.kind}»`,
  );
}

/** For test names only — the label (or dynamic marker), never the target. */
function labelsOf(trail: CrumbDescriptor[]): string {
  return trail.map((crumb) => (Array.isArray(crumb) ? crumb[0] : crumb)).join(' / ');
}

/**
 * FR-CRUMB-4's table, one row for one row. This is a pure data test — no
 * render, no providers — and it is the highest-value test in the task: every
 * other breadcrumb assertion checks one representative route, and this checks
 * all twelve. Each static crumb is checked as `[label, to]` so a target
 * swapped between two otherwise-correct labels (e.g. Vehicles' target
 * accidentally set to Activity's) fails here, not just a bare label mismatch.
 */
const EXPECTED: Array<{ path: string; trail: CrumbDescriptor[] }> = [
  { path: '/', trail: [['Home', '/']] },
  {
    path: '/vehicles',
    trail: [
      ['Home', '/'],
      ['Vehicles', '/vehicles'],
    ],
  },
  {
    path: `/vehicles/${VEHICLE_ID}`,
    trail: [['Home', '/'], ['Vehicles', '/vehicles'], '«vehicle»'],
  },
  {
    path: '/activity',
    trail: [
      ['Home', '/'],
      ['Activity', '/activity'],
    ],
  },
  {
    path: '/notifications',
    trail: [
      ['Home', '/'],
      ['Notifications', '/notifications'],
    ],
  },
  {
    path: '/settings',
    trail: [
      ['Home', '/'],
      ['Settings', '/settings'],
    ],
  },
  { path: '/admin', trail: [['Admin', '/admin']] },
  {
    path: '/admin/fleets',
    trail: [
      ['Admin', '/admin'],
      ['Fleets', '/admin/fleets'],
    ],
  },
  {
    path: `/admin/fleets/${FLEET_ID}`,
    trail: [['Admin', '/admin'], ['Fleets', '/admin/fleets'], '«fleet»'],
  },
  {
    path: '/admin/users',
    trail: [
      ['Admin', '/admin'],
      ['Users', '/admin/users'],
    ],
  },
  {
    path: '/admin/purges',
    trail: [
      ['Admin', '/admin'],
      ['Purges', '/admin/purges'],
    ],
  },
  {
    path: '/admin/audit',
    trail: [
      ['Admin', '/admin'],
      ['Audit log', '/admin/audit'],
    ],
  },
];

describe('resolveTrail', () => {
  for (const { path, trail } of EXPECTED) {
    it(`resolves ${path} to ${labelsOf(trail)}`, () => {
      expect(describeTrail(path)).toEqual(trail);
    });
  }

  // FR-CRUMB-7: these render no shell, so there is no breadcrumb to suppress —
  // resolveTrail simply has nothing to say about them.
  for (const path of [
    '/login',
    '/onboarding',
    '/invites/abc123/accept',
    '/nope',
    '/vehicles/a/b',
  ]) {
    it(`resolves ${path} to nothing`, () => {
      expect(resolveTrail(path)).toBeNull();
    });
  }

  it('captures the vehicle id from /vehicles/:id', () => {
    expect(resolveTrail(`/vehicles/${VEHICLE_ID}`)?.id).toBe(VEHICLE_ID);
  });

  it('captures the fleet id from /admin/fleets/:id', () => {
    expect(resolveTrail(`/admin/fleets/${FLEET_ID}`)?.id).toBe(FLEET_ID);
  });

  it('leaves the id undefined on static routes', () => {
    expect(resolveTrail('/vehicles')?.id).toBeUndefined();
  });
});

describe('TRAILS', () => {
  it('covers exactly the twelve routes in FR-CRUMB-4', () => {
    expect(TRAILS).toHaveLength(EXPECTED.length);
  });

  it('has no duplicate patterns', () => {
    const patterns = TRAILS.map((trail) => trail.pattern);
    expect(new Set(patterns).size).toBe(patterns.length);
  });

  // FR-CRUMB-2: each shell's trail is rooted at that shell's own root. The
  // console is a SIBLING of the dashboard, not a descendant of it — a Home
  // crumb inside /admin would be a second, differently worded exit sitting in
  // the one row that describes location rather than offering destinations.
  // "Back to my fleet" (FR-NAV-7) is the way out.
  it('roots admin trails at Admin and never at Home', () => {
    for (const trail of TRAILS.filter((t) => t.pattern.startsWith('/admin'))) {
      const [first] = trail.crumbs;
      expect(first).toEqual({ kind: 'static', label: 'Admin', to: '/admin' });
      const labels = trail.crumbs.map((crumb) => (crumb.kind === 'static' ? crumb.label : ''));
      expect(labels).not.toContain('Home');
    }
  });

  it('roots fleet trails at Home', () => {
    for (const trail of TRAILS.filter((t) => !t.pattern.startsWith('/admin'))) {
      const [first] = trail.crumbs;
      expect(first).toEqual({ kind: 'static', label: 'Home', to: '/' });
    }
  });

  // FR-CRUMB-6: an intermediate crumb links to its own route, so "Vehicles"
  // goes to /vehicles rather than anywhere clever.
  it('gives every static crumb a target that is itself a known pattern', () => {
    const patterns = new Set(TRAILS.map((trail) => trail.pattern));
    for (const trail of TRAILS) {
      for (const crumb of trail.crumbs) {
        if (crumb.kind === 'static') expect(patterns.has(crumb.to)).toBe(true);
      }
    }
  });

  // The resolving crumbs are their own components mounted only when their
  // trail is active (FR-CRUMBNAME-7); a dynamic crumb anywhere but last would
  // mean a link whose href we cannot build.
  it('only ever places a dynamic crumb last', () => {
    for (const trail of TRAILS) {
      trail.crumbs.forEach((crumb, index) => {
        if (crumb.kind !== 'static') expect(index).toBe(trail.crumbs.length - 1);
      });
    }
  });
});

/**
 * Everything above pins TRAILS against ITSELF — twelve internally-consistent
 * rows. Nothing so far pins it against the actual route tree: add a route to
 * App.tsx and forget its trail here, and nothing goes red. That page simply
 * gets no breadcrumb — AppBreadcrumb resolves null and renders nothing — which
 * is invisible in both a diff and a screenshot.
 *
 * This reads App.tsx as source (the same idiom as src/test/conventions.test.ts
 * and sidebarTokens.test.ts) and extracts every path that renders INSIDE one
 * of the two authenticated shells, then asserts resolveTrail knows each one.
 *
 * Deliberately excluded, by scope rather than by accident:
 *  - /login: unauthenticated, no shell.
 *  - /onboarding and /invites/:token/accept: RequireAuth-guarded, but NOT
 *    nested inside AppLayout or AdminLayout — they render no shell, so
 *    FR-CRUMB-7 says there is no breadcrumb region to suppress.
 * Both shells wrap their children in a single non-self-closing <Route>, and
 * every child Route is self-closing, so the block from the shell component's
 * tag to the next literal `</Route>` is exactly that shell's route list —
 * no comment-text dependency, no JSX parser required.
 */
describe('App.tsx route tree vs breadcrumbTrails', () => {
  const FRAME_DIR = dirname(fileURLToPath(import.meta.url));
  const WEB_ROOT = resolve(FRAME_DIR, '../../..');
  const APP_TSX = readFileSync(resolve(WEB_ROOT, 'src/App.tsx'), 'utf8');

  /** The child-route block for the shell component whose tag is `marker`. */
  function shellBlock(marker: string): string {
    const start = APP_TSX.indexOf(marker);
    if (start === -1) throw new Error(`App.tsx: could not find ${marker}`);
    const end = APP_TSX.indexOf('</Route>', start);
    if (end === -1) throw new Error(`App.tsx: ${marker}'s enclosing <Route> is never closed`);
    return APP_TSX.slice(start, end);
  }

  /** Every route inside a shell block, as absolute pathnames. */
  function extractPaths(block: string, rootPrefix: string): string[] {
    const paths: string[] = [];
    if (/<Route\s+index\b/.test(block)) paths.push(rootPrefix === '' ? '/' : rootPrefix);
    const pathPattern = /<Route\s+path="([^"]+)"/g;
    let match: RegExpExecArray | null;
    while ((match = pathPattern.exec(block))) {
      const raw = match[1] as string;
      paths.push(raw.startsWith('/') ? raw : `${rootPrefix}/${raw}`);
    }
    return paths;
  }

  const appShellPaths = extractPaths(shellBlock('<AppLayout'), '');
  const adminShellPaths = extractPaths(shellBlock('<AdminLayout'), '/admin');
  const shellRoutes = [...appShellPaths, ...adminShellPaths];

  // A guard on the extraction itself: if this count drifts, the paths below
  // are extracting the wrong thing rather than reflecting a route change.
  it('finds twelve routes rendering inside a shell today', () => {
    expect(shellRoutes).toHaveLength(12);
  });

  for (const path of shellRoutes) {
    it(`resolveTrail knows the shell route ${path}`, () => {
      expect(resolveTrail(path)).not.toBeNull();
    });
  }
});
