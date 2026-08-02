import { BrowserRouter, Routes, Route } from 'react-router-dom';
import { RequireAuth } from './components/RequireAuth';
import { AppLayout } from './components/AppLayout';
import { LoginPage } from './pages/LoginPage';
import { OnboardingPage } from './pages/OnboardingPage';
import { VehiclesPage } from './pages/VehiclesPage';
import { VehicleDetailPage } from './pages/VehicleDetailPage';
import { ActivityPage } from './pages/ActivityPage';
import { NotificationsPage } from './pages/NotificationsPage';
import { DashboardPage } from './pages/DashboardPage';
import { SettingsPage } from './pages/SettingsPage';
import { InviteAcceptPage } from './pages/InviteAcceptPage';
import { RequirePlatformAdmin } from './components/admin/RequirePlatformAdmin';
import { AdminLayout } from './components/admin/AdminLayout';
import { AdminOverviewPage } from './pages/admin/AdminOverviewPage';
import { AdminFleetsPage } from './pages/admin/AdminFleetsPage';
import { AdminUsersPage } from './pages/admin/AdminUsersPage';
import { AdminPurgesPage } from './pages/admin/AdminPurgesPage';
import { AdminAuditPage } from './pages/admin/AdminAuditPage';

export function App() {
  return (
    <BrowserRouter>
      <Routes>
        {/* Public */}
        <Route path="/login" element={<LoginPage />} />

        {/* Invite accept — requires auth (user must be logged in to accept) */}
        <Route
          path="/invites/:token/accept"
          element={
            <RequireAuth>
              <InviteAcceptPage />
            </RequireAuth>
          }
        />

        {/* Authenticated-but-fleetless onboarding (guarded, allowed without a fleet) */}
        <Route
          path="/onboarding"
          element={
            <RequireAuth>
              <OnboardingPage />
            </RequireAuth>
          }
        />

        {/* Authenticated app shell */}
        <Route
          element={
            <RequireAuth>
              <AppLayout />
            </RequireAuth>
          }
        >
          <Route index element={<DashboardPage />} />
          <Route path="/vehicles" element={<VehiclesPage />} />
          <Route path="/vehicles/:id" element={<VehicleDetailPage />} />
          <Route path="/activity" element={<ActivityPage />} />
          <Route path="/notifications" element={<NotificationsPage />} />
          <Route path="/settings" element={<SettingsPage />} />
        </Route>

        {/*
          Admin console. A SIBLING of the authenticated shell above, not a child:
          RequireAuth redirects fleetless users to /onboarding, and an admin with
          no fleet — including one who has just run a system purge — must still
          reach every admin screen (FR-ADMIN-UI-4, risks.md R5). Nesting this
          under RequireAuth would reintroduce that redirect; RequireAuth itself
          is deliberately unmodified.
        */}
        <Route
          path="/admin"
          element={
            <RequirePlatformAdmin>
              <AdminLayout />
            </RequirePlatformAdmin>
          }
        >
          <Route index element={<AdminOverviewPage />} />
          <Route path="fleets" element={<AdminFleetsPage />} />
          <Route path="fleets/:id" element={<AdminFleetsPage />} />
          <Route path="users" element={<AdminUsersPage />} />
          <Route path="purges" element={<AdminPurgesPage />} />
          <Route path="audit" element={<AdminAuditPage />} />
        </Route>
      </Routes>
    </BrowserRouter>
  );
}
