/**
 * DashboardPage — the two role bugs task-015 exists to fix.
 *
 * Before this task the page title lived inside `isOwner &&` in DashboardGrid,
 * so a member or a viewer landed on a page with no title at all. These tests
 * are what stop that regressing: the title is unconditional, only the Add
 * Widget control is owner-gated (FR-13).
 */
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { screen } from '@testing-library/react';
import { renderWithProviders } from '../test/renderWithProviders';
import type { AuthContextValue } from '../context/AuthContext';
import type { FleetRole } from '../types/models/user';
import { DashboardPage } from './DashboardPage';

const mockAuth = vi.fn<() => AuthContextValue>();
vi.mock('../context/AuthContext', () => ({
  useAuth: () => mockAuth(),
}));

// The grid is not under test here; stubbing it keeps this file away from the
// widget components' own data fetching.
vi.mock('../components/features/dashboard/DashboardGrid', () => ({
  DashboardGrid: () => <div data-testid="dashboard-grid" />,
}));

// Mutable so individual tests can flip `isLoading` (Finding 1: the Add Widget
// control must disappear while the initial layout fetch is in flight). A
// fresh object per test would be cleaner, but the mock factory below is
// hoisted above this file's imports, so it has to close over something
// declared inline via vi.hoisted instead.
const dashboardWidgetsMock = vi.hoisted(() => ({
  widgets: [] as { type: string }[],
  isLoading: false,
  addWidget: vi.fn(),
  removeWidget: vi.fn(),
  moveUp: vi.fn(),
  moveDown: vi.fn(),
}));

vi.mock('../components/features/dashboard/useDashboardWidgets', () => ({
  useDashboardWidgets: () => dashboardWidgetsMock,
}));

// Built out in full rather than cast, mirroring baseAuth in AppLayout.test.tsx
// — a cast would silently keep compiling if AuthContextValue gained a field the
// page starts depending on.
function authAs(role: FleetRole | null, activeFleetId: string | null = 'f1'): AuthContextValue {
  return {
    user: null,
    activeFleetId,
    role,
    isAuthenticated: true,
    isLoading: false,
    login: vi.fn(),
    logout: vi.fn(),
  };
}

beforeEach(() => {
  vi.clearAllMocks();
  dashboardWidgetsMock.widgets = [];
  dashboardWidgetsMock.isLoading = false;
});

describe('DashboardPage', () => {
  it.each<FleetRole>(['owner', 'member', 'viewer'])('renders the h1 title for a %s', (role) => {
    mockAuth.mockReturnValue(authAs(role));

    renderWithProviders(<DashboardPage />);

    expect(screen.getByRole('heading', { level: 1, name: 'Dashboard' })).toBeInTheDocument();
  });

  it.each<FleetRole>(['member', 'viewer'])('hides Add Widget from a %s', (role) => {
    mockAuth.mockReturnValue(authAs(role));

    renderWithProviders(<DashboardPage />);

    expect(screen.queryByRole('button', { name: /add widget/i })).not.toBeInTheDocument();
  });

  it('shows Add Widget to an owner', () => {
    mockAuth.mockReturnValue(authAs('owner'));

    renderWithProviders(<DashboardPage />);

    expect(screen.getByRole('button', { name: /add widget/i })).toBeInTheDocument();
  });

  // Finding 1: while the initial layout GET is in flight, `widgets` is `[]`
  // (no server data yet, no local copy either). If Add Widget were live here,
  // an owner could add a widget and the hook's full-replace PUT would wipe
  // out whatever the server actually has once it responds. The control must
  // not even render during the fetch, not just be inert on click.
  it('hides Add Widget from an owner while the layout is still loading', () => {
    mockAuth.mockReturnValue(authAs('owner'));
    dashboardWidgetsMock.isLoading = true;

    renderWithProviders(<DashboardPage />);

    expect(screen.queryByRole('button', { name: /add widget/i })).not.toBeInTheDocument();
  });

  // FR-14: the no-fleet branch differs from the loaded branch in body content
  // only — same header, same container.
  it('renders the same title when no fleet is active', () => {
    mockAuth.mockReturnValue(authAs('owner', null));

    renderWithProviders(<DashboardPage />);

    expect(screen.getByRole('heading', { level: 1, name: 'Dashboard' })).toBeInTheDocument();
    expect(screen.getByText(/no fleet selected/i)).toBeInTheDocument();
  });
});
