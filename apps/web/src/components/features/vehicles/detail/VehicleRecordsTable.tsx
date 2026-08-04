import { useState } from 'react';
import { Loader2 } from 'lucide-react';
import { formatMileage, formatMoney } from '@myfleet/ui-components';
import { cn } from '../../../../lib/utils';
import { Card, CardContent, CardHeader, CardTitle } from '../../../ui/card';
import { Button } from '../../../ui/button';
import { Skeleton } from '../../../ui/skeleton';
import { filterVehicleRecords } from '../../../../lib/vehicleRecords';
import type { VehicleRecordKind, VehicleRecordRow } from '../../../../lib/vehicleRecords';

interface VehicleRecordsTableProps {
  rows: VehicleRecordRow[];
  total: number;
  hasMore: boolean;
  isLoading: boolean;
  /**
   * A fetchNextPage request is in flight. Disables Load More: React Query
   * cancels and reissues the in-flight request on each call, so sustained
   * clicking starves the fetch and rows never arrive.
   */
  isFetchingNextPage: boolean;
  onLoadMore: () => void;
  onSelectRow: (row: VehicleRecordRow) => void;
}

const CHIPS: ReadonlyArray<{ value: VehicleRecordKind | 'all'; label: string }> = [
  { value: 'all', label: 'All' },
  { value: 'maintenance', label: 'Maintenance' },
  { value: 'modification', label: 'Mods' },
  { value: 'fuel', label: 'Fuel' },
  { value: 'mileage', label: 'Mileage' },
];

/**
 * Type-cell badge classes, keyed by kind. mileage/fuel/maintenance use the
 * semantic token families; modification reuses the exact violet chip and
 * "intentional status colors" comment already carried by
 * VehicleMaintenanceSection.tsx:312-322 (moved here in Task 17, not
 * duplicated — that file is deleted then). kind has no shadcn semantic
 * equivalent, so violet is deliberately outside the info/success/warning/
 * danger families.
 */
const KIND_BADGE: Record<VehicleRecordKind, string> = {
  mileage: 'bg-info-subtle text-info-subtle-foreground border-info-border',
  fuel: 'bg-muted text-muted-foreground border-border',
  maintenance: 'bg-success-subtle text-success-subtle-foreground border-success-border',
  /* Intentional status colors: kind has no shadcn semantic equivalent; violet marks a modification record. */
  modification: 'border-violet-200 bg-violet-100 text-violet-800',
};

const KIND_LABEL: Record<VehicleRecordKind, string> = {
  mileage: 'Mileage',
  fuel: 'Fuel',
  maintenance: 'Maintenance',
  modification: 'Mod',
};

function TypeBadge({ kind }: { kind: VehicleRecordKind }) {
  return (
    <span
      className={cn(
        'inline-flex items-center rounded-full border px-2.5 py-0.5 text-xs font-medium',
        KIND_BADGE[kind],
      )}
    >
      {KIND_LABEL[kind]}
    </span>
  );
}

function SkeletonRows() {
  return (
    <>
      {[1, 2, 3].map((i) => (
        <tr key={i}>
          <td className="p-2" colSpan={5}>
            <Skeleton className="h-8 w-full" />
          </td>
        </tr>
      ))}
    </>
  );
}

/**
 * The unified records table: one feed across maintenance, modifications,
 * fuel and mileage, narrowed by a chip filter.
 *
 * The footer avoids two traps that both stem from `total` and
 * `withheldCount` (from `mergeVehicleRecords`/`useVehicleRecords`) being
 * cross-source, not per-kind:
 *  - It never renders a "withheldCount more hidden" message — that count is
 *    not kind-aware, so it would over-report once a chip narrows the view.
 *  - Under "All" it reads "Showing X of Y" (Y is the honest cross-source
 *    total). Under any other chip, there is no accurate per-kind total to
 *    compare against, so it reads "Showing X matching <Kind>" instead of
 *    implying `total` applies to that one kind.
 */
