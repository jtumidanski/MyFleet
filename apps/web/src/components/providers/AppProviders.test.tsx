import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import { AppProviders } from './AppProviders';
import { resetMatchMedia } from '../../test/setup';
import { ACCESS_TOKEN_KEY } from '../../lib/api/token';
import { THEME_STORAGE_KEY } from '../../lib/theme';

/**
 * Smoke test through the REAL provider nesting (no useAuth mock, no hand-built
 * wrapper). ThemeSync.test.tsx exercises the bridge's logic in isolation, but
 * it mocks useAuth entirely, so it cannot catch a provider-tree reordering —
 * e.g. ThemeSync ending up outside AuthProvider, or ThemeProvider ending up
 * below AuthProvider — the exact requirement this task exists to satisfy.
 * This test renders the actual QueryClientProvider > ThemeProvider >
 * AuthProvider > ThemeSync tree end to end: a real (mocked-at-fetch) useMe()
 * call must reach theme state and flip the `dark` class on <html>.
 */
describe('AppProviders', () => {
  beforeEach(() => {
    localStorage.clear();
    resetMatchMedia();
    document.documentElement.classList.remove('dark');
    vi.restoreAllMocks();
  });

  it('adopts the server theme preference through the real provider tree', async () => {
    localStorage.setItem(ACCESS_TOKEN_KEY, 'tok-123');
    localStorage.setItem(THEME_STORAGE_KEY, 'light');
    vi.spyOn(globalThis, 'fetch').mockResolvedValue(
      new Response(
        JSON.stringify({
          data: {
            id: 'u1',
            type: 'users',
            attributes: {
              email: 'a@b.com',
              displayName: 'A',
              avatarUrl: '',
              themePreference: 'dark',
            },
          },
          meta: { activeFleetId: 'f1', role: 'owner' },
        }),
        { status: 200 },
      ),
    );

    render(
      <AppProviders>
        <div>app content</div>
      </AppProviders>,
    );

    expect(screen.getByText('app content')).toBeInTheDocument();
    await waitFor(() => expect(document.documentElement.classList.contains('dark')).toBe(true));
  });
});
