import type { MileageRecord } from '../../../../types/models/mileage';

interface MileageSparklineProps {
  records: MileageRecord[];
  width?: number;
  height?: number;
  className?: string;
}

/**
 * Inline SVG sparkline for the mileage trend.
 * Plots mileage (y-axis) over time (x-axis) using chronologically ordered records.
 * No external dependency — pure SVG path.
 */
export function MileageSparkline({
  records,
  width = 200,
  height = 48,
  className,
}: MileageSparklineProps) {
  if (records.length < 2) {
    return (
      <p className="text-xs text-muted-foreground">
        {records.length === 0
          ? 'No mileage data yet.'
          : 'Add at least two records to see the trend.'}
      </p>
    );
  }

  // Sort chronologically (backend returns chronological order, but sort defensively)
  const sorted = [...records].sort(
    (a, b) =>
      new Date(a.attributes.recordedAt).getTime() -
      new Date(b.attributes.recordedAt).getTime(),
  );

  const times = sorted.map((r) => new Date(r.attributes.recordedAt).getTime());
  const values = sorted.map((r) => r.attributes.mileage);

  const minTime = times[0]!;
  const maxTime = times[times.length - 1]!;
  const minVal = Math.min(...values);
  const maxVal = Math.max(...values);

  const padding = 4;
  const w = width - padding * 2;
  const h = height - padding * 2;

  const timeRange = maxTime - minTime || 1;
  const valRange = maxVal - minVal || 1;

  const points = sorted.map((r, i) => {
    const x = padding + ((times[i]! - minTime) / timeRange) * w;
    const y = padding + h - ((r.attributes.mileage - minVal) / valRange) * h;
    return `${x.toFixed(1)},${y.toFixed(1)}`;
  });

  const polyline = points.join(' ');

  return (
    <svg
      width={width}
      height={height}
      viewBox={`0 0 ${width} ${height}`}
      aria-label="Mileage trend"
      className={className}
    >
      <polyline
        fill="none"
        stroke="currentColor"
        strokeWidth="2"
        strokeLinecap="round"
        strokeLinejoin="round"
        points={polyline}
        className="text-primary"
      />
      {/* Dots at each data point */}
      {points.map((pt, i) => {
        const [x, y] = pt.split(',').map(Number);
        return (
          <circle
            key={i}
            cx={x}
            cy={y}
            r={2.5}
            className="fill-primary"
          />
        );
      })}
    </svg>
  );
}