export function VehicleRecordsTable({
  rows,
  total,
  hasMore,
  isLoading,
  isFetchingNextPage,
  onLoadMore,
  onSelectRow,
}: VehicleRecordsTableProps) {
  const [activeKind, setActiveKind] = useState<VehicleRecordKind | 'all'>('all');

  const visible = filterVehicleRecords(rows, activeKind);

  // `total` sums across all three record sources (see useVehicleRecords), so
  // comparing it against `visible.length` is only honest under "All" — no
  // per-kind total exists to compare a filtered count against instead.
  // Filtered chips report only the loaded, matching count rather than
  // implying a cross-kind total applies to one kind.
  const activeChipLabel = CHIPS.find((c) => c.value === activeKind)?.label ?? 'records';
  const footerText =
    activeKind === 'all'
      ? `Showing ${visible.length} of ${total}`
      : `Showing ${visible.length} matching ${activeChipLabel}`;

  return (
    <Card>
      <CardHeader className="flex-row flex-wrap items-center justify-between gap-2 space-y-0">
        <CardTitle className="text-base font-semibold">Records</CardTitle>
        <div className="flex flex-wrap gap-2">
          {CHIPS.map((chip) => {
            const isActive = activeKind === chip.value;
            return (
              <Button
                key={chip.value}
                type="button"
                size="sm"
                variant={isActive ? 'default' : 'outline'}
                aria-pressed={isActive}
                onClick={() => setActiveKind(chip.value)}
              >
                {chip.label}
              </Button>
            );
          })}
        </div>
      </CardHeader>
      <CardContent>
        <div className="overflow-x-auto">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b text-left text-xs text-muted-foreground">
                <th className="p-2 font-medium">Date</th>
                <th className="p-2 font-medium">Type</th>
                <th className="p-2 font-medium">Item</th>
                <th className="p-2 font-medium">Odometer</th>
                <th className="p-2 font-medium">Cost</th>
              </tr>
            </thead>
            <tbody>
              {isLoading ? (
                <SkeletonRows />
              ) : visible.length === 0 ? (
                <tr>
                  <td className="p-4 text-center text-sm text-muted-foreground" colSpan={5}>
                    No records yet.
                  </td>
                </tr>
              ) : (
                visible.map((row) => (
                  <tr
                    key={row.id}
                    role="button"
                    tabIndex={0}
                    className="cursor-pointer border-b last:border-b-0 hover:bg-accent/50 focus-visible:outline-hidden focus-visible:ring-2 focus-visible:ring-ring"
                    onClick={() => onSelectRow(row)}
                    onKeyDown={(e) => {
                      if (e.key === 'Enter' || e.key === ' ') {
                        e.preventDefault();
                        onSelectRow(row);
                      }
                    }}
                  >
                    <td className="p-2 text-muted-foreground">
                      {new Date(row.date).toLocaleDateString()}
                    </td>
                    <td className="p-2">
                      <TypeBadge kind={row.kind} />
                    </td>
                    <td className="max-w-[240px] truncate p-2">{row.title}</td>
                    <td className="p-2 text-muted-foreground">
                      {typeof row.mileage === 'number' ? formatMileage(row.mileage) : '—'}
                    </td>
                    <td className="p-2 text-muted-foreground">
                      {typeof row.cost === 'number' ? formatMoney(row.cost) : '—'}
                    </td>
                  </tr>
                ))
              )}
            </tbody>
          </table>
        </div>

        <div className="mt-3 flex items-center justify-between gap-2">
          <p className="text-xs text-muted-foreground">{footerText}</p>
          {hasMore && (
            <Button
              type="button"
              size="sm"
              variant="outline"
              onClick={onLoadMore}
              disabled={isFetchingNextPage}
            >
              {isFetchingNextPage && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
              Load More
            </Button>
          )}
        </div>
      </CardContent>
    </Card>
  );
}
