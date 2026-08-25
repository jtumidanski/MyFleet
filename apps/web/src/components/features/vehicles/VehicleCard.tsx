import { Link } from 'react-router-dom';
import {
  AlertTriangle,
  CheckCircle,
  Clock,
  HelpCircle,
  History,
  Moon,
  type LucideIcon,
} from 'lucide-react';
import { formatMileage, formatRelativeTime } from '@myfleet/ui-components';
import { Button } from '../../ui/button';
import { Card } from '../../ui/card';
import { Skeleton } from '../../ui/skeleton';
import { VehiclePhotoThumbnail } from './VehiclePhotoThumbnail';
import { vehicleBanner, type BannerIcon, type BannerTone } from './vehicleBanner';
import { buildCarfaxUrl } from '../../../lib/carfax';
import { useRuntimeConfig } from '../../../lib/hooks/useRuntimeConfig';
import { vehicleTitle } from '../../../lib/utils/vehicleTitle';
import { cn } from '../../../lib/utils';
import type { Vehicle } from '../../../types/models/vehicle';

/**
 * The banner's colour treatment. Only the two states that need action are
 * tinted, so colour anywhere in the grid always means "look here" — a per-card
 * chip cannot achieve that.
 *
 * These are the semantic token pairs task-003 measured for AA contrast in both
 * themes (docs/tasks/task-003-dark-mode-branding/contrast.md); no new colour
 * combination is introduced here.
 */
const TONE_CLASSES: Record<BannerTone, string> = {
  danger: 'bg-danger-subtle text-danger-subtle-foreground border-danger-border',
  warning: 'bg-warning-subtle text-warning-subtle-foreground border-warning-border',
  quiet: 'bg-card text-muted-foreground border-border',
};

/**
 * The one place a banner icon token becomes a component. Keeping lucide out of
 * vehicleBanner.ts is what lets its tests assert on plain data.
 */
const BANNER_ICONS: Record<BannerIcon, LucideIcon> = {
  overdue: AlertTriangle,
  upcoming: Clock,
  healthy: CheckCircle,
  inactive: Moon,
  unknown: HelpCircle,
};

const EM_DASH = '—';

export function VehicleCard({ vehicle }: { vehicle: Vehicle }) {
  const { attributes } = vehicle;
  const title = vehicleTitle(attributes);

  const banner = vehicleBanner(attributes, new Date());
  const BannerIconComponent = BANNER_ICONS[banner.icon];

  // Read through the hook, not the module getter: the tree mounts before the
  // runtime config fetch resolves, so a card rendered in that window has to
  // re-render when the real template lands or a ConfigMap override would never
  // reach the user.
  const { carfaxUrlTemplate } = useRuntimeConfig();
  // null means "render no button": no VIN, a template that ignores {vin}, or a
  // template whose scheme is not https:. Nothing contacts Carfax until a click.
  const carfaxUrl = buildCarfaxUrl(carfaxUrlTemplate, attributes.vin);

  const lastActivity = attributes.lastActivityAt
    ? formatRelativeTime(attributes.lastActivityAt)
    : '';

  return (
    // `relative` anchors the card link's overlay to the whole card. `isolate`
    // creates a local stacking context so the z-indices inside cannot leak in or
    // out. `group` drives the hover treatment.
    //
    // Note there is no `overflow-hidden` here: on the root it would clip the
    // card link's focus ring. It sits on the photo wrapper instead, where it
    // does the one job it is needed for.
    <Card
      className={cn(
        'group relative isolate flex flex-col p-0 transition-shadow hover:shadow-md',
        // The overlay anchor has no visible box of its own, so its own ring is
        // suppressed and the card takes it. The data attribute scopes the
        // selector, so focusing Carfax does not also ring the whole card.
        'has-[a[data-card-link]:focus-visible]:ring-2',
        'has-[a[data-card-link]:focus-visible]:ring-ring',
        'has-[a[data-card-link]:focus-visible]:ring-offset-2',
        // Unprefixed and inert until a ring is actually drawn: without it,
        // Tailwind preflight's default #fff ring-offset-color wins, so a
        // keyboard-focused card in dark mode shows a white band between the
        // card and its ring instead of the card's own background.
        'ring-offset-background',
      )}
    >
      {/* Clips the hero's top corners to the card's radius. */}
      <div className="overflow-hidden rounded-t-lg">
        <VehiclePhotoThumbnail
          mediaId={attributes.primaryImageMediaId}
          vehicleLabel={title}
          boxClassName="aspect-[16/9] w-full rounded-none"
        />
      </div>

      {/* Fixed height on every card regardless of tone, so cards in a grid row
          align whatever their status. */}
      <div
        data-testid="vehicle-card-banner"
        className={cn(
          'flex h-9 shrink-0 items-center gap-2 border-b px-4 text-sm',
          TONE_CLASSES[banner.tone],
        )}
      >
        <BannerIconComponent className="h-4 w-4 shrink-0" aria-hidden="true" />
        <span className="truncate">{banner.text}</span>
      </div>

      <div className="min-w-0 px-4 pt-3">
        {/* The ::after overlay is what makes the whole card clickable without the
            anchor ever becoming a DOM ancestor of the Carfax link — the nesting
            that made task-005 remove whole-card navigation in the first place.
            Being a real <a href>, it keeps middle-click, cmd/ctrl-click, and the
            link context menu. Its text is the vehicle title, so a grid of cards
            is a list of distinctly-named links with no aria-label needed. */}
        <Link
          to={`/vehicles/${vehicle.id}`}
          data-card-link
          className="block truncate font-medium text-foreground after:absolute after:inset-0 after:content-[''] focus-visible:outline-hidden"
        >
          {title}
        </Link>
        <div className="truncate text-sm text-muted-foreground">
          {attributes.year} {attributes.make} {attributes.model}
          {attributes.trim ? ` ${attributes.trim}` : ''}
        </div>
      </div>

      {/* Both slots always render; an absent value is an em-dash, never a
          missing column, so values line up across cards. */}
      <div className="mt-3 grid grid-cols-2 gap-4 border-t border-border px-4 py-3">
        <Stat
          label="Odometer"
          value={
            typeof attributes.currentMileage === 'number'
              ? formatMileage(attributes.currentMileage)
              : EM_DASH
          }
        />
        <Stat label="Last activity" value={lastActivity || EM_DASH} />
      </div>

      {/* Fixed height so a VIN-less card is exactly as tall as its neighbours. */}
      <div className="flex h-12 items-center justify-end px-4 pb-2">
        {carfaxUrl && (
          // A plain <a>, not a react-router <Link> — this leaves the SPA.
          // rel="noopener noreferrer" stops the opened page reaching back
          // through window.opener and suppresses the referrer, which matters
          // because the URL carries the VIN.
          //
          // `relative z-10` lifts it above the card link's overlay so it
          // receives its own clicks. Without it, clicking Carfax silently
          // navigates to the detail page instead — it looks fine and behaves
          // wrong.
          <Button asChild variant="ghost" size="icon" className="relative z-10">
            <a
              href={carfaxUrl}
              target="_blank"
              rel="noopener noreferrer"
              aria-label={`View Carfax report for ${title} (opens in a new tab)`}
            >
              <History className="h-4 w-4" aria-hidden="true" />
            </a>
          </Button>
        )}
      </div>
    </Card>
  );
}

