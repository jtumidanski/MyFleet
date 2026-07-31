import { NavLink, Outlet } from 'react-router-dom';
import { useAuth } from '../context/AuthContext';
import { cn } from '../lib/utils';
import { BrandMark } from './BrandMark';
import { ThemeToggle } from './ThemeToggle';
import { Button } from './ui/button';

const NAV = [
  { to: '/', label: 'Dashboard', end: true },
  { to: '/vehicles', label: 'Vehicles' },
  { to: '/maintenance', label: 'Maintenance' },
  { to: '/fuel', label: 'Fuel' },
  { to: '/activity', label: 'Activity' },
  { to: '/notifications', label: 'Notifications' },
  { to: '/settings', label: 'Settings' },
];

export function AppLayout() {
  const { user, logout } = useAuth();

  return (
    <div className="flex min-h-screen">
      {/*
        bg-card, not bg-muted: --muted and --accent are the SAME value in both
        themes, so a muted sidebar would swallow the bg-accent active state and
        the hover state, flattening the nav into one colour.
      */}
      <aside className="w-56 shrink-0 border-r border-border bg-card p-4">
        <div className="mb-6 flex items-center gap-2 text-lg font-semibold">
          <BrandMark className="h-5 w-5" />
          MyFleet
        </div>
        <nav className="flex flex-col gap-1">
          {NAV.map((item) => (
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
        <main className="flex-1 p-6">
          <Outlet />
        </main>
      </div>
    </div>
  );
}
