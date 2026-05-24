// Placeholder pages for route areas implemented in later phases (Phase 15).
// Keeps the router shell complete and navigable.
export function PlaceholderPage({ title }: { title: string }) {
  return (
    <div>
      <h1 className="text-2xl font-semibold">{title}</h1>
      <p className="mt-2 text-sm text-gray-500">Coming soon.</p>
    </div>
  );
}

export const DashboardPage = () => <PlaceholderPage title="Dashboard" />;
export const MaintenancePage = () => <PlaceholderPage title="Maintenance" />;
export const FuelPage = () => <PlaceholderPage title="Fuel" />;
export const ActivityPage = () => <PlaceholderPage title="Activity" />;
export const NotificationsPage = () => <PlaceholderPage title="Notifications" />;
export const SettingsPage = () => <PlaceholderPage title="Settings" />;
