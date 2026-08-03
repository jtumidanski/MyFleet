import { describe, it, expect } from 'vitest';
import { render, screen } from '@testing-library/react';
import { Separator } from './separator';

describe('Separator', () => {
  // Decorative by default: a divider that announces itself in every sidebar
  // adds noise without adding meaning.
  it('is decorative by default', () => {
    const { container } = render(<Separator />);
    const element = container.firstElementChild;
    expect(element).toHaveAttribute('data-orientation', 'horizontal');
    expect(element).toHaveAttribute('role', 'none');
  });

  it('exposes a separator role when explicitly non-decorative', () => {
    render(<Separator decorative={false} />);
    expect(screen.getByRole('separator')).toBeInTheDocument();
  });

  it('honours the vertical orientation', () => {
    const { container } = render(<Separator orientation="vertical" />);
    expect(container.firstElementChild).toHaveAttribute('data-orientation', 'vertical');
  });
});
