import { describe, it, expect } from 'vitest';
import { render, screen } from '@testing-library/react';
import { Badge } from './badge';

describe('Badge', () => {
  it('renders its children', () => {
    render(<Badge>Recoverable</Badge>);
    expect(screen.getByText('Recoverable')).toBeInTheDocument();
  });

  // The console leans on these for purge status, so they must exist as named
  // variants rather than as ad-hoc classNames at each call site — that is how
  // the same status ends up two different colours on two screens.
  it.each(['success', 'warning', 'danger', 'info'] as const)(
    'supports the %s status variant',
    (variant) => {
      const { container } = render(<Badge variant={variant}>x</Badge>);
      expect(container.firstChild).toHaveClass(`bg-${variant}-subtle`);
    },
  );
});
