import { cn } from '../../../../lib/utils';

type Severity = 'urgent' | 'recommended' | 'informational';

interface SeverityChipProps {
  severity: string;
  className?: string;
}

// Status colours come from the semantic families in apps/web/src/index.css:
// urgent -> danger, recommended -> warning, informational -> info. Each chip
// also carries a text label, so colour is never the only signal (FR-A11Y-2).
const severityConfig: Record<Severity, { label: string; className: string }> = {
  urgent: {
    label: 'Urgent',
    className: 'bg-danger-subtle text-danger-subtle-foreground border-danger-border',
  },
  recommended: {
    label: 'Recommended',
    className: 'bg-warning-subtle text-warning-subtle-foreground border-warning-border',
  },
  informational: {
    label: 'Info',
    className: 'bg-info-subtle text-info-subtle-foreground border-info-border',
  },
};

/**
 * Displays a severity badge for maintenance schedule items.
 * Maps severity (urgent | recommended | informational) to a color-coded chip.
 */
export function SeverityChip({ severity, className }: SeverityChipProps) {
  const config = severityConfig[severity as Severity];

  if (!config) {
    return (
      <span
        className={cn(
          'inline-flex items-center rounded-full border px-2.5 py-0.5 text-xs font-medium',
          'bg-muted text-muted-foreground border-border',
          className,
        )}
      >
        {severity}
      </span>
    );
  }

  return (
    <span
      className={cn(
        'inline-flex items-center rounded-full border px-2.5 py-0.5 text-xs font-medium',
        config.className,
        className,
      )}
    >
      {config.label}
    </span>
  );
}
