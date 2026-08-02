import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, act } from '@testing-library/react';
import { ThemeToggleButton } from './ThemeToggleButton';
import type { ThemePreference } from '../types/models/user';

describe('ThemeToggleButton', () => {
  const onSelect = vi.fn<(next: ThemePreference) => void>();

  beforeEach(() => {
    onSelect.mockReset();
  });

  // FR-TOGGLE-2 / FR-TOGGLE-4: the label names the current state AND the next
  // action, so a screen-reader user can operate the cycle without sighted
  // feedback. Now this component's contract rather than ThemeToggle's.
  it.each<[ThemePreference, ThemePreference, string]>([
    ['light', 'dark', 'Theme: light. Switch to dark.'],
    ['dark', 'system', 'Theme: dark. Switch to system.'],
    ['system', 'light', 'Theme: system. Switch to light.'],
  ])('from %s, selects %s and labels itself %s', (preference, next, label) => {
    render(<ThemeToggleButton preference={preference} onSelect={onSelect} />);

    const button = screen.getByRole('button');
    expect(button).toHaveAttribute('aria-label', label);
    expect(button).toHaveAttribute('title', `Theme: ${preference}`);

    act(() => button.click());
    expect(onSelect).toHaveBeenCalledTimes(1);
    expect(onSelect).toHaveBeenCalledWith(next);
  });

  // FR-TOGGLE-3: the icon tracks the PREFERENCE, not the resolved theme, or
  // `system` would be indistinguishable from whichever theme it resolved to.
  // lucide-react stamps a `lucide-<kebab-name>` class onto each <svg>, which is
  // what lets the assertion tell the icons apart.
  it('shows the icon for the given preference', () => {
    render(<ThemeToggleButton preference="system" onSelect={onSelect} />);

    const svg = screen.getByRole('button').querySelector('svg');
    expect(svg).toHaveClass('lucide-monitor');
    expect(svg).not.toHaveClass('lucide-moon');
  });

  // It reads no context and holds no state, so it can be rendered bare — which
  // is the point: it knows nothing about auth (FR-PRETOGGLE-5).
  it('renders without a ThemeProvider', () => {
    expect(() =>
      render(<ThemeToggleButton preference="light" onSelect={onSelect} />),
    ).not.toThrow();
  });
});
