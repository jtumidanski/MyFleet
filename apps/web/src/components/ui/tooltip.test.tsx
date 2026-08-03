import { describe, it, expect } from 'vitest';
import { render, screen } from '@testing-library/react';
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from './tooltip';

/**
 * Driven by the controlled `open` prop rather than by hover. Radix opens
 * tooltips from pointer and focus events that jsdom models imperfectly, and
 * the hover BEHAVIOUR is CSS-and-pointer territory that only real Chromium can
 * prove (design §8.4). What is worth pinning here is that the vendored content
 * renders with the tooltip role, because that is what makes the collapsed
 * rail's labels reachable at all.
 */
describe('Tooltip', () => {
  it('renders its content with the tooltip role when open', () => {
    render(
      <TooltipProvider>
        <Tooltip open>
          <TooltipTrigger>Vehicles</TooltipTrigger>
          <TooltipContent>Vehicles</TooltipContent>
        </Tooltip>
      </TooltipProvider>,
    );

    expect(screen.getByRole('tooltip')).toHaveTextContent('Vehicles');
  });

  it('renders nothing when closed', () => {
    render(
      <TooltipProvider>
        <Tooltip open={false}>
          <TooltipTrigger>Vehicles</TooltipTrigger>
          <TooltipContent>Vehicles</TooltipContent>
        </Tooltip>
      </TooltipProvider>,
    );

    expect(screen.queryByRole('tooltip')).not.toBeInTheDocument();
  });
});
