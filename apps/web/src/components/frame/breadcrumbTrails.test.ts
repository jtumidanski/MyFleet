import { describe, it, expect } from 'vitest';
import { TRAILS, resolveTrail } from './breadcrumbTrails';

const VEHICLE_ID = '8f14e45f-ceea-467a-9f8e-1b2c3d4e5f60';
const FLEET_ID = '3fa85f64-5717-4562-b3fc-2c963f66afa6';

/** A trail as a readable list: static crumbs by label, dynamic ones by kind. */
function describeTrail(pathname: string): string[] {
  const resolved = resolveTrail(pathname);
  if (!resolved) return [];
  return resolved.crumbs.map((crumb) => (crumb.kind === 'static' ? crumb.label : `«${crumb.kind}»`));
}

/**
 * FR-CRUMB-4's table, one row for one row. This is a pure data test — no
 * render, no providers — and it is the highest-value test in the task: every
 * other breadcrumb assertion checks one representative route, and this checks
 * all twelve.
 */
const EXPECTED: Array<{ path: string; trail: string[] }> = [
  { path: '/', trail: ['Home'] },
  { path: '/vehicles', trail: ['Home', 'Vehicles'] },
  { path: `/vehicles/${VEHICLE_ID}`, trail: ['Home', 'Vehicles', '«vehicle»'] },
  { path: '/activity', trail: ['Home', 'Activity'] },
  { path: '/notifications', trail: ['Home', 'Notifications'] },
  { path: '/settings', trail: ['Home', 'Settings'] },
  { path: '/admin', trail: ['Admin'] },
  { path: '/admin/fleets', trail: ['Admin', 'Fleets'] },
  { path: `/admin/fleets/${FLEET_ID}`, trail: ['Admin', 'Fleets', '«fleet»'] },
  { path: '/admin/users', trail: ['Admin', 'Users'] },
  { path: '/admin/purges', trail: ['Admin', 'Purges'] },
  { path: '/admin/audit', trail: ['Admin', 'Audit log'] },
];

describe('resolveTrail', () => {
  for (const { path, trail } of EXPECTED) {
    it(`resolves ${path} to ${trail.join(' / ')}`, () => {
      expect(describeTrail(path)).toEqual(trail);
    });
  }

  // FR-CRUMB-7: these render no shell, so there is no breadcrumb to suppress —
  // resolveTrail simply has nothing to say about them.
  for (const path of ['/login', '/onboarding', '/invites/abc123/accept', '/nope', '/vehicles/a/b']) {
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
