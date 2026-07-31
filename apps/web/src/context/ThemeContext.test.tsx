import { describe, it, expect, beforeEach } from 'vitest';
import { render, screen, act } from '@testing-library/react';
import { ThemeProvider, useTheme } from './ThemeContext';
import { THEME_STORAGE_KEY } from '../lib/theme';
import { setPrefersDark, resetMatchMedia, mediaListenerCount } from '../test/setup';

function Probe() {
  const { preference, resolvedTheme, setPreference, adoptServerPreference } = useTheme();
  return (
    <div>
      <span data-testid="preference">{preference}</span>
      <span data-testid="resolved">{resolvedTheme}</span>
      <button type="button" onClick={() => setPreference('dark')}>
        choose dark
      </button>
      <button type="button" onClick={() => adoptServerPreference('light')}>
        adopt light
      </button>
    </div>
  );
}

function renderProvider() {
  return render(
    <ThemeProvider>
      <Probe />
    </ThemeProvider>,
  );
}

describe('ThemeProvider', () => {
  beforeEach(() => {
    localStorage.clear();
    resetMatchMedia();
    document.documentElement.classList.remove('dark');
  });

  it('defaults to system when nothing is cached', () => {
    renderProvider();
    expect(screen.getByTestId('preference')).toHaveTextContent('system');
    expect(screen.getByTestId('resolved')).toHaveTextContent('light');
  });

  it('resolves the initial theme from the cache', () => {
    localStorage.setItem(THEME_STORAGE_KEY, 'dark');
    renderProvider();
    expect(screen.getByTestId('preference')).toHaveTextContent('dark');
    expect(screen.getByTestId('resolved')).toHaveTextContent('dark');
    expect(document.documentElement.classList.contains('dark')).toBe(true);
  });

  it('falls back to system for a corrupted cached value', () => {
    localStorage.setItem(THEME_STORAGE_KEY, 'purple');
    renderProvider();
    expect(screen.getByTestId('preference')).toHaveTextContent('system');
  });

  // FR-THEME-5: the OS flipping at sunset updates the app with no reload.
  it('follows live media changes while on system', () => {
    renderProvider();
    expect(screen.getByTestId('resolved')).toHaveTextContent('light');

    act(() => setPrefersDark(true));
    expect(screen.getByTestId('resolved')).toHaveTextContent('dark');
    expect(document.documentElement.classList.contains('dark')).toBe(true);
  });

  // FR-THEME-6: an explicit choice pins the theme against the OS.
  it('ignores media changes while on an explicit preference', () => {
    localStorage.setItem(THEME_STORAGE_KEY, 'light');
    renderProvider();

    act(() => setPrefersDark(true));
    expect(screen.getByTestId('resolved')).toHaveTextContent('light');
    expect(document.documentElement.classList.contains('dark')).toBe(false);
  });

  it('removes the media listener on unmount', () => {
    const { unmount } = renderProvider();
    expect(mediaListenerCount()).toBeGreaterThan(0);
    unmount();
    expect(mediaListenerCount()).toBe(0);
  });

  it('setPreference applies and caches the choice', () => {
    renderProvider();
    act(() => screen.getByText('choose dark').click());

    expect(screen.getByTestId('resolved')).toHaveTextContent('dark');
    expect(localStorage.getItem(THEME_STORAGE_KEY)).toBe('dark');
  });

  // FR-PERSIST-3: the first server value of a session wins over the cache,
  // which is what makes the preference land on a newly signed-in device.
  it('adoptServerPreference overrides the cache before the user chooses', () => {
    localStorage.setItem(THEME_STORAGE_KEY, 'dark');
    renderProvider();
    act(() => screen.getByText('adopt light').click());

    expect(screen.getByTestId('preference')).toHaveTextContent('light');
    expect(localStorage.getItem(THEME_STORAGE_KEY)).toBe('light');
  });

  // FR-PERSIST-5: once the user has chosen, a background refetch carrying the
  // pre-change server value must not flip the theme out from under them.
  it('adoptServerPreference is a no-op after the user has chosen', () => {
    renderProvider();
    act(() => screen.getByText('choose dark').click());
    act(() => screen.getByText('adopt light').click());

    expect(screen.getByTestId('preference')).toHaveTextContent('dark');
    expect(localStorage.getItem(THEME_STORAGE_KEY)).toBe('dark');
  });
});
