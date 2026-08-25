/** The record types that share the unified vehicle feed. */
export type VehicleRecordKind = 'maintenance' | 'modification' | 'fuel' | 'mileage';

/** One row in the unified feed, normalized from any of the four sources. */
export interface VehicleRecordRow {
  /** `${kind}:${sourceId}` — unique across sources, which may reuse ids. */
  id: string;
  sourceId: string;
  kind: VehicleRecordKind;
  /**
   * ISO 8601. The sort key, compared as a string (see `compareRows`) rather
   * than parsed into a `Date`. That is only safe while every source is
   * consistent: a non-UTC offset would invert ordering and corrupt the
   * watermark (the same comparison drives `oldestDate`), and mixing
   * fractional-second precision (`T00:00:00Z` vs `T00:00:00.000Z`) would make
   * identical instants compare unequal, silently bypassing the id tiebreak in
   * `compareRows`. A date-only string (`'2026-03-01'`) also sorts older than
   * any same-day timestamp — adapters must pick one shape per source and use
   * it consistently.
   */
  date: string;
  title: string;
  mileage?: number;
  cost?: number;
  /** Fuel volume in gallons. Populated by the fuel adapter; used to derive average economy. */
  gallons?: number;
  /**
   * Count of documents attached to this record. Populated (possibly 0) only by
   * the maintenance adapter; fuel and mileage have no document concept and
   * leave it unset. Deliberately a plain number, not an icon or component —
   * this module is pure data and its tests assert on plain values (the same
   * reason VehicleCard.tsx:37-40 keeps lucide-react out of modules like this).
   * A count rather than the raw id array: the row never uses the ids, and
   * carrying them would invite exactly the per-attachment fan-out that
   * RecordAttachmentList.tsx:84-87 exists to avoid.
   */
  documentCount?: number;
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
  /**
   * Loaded rows suppressed because they fall below the watermark, counted
   * across all kinds. Not kind-aware: a caller that narrows the visible rows
   * with `filterVehicleRecords` still holds this cross-kind count, so a "N
   * more hidden" footer built from it will over-report while filtered to one
   * kind. (Task 14 renders that footer; fix it there, not here.)
   */
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
 * have unloaded pages — the most constraining source. Rows above it are
 * provably ordered. Rows exactly at it are not fully provable: none of the
 * three backend list queries carries an ORDER BY tiebreaker, so an unfetched
 * row can share the watermark's timestamp and later insert above a row at
 * that same timestamp already shown. `>=` is kept anyway — `>` would blank
 * the feed whenever an incomplete source's loaded page sits entirely on one
 * timestamp (plausible for fuel's day-granularity `date` column). The
 * residual risk is confined to reordering among rows sharing one exact
 * timestamp; it never crosses dates. Rows below the watermark are withheld
 * until more pages load.
 *
 * The watermark is computed from each source's own original rows, before any
 * dedupe — a source's true oldest-loaded date must count every row it
 * actually loaded, even one that another source also reported and therefore
 * loses the dedupe pass below. Deduping the *output* first and reusing the
 * deduped rows for the watermark would let a source's rows be silently
 * emptied by another source's earlier duplicate, tripping the zero-loaded
 * guard and blanking the whole feed even though the source did load a row.
 * Dedupe by id only ever touches the flattened list that becomes `rows`.
 * None of the three backend list queries carries an ORDER BY tiebreaker, so
 * a row sharing a timestamp with a page boundary can be re-fetched on a
 * later page (or reported by a second source) as pages accumulate.
 */
export function mergeVehicleRecords(sources: RecordSource[]): MergeResult {
  const all = dedupeById(sources.flatMap((s) => s.rows));
  const sorted = [...all].sort(compareRows);

  const incomplete = sources.filter((s) => s.hasMore);

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
 * Collapses rows sharing an id, keeping the first occurrence. A shared id can
 * arise within one source's own accumulated pages, or — if a caller passes
 * overlapping data — across two sources.
 */
function dedupeById(rows: VehicleRecordRow[]): VehicleRecordRow[] {
  const seen = new Set<string>();
  const result: VehicleRecordRow[] = [];
  for (const r of rows) {
    if (seen.has(r.id)) continue;
    seen.add(r.id);
    result.push(r);
  }
  return result;
}

/**
 * The oldest (smallest) date among a non-empty array of rows. Callers must
 * only invoke this on rows known to be non-empty — the incomplete-source
 * check above guarantees that.
 */
function oldestDate(rows: VehicleRecordRow[]): string {
  return rows.map((r) => r.date).reduce((oldest, d) => (d < oldest ? d : oldest));
}

/**
 * Newest first, with a stable id tiebreak so equal dates never reorder.
 * Dates compare as strings — see the caveat on `VehicleRecordRow.date`: this
 * only produces correct results (and, via `oldestDate`, a correct watermark)
 * when every row's `date` shares an offset and precision.
 */
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
