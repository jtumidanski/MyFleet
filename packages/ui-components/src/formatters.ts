export function formatMoney(n: number): string {
  return new Intl.NumberFormat('en-US', { style: 'currency', currency: 'USD' }).format(n);
}

export function formatMileage(n: number): string {
  return `${new Intl.NumberFormat('en-US').format(n)} mi`;
}

const DAY_MS = 24 * 60 * 60 * 1000;

// numeric: 'auto' is what yields "yesterday", "last week", and "last month"
// instead of "1 day ago", "1 week ago", "1 month ago".
const RELATIVE = new Intl.RelativeTimeFormat('en-US', { numeric: 'auto' });

/**
 * Formats a timestamp as coarse relative time ("6 days ago", "3 weeks ago",
 * "yesterday"), for slots where the exact instant is noise.
 *
 * `now` is injectable so tests can pin it; a helper whose output depends on the
 * wall clock cannot be asserted on.
 *
 * Returns '' for an unparseable input, which callers render as an em-dash — the
 * same treatment an absent value gets.
 */
export function formatRelativeTime(iso: string, now: Date = new Date()): string {
  const then = Date.parse(iso);
  if (Number.isNaN(then)) return '';

  // A future timestamp means clock skew, not a scheduled event. Clamping to 0
  // keeps "in 3 days" off a slot labelled "Last activity".
  const days = Math.max(0, Math.floor((now.getTime() - then) / DAY_MS));

  if (days < 1) return RELATIVE.format(0, 'day');
  if (days < 7) return RELATIVE.format(-days, 'day');
  if (days < 35) return RELATIVE.format(-Math.floor(days / 7), 'week');
  if (days < 365) return RELATIVE.format(-Math.floor(days / 30), 'month');
  return RELATIVE.format(-Math.floor(days / 365), 'year');
}
