import { useEffect, useState } from 'react';
import { createErrorFromUnknown } from '@myfleet/shared-ts';
import { Button } from '../ui/button';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '../ui/dialog';
import { Input } from '../ui/input';
import { Label } from '../ui/label';
import { RequiredMarker } from '../ui/required';
import { useAdminFleets, useVehicleTransferPreview } from '../../lib/hooks/api/admin';
import { cn } from '../../lib/utils';
import type { VehicleTransferResult } from '../../services/api/AdminService';

/**
 * Move one vehicle, with its whole history, to another fleet.
 *
 * Modelled on PurgeConfirmDialog and keeping its three load-bearing mechanics:
 * the box is cleared during RENDER rather than from an effect, so the first
 * frame after opening is empty; the dialog cannot be dismissed while a request
 * is in flight; and onConfirm receives WHAT WAS TYPED, never the expected
 * phrase, so the server performs the real comparison and its 409 stays
 * reachable.
 *
 * It does NOT reuse BlastRadiusPanel: that component hard-codes a "Delete this
 * fleet" heading and a destructive purge button, and bending it to serve a
 * second, non-destructive caller would mean changing a component that sits
 * under a live purge control. The counts list here is a dozen lines.
 *
 * The destination picker is a search box over a result list rather than a
 * <Select>, because FR-XFER-UI-3 requires searching LIVE fleets by name — a
 * select of preloaded options cannot do that against a platform with more
 * fleets than one page.
 */
export interface VehicleTransferDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  vehicleId: string;
  sourceFleetId: string;
  onConfirm: (args: { destinationFleetId: string; destinationName: string; typed: string }) => void;
  isPending: boolean;
  /**
   * The rejection from the transfer attempt, if there was one.
   *
   * The caller owns the mutation, so it owns the error too — but it is THIS
   * surface that has to show it. Every 403/404/409/422/503 the transfer
   * contract defines carries an actionable `detail`, and the operator is
   * standing in front of this dialog with the destination they picked and the
   * phrase they typed still on screen; a toast that vanishes takes the one
   * sentence that says which of the four 409 conditions they hit with it
   * (FR-XFER-UI-7). Typed `unknown` because that is what React Query's
   * `error` is; it is narrowed through createErrorFromUnknown.
   */
  error?: unknown;
  /**
   * The completed transfer, once there is one.
   *
   * Present only so `meta.count_semantics` reaches a human. `affected_counts`
   * contains two keys — `media_objects` and `notifications` — that are read
   * back from the downstream service as "live rows NOW ON the destination",
   * not "rows this transfer moved", so both can be inflated by rows that were
   * already there. `count_semantics` is the only place in the entire response
   * that says so, and this dialog is its last hop before a human reads the
   * numbers. `meta` is optional at every level: the server omits it whenever
   * neither key applies.
   */
  result?: VehicleTransferResult | null;
}

/** Fixed order so the list does not reshuffle between fetches. */
const COUNT_ORDER = [
  'vehicle_media',
  'media_objects',
  'maintenance_records',
  'maintenance_schedules',
  'fuel_logs',
  'mileage_records',
  'activity_events',
  'notifications',
  'categories_created',
  'widgets_removed',
];

const COUNT_LABELS: Record<string, string> = {
  vehicle_media: 'Photos',
  media_objects: 'Media files',
  maintenance_records: 'Maintenance records',
  maintenance_schedules: 'Maintenance schedules',
  fuel_logs: 'Fuel logs',
  mileage_records: 'Mileage records',
  activity_events: 'Activity events',
  notifications: 'Notifications',
  categories_created: 'Categories to create',
  widgets_removed: 'Dashboard widgets removed',
};

/**
 * A key the server sent that this build does not label still renders, appended
 * after the known order. Omitting it would UNDERSTATE the blast radius, which
 * is the one error that matters on this screen.
 */
function humanise(key: string): string {
  const label = COUNT_LABELS[key];
  if (label) return label;
  const words = key.replace(/_/g, ' ');
  return words.charAt(0).toUpperCase() + words.slice(1);
}

/** Known keys first, in a stable order; anything unrecognised after, sorted. */
function orderedKeys(counts: Record<string, number>): string[] {
  const known = COUNT_ORDER.filter((k) => k in counts);
  const extra = Object.keys(counts)
    .filter((k) => !COUNT_ORDER.includes(k))
    .sort();
  return [...known, ...extra];
}

/** Debounce delay for the fleet search, in ms. */
const SEARCH_DEBOUNCE_MS = 250;

