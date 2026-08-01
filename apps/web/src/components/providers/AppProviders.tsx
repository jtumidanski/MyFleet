import { useState, type ReactNode } from 'react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { Toaster } from 'sonner';
import { AuthProvider } from '../../context/AuthContext';
import { ThemeProvider, useTheme } from '../../context/ThemeContext';
import { ThemeSync } from '../ThemeSync';

/**
 * sonner renders into its own portal AND computes its own colours from a
 * `theme` prop, so unlike the Radix portals it cannot inherit the token cascade
 * and needs the resolved theme passed explicitly (FR-3P-1).
 *
 * This is a separate component because <Toaster> cannot read the theme context
 * from where AppProviders itself renders it — that call site is outside
 * ThemeProvider's subtree.
 */
function ThemedToaster() {
  const { resolvedTheme } = useTheme();
  return <Toaster richColors position="top-right" theme={resolvedTheme} />;
}

/**
 * Root provider stack: React Query client, theme, auth context, and the toast
 * portal. The QueryClient is created once per app instance via useState
 * initializer.
 *
 * ThemeProvider sits ABOVE the toaster (FR-3P-2) and above the app shell, but
 * the authoritative preference arrives from useMe(), which lives BELOW it —
 * ThemeSync bridges the two without either context importing the other.
 */
export function AppProviders({ children }: { children: ReactNode }) {
  const [queryClient] = useState(
    () =>
      new QueryClient({
        defaultOptions: {
          queries: {
            retry: 1,
            refetchOnWindowFocus: false,
          },
        },
      }),
  );

  return (
    <QueryClientProvider client={queryClient}>
      <ThemeProvider>
        <AuthProvider>
          <ThemeSync />
          {children}
        </AuthProvider>
        <ThemedToaster />
      </ThemeProvider>
    </QueryClientProvider>
  );
}