function Stat({ label, value }: { label: string; value: string }) {
  return (
    <div className="min-w-0">
      <div className="text-xs uppercase tracking-wide text-muted-foreground">{label}</div>
      <div className="truncate tabular-nums text-sm text-foreground">{value}</div>
    </div>
  );
}

/**
 * The loading placeholder for a VehicleCard.
 *
 * Deliberately built from the same region structure as the card above — hero,
 * banner, title, subtitle, stat strip, footer — rather than one fixed-height
 * block. It lives in this file so that adding a region to the card and
 * forgetting the skeleton is visible in the same diff.
 *
 * Every region's height matches the populated card's exactly, including the
 * title/subtitle and stat-strip text line boxes, not just the regions with an
 * explicit fixed-height class — that is what stops the grid shifting once
 * data arrives.
 */
export function VehicleCardSkeleton() {
  return (
    <Card className="flex flex-col p-0">
      <Skeleton className="aspect-[16/9] w-full rounded-none rounded-t-lg" />
      <div className="flex h-9 items-center border-b border-border px-4">
        <Skeleton className="h-3 w-40" />
      </div>
      {/* 24px + 20px, matching the card's base-size title and text-sm subtitle
          line boxes exactly — the bars are inset inside those boxes so the
          skeleton reads as text without being shorter than the text it stands
          in for. No space-y here: the real title and subtitle have no gap. */}
      <div className="px-4 pt-3">
        <div className="flex h-6 items-center">
          <Skeleton className="h-4 w-2/3" />
        </div>
        <div className="flex h-5 items-center">
          <Skeleton className="h-3 w-1/2" />
        </div>
      </div>
      <div className="mt-3 grid grid-cols-2 gap-4 border-t border-border px-4 py-3">
        {/* Each slot mirrors Stat's text-xs label over its text-sm value:
            16px + 20px, not a single 32px bar. */}
        <div>
          <div className="flex h-4 items-center">
            <Skeleton className="h-2.5 w-16" />
          </div>
          <div className="flex h-5 items-center">
            <Skeleton className="h-3 w-20" />
          </div>
        </div>
        <div>
          <div className="flex h-4 items-center">
            <Skeleton className="h-2.5 w-16" />
          </div>
          <div className="flex h-5 items-center">
            <Skeleton className="h-3 w-20" />
          </div>
        </div>
      </div>
      <div className="flex h-12 items-center justify-end px-4 pb-2">
        <Skeleton className="h-8 w-8 rounded-md" />
      </div>
    </Card>
  );
}
