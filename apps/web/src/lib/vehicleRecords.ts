/** The record types that share the unified vehicle feed. */
export type VehicleRecordKind = 'maintenance' | 'modification' | 'fuel' | 'mileage';

/** One row in the unified feed, normalized from any of the four sources. */
export interface VehicleRecordRow {
  /** `${kind}:${sourceId}` — unique across sources, which may reuse ids. */
  id: string;
  sourceId: string;
  kind: VehicleRecordKind;
  /** ISO 8601. The sort key. */
  date: string;
  title: string;
  mileage?: number;
  cost?: number;
  /** Fuel volume in gallons. Populated by the fuel adapter; used to derive average economy. */
  gallons?: number;
}

/** One paginated source's currently-loaded rows. */
export interface RecordSource {
  /** Loaded rows, any order. */
  rows: VehicleRecordRow[];
  /** True when the source has pages that have not been fetched. */
  hasMore: boolean;
}

export interface MergeResult {
  /** Safe rows, newest first. */
  rows: VehicleRecordRow[];
  /** Loaded rows suppressed because they fall below the watermark. */
  withheldCount: number;
}

/**
 * Merges independently-paginated sources into one newest-first feed.
 *
 * Each source is paginated on its own, so simply concatenating loaded pages
 * produces an order that is wrong past the point where the shallowest source
 * runs out of loaded rows: that source may still hold unfetched rows that
 * belong between the ones already shown.
 *
 * The watermark is the NEWEST "oldest loaded row" among sources that still
 * have unloaded pages — the most constraining source. Rows at or above it are
 * provably ordered; rows below it are withheld until more pages load.
 *
 * Rows are deduped by id before anything else runs. None of the three backend
 * list queries carries an ORDER BY tiebreaker, so a row sharing a timestamp
 * with a page boundary can be re-fetched on a later page as pages accumulate.
 */
export function mergeVehicleRecords(sources: RecordSource[]): MergeResult {
  const dedupedSources = dedupeAcrossSources(sources);

  const all = dedupedSources.flatMap((s) => s.rows);
  const sorted = [...all].sort(compareRows);

  const incomplete = dedupedSources.filter((s) => s.hasMore);

  // A source that has more to load but has loaded nothing tells us nothing
  // about where its rows belong, so no row can be trusted yet.
  if (incomplete.some((s) => s.rows.length === 0)) {
    return { rows: [], withheldCount: sorted.length };
  }

  const watermarks = incomplete.map((s) => oldestDate(s.rows));

  if (watermarks.length === 0) {
    return { rows: sorted, withheldCount: 0 };
  }

  const safeUntil = watermarks.reduce((newest, d) => (d > newest ? d : newest));
  const rows = sorted.filter((r) => r.date >= safeUntil);
  return { rows, withheldCount: sorted.length - rows.length };
}

/**
 * Collapses rows sharing an id, keeping the first occurrence and preserving
 * each source's shape (so per-source watermark logic still applies to the
 * deduped rows). A shared id can arise within one source's own accumulated
 * pages, or — if a caller passes overlapping data — across two sources.
 */
function dedupeAcrossSources(sources: RecordSource[]): RecordSource[] {
  const seen = new Set<string>();
  return sources.map((s) => {
    const rows: VehicleRecordRow[] = [];
    for (const r of s.rows) {
      if (seen.has(r.id)) continue;
      seen.add(r.id);
      rows.push(r);
    }
    return { rows, hasMore: s.hasMore };
  });
}

/**
 * The oldest (smallest) date among a non-empty array of rows. Callers must
 * only invoke this on rows known to be non-empty — the incomplete-source
 * check above guarantees that.
 */
function oldestDate(rows: VehicleRecordRow[]): string {
  return rows.map((r) => r.date).reduce((oldest, d) => (d < oldest ? d : oldest));
}

/** Newest first, with a stable id tiebreak so equal dates never reorder. */
function compareRows(a: VehicleRecordRow, b: VehicleRecordRow): number {
  if (a.date !== b.date) return a.date < b.date ? 1 : -1;
  return a.id < b.id ? -1 : a.id > b.id ? 1 : 0;
}

/** Narrows the feed to one kind. 'all' passes everything through. */
export function filterVehicleRecords(
  rows: VehicleRecordRow[],
  kind: VehicleRecordKind | 'all',
): VehicleRecordRow[] {
  return kind === 'all' ? rows : rows.filter((r) => r.kind === kind);
}
