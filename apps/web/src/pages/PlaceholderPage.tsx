// Placeholder pages for route areas not yet fully implemented.
// Keeps the router shell complete and navigable.
export function PlaceholderPage({ title }: { title: string }) {
  return (
    <div>
      <h1 className="text-2xl font-semibold">{title}</h1>
      <p className="mt-2 text-sm text-gray-500">Coming soon.</p>
    </div>
  );
}

// DashboardPage → apps/web/src/pages/DashboardPage.tsx (Task 15.7)
// ActivityPage → apps/web/src/pages/ActivityPage.tsx (Task 15.5)
// NotificationsPage → apps/web/src/pages/NotificationsPage.tsx (Task 15.6)
// SettingsPage → apps/web/src/pages/SettingsPage.tsx (Task 15.8)
export const MaintenancePage = () => <PlaceholderPage title="Maintenance" />;
export const FuelPage = () => <PlaceholderPage title="Fuel" />;
