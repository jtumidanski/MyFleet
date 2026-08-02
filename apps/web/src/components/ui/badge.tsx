import * as React from 'react';
import { cva, type VariantProps } from 'class-variance-authority';
import { cn } from '../../lib/utils';

/**
 * Status chip.
 *
 * The four status variants use the -subtle / -subtle-foreground / -border token
 * trio from index.css, not the bare --success/--warning/--danger/--info tokens:
 * the bare ones are for TEXT on --background, and a chip needs a fill. `danger`
 * is deliberately not `destructive` — that token is reserved for destructive
 * CONTROLS under the task-003 contract, and a chip is a label, not a button.
 */
const badgeVariants = cva(
  'inline-flex items-center rounded-full border px-2.5 py-0.5 text-xs font-medium transition-colors focus:outline-none focus:ring-2 focus:ring-ring focus:ring-offset-2',
  {
    variants: {
      variant: {
        default: 'border-transparent bg-primary text-primary-foreground',
        secondary: 'border-transparent bg-secondary text-secondary-foreground',
        outline: 'border-border text-foreground',
        success: 'bg-success-subtle text-success-subtle-foreground border-success-border',
        warning: 'bg-warning-subtle text-warning-subtle-foreground border-warning-border',
        danger: 'bg-danger-subtle text-danger-subtle-foreground border-danger-border',
        info: 'bg-info-subtle text-info-subtle-foreground border-info-border',
      },
    },
    defaultVariants: { variant: 'default' },
  },
);

export interface BadgeProps
  extends React.HTMLAttributes<HTMLSpanElement>,
    VariantProps<typeof badgeVariants> {}

const Badge = React.forwardRef<HTMLSpanElement, BadgeProps>(
  ({ className, variant, ...props }, ref) => (
    <span ref={ref} className={cn(badgeVariants({ variant, className }))} {...props} />
  ),
);
Badge.displayName = 'Badge';

export { Badge, badgeVariants };
