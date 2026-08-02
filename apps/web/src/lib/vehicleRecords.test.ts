import { describe, it, expect } from 'vitest';
import {
  mergeVehicleRecords,
  filterVehicleRecords,
  type RecordSource,
  type VehicleRecordRow,
} from './vehicleRecords';

function row(kind: VehicleRecordRow['kind'], date: string): VehicleRecordRow {
  return { id: `${kind}:${date}`, sourceId: date, kind, date, title: `${kind} ${date}` };
}

describe('mergeVehicleRecords', () => {
  it('returns an empty result for no sources', () => {
    expect(mergeVehicleRecords([])).toEqual({ rows: [], withheldCount: 0 });
  });

  it('sorts newest first across sources when everything is loaded', () => {
    const sources: RecordSource[] = [
      { rows: [row('fuel', '2026-03-01'), row('fuel', '2026-01-01')], hasMore: false },
      { rows: [row('mileage', '2026-02-01')], hasMore: false },
    ];

    const { rows, withheldCount } = mergeVehicleRecords(sources);

    expect(rows.map((r) => r.date)).toEqual(['2026-03-01', '2026-02-01', '2026-01-01']);
    expect(withheldCount).toBe(0);
  });

  // The watermark: a source with unloaded pages may still hold rows older than
  // its oldest loaded one, so nothing below that date can be safely ordered.
  it('withholds rows older than an incomplete source oldest loaded row', () => {
    const sources: RecordSource[] = [
      // Incomplete, oldest loaded = 2026-02-01. Rows below that may interleave.
      { rows: [row('fuel', '2026-03-01'), row('fuel', '2026-02-01')], hasMore: true },
      { rows: [row('mileage', '2026-04-01'), row('mileage', '2026-01-01')], hasMore: false },
    ];

    const { rows, withheldCount } = mergeVehicleRecords(sources);

    expect(rows.map((r) => r.date)).toEqual(['2026-04-01', '2026-03-01', '2026-02-01']);
    expect(withheldCount).toBe(1); // mileage 2026-01-01 is below the watermark
  });

  // The MOST constraining incomplete source wins: the shallowest coverage.
  it('uses the newest oldest-loaded date among incomplete sources', () => {
    const sources: RecordSource[] = [
      { rows: [row('fuel', '2026-05-01')], hasMore: true }, // oldest = 05-01
      { rows: [row('mileage', '2026-06-01'), row('mileage', '2026-01-01')], hasMore: true }, // oldest = 01-01
    ];

    const { rows } = mergeVehicleRecords(sources);

    // Watermark is 2026-05-01, so mileage 2026-01-01 is withheld.
    expect(rows.map((r) => r.date)).toEqual(['2026-06-01', '2026-05-01']);
  });

  it('withholds everything when an incomplete source has loaded nothing', () => {
    const sources: RecordSource[] = [
      { rows: [], hasMore: true },
      { rows: [row('mileage', '2026-06-01')], hasMore: false },
    ];

    const { rows, withheldCount } = mergeVehicleRecords(sources);

    expect(rows).toEqual([]);
    expect(withheldCount).toBe(1);
  });

  it('ignores empty sources that have nothing more to load', () => {
    const sources: RecordSource[] = [
      { rows: [], hasMore: false },
      { rows: [row('mileage', '2026-06-01')], hasMore: false },
    ];

    expect(mergeVehicleRecords(sources).rows.map((r) => r.date)).toEqual(['2026-06-01']);
  });

  it('breaks date ties deterministically by id', () => {
    const sources: RecordSource[] = [
      { rows: [row('fuel', '2026-03-01')], hasMore: false },
      { rows: [row('mileage', '2026-03-01')], hasMore: false },
    ];

    const first = mergeVehicleRecords(sources).rows.map((r) => r.id);
    const second = mergeVehicleRecords([...sources].reverse()).rows.map((r) => r.id);

    expect(first).toEqual(second);
  });

  // No backend list query has an ORDER BY tiebreaker, so a row sharing a
  // timestamp with a page boundary can be re-fetched on the next page. The
  // same id must never appear twice in the merged feed.
  it("dedupes a row that arrives twice within one source's accumulated pages", () => {
    const duplicate = row('fuel', '2026-03-01');
    const sources: RecordSource[] = [
      // Simulates two accumulated pages that both included the same boundary row.
      { rows: [duplicate, row('fuel', '2026-02-01'), { ...duplicate }], hasMore: false },
    ];

    const { rows, withheldCount } = mergeVehicleRecords(sources);

    expect(rows.map((r) => r.id)).toEqual(['fuel:2026-03-01', 'fuel:2026-02-01']);
    expect(withheldCount).toBe(0);
  });

  it('dedupes a row that arrives twice across two different sources reporting the same id', () => {
    // Same kind+sourceId can legitimately appear in two RecordSource entries
    // if a caller re-passes overlapping data; the merge must still collapse it.
    const shared = row('mileage', '2026-03-01');
    const sources: RecordSource[] = [
      { rows: [shared], hasMore: false },
      { rows: [{ ...shared }, row('mileage', '2026-01-01')], hasMore: false },
    ];

    const { rows } = mergeVehicleRecords(sources);

    expect(rows.map((r) => r.id)).toEqual(['mileage:2026-03-01', 'mileage:2026-01-01']);
  });

  // Regression: dedupe must not run before the watermark is computed. An
  // incomplete source's true oldest-loaded date has to count every row it
  // actually loaded, even one that an earlier source also reported (and that
  // therefore loses the cross-source dedupe pass). If dedupe runs first and
  // the watermark is read off the already-deduped rows, a source that loaded
  // only rows duplicated elsewhere is left looking like it loaded nothing —
  // tripping the zero-loaded guard and blanking the entire feed, even though
  // the source did load a row and its oldest-loaded date (03-01) is known.
  it('computes an incomplete source watermark from its own rows, even when every one is a duplicate', () => {
    const shared = row('mileage', '2026-03-01');
    const sources: RecordSource[] = [
      { rows: [shared, row('mileage', '2026-02-01')], hasMore: false },
      // Everything this source loaded duplicates a row from the source
      // above. It still genuinely loaded that row, and 2026-03-01 is its
      // real oldest-loaded date.
      { rows: [{ ...shared }], hasMore: true },
    ];

    const { rows, withheldCount } = mergeVehicleRecords(sources);

    expect(rows.map((r) => r.date)).toEqual(['2026-03-01']);
    expect(withheldCount).toBe(1); // mileage 2026-02-01 is below the 03-01 watermark
  });
});

describe('filterVehicleRecords', () => {
  const rows = [
    row('fuel', '2026-03-01'),
    row('mileage', '2026-02-01'),
    row('modification', '2026-01-01'),
  ];

  it('returns everything for "all"', () => {
    expect(filterVehicleRecords(rows, 'all')).toHaveLength(3);
  });

  it('narrows to one kind', () => {
    expect(filterVehicleRecords(rows, 'fuel').map((r) => r.kind)).toEqual(['fuel']);
  });

  it('keeps maintenance and modification distinct', () => {
    expect(filterVehicleRecords(rows, 'maintenance')).toHaveLength(0);
    expect(filterVehicleRecords(rows, 'modification')).toHaveLength(1);
  });
});
