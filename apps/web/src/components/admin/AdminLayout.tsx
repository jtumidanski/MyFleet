import { NavLink, Outlet, Link } from 'react-router-dom';
import { useAuth } from '../../context/AuthContext';
import { cn } from '../../lib/utils';
import { BrandMark } from '../BrandMark';
import { ThemeToggle } from '../ThemeToggle';
import { Button } from '../ui/button';

const ADMIN_NAV = [
  { to: '/admin', label: 'Overview', end: true },
  { to: '/admin/fleets', label: 'Fleets' },
  { to: '/admin/users', label: 'Users' },
  { to: '/admin/purges', label: 'Purges' },
  { to: '/admin/audit', label: 'Audit log' },
];

/**
 * The admin shell — deliberately NOT AppLayout.
 *
 * A dedicated shell gives destructive tooling an unmistakable mode boundary,
 * makes fleet browsing the centre of the console rather than a side trip, and
 * resolves the fleetless-admin routing problem structurally (FR-ADMIN-UI-2).
 */
export function AdminLayout() {
  const { user, logout } = useAuth();

  return (
    <div className="flex min-h-screen">
      <aside className="w-56 shrink-0 border-r border-border bg-card p-4">
        <div className="mb-6 flex items-center gap-2 text-lg font-semibold">
          <BrandMark className="h-5 w-5" />
          <span>
            MyFleet <span className="text-muted-foreground">admin</span>
          </span>
        </div>
        <nav className="flex flex-col gap-1">
          {ADMIN_NAV.map((item) => (
            <NavLink
              key={item.to}
              to={item.to}
              end={item.end}
              className={({ isActive }) =>
                cn(
                  'rounded px-3 py-2 text-sm font-medium',
                  isActive
                    ? 'bg-accent text-accent-foreground'
                    : 'text-muted-foreground hover:bg-accent hover:text-accent-foreground',
                )
              }
            >
              {item.label}
            </NavLink>
          ))}
        </nav>
        <div className="mt-6 border-t border-border pt-4">
          <Link
            to="/"
            className="rounded px-3 py-2 text-sm font-medium text-muted-foreground hover:bg-accent hover:text-accent-foreground"
          >
            Back to my fleet
          </Link>
        </div>
      </aside>
      <div className="flex flex-1 flex-col">
        <header className="flex items-center justify-between border-b border-border px-6 py-3">
          <span className="text-sm text-muted-foreground">
            {user?.attributes.displayName ?? ''}
          </span>
          <div className="flex items-center gap-2">
            <ThemeToggle />
            <Button type="button" variant="outline" size="sm" onClick={() => void logout()}>
              Sign out
            </Button>
          </div>
        </header>
        {/*
          The persistent mode band (FR-ADMIN-UI-3). danger-subtle, NOT
          --destructive: that token is reserved for destructive CONTROLS under
          the task-003 contract, and this is a mode indicator, not a button.

          It also states the stale-claim caveat in plain words rather than a
          tooltip. An operator who does not know that revoking admin takes up to
          15 minutes will assume a revocation took effect immediately, which is
          the one misunderstanding with an irreversible consequence.
        */}
        <div className="border-b border-danger-border bg-danger-subtle px-6 py-2 text-sm text-danger-subtle-foreground">
          <strong className="font-semibold">Platform admin.</strong> You can see and delete data
          across every fleet on this platform. Admin access is read from your sign-in token, so
          granting or revoking it takes up to 15 minutes to take effect.
        </div>
        <main className="flex-1 p-6">
          <Outlet />
        </main>
      </div>
    </div>
  );
}
