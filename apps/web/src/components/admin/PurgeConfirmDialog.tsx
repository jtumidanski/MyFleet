import { useState } from 'react';
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

/**
 * The purge confirmation (FR-ADMIN-UI-10).
 *
 * Four things this dialog does that a plain "are you sure" does not:
 *
 *  - the confirm control stays unavailable until the typed value matches
 *    EXACTLY — no trimming, no case folding, mirroring the server's comparison;
 *  - it states the blast radius in PEOPLE as well as rows, because rows are not
 *    what an operator is actually worried about;
 *  - the recovery deadline is an absolute date and time, not a duration. A
 *    duration makes someone do arithmetic under pressure and get it wrong;
 *  - a system purge additionally names what SURVIVES, so the operator is not
 *    left guessing whether they are about to destroy the accounts too.
 *
 * The disabled button is a courtesy. The server's 409 on a mismatched phrase is
 * the actual control (risks.md R9) — which is why onConfirm receives what was
 * TYPED rather than the expected phrase, so the server has something real to
 * compare.
 */
export interface PurgeConfirmDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  scope: 'fleet' | 'system';
  /** The exact phrase the operator must type: the fleet name, or PURGE EVERYTHING. */
  confirmationPhrase: string;
  counts: Record<string, number>;
  /**
   * People affected — stated separately because rows are not the point.
   *
   * null means auth-service could not be reached. That renders as an em dash,
   * never as 0: telling an operator "this affects 0 people" immediately before
   * they destroy a platform, when the truth is that nobody could ask, is the
   * worst possible place to get that distinction wrong (FR-ADMIN-UI-6).
   */
  peopleCount: number | null;
  /** ISO deadline after which the purge becomes irreversible. */
  recoveryDeadline: string;
  /**
   * Receives WHAT THE OPERATOR TYPED, not the expected phrase.
   *
   * This is the whole point: the caller forwards this verbatim so the server
   * performs the real comparison. Passing the expected phrase instead would
   * make the client-side disabled button the only gate, and the server's 409
   * unreachable from the UI.
   */
  onConfirm: (typed: string) => void;
  isPending: boolean;
}

// Lowercase on purpose: these render mid-sentence, as "4 vehicles". That is the
// opposite of BlastRadiusPanel's LABELS, where each one starts its own row.
const COUNT_LABELS: Record<string, string> = {
  vehicles: 'vehicles',
  maintenance_records: 'maintenance records',
  maintenance_record_documents: 'maintenance record documents',
  maintenance_schedules: 'maintenance schedules',
  fuel_logs: 'fuel logs',
  mileage_records: 'mileage records',
  activity_events: 'activity events',
  memberships: 'memberships',
  invites: 'invites',
  dashboards: 'dashboards',
  dashboard_widgets: 'dashboard widgets',
  vehicle_media: 'vehicle media',
  fleets: 'fleets',
};

function formatDeadline(iso: string): string {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return iso;
  return new Intl.DateTimeFormat(undefined, { dateStyle: 'long', timeStyle: 'short' }).format(d);
}

export function PurgeConfirmDialog({
  open,
  onOpenChange,
  scope,
  confirmationPhrase,
  counts,
  peopleCount,
  recoveryDeadline,
  onConfirm,
  isPending,
}: PurgeConfirmDialogProps) {
  const [typed, setTyped] = useState('');

  // Clear the box whenever the dialog opens. Leaving a previously-correct
  // phrase in place would mean the second purge of a session starts with the
  // confirm button already live.
  //
  // Adjusted during render off a remembered `open` rather than from an effect
  // (react-hooks/set-state-in-effect): React re-runs this component before
  // committing, so the empty box is what the user's first frame shows — an
  // effect would paint the stale phrase, and its live confirm button, once.
  const [wasOpen, setWasOpen] = useState(open);
  if (wasOpen !== open) {
    setWasOpen(open);
    if (open) setTyped('');
  }

  // Exact comparison, deliberately. Trimming or case folding here would make the
  // phrase a formality and put the client out of step with the server, which
  // does neither.
  const matches = typed === confirmationPhrase;

  const entries = Object.entries(counts).filter(([, n]) => n > 0);
  const label = scope === 'system' ? 'Delete everything' : 'Delete this fleet';
  const promptLabel =
    scope === 'system' ? 'Type PURGE EVERYTHING to confirm' : 'Type the fleet name to confirm';

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      {/*
        dismissible={false} while the purge is in flight (main's Dialog gained
        this): Escape, outside-click and the close button are refused together,
        so an operator cannot half-dismiss a dialog whose request is already on
        its way to a server that will act on it.
      */}
      <DialogContent dismissible={!isPending}>
        <DialogHeader>
          <DialogTitle>{label}</DialogTitle>
          <DialogDescription>
            This removes data across every affected fleet. It can be undone until the deadline
            below, after which it is permanent.
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-4 text-sm">
          <div>
            {peopleCount === null ? (
              <>
                <p className="font-medium">This affects an unknown number of people.</p>
                <p className="text-muted-foreground">
                  We could not reach the account service to count them — the figure is not zero, it
                  is unavailable.
                </p>
              </>
            ) : (
              <>
                <p className="font-medium">This affects {peopleCount} people.</p>
                <p className="text-muted-foreground">
                  They will lose access to the data listed below the next time they sign in.
                </p>
              </>
            )}
          </div>

          <div>
            <p className="font-medium">What will be deleted</p>
            <ul className="mt-1 list-inside list-disc text-muted-foreground">
              {entries.length === 0 ? (
                <li>No records</li>
              ) : (
                entries.map(([key, n]) => (
                  <li key={key}>
                    {n} {COUNT_LABELS[key] ?? key.replace(/_/g, ' ')}
                  </li>
                ))
              )}
            </ul>
          </div>

          {/*
            Absolute, not "recoverable for 5 days". An operator reading a
            duration has to work out when that lands, under pressure, and a
            wrong answer here is unrecoverable.
          */}
          <div>
            <p className="font-medium">Recoverable until</p>
            <p className="text-muted-foreground">{formatDeadline(recoveryDeadline)}</p>
          </div>

          {scope === 'system' ? (
            <div className="rounded-sm border border-border p-3">
              <p className="font-medium">What survives</p>
              <ul className="mt-1 list-inside list-disc text-muted-foreground">
                <li>User accounts</li>
                <li>Sign-ins and sessions</li>
                <li>Seeded maintenance categories</li>
              </ul>
            </div>
          ) : null}

          <div className="space-y-1">
            <Label htmlFor="purge-confirmation">
              {promptLabel}
              <RequiredMarker />
            </Label>
            <Input
              id="purge-confirmation"
              aria-required="true"
              value={typed}
              autoComplete="off"
              onChange={(e) => setTyped(e.target.value)}
            />
          </div>
        </div>

        <DialogFooter>
          <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
            Cancel
          </Button>
          <Button
            type="button"
            variant="destructive"
            disabled={!matches || isPending}
            onClick={() => onConfirm(typed)}
          >
            {scope === 'system' ? 'Purge everything' : 'Purge this fleet'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
