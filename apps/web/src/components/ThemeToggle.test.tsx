import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, waitFor, act } from '@testing-library/react';
import { ThemeToggle } from './ThemeToggle';
import { ThemeProvider } from '../context/ThemeContext';
import { THEME_STORAGE_KEY } from '../lib/theme';
import { resetMatchMedia } from '../test/setup';

const mutate = vi.fn();
vi.mock('../lib/hooks/api/auth', () => ({
  useUpdateTheme: () => ({ mutate }),
}));

const toastError = vi.fn();
vi.mock('sonner', () => ({
  toast: { error: (...args: unknown[]) => toastError(...args) },
}));

function renderToggle() {
  return render(
    <ThemeProvider>
      <ThemeToggle />
    </ThemeProvider>,
  );
}

describe('ThemeToggle', () => {
  beforeEach(() => {
    localStorage.clear();
    resetMatchMedia();
    document.documentElement.classList.remove('dark');
    mutate.mockReset();
    toastError.mockReset();
  });

  // FR-TOGGLE-2 / FR-TOGGLE-4: the label names the current state AND the next
  // action, so a screen-reader user can operate the cycle without sighted
  // feedback.
  it('cycles light -> dark -> system -> light with matching labels', () => {
    localStorage.setItem(THEME_STORAGE_KEY, 'light');
    renderToggle();

    const button = () => screen.getByRole('button');
    expect(button()).toHaveAttribute('aria-label', 'Theme: light. Switch to dark.');
    expect(button()).toHaveAttribute('title', 'Theme: light');

    act(() => button().click());
    expect(button()).toHaveAttribute('aria-label', 'Theme: dark. Switch to system.');

    act(() => button().click());
    expect(button()).toHaveAttribute('aria-label', 'Theme: system. Switch to light.');

    act(() => button().click());
    expect(button()).toHaveAttribute('aria-label', 'Theme: light. Switch to dark.');
  });

  it('applies the choice to the document immediately', () => {
    localStorage.setItem(THEME_STORAGE_KEY, 'light');
    renderToggle();

    act(() => screen.getByRole('button').click());
    expect(document.documentElement.classList.contains('dark')).toBe(true);
  });

  it('issues the mutation with the newly chosen preference', () => {
    localStorage.setItem(THEME_STORAGE_KEY, 'light');
    renderToggle();

    act(() => screen.getByRole('button').click());
    expect(mutate).toHaveBeenCalledTimes(1);
    expect(mutate.mock.calls[0]![0]).toBe('dark');
  });

  // FR-PERSIST-5 / FR-TEST-7: the user's intent stays applied on failure. The
  // theme is NOT rolled back — reverting under the cursor is more jarring than
  // a preference that fails to stick — and a non-blocking toast explains it.
  it('keeps the theme applied and toasts when the save fails', async () => {
    localStorage.setItem(THEME_STORAGE_KEY, 'light');
    mutate.mockImplementation((_preference: string, options: { onError: () => void }) => {
      options.onError();
    });
    renderToggle();

    act(() => screen.getByRole('button').click());

    expect(document.documentElement.classList.contains('dark')).toBe(true);
    expect(localStorage.getItem(THEME_STORAGE_KEY)).toBe('dark');
    await waitFor(() =>
      expect(toastError).toHaveBeenCalledWith(
        "Couldn't save your theme preference. It'll reset next time you sign in.",
      ),
    );
  });

  // FR-TOGGLE-3: the icon tracks the PREFERENCE, not the resolved theme, or
  // `system` would be indistinguishable from whichever theme it resolved to.
  it('shows an icon per preference', () => {
    localStorage.setItem(THEME_STORAGE_KEY, 'system');
    renderToggle();

    expect(screen.getByRole('button').querySelector('svg')).toBeTruthy();
  });
});
