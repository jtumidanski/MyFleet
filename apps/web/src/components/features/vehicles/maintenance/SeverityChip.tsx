import { cn } from '../../../../lib/utils';

type Severity = 'urgent' | 'recommended' | 'informational';

interface SeverityChipProps {
  severity: string;
  className?: string;
}

// Intentional status colors: severity values have no shadcn semantic equivalent.
// red=urgent, amber=recommended, blue=informational are consistent severity indicators.
const severityConfig: Record<Severity, { label: string; className: string }> = {
  urgent: {
    label: 'Urgent',
    className: 'bg-red-100 text-red-800 border-red-200',
  },
  recommended: {
    label: 'Recommended',
    className: 'bg-amber-100 text-amber-800 border-amber-200',
  },
  informational: {
    label: 'Info',
    className: 'bg-blue-100 text-blue-800 border-blue-200',
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