export function VehicleTransferDialog({
  open,
  onOpenChange,
  vehicleId,
  sourceFleetId,
  onConfirm,
  isPending,
  error,
  result,
}: VehicleTransferDialogProps) {
  const [query, setQuery] = useState('');
  const [debouncedQuery, setDebouncedQuery] = useState('');
  const [destination, setDestination] = useState<{ id: string; name: string } | null>(null);
  const [typed, setTyped] = useState('');

  // Reset during render off a remembered `open`, not from an effect. React
  // re-runs this component before committing, so an empty box is what the
  // operator's FIRST frame shows; an effect would paint the previous phrase —
  // and its live confirm button — once.
  const [wasOpen, setWasOpen] = useState(open);
  if (wasOpen !== open) {
    setWasOpen(open);
    if (open) {
      setQuery('');
      setDebouncedQuery('');
      setDestination(null);
      setTyped('');
    }
  }

  // adminKeys.fleetList inlines the params object into the query key and
  // nothing in this console debounces, so search-as-you-type would mint one
  // cache entry and one request PER KEYSTROKE. Debounced here rather than in
  // the hook, because the fleet list's other callers are filter buttons where
  // a delay would feel broken.
  useEffect(() => {
    const t = setTimeout(() => setDebouncedQuery(query), SEARCH_DEBOUNCE_MS);
    return () => clearTimeout(t);
  }, [query]);

  const fleets = useAdminFleets({ q: debouncedQuery, deleted: 'exclude', page: 1 });
  const preview = useVehicleTransferPreview(vehicleId, destination?.id ?? '', open);

  const attrs = preview.data?.attributes;
  // The phrase comes from the PREVIEW, never derived here, so client and server
  // agree on one string (FR-XFER-CONF-2).
  const phrase = attrs?.vehicle_label ?? '';
  // Exact comparison, deliberately: no trim, no case fold, matching the server.
  const matches = phrase !== '' && typed === phrase;

  const options = (fleets.data?.data ?? []).filter((f) => f.id !== sourceFleetId);

  // The preview hook is GATED on a destination — it does not fire until one is
  // chosen. The picker therefore cannot live inside the preview-loaded branch,
  // or the operator would need a preview to choose the thing the preview needs.
  const previewFailed = preview.isError;
  const showConfirmControls = !!attrs && !previewFailed;

  const apiError = error === undefined || error === null ? null : createErrorFromUnknown(error);
  const done = result ?? null;

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      {/* Escape, outside-click and the close button are refused together while
          the request is in flight, so the dialog cannot be half-dismissed over
          a server that is already acting on it. */}
      <DialogContent dismissible={!isPending}>
        <DialogHeader>
          <DialogTitle>{done ? 'Vehicle transferred' : 'Transfer this vehicle'}</DialogTitle>
          <DialogDescription>
            {done
              ? 'The vehicle and its history now belong to the destination fleet.'
              : 'The vehicle and its full history move to another fleet. There is no undo — the correction is a second transfer back.'}
          </DialogDescription>
        </DialogHeader>

        {done ? (
          <CompletedTransfer result={done} destinationName={destination?.name ?? ''} />
        ) : (
          <div className="space-y-4 text-sm">
            <div className="space-y-1">
              <Label htmlFor="transfer-destination-search">
                Search fleets by name
                <RequiredMarker />
              </Label>
              <Input
                id="transfer-destination-search"
                aria-required="true"
                autoComplete="off"
                placeholder="Destination fleet"
                value={query}
                onChange={(e) => setQuery(e.target.value)}
              />
              <ul className="max-h-40 overflow-y-auto rounded-sm border border-border">
                {options.length === 0 ? (
                  <li className="px-3 py-2 text-muted-foreground">No other fleets match.</li>
                ) : (
                  options.map((f) => (
                    <li key={f.id}>
                      <button
                        type="button"
                        className={cn(
                          'block w-full px-3 py-2 text-left hover:bg-accent hover:text-accent-foreground',
                          destination?.id === f.id && 'bg-accent text-accent-foreground',
                        )}
                        aria-pressed={destination?.id === f.id}
                        onClick={() => setDestination({ id: f.id, name: f.attributes.name })}
                      >
                        {f.attributes.name}
                      </button>
                    </li>
                  ))
                )}
              </ul>
            </div>

            {previewFailed ? (
              <div
                role="alert"
                className="rounded-sm border border-danger-border bg-danger-subtle p-3 text-danger-subtle-foreground"
              >
                We could not work out what would move, so the transfer control is unavailable. Close
                this and try again.
              </div>
            ) : !attrs ? (
              // Not a failure: nothing has been asked for yet. Saying so keeps
              // an empty panel from reading as a broken one.
              <p className="text-muted-foreground">
                {destination
                  ? 'Working out what would move…'
                  : 'Choose a destination fleet to see what would move.'}
              </p>
            ) : (
              <>
                <div data-testid="transfer-blast-radius">
                  <p className="font-medium">What moves with it</p>
                  <CountList counts={attrs.counts} />
                </div>

                {attrs.categories_to_create.length > 0 ? (
                  <div>
                    <p className="font-medium">Categories created in the destination fleet</p>
                    <ul className="mt-1 list-inside list-disc text-muted-foreground">
                      {attrs.categories_to_create.map((c) => (
                        <li key={`${c.kind}:${c.name}`}>
                          {c.name} ({c.kind})
                        </li>
                      ))}
                    </ul>
                  </div>
                ) : null}

                {attrs.warnings.length > 0 ? (
                  <div
                    role="status"
                    className="rounded-sm border border-warning-border bg-warning-subtle p-3 text-warning-subtle-foreground"
                  >
                    {attrs.warnings.join(' ')}
                  </div>
                ) : null}

                <div className="space-y-1">
                  <Label htmlFor="transfer-confirmation">
                    Type the vehicle name to confirm
                    <RequiredMarker />
                  </Label>
                  <Input
                    id="transfer-confirmation"
                    aria-required="true"
                    autoComplete="off"
                    value={typed}
                    onChange={(e) => setTyped(e.target.value)}
                  />
                  <p className="text-muted-foreground">{phrase}</p>
                </div>
              </>
            )}

            {apiError ? <TransferRejection error={apiError} /> : null}
          </div>
        )}

        <DialogFooter>
          {done ? (
            <Button type="button" onClick={() => onOpenChange(false)}>
              Done
            </Button>
          ) : (
            <>
              <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
                Cancel
              </Button>
              {showConfirmControls ? (
                <Button
                  type="button"
                  disabled={!matches || !destination || isPending}
                  onClick={() =>
                    destination &&
                    onConfirm({
                      // `typed`, never `phrase`: the server compares this
                      // exactly, and sending the expected value would make its
                      // 409 unreachable from the UI.
                      destinationFleetId: destination.id,
                      destinationName: destination.name,
                      typed,
                    })
                  }
                >
                  Transfer vehicle
                </Button>
              ) : null}
            </>
          )}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

/**
 * One count per row, with the server's correction attached to the row it
 * corrects rather than as a footnote at the bottom of the panel — the reader
 * has to meet the sentence at the number, not after deciding what the number
 * meant.
 */
function CountList({
  counts,
  semantics,
}: {
  counts: Record<string, number>;
  semantics?: Record<string, string>;
}) {
  return (
    <dl className="mt-2 grid gap-2 sm:grid-cols-2">
      {orderedKeys(counts).map((k) => {
        const note = semantics?.[k];
        return (
          <div
            key={k}
            data-testid={`count-${k}`}
            className="flex flex-wrap items-baseline justify-between gap-x-4"
          >
            <dt className="text-muted-foreground">{humanise(k)}</dt>
            <dd className="font-semibold tabular-nums">{counts[k]}</dd>
            {note ? <dd className="w-full text-xs text-muted-foreground">{note}</dd> : null}
          </div>
        );
      })}
    </dl>
  );
}

/** The transfer that already happened, and what its numbers actually mean. */
function CompletedTransfer({
  result,
  destinationName,
}: {
  result: VehicleTransferResult;
  destinationName: string;
}) {
  const attrs = result.data.attributes;
  return (
    <div className="space-y-4 text-sm" data-testid="transfer-outcome">
      <p className="font-medium">Moved to {destinationName || attrs.destination_fleet_id}.</p>
      <div>
        <p className="font-medium">What moved</p>
        {/* meta is absent whenever neither annotated key applies — the counts
            still render, just without corrections. */}
        <CountList counts={attrs.affected_counts} semantics={result.meta?.count_semantics} />
      </div>
    </div>
  );
}

/**
 * The server's own sentence, verbatim.
 *
 * Every rejection this endpoint defines carries a `detail` chosen to tell the
 * operator what to do next — four distinct 409 conditions alone. Replacing it
 * with a generic string here would throw away the only part of the response
 * that distinguishes them.
 */
function TransferRejection({
  error,
}: {
  error: { status: number; message: string; detail?: string };
}) {
  return (
    <div
      role="alert"
      data-testid="transfer-error"
      className="rounded-sm border border-danger-border bg-danger-subtle p-3 text-danger-subtle-foreground"
    >
      <p>{error.detail || error.message || 'The transfer was refused.'}</p>
      {error.status === 503 ? (
        // The server rolls a failed downstream call back WHOLE. Saying so
        // explicitly matters more than it looks: an operator who suspects a
        // half-done move will go hunting for records to fix by hand.
        <p className="mt-1">Nothing was moved. The vehicle is still in its original fleet.</p>
      ) : null}
    </div>
  );
}
